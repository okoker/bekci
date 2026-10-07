package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestSystemInfoAdminOnly(t *testing.T) {
	ts, st := setupTestServer(t)

	// Unauthenticated → 401
	resp, err := http.Get(ts.URL + "/api/system/info")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401 unauthenticated, got %d", resp.StatusCode)
	}

	// Operator → 403
	createUser(t, st, "opsys", "testpassword12345", "operator")
	op := loginAs(t, ts, "opsys", "testpassword12345")
	resp, err = op.Get(ts.URL + "/api/system/info")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403 for operator, got %d", resp.StatusCode)
	}
}

func TestSystemInfoAdminPayload(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "adminsys", "testpassword12345", "admin")
	admin := loginAs(t, ts, "adminsys", "testpassword12345")

	resp, err := admin.Get(ts.URL + "/api/system/info")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for admin, got %d", resp.StatusCode)
	}
	m := parseJSON(t, resp)

	for _, key := range []string{"disk", "ram", "cpu", "uptime", "server_ip", "version", "build", "go_version", "database", "email", "updates", "backup", "log", "scheduler"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing key %q in payload", key)
		}
	}
	if m["version"] != "test" {
		t.Errorf("version = %v, want test", m["version"])
	}
	if gv, _ := m["go_version"].(string); !strings.HasPrefix(gv, "go") {
		t.Errorf("go_version = %q, want go*", gv)
	}
	db := m["database"].(map[string]any)
	if db["status"] != "ok" {
		t.Errorf("database.status = %v, want ok", db["status"])
	}
	if v, _ := db["schema_version"].(float64); v <= 0 {
		t.Errorf("database.schema_version = %v, want > 0", db["schema_version"])
	}
	email := m["email"].(map[string]any)
	if email["configured"] != false {
		t.Errorf("email.configured = %v, want false on a fresh DB", email["configured"])
	}
	backup := m["backup"].(map[string]any)
	if c, _ := backup["count"].(float64); c != 0 {
		t.Errorf("backup.count = %v, want 0 on a fresh server", backup["count"])
	}
	// Test server runs without a scheduler — must not panic, must say so.
	sched := m["scheduler"].(map[string]any)
	if sched["status"] != "unavailable" {
		t.Errorf("scheduler.status = %v, want unavailable when scheduler is nil", sched["status"])
	}
}

func TestSystemInfoEmailConfigured(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "adminmail", "testpassword12345", "admin")
	admin := loginAs(t, ts, "adminmail", "testpassword12345")

	if err := st.SetSettings(map[string]string{
		"email_provider":   "ms365",
		"alert_from_email": "bekci@example.com",
		"smtp_host":        "smtp.office365.com",
		"smtp_username":    "bekci@example.com",
		"smtp_password":    "secret",
	}); err != nil {
		t.Fatal(err)
	}

	resp, err := admin.Get(ts.URL + "/api/system/info")
	if err != nil {
		t.Fatal(err)
	}
	email := parseJSON(t, resp)["email"].(map[string]any)
	if email["configured"] != true {
		t.Errorf("email.configured = %v, want true", email["configured"])
	}
	if email["provider"] != "Microsoft 365" {
		t.Errorf("email.provider = %v, want Microsoft 365", email["provider"])
	}
}

func TestParseAptSimulation(t *testing.T) {
	out := `NOTE: This is only a simulation!
      apt-get needs root privileges for real execution.
      Keep also in mind that locking is deactivated,
      so don't depend on the relevance to the real current situation!
Reading package lists...
Building dependency tree...
Reading state information...
Calculating upgrade...
The following packages will be upgraded:
  iproute2 libfreetype6 sg3-utils
3 upgraded, 0 newly installed, 0 to remove and 0 not upgraded.
Inst iproute2 [5.15.0-1ubuntu2.1] (5.15.0-1ubuntu2.2 Ubuntu:22.04/jammy-updates [amd64])
Inst libfreetype6 [2.11.1+dfsg-1ubuntu0.3] (2.11.1+dfsg-1ubuntu0.4 Ubuntu:22.04/jammy-updates, Ubuntu:22.04/jammy-security [amd64])
Inst sg3-utils [1.46-1ubuntu0.22.04.1] (1.46-1ubuntu0.22.04.2 Ubuntu:22.04/jammy-updates, Ubuntu:22.04/jammy-security [amd64])
Conf iproute2 (5.15.0-1ubuntu2.2 Ubuntu:22.04/jammy-updates [amd64])
Conf libfreetype6 (2.11.1+dfsg-1ubuntu0.4 Ubuntu:22.04/jammy-updates, Ubuntu:22.04/jammy-security [amd64])
Conf sg3-utils (1.46-1ubuntu0.22.04.2 Ubuntu:22.04/jammy-updates, Ubuntu:22.04/jammy-security [amd64])
`
	total, security := parseAptSimulation(out)
	if total != 3 || security != 2 {
		t.Fatalf("got total=%d security=%d, want 3/2", total, security)
	}

	total, security = parseAptSimulation("Reading package lists...\n0 upgraded, 0 newly installed, 0 to remove and 0 not upgraded.\n")
	if total != 0 || security != 0 {
		t.Fatalf("got total=%d security=%d, want 0/0 for no updates", total, security)
	}
}

func TestUpdatesStatus(t *testing.T) {
	cases := []struct {
		total, security int
		want            string
	}{
		{0, 0, "up_to_date"},
		{5, 0, "updates"},
		{5, 1, "security"},
	}
	for _, c := range cases {
		if got := updatesStatus(c.total, c.security); got != c.want {
			t.Errorf("updatesStatus(%d,%d) = %q, want %q", c.total, c.security, got, c.want)
		}
	}
}

func TestParseMeminfo(t *testing.T) {
	data := "MemTotal:        8136800 kB\nMemFree:          912344 kB\nMemAvailable:    7380000 kB\nBuffers:          123 kB\n"
	total, avail, ok := parseMeminfo(data)
	if !ok || total != 8136800*1024 || avail != 7380000*1024 {
		t.Fatalf("got total=%d avail=%d ok=%v", total, avail, ok)
	}
	if _, _, ok := parseMeminfo("garbage"); ok {
		t.Fatal("expected ok=false for unparseable meminfo")
	}
}

func TestParseLoadAvg(t *testing.T) {
	l1, l5, l15, ok := parseLoadAvg("0.03 0.01 0.00 1/123 4567\n")
	if !ok || l1 != 0.03 || l5 != 0.01 || l15 != 0 {
		t.Fatalf("got %v %v %v ok=%v", l1, l5, l15, ok)
	}
	if _, _, _, ok := parseLoadAvg(""); ok {
		t.Fatal("expected ok=false for empty loadavg")
	}
}

func TestParseProcUptime(t *testing.T) {
	sec, ok := parseProcUptime("9258214.53 18392011.20\n")
	if !ok || sec != 9258214 {
		t.Fatalf("got %d ok=%v", sec, ok)
	}
	if _, ok := parseProcUptime("x"); ok {
		t.Fatal("expected ok=false")
	}
}
