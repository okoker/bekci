package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/bekci/internal/alerter"
	"github.com/bekci/internal/auth"
	"github.com/bekci/internal/scheduler"
	"github.com/bekci/internal/store"
)

func setupTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	st, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	authSvc := auth.New(st, "test-secret-key-for-testing")
	sched := scheduler.New(st)
	alertSvc := alerter.New(st)

	srv := New(st, authSvc, sched, alertSvc, "test", nil, "", "", "", t.TempDir())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { ts.Close(); srv.Close() })

	return ts, st
}

func createUser(t *testing.T, st *store.Store, username, password, role string) {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	u := &store.User{
		ID: uuid.New().String(), Username: username,
		PasswordHash: hash, Role: role, Status: "active",
	}
	if err := st.CreateUser(u); err != nil {
		t.Fatal(err)
	}
}

func loginAs(t *testing.T, ts *httptest.Server, username, password string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	resp, err := client.Post(ts.URL+"/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("login failed: %d %s", resp.StatusCode, string(b))
	}
	return client
}

func parseJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestHealthEndpoint(t *testing.T) {
	ts, _ := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	m := parseJSON(t, resp)
	if m["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", m["status"])
	}
	if m["version"] != "test" {
		t.Fatalf("expected version test, got %v", m["version"])
	}
}

func TestLoginSuccess(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "adminuser", "testpassword12345", "admin")

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	body, _ := json.Marshal(map[string]string{"username": "adminuser", "password": "testpassword12345"})
	resp, err := client.Post(ts.URL+"/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}
	m := parseJSON(t, resp)
	user, ok := m["user"].(map[string]any)
	if !ok {
		t.Fatal("expected user object in response")
	}
	if user["username"] != "adminuser" {
		t.Fatalf("expected username adminuser, got %v", user["username"])
	}

	var foundToken bool
	for _, c := range jar.Cookies(resp.Request.URL) {
		if c.Name == "token" && c.Value != "" {
			foundToken = true
		}
	}
	if !foundToken {
		t.Fatal("expected token cookie to be set")
	}
}

func TestLoginWrongPassword(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "testuser", "correctpassword!!", "admin")

	body, _ := json.Marshal(map[string]string{"username": "testuser", "password": "wrongpassword!!!!"})
	resp, err := http.Post(ts.URL+"/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAuthRequired(t *testing.T) {
	ts, _ := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/api/targets")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestGetMe(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "meuser", "testpassword12345", "admin")
	client := loginAs(t, ts, "meuser", "testpassword12345")

	resp, err := client.Get(ts.URL + "/api/me")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	m := parseJSON(t, resp)
	if m["username"] != "meuser" {
		t.Fatalf("expected username meuser, got %v", m["username"])
	}
	if m["role"] != "admin" {
		t.Fatalf("expected role admin, got %v", m["role"])
	}
}

func TestCreateUserAdminOnly(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "opuser", "testpassword12345", "operator")
	client := loginAs(t, ts, "opuser", "testpassword12345")

	body, _ := json.Marshal(map[string]string{
		"username": "newuser",
		"password": "newuserpassword!",
		"role":     "viewer",
	})
	resp, err := client.Post(ts.URL+"/api/users", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestCreateAndListTargets(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "admin1", "testpassword12345", "admin")
	client := loginAs(t, ts, "admin1", "testpassword12345")

	payload := map[string]any{
		"name":     "Test Target",
		"host":     "example.com",
		"category": "Network",
		"operator": "AND",
		"conditions": []map[string]any{
			{
				"check_type":      "http",
				"check_name":      "HTTP Check",
				"config":          "{}",
				"interval_s":      300,
				"field":           "status",
				"comparator":      "eq",
				"value":           "down",
				"fail_count":      1,
				"fail_window":     0,
				"condition_group": 0,
				"group_operator":  "AND",
			},
		},
	}
	body, _ := json.Marshal(payload)
	resp, err := client.Post(ts.URL+"/api/targets", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(b))
	}
	created := parseJSON(t, resp)
	if created["name"] != "Test Target" {
		t.Fatalf("expected name Test Target, got %v", created["name"])
	}

	resp, err = client.Get(ts.URL + "/api/targets")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	var targets []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		t.Fatal(err)
	}
	if len(targets) == 0 {
		t.Fatal("expected at least one target")
	}
	if targets[0]["name"] != "Test Target" {
		t.Fatalf("expected target name Test Target, got %v", targets[0]["name"])
	}
}

