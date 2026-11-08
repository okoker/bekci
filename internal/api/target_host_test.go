package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
)

type hostRuleCase struct {
	Host  string `json:"host"`
	Error string `json:"error"`
}

// loadHostRules reads the Host table shared with the checker and the target form.
func loadHostRules(t *testing.T) []hostRuleCase {
	t.Helper()
	var table struct {
		Cases []hostRuleCase `json:"cases"`
	}
	data, err := os.ReadFile("../checker/testdata/host_rules.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &table); err != nil {
		t.Fatal(err)
	}
	if len(table.Cases) == 0 {
		t.Fatal("host_rules.json has no cases")
	}
	return table.Cases
}

func hostTargetPayload(name, host string, checkID string) map[string]any {
	cond := map[string]any{
		"check_type":      "http",
		"check_name":      name + " Check",
		"config":          "{}",
		"interval_s":      300,
		"field":           "status",
		"comparator":      "eq",
		"value":           "down",
		"fail_count":      1,
		"fail_window":     0,
		"condition_group": 0,
		"group_operator":  "AND",
	}
	if checkID != "" {
		cond["check_id"] = checkID
	}
	return map[string]any{
		"name":       name,
		"host":       host,
		"category":   "Network",
		"operator":   "AND",
		"conditions": []map[string]any{cond},
	}
}

// A-082: create and update accept a Host that is a name or IP with an optional
// path, store it as typed, and refuse what breaks a check with the shared message.
func TestTargetHostRules(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "hostadmin", "testpassword12345", "admin")
	client := loginAs(t, ts, "hostadmin", "testpassword12345")

	editID, editCheckID := createTargetViaAPI(t, client, ts.URL, "host-edit", "edit.example.com", "Network", "http")

	for i, c := range loadHostRules(t) {
		if c.Host == "" {
			continue // the "required" check answers an empty Host
		}
		wantStatus := map[bool]int{true: http.StatusCreated, false: http.StatusBadRequest}[c.Error == ""]

		resp := doJSON(t, client, http.MethodPost, ts.URL+"/api/targets", hostTargetPayload(fmt.Sprintf("host-new-%d", i), c.Host, ""))
		checkHostResponse(t, "create", resp, c, wantStatus)

		resp = doJSON(t, client, http.MethodPut, ts.URL+"/api/targets/"+editID, hostTargetPayload("host-edit", c.Host, editCheckID))
		checkHostResponse(t, "update", resp, c, map[bool]int{true: http.StatusOK, false: http.StatusBadRequest}[c.Error == ""])
	}

	// Surrounding spaces are trimmed before the rules apply; the path is kept as typed.
	resp := doJSON(t, client, http.MethodPost, ts.URL+"/api/targets", hostTargetPayload("host-trim", "  www.somehost.com/health/  ", ""))
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("trimmed host: expected 201, got %d: %s", resp.StatusCode, b)
	}
	if got := parseJSON(t, resp)["host"]; got != "www.somehost.com/health/" {
		t.Fatalf("trimmed host stored as %q, want %q", got, "www.somehost.com/health/")
	}
}

func checkHostResponse(t *testing.T, op string, resp *http.Response, c hostRuleCase, wantStatus int) {
	t.Helper()
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Errorf("%s host %q: status %d, want %d (%s)", op, c.Host, resp.StatusCode, wantStatus, body)
		return
	}
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	if c.Error != "" {
		if m["error"] != c.Error {
			t.Errorf("%s host %q: error %q, want %q", op, c.Host, m["error"], c.Error)
		}
		return
	}
	if m["host"] != c.Host {
		t.Errorf("%s host %q: stored as %q", op, c.Host, m["host"])
	}
}
