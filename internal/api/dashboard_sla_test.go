package api

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/bekci/internal/store"
)

// createSLATarget creates a target whose checks are named after their type
// ("a-http", "b-ping", …) so the dashboard lists them in that order. It returns
// the target ID and the check IDs by type.
func createSLATarget(t *testing.T, client *http.Client, baseURL, name, category, preferred string, checkNames map[string]string) (string, map[string]string) {
	t.Helper()
	var conds []map[string]any
	for checkType, checkName := range checkNames {
		conds = append(conds, map[string]any{
			"check_type": checkType, "check_name": checkName, "config": "{}",
			"interval_s": 300, "field": "status", "comparator": "eq", "value": "down",
			"fail_count": 1, "fail_window": 0, "condition_group": 0, "group_operator": "AND",
		})
	}
	resp := doJSON(t, client, http.MethodPost, baseURL+"/api/targets", map[string]any{
		"name": name, "host": name + ".example.com", "category": category,
		"operator": "AND", "preferred_check_type": preferred, "conditions": conds,
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create %s: %d %s", name, resp.StatusCode, body)
	}
	created := parseJSON(t, resp)
	ids := map[string]string{}
	for _, raw := range created["conditions"].([]any) {
		c := raw.(map[string]any)
		ids[c["check_type"].(string)] = c["check_id"].(string)
	}
	return created["id"].(string), ids
}

func saveUpResult(t *testing.T, st *store.Store, checkID string) {
	t.Helper()
	if err := st.SaveResult(&store.CheckResult{
		CheckID: checkID, Status: "up", ResponseMs: 10, Message: "ok", Metrics: "{}",
		CheckedAt: time.Now().Add(-10 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
}

func dashboardTargets(t *testing.T, client *http.Client, baseURL string) map[string]map[string]any {
	t.Helper()
	resp, err := client.Get(baseURL + "/api/dashboard/status")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]map[string]any{}
	for _, target := range parseJSONArray(t, resp) {
		byID[target["id"].(string)] = target
	}
	return byID
}

func slaThresholdOf(t *testing.T, client *http.Client, baseURL, category string) float64 {
	t.Helper()
	resp, err := client.Get(baseURL + "/api/sla/history")
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range parseJSON(t, resp)["categories"].([]any) {
		c := raw.(map[string]any)
		if c["name"] == category {
			return c["sla_threshold"].(float64)
		}
	}
	t.Fatalf("category %q not on the SLA page", category)
	return 0
}

// T-P48: a check without results has no uptime (-1) and no SLA status; the
// preferred check's missing data is not replaced by another check's (SR-16,
// review item D-31).
func TestDashboardSLAWithoutResults(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "slaadmin", "testpassword12345", "admin")
	client := loginAs(t, ts, "slaadmin", "testpassword12345")
	if err := st.SetSettings(map[string]string{"sla_network": "99.9"}); err != nil {
		t.Fatal(err)
	}

	fresh, _ := createSLATarget(t, client, ts.URL, "fresh", "Network", "http", map[string]string{"http": "a-http"})
	// Preferred ping has no data; the http check, listed first, is all up.
	mixed, mixedIDs := createSLATarget(t, client, ts.URL, "mixed", "Network", "ping", map[string]string{"http": "a-http", "ping": "b-ping"})
	saveUpResult(t, st, mixedIDs["http"])
	healthy, healthyIDs := createSLATarget(t, client, ts.URL, "healthy", "Network", "http", map[string]string{"http": "a-http"})
	saveUpResult(t, st, healthyIDs["http"])

	got := dashboardTargets(t, client, ts.URL)

	f := got[fresh]
	if s := f["sla_status"]; s != "" {
		t.Errorf("fresh target: sla_status %q, want empty (no results yet)", s)
	}
	if u := f["checks"].([]any)[0].(map[string]any)["uptime_90d_pct"]; u != float64(-1) {
		t.Errorf("fresh target: uptime_90d_pct %v, want -1", u)
	}
	if s := got[mixed]["sla_status"]; s != "" {
		t.Errorf("preferred check without results: sla_status %q, want empty", s)
	}
	if s := got[healthy]["sla_status"]; s != "healthy" {
		t.Errorf("all-up target: sla_status %q, want healthy", s)
	}
}

// T-P49: a category without a saved sla_<category> setting uses the 99.9
// default on the dashboard and the SLA page; an explicit 0 disables it
// (SR-16, review item D-31).
func TestSLADefaultThresholdForNewCategory(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "catadmin", "testpassword12345", "admin")
	client := loginAs(t, ts, "catadmin", "testpassword12345")

	resp := doJSON(t, client, http.MethodPost, ts.URL+"/api/tags", map[string]string{"group": "category", "value": "Lab Gear"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create category: %d", resp.StatusCode)
	}
	resp.Body.Close()
	if v, _ := st.GetSetting("sla_lab_gear"); v != "" {
		t.Fatalf("precondition: sla_lab_gear = %q, want no saved setting", v)
	}
	id, ids := createSLATarget(t, client, ts.URL, "lab", "Lab Gear", "http", map[string]string{"http": "a-http"})
	saveUpResult(t, st, ids["http"])

	target := dashboardTargets(t, client, ts.URL)[id]
	if v := target["sla_target"]; v != 99.9 {
		t.Errorf("dashboard sla_target %v, want 99.9", v)
	}
	if v := target["sla_status"]; v != "healthy" {
		t.Errorf("dashboard sla_status %q, want healthy", v)
	}
	if v := slaThresholdOf(t, client, ts.URL, "Lab Gear"); v != 99.9 {
		t.Errorf("SLA page sla_threshold %v, want 99.9", v)
	}

	if err := st.SetSettings(map[string]string{"sla_lab_gear": "0"}); err != nil {
		t.Fatal(err)
	}
	target = dashboardTargets(t, client, ts.URL)[id]
	if v, s := target["sla_target"], target["sla_status"]; v != float64(0) || s != "" {
		t.Errorf("explicit 0: sla_target %v sla_status %q, want 0 and empty", v, s)
	}
	if v := slaThresholdOf(t, client, ts.URL, "Lab Gear"); v != 0 {
		t.Errorf("explicit 0: SLA page sla_threshold %v, want 0", v)
	}
}