func TestUpdateSettings(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "admin2", "testpassword12345", "admin")
	client := loginAs(t, ts, "admin2", "testpassword12345")

	body, _ := json.Marshal(map[string]string{"history_days": "90"})
	resp, err := client.Do(putReq(t, ts.URL+"/api/settings", body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	resp, err = client.Get(ts.URL + "/api/settings")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	m := parseJSON(t, resp)
	if m["history_days"] != "90" {
		t.Fatalf("expected history_days 90, got %v", m["history_days"])
	}
}

func TestSettingsAdminOnly(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "opuser2", "testpassword12345", "operator")
	client := loginAs(t, ts, "opuser2", "testpassword12345")

	body, _ := json.Marshal(map[string]string{"history_days": "60"})
	resp, err := client.Do(putReq(t, ts.URL+"/api/settings", body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403 for PUT, got %d", resp.StatusCode)
	}

	resp, err = client.Get(ts.URL + "/api/settings")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for GET, got %d", resp.StatusCode)
	}
}

func TestPauseUnpauseTarget(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "admin3", "testpassword12345", "admin")
	client := loginAs(t, ts, "admin3", "testpassword12345")

	payload := map[string]any{
		"name":     "Pause Target",
		"host":     "pause.example.com",
		"category": "Network",
		"operator": "AND",
		"conditions": []map[string]any{
			{
				"check_type":      "ping",
				"check_name":      "Ping Check",
				"config":          "{}",
				"interval_s":      300,
				"field":           "status",
				"comparator":      "eq",
				"value":           "down",
				"fail_count":      1,
				"fail_window":     0,
				"condition_group": 0,
				"group_operator":  "AND",
			},
		},
	}
	body, _ := json.Marshal(payload)
	resp, err := client.Post(ts.URL+"/api/targets", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(b))
	}
	created := parseJSON(t, resp)
	targetID, ok := created["id"].(string)
	if !ok || targetID == "" {
		t.Fatal("expected target id in response")
	}

	resp, err = client.Post(ts.URL+"/api/targets/"+targetID+"/pause", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for pause, got %d", resp.StatusCode)
	}

	resp, err = client.Get(ts.URL + "/api/targets/" + targetID)
	if err != nil {
		t.Fatal(err)
	}
	m := parseJSON(t, resp)
	if m["paused_at"] == nil {
		t.Fatal("expected paused_at to be non-null after pause")
	}

	resp, err = client.Post(ts.URL+"/api/targets/"+targetID+"/unpause", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for unpause, got %d", resp.StatusCode)
	}
}

func TestListAuditLog(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "admin4", "testpassword12345", "admin")
	client := loginAs(t, ts, "admin4", "testpassword12345")

	body, _ := json.Marshal(map[string]string{"history_days": "30"})
	resp, err := client.Do(putReq(t, ts.URL+"/api/settings", body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	resp, err = client.Get(ts.URL + "/api/audit-log")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	m := parseJSON(t, resp)
	entries, ok := m["entries"].([]any)
	if !ok {
		t.Fatal("expected entries array")
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one audit entry")
	}
}

func TestViewerReadOnly(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "viewuser", "testpassword12345", "viewer")
	client := loginAs(t, ts, "viewuser", "testpassword12345")

	payload := map[string]any{
		"name":     "Viewer Target",
		"host":     "viewer.example.com",
		"category": "Network",
		"operator": "AND",
		"conditions": []map[string]any{
			{
				"check_type":      "http",
				"check_name":      "HTTP",
				"config":          "{}",
				"interval_s":      300,
				"field":           "status",
				"comparator":      "eq",
				"value":           "down",
				"fail_count":      1,
				"fail_window":     0,
				"condition_group": 0,
				"group_operator":  "AND",
			},
		},
	}
	body, _ := json.Marshal(payload)
	resp, err := client.Post(ts.URL+"/api/targets", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403 for POST targets as viewer, got %d", resp.StatusCode)
	}

	resp, err = client.Get(ts.URL + "/api/targets")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for GET targets as viewer, got %d", resp.StatusCode)
	}
}

func putReq(t *testing.T, url string, body []byte) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req
}
