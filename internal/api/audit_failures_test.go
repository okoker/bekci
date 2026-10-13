package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/bekci/internal/store"
)

// countFailureAudits returns how many audit entries for action have status "failure".
func countFailureAudits(t *testing.T, st *store.Store, action string) int {
	t.Helper()
	entries, _, err := st.ListAuditEntries(1000, 0, action)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if e.Action == action && e.Status == "failure" {
			n++
		}
	}
	return n
}

// Review item D-16: mutations that fail on a business rule or in the database
// are audited as "failure" (phase-12 policy). Tags, API tokens and saved-backup
// deletion were added later and returned the error without an audit entry.
func TestMutationFailuresAudited(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "admin", "adminpass1234", "admin")
	client := loginAs(t, ts, "admin", "adminpass1234")

	createTag := func(group, value string) int {
		t.Helper()
		resp := doJSON(t, client, http.MethodPost, ts.URL+"/api/tags", map[string]string{"group": group, "value": value})
		if resp.StatusCode != http.StatusCreated {
			resp.Body.Close()
			t.Fatalf("create %s %q: status %d", group, value, resp.StatusCode)
		}
		return int(parseJSON(t, resp)["id"].(float64))
	}
	createTag("tag", "ALPHA")
	betaID := createTag("tag", "BETA")
	labID := createTag("category", "Lab")
	createTargetViaAPI(t, client, ts.URL, "lab-host", "10.0.0.1", "Lab", "ping")

	otherID := 0
	resp := doJSON(t, client, http.MethodGet, ts.URL+"/api/tags?group=category", nil)
	for _, c := range parseJSONArray(t, resp) {
		if c["value"] == "Other" {
			otherID = int(c["id"].(float64))
		}
	}
	if otherID == 0 {
		t.Fatal("no 'Other' category in a fresh store")
	}

	resp = doJSON(t, client, http.MethodPost, ts.URL+"/api/api-tokens", map[string]string{"name": "grafana"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create token: status %d", resp.StatusCode)
	}
	resp.Body.Close()

	cases := []struct {
		name       string
		method     string
		path       string
		body       any
		wantStatus int
		action     string
	}{
		{"duplicate tag", http.MethodPost, "/api/tags", map[string]string{"group": "tag", "value": "ALPHA"}, http.StatusConflict, "create_tag"},
		{"delete unknown tag", http.MethodDelete, "/api/tags/999999", nil, http.StatusNotFound, "delete_tag"},
		{"delete the Other category", http.MethodDelete, fmt.Sprintf("/api/tags/%d", otherID), nil, http.StatusBadRequest, "delete_tag"},
		{"delete a category in use", http.MethodDelete, fmt.Sprintf("/api/tags/%d", labID), nil, http.StatusConflict, "delete_tag"},
		{"rename unknown tag", http.MethodPut, "/api/tags/999999", map[string]string{"value": "GAMMA"}, http.StatusNotFound, "rename_tag"},
		{"rename the Other category", http.MethodPut, fmt.Sprintf("/api/tags/%d", otherID), map[string]string{"value": "Misc"}, http.StatusBadRequest, "rename_tag"},
		{"rename to an existing value", http.MethodPut, fmt.Sprintf("/api/tags/%d", betaID), map[string]string{"value": "ALPHA"}, http.StatusConflict, "rename_tag"},
		{"duplicate token name", http.MethodPost, "/api/api-tokens", map[string]string{"name": "grafana"}, http.StatusConflict, "api_token_create"},
		{"revoke unknown token", http.MethodDelete, "/api/api-tokens/no-such-token", nil, http.StatusNotFound, "api_token_revoke"},
		{"delete missing saved backup", http.MethodDelete, "/api/backup/full/saved/bekci-full-20300101-000000.tar.gz", nil, http.StatusNotFound, "delete_saved_backup"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := countFailureAudits(t, st, tc.action)
			resp := doJSON(t, client, tc.method, ts.URL+tc.path, tc.body)
			resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status %d, want %d", resp.StatusCode, tc.wantStatus)
			}
			if got := countFailureAudits(t, st, tc.action) - before; got != 1 {
				t.Fatalf("%d new %s failure audit entries, want 1", got, tc.action)
			}
		})
	}
}
