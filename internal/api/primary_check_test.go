package api

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/bekci/internal/store"
)

func checkIDsByName(t *testing.T, detail map[string]any) map[string]string {
	t.Helper()
	ids := map[string]string{}
	for _, raw := range detail["conditions"].([]any) {
		c := raw.(map[string]any)
		ids[c["check_name"].(string)] = c["check_id"].(string)
	}
	return ids
}

// T-P56: a target with two checks of its preferred type has one primary check,
// the first by name, and every response names that same check: the targets
// list, the target detail (GET, POST, PUT), the dashboard and SOC payloads.
// The SLA page and /api/v1/hosts use that check's data (backlog A-062).
// Condition order (z-page first) differs from name order (a-slow first), as on
// the Search page, which lists checks in condition order.
func TestPrimaryCheckIsOneCheck(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "primadmin", "testpassword12345", "admin")
	client := loginAs(t, ts, "primadmin", "testpassword12345")

	cond := func(checkType, name, checkID string) map[string]any {
		return map[string]any{
			"check_id": checkID, "check_type": checkType, "check_name": name, "config": "{}",
			"interval_s": 300, "field": "status", "comparator": "eq", "value": "down",
			"fail_count": 1, "fail_window": 0, "condition_group": 0, "group_operator": "AND",
		}
	}
	request := func(ids map[string]string) map[string]any {
		return map[string]any{
			"name": "shop", "host": "shop.example.com", "category": "Network",
			"operator": "AND", "preferred_check_type": "http",
			"conditions": []map[string]any{
				cond("http", "z-page", ids["z-page"]), cond("http", "a-slow", ids["a-slow"]), cond("ping", "m-ping", ids["m-ping"]),
			},
		}
	}

	resp := doJSON(t, client, http.MethodPost, ts.URL+"/api/targets", request(map[string]string{}))
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	created := parseJSON(t, resp)
	targetID := created["id"].(string)
	ids := checkIDsByName(t, created)
	slow := ids["a-slow"]
	if first := created["conditions"].([]any)[0].(map[string]any)["check_name"]; first != "z-page" {
		t.Fatalf("precondition: conditions start with %v, want z-page", first)
	}
	wantPrimary := func(where string, got any) {
		t.Helper()
		if got != slow {
			t.Errorf("%s: primary_check_id %v, want a-slow's %s", where, got, slow)
		}
	}
	wantPrimary("POST /api/targets", created["primary_check_id"])

	// Only the primary check is up; the other HTTP check is down.
	now := time.Now().Add(-10 * time.Minute)
	for _, r := range []store.CheckResult{
		{CheckID: slow, Status: "up", ResponseMs: 10, Message: "slow ok", Metrics: "{}", CheckedAt: now},
		{CheckID: ids["z-page"], Status: "down", ResponseMs: 10, Message: "page down", Metrics: "{}", CheckedAt: now},
	} {
		if err := st.SaveResult(&r); err != nil {
			t.Fatal(err)
		}
	}

	resp, _ = client.Get(ts.URL + "/api/targets/" + targetID)
	wantPrimary("GET /api/targets/{id}", parseJSON(t, resp)["primary_check_id"])

	resp, _ = client.Get(ts.URL + "/api/targets")
	found := false
	for _, item := range parseJSONArray(t, resp) {
		if item["id"] == targetID {
			found = true
			wantPrimary("GET /api/targets", item["primary_check_id"])
		}
	}
	if !found {
		t.Error("GET /api/targets: target missing")
	}

	resp = doJSON(t, client, http.MethodPut, ts.URL+"/api/targets/"+targetID, request(ids))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: %d", resp.StatusCode)
	}
	updated := parseJSON(t, resp)
	if got := checkIDsByName(t, updated)["a-slow"]; got != slow {
		t.Fatalf("precondition: update replaced a-slow (%s → %s)", slow, got)
	}
	wantPrimary("PUT /api/targets/{id}", updated["primary_check_id"])

	dash := dashboardTargets(t, client, ts.URL)[targetID]
	wantPrimary("dashboard", dash["primary_check_id"])
	if s := dash["sla_status"]; s != "healthy" {
		t.Errorf("dashboard sla_status %q, want healthy (a-slow is all up)", s)
	}

	resp, _ = client.Get(ts.URL + "/api/soc/status")
	found = false
	for _, item := range parseJSONArray(t, resp) {
		if item["id"] == targetID {
			found = true
			wantPrimary("SOC", item["primary_check_id"])
		}
	}
	if !found {
		t.Error("SOC: target missing")
	}

	resp, _ = client.Get(ts.URL + "/api/sla/history")
	found = false
	for _, rawCat := range parseJSON(t, resp)["categories"].([]any) {
		for _, rawTarget := range rawCat.(map[string]any)["targets"].([]any) {
			tg := rawTarget.(map[string]any)
			if tg["id"] != targetID {
				continue
			}
			found = true
			days := tg["daily_uptime"].([]any)
			if len(days) != 1 || days[0].(map[string]any)["uptime_pct"] != float64(100) {
				t.Errorf("SLA page daily_uptime %v, want one day at 100 (a-slow)", days)
			}
		}
	}
	if !found {
		t.Error("SLA page: target missing")
	}

	_, token, err := st.CreateAPIToken("primary-test", "admin")
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/hosts?host=shop.example.com", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	hosts := parseJSON(t, resp)["targets"].([]any)
	if len(hosts) != 1 {
		t.Fatalf("v1 hosts: %d results, want 1", len(hosts))
	}
	if msg := hosts[0].(map[string]any)["last_check"].(map[string]any)["message"]; msg != "slow ok" {
		t.Errorf("v1 last_check message %q, want a-slow's \"slow ok\"", msg)
	}
}
