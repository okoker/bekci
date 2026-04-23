package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/bekci/internal/store"
)

// Build a minimal target via the store, bypassing the HTTP handlers,
// because condition plumbing isn't what we're testing here.
func seedHostTarget(t *testing.T, st *store.Store, name, host string, tags []string, notes, contacts, project, location string) string {
	t.Helper()
	tgt := &store.Target{
		Name:               name,
		Host:               host,
		Enabled:            true,
		Operator:           "AND",
		Category:           "Other",
		PreferredCheckType: "http",
		Tags:               tags,
	}
	if notes != "" {
		tgt.Notes = &notes
	}
	if contacts != "" {
		tgt.Contacts = &contacts
	}
	if project != "" {
		tgt.Project = &project
	}
	if location != "" {
		tgt.Location = &location
	}
	conds := []store.TargetCondition{{
		CheckType:     "http",
		CheckName:     name + " HTTP",
		Config:        "{}",
		IntervalS:     60,
		Field:         "status",
		Comparator:    "eq",
		Value:         "down",
		FailCount:     1,
		GroupOperator: "AND",
	}}
	if err := st.CreateTargetWithConditions(tgt, conds, ""); err != nil {
		t.Fatal(err)
	}
	return tgt.ID
}

func TestV1Hosts_RateLimit(t *testing.T) {
	ts, st := setupTestServer(t)

	// Tighten the limit to 3 so the test finishes fast.
	if err := st.SetSettings(map[string]string{"api_rate_limit_per_min": "3"}); err != nil {
		t.Fatalf("set rate limit: %v", err)
	}

	seedHostTarget(t, st, "ratelimit-host", "rl.example.com", nil, "", "", "", "")
	_, plaintext, err := st.CreateAPIToken("rl-token", "admin")
	if err != nil {
		t.Fatal(err)
	}

	fire := func() int {
		req, _ := http.NewRequest("GET", ts.URL+"/api/v1/hosts?host=rl.example.com", nil)
		req.Header.Set("Authorization", "Bearer "+plaintext)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	// First 3 within the window succeed.
	for i := 0; i < 3; i++ {
		if code := fire(); code != 200 {
			t.Fatalf("request %d: expected 200, got %d", i+1, code)
		}
	}
	// 4th must be rejected with 429 + Retry-After.
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/hosts?host=rl.example.com", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 429 {
		t.Fatalf("expected 429, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Fatalf("expected Retry-After header")
	}
	body, _ := io.ReadAll(resp.Body)
	var payload struct {
		Error        string `json:"error"`
		RetryAfterS  int    `json:"retry_after_s"`
		LimitPerMin  int    `json:"limit_per_min"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Error == "" || payload.LimitPerMin != 3 {
		t.Fatalf("unexpected body: %+v", payload)
	}
}

func TestV1Hosts_BearerRequired(t *testing.T) {
	ts, _ := setupTestServer(t)
	resp, err := http.Get(ts.URL + "/api/v1/hosts?host=anything")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401 without bearer, got %d", resp.StatusCode)
	}
	if h := resp.Header.Get("WWW-Authenticate"); !strings.Contains(h, "Bearer") {
		t.Fatalf("expected WWW-Authenticate Bearer challenge, got %q", h)
	}
}

func TestV1Hosts_Snapshot(t *testing.T) {
	ts, st := setupTestServer(t)

	// Seed a project tag so the target field validates via direct store use.
	_, _ = st.CreateTagOption("project", "DIAS")
	_, _ = st.CreateTagOption("location", "DC-1")
	for _, v := range []string{"P1", "IT"} {
		_, _ = st.CreateTagOption("tag", v)
	}

	tid := seedHostTarget(t, st,
		"Web Server", "10.0.9.20",
		[]string{"P1", "IT"},
		"primary prod; owner alice",
		"ops@example.com",
		"DIAS", "DC-1",
	)

	// Create an API token directly.
	_, plaintext, err := st.CreateAPIToken("grafana", "admin")
	if err != nil {
		t.Fatal(err)
	}

	// Call with bogus token — rejected.
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/hosts?host=10.0.9.20", nil)
	req.Header.Set("Authorization", "Bearer bk_notatoken")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401 for bogus token, got %d", resp.StatusCode)
	}

	// Call with real token — OK, returns one element.
	req, _ = http.NewRequest("GET", ts.URL+"/api/v1/hosts?host=10.0.9.20", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var envelope struct {
		Targets []v1HostSnapshot `json:"targets"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(envelope.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(envelope.Targets))
	}
	got := envelope.Targets[0]
	if got.TargetID != tid {
		t.Fatalf("target_id mismatch: %s vs %s", got.TargetID, tid)
	}
	if got.Name != "Web Server" || got.Host != "10.0.9.20" {
		t.Fatalf("unexpected name/host: %+v", got)
	}
	if got.Project != "DIAS" || got.Location != "DC-1" {
		t.Fatalf("project/location not carried: %+v", got)
	}
	if got.Contacts != "ops@example.com" || !strings.Contains(got.Notes, "owner alice") {
		t.Fatalf("contacts/notes not carried: %+v", got)
	}
	if len(got.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %v", got.Tags)
	}
	if got.LastCheck.Status != "none" {
		t.Fatalf("expected last_check.status=none before any check runs, got %q", got.LastCheck.Status)
	}
	if got.LastCheck.CheckType != "http" {
		t.Fatalf("expected check_type=http, got %q", got.LastCheck.CheckType)
	}

	// Case-insensitive match.
	req, _ = http.NewRequest("GET", ts.URL+"/api/v1/hosts?host=10.0.9.20", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("case-ins match: got %d", resp.StatusCode)
	}

	// Unknown host returns empty array, not 404.
	req, _ = http.NewRequest("GET", ts.URL+"/api/v1/hosts?host=no.such.host", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	resp, _ = http.DefaultClient.Do(req)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 empty, got %d: %s", resp.StatusCode, string(body))
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode empty: %v", err)
	}
	if len(envelope.Targets) != 0 {
		t.Fatalf("expected 0 results, got %d", len(envelope.Targets))
	}

	// Missing host param → 400.
	req, _ = http.NewRequest("GET", ts.URL+"/api/v1/hosts", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for missing host, got %d", resp.StatusCode)
	}

	// Revoked token rejected.
	tokens, _ := st.ListAPITokens()
	if err := st.RevokeAPIToken(tokens[0].ID); err != nil {
		t.Fatal(err)
	}
	req, _ = http.NewRequest("GET", ts.URL+"/api/v1/hosts?host=10.0.9.20", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401 after revoke, got %d", resp.StatusCode)
	}
}
