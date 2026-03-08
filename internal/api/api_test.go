package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	var sched *scheduler.Scheduler
	alertSvc := alerter.New(st)

	srv := New(st, authSvc, sched, alertSvc, "test", nil, "", "", "", t.TempDir())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { ts.Close(); srv.Close() })

	return ts, st
}

func createUser(t *testing.T, st *store.Store, username, password, role string) {
	t.Helper()
	createUserWithContact(t, st, username, password, role, "", "")
}

func createUserWithContact(t *testing.T, st *store.Store, username, password, role, email, phone string) *store.User {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	u := &store.User{
		ID: uuid.New().String(), Username: username,
		Email: email, Phone: phone,
		PasswordHash: hash, Role: role, Status: "active",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := st.CreateUser(u); err != nil {
		t.Fatal(err)
	}
	return u
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

func loginStatus(t *testing.T, ts *httptest.Server, username, password string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	resp, err := http.Post(ts.URL+"/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
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

func parseJSONArray(t *testing.T, resp *http.Response) []map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var items []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		t.Fatal(err)
	}
	return items
}

func doJSON(t *testing.T, client *http.Client, method, url string, payload any) *http.Response {
	t.Helper()
	if client == nil {
		client = http.DefaultClient
	}

	var body io.Reader = http.NoBody
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func createTargetViaAPI(t *testing.T, client *http.Client, baseURL, name, host, category, checkType string) (string, string) {
	t.Helper()

	payload := map[string]any{
		"name":     name,
		"host":     host,
		"category": category,
		"operator": "AND",
		"conditions": []map[string]any{
			{
				"check_type":      checkType,
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
			},
		},
	}

	resp := doJSON(t, client, http.MethodPost, baseURL+"/api/targets", payload)
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}
	created := parseJSON(t, resp)

	targetID, ok := created["id"].(string)
	if !ok || targetID == "" {
		t.Fatal("expected target id in create target response")
	}

	conditions, ok := created["conditions"].([]any)
	if !ok || len(conditions) == 0 {
		t.Fatal("expected conditions in create target response")
	}
	first, ok := conditions[0].(map[string]any)
	if !ok {
		t.Fatal("expected condition object in create target response")
	}
	checkID, ok := first["check_id"].(string)
	if !ok || checkID == "" {
		t.Fatal("expected check_id in create target response")
	}

	return targetID, checkID
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

func TestLogout(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "logoutuser", "testpassword12345", "admin")
	client := loginAs(t, ts, "logoutuser", "testpassword12345")

	resp := doJSON(t, client, http.MethodPost, ts.URL+"/api/logout", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var cleared bool
	for _, c := range resp.Cookies() {
		if c.Name == "token" && c.Value == "" && c.MaxAge < 0 {
			cleared = true
		}
	}
	resp.Body.Close()
	if !cleared {
		t.Fatal("expected logout to clear token cookie")
	}

	resp, err := client.Get(ts.URL + "/api/me")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401 after logout, got %d", resp.StatusCode)
	}
}

func TestUpdateMe(t *testing.T) {
	ts, st := setupTestServer(t)
	createUserWithContact(t, st, "profileuser", "testpassword12345", "admin", "old@example.com", "+440000")
	client := loginAs(t, ts, "profileuser", "testpassword12345")

	resp := doJSON(t, client, http.MethodPut, ts.URL+"/api/me", map[string]string{
		"email": "new@example.com",
		"phone": "+44123456789",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, err := client.Get(ts.URL + "/api/me")
	if err != nil {
		t.Fatal(err)
	}
	me := parseJSON(t, resp)
	if me["email"] != "new@example.com" {
		t.Fatalf("expected updated email, got %v", me["email"])
	}
	if me["phone"] != "+44123456789" {
		t.Fatalf("expected updated phone, got %v", me["phone"])
	}
}

func TestChangePassword(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "changepwuser", "originalpassword123", "admin")
	client1 := loginAs(t, ts, "changepwuser", "originalpassword123")
	client2 := loginAs(t, ts, "changepwuser", "originalpassword123")

	resp := doJSON(t, client1, http.MethodPut, ts.URL+"/api/me/password", map[string]string{
		"current_password": "wrongpassword12345",
		"new_password":     "updatedpassword12345",
	})
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401 for wrong current password, got %d", resp.StatusCode)
	}
	errResp := parseJSON(t, resp)
	if !strings.Contains(errResp["error"].(string), "current password") {
		t.Fatalf("unexpected error: %v", errResp["error"])
	}

	resp = doJSON(t, client1, http.MethodPut, ts.URL+"/api/me/password", map[string]string{
		"current_password": "originalpassword123",
		"new_password":     "updatedpassword12345",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, err := client2.Get(ts.URL + "/api/me")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected other sessions to be invalidated, got %d", resp.StatusCode)
	}

	if status := loginStatus(t, ts, "changepwuser", "originalpassword123"); status != 401 {
		t.Fatalf("expected old password login to fail, got %d", status)
	}

	client3 := loginAs(t, ts, "changepwuser", "updatedpassword12345")
	resp, err = client3.Get(ts.URL + "/api/me")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected login with new password to succeed, got %d", resp.StatusCode)
	}
}

func TestUserAdminLifecycle(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "adminlifecycle", "testpassword12345", "admin")
	client := loginAs(t, ts, "adminlifecycle", "testpassword12345")

	resp := doJSON(t, client, http.MethodPost, ts.URL+"/api/users", map[string]string{
		"username": "manageduser",
		"email":    "managed@example.com",
		"password": "managedpassword12345",
		"role":     "viewer",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	created := parseJSON(t, resp)
	userID, ok := created["id"].(string)
	if !ok || userID == "" {
		t.Fatal("expected created user id")
	}

	resp, err := client.Get(ts.URL + "/api/users")
	if err != nil {
		t.Fatal(err)
	}
	users := parseJSONArray(t, resp)
	var found bool
	for _, user := range users {
		if user["username"] == "manageduser" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected manageduser in user list")
	}

	resp, err = client.Get(ts.URL + "/api/users/" + userID)
	if err != nil {
		t.Fatal(err)
	}
	got := parseJSON(t, resp)
	if got["role"] != "viewer" {
		t.Fatalf("expected viewer role, got %v", got["role"])
	}

	resp = doJSON(t, client, http.MethodPut, ts.URL+"/api/users/"+userID, map[string]string{
		"email": "updated@example.com",
		"phone": "+44111111111",
		"role":  "operator",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	managed, err := st.GetUserByID(userID)
	if err != nil {
		t.Fatal(err)
	}
	if managed == nil {
		t.Fatal("expected managed user to exist")
	}
	if managed.Email != "updated@example.com" || managed.Phone != "+44111111111" || managed.Role != "operator" {
		t.Fatalf("unexpected user update result: %+v", managed)
	}

	resp = doJSON(t, client, http.MethodPut, ts.URL+"/api/users/"+userID+"/password", map[string]string{
		"password": "managedpassword67890",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	if status := loginStatus(t, ts, "manageduser", "managedpassword12345"); status != 401 {
		t.Fatalf("expected old password login to fail after reset, got %d", status)
	}

	managedClient := loginAs(t, ts, "manageduser", "managedpassword67890")
	resp, err = managedClient.Get(ts.URL + "/api/me")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected reset password login to succeed, got %d", resp.StatusCode)
	}

	resp = doJSON(t, client, http.MethodPut, ts.URL+"/api/users/"+userID+"/suspend", map[string]bool{
		"suspended": true,
	})
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	if status := loginStatus(t, ts, "manageduser", "managedpassword67890"); status != 401 {
		t.Fatalf("expected suspended user login to fail, got %d", status)
	}

	resp = doJSON(t, client, http.MethodPut, ts.URL+"/api/users/"+userID+"/suspend", map[string]bool{
		"suspended": false,
	})
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	managedClient = loginAs(t, ts, "manageduser", "managedpassword67890")
	resp, err = managedClient.Get(ts.URL + "/api/me")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected unsuspended user login to succeed, got %d", resp.StatusCode)
	}
}

func TestLastAdminProtection(t *testing.T) {
	ts, st := setupTestServer(t)
	admin := createUserWithContact(t, st, "soleadmin", "testpassword12345", "admin", "", "")
	client := loginAs(t, ts, "soleadmin", "testpassword12345")

	resp := doJSON(t, client, http.MethodPut, ts.URL+"/api/users/"+admin.ID, map[string]string{
		"role": "viewer",
	})
	if resp.StatusCode != 409 {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
	updateErr := parseJSON(t, resp)
	if !strings.Contains(updateErr["error"].(string), "last active admin") {
		t.Fatalf("unexpected error: %v", updateErr["error"])
	}

	resp = doJSON(t, client, http.MethodPut, ts.URL+"/api/users/"+admin.ID+"/suspend", map[string]bool{
		"suspended": true,
	})
	if resp.StatusCode != 409 {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
	suspendErr := parseJSON(t, resp)
	if !strings.Contains(suspendErr["error"].(string), "last active admin") {
		t.Fatalf("unexpected error: %v", suspendErr["error"])
	}
}

func TestTargetCheckAndDeleteFlow(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "targetadmin", "testpassword12345", "admin")
	client := loginAs(t, ts, "targetadmin", "testpassword12345")

	targetID, checkID := createTargetViaAPI(t, client, ts.URL, "flow-target", "flow.example.com", "Network", "http")

	resp := doJSON(t, client, http.MethodPut, ts.URL+"/api/targets/"+targetID, map[string]any{
		"name":                 "flow-target-updated",
		"host":                 "updated.example.com",
		"description":          "updated description",
		"category":             "Network",
		"operator":             "AND",
		"preferred_check_type": "http",
		"conditions": []map[string]any{
			{
				"check_id":        checkID,
				"check_type":      "http",
				"check_name":      "Updated HTTP Check",
				"config":          "{}",
				"interval_s":      120,
				"field":           "status",
				"comparator":      "eq",
				"value":           "down",
				"fail_count":      2,
				"fail_window":     0,
				"condition_group": 0,
				"group_operator":  "AND",
			},
		},
	})
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	updated := parseJSON(t, resp)
	if updated["name"] != "flow-target-updated" {
		t.Fatalf("expected updated target name, got %v", updated["name"])
	}
	if updated["host"] != "updated.example.com" {
		t.Fatalf("expected updated target host, got %v", updated["host"])
	}

	resp, err := client.Get(ts.URL + "/api/targets/" + targetID + "/checks")
	if err != nil {
		t.Fatal(err)
	}
	checks := parseJSONArray(t, resp)
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
	if checks[0]["name"] != "Updated HTTP Check" {
		t.Fatalf("expected updated check name, got %v", checks[0]["name"])
	}

	now := time.Now()
	for _, result := range []store.CheckResult{
		{CheckID: checkID, Status: "up", ResponseMs: 120, Message: "ok", Metrics: "{}", CheckedAt: now.Add(-2 * time.Hour)},
		{CheckID: checkID, Status: "down", ResponseMs: 350, Message: "failed", Metrics: "{}", CheckedAt: now.Add(-30 * time.Minute)},
	} {
		r := result
		if err := st.SaveResult(&r); err != nil {
			t.Fatal(err)
		}
	}

	resp, err = client.Get(ts.URL + "/api/checks/" + checkID + "/results")
	if err != nil {
		t.Fatal(err)
	}
	results := parseJSONArray(t, resp)
	if len(results) != 2 {
		t.Fatalf("expected 2 check results, got %d", len(results))
	}

	resp = doJSON(t, client, http.MethodPost, ts.URL+"/api/checks/"+checkID+"/run", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	runNow := parseJSON(t, resp)
	if runNow["status"] != "queued" {
		t.Fatalf("expected queued status, got %v", runNow["status"])
	}

	resp = doJSON(t, client, http.MethodDelete, ts.URL+"/api/targets/"+targetID, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, err = client.Get(ts.URL + "/api/targets/" + targetID)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404 after delete, got %d", resp.StatusCode)
	}
}

func TestAlertRecipientsAlertsAndTestEndpoints(t *testing.T) {
	ts, st := setupTestServer(t)
	createUserWithContact(t, st, "alertadmin", "testpassword12345", "admin", "admin@example.com", "+440001")
	recipient := createUserWithContact(t, st, "recipientuser", "testpassword12345", "viewer", "recipient@example.com", "+44123456789")
	client := loginAs(t, ts, "alertadmin", "testpassword12345")

	targetID, _ := createTargetViaAPI(t, client, ts.URL, "alert-target", "alerts.example.com", "Network", "http")

	resp := doJSON(t, client, http.MethodPut, ts.URL+"/api/targets/"+targetID+"/recipients", map[string]any{
		"user_ids": []string{recipient.ID},
	})
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, err := client.Get(ts.URL + "/api/targets/" + targetID + "/recipients")
	if err != nil {
		t.Fatal(err)
	}
	recipients := parseJSONArray(t, resp)
	if len(recipients) != 1 {
		t.Fatalf("expected 1 recipient, got %d", len(recipients))
	}
	if recipients[0]["username"] != "recipientuser" {
		t.Fatalf("expected recipientuser, got %v", recipients[0]["username"])
	}

	target, err := st.GetTarget(targetID)
	if err != nil {
		t.Fatal(err)
	}
	if target == nil || target.RuleID == nil {
		t.Fatal("expected target with rule id")
	}
	if err := st.LogAlert(targetID, *target.RuleID, recipient.ID, "firing", "Target down"); err != nil {
		t.Fatal(err)
	}

	resp, err = client.Get(ts.URL + "/api/alerts?page=1&limit=10")
	if err != nil {
		t.Fatal(err)
	}
	alerts := parseJSON(t, resp)
	entries, ok := alerts["entries"].([]any)
	if !ok {
		t.Fatal("expected entries array in alert history")
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 alert entry, got %d", len(entries))
	}
	if int(alerts["total"].(float64)) != 1 {
		t.Fatalf("expected total=1, got %v", alerts["total"])
	}

	resp = doJSON(t, client, http.MethodPost, ts.URL+"/api/settings/test-email", nil)
	if resp.StatusCode != 500 {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
	emailErr := parseJSON(t, resp)
	if !strings.Contains(emailErr["error"].(string), "failed to send test email") {
		t.Fatalf("unexpected test email error: %v", emailErr["error"])
	}

	resp = doJSON(t, client, http.MethodPost, ts.URL+"/api/settings/test-signal", map[string]string{
		"phone": "+44123456789",
	})
	if resp.StatusCode != 500 {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
	signalErr := parseJSON(t, resp)
	if !strings.Contains(signalErr["error"].(string), "failed to send test signal") {
		t.Fatalf("unexpected test signal error: %v", signalErr["error"])
	}
}

func TestDashboardAndSLAEndpoints(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "dashadmin", "testpassword12345", "admin")
	client := loginAs(t, ts, "dashadmin", "testpassword12345")

	if err := st.SetSettings(map[string]string{"sla_network": "99.9"}); err != nil {
		t.Fatal(err)
	}

	targetID, checkID := createTargetViaAPI(t, client, ts.URL, "dashboard-target", "dashboard.example.com", "Network", "http")
	now := time.Now()
	for _, result := range []store.CheckResult{
		{CheckID: checkID, Status: "down", ResponseMs: 500, Message: "timeout", Metrics: "{}", CheckedAt: now.Add(-26 * time.Hour)},
		{CheckID: checkID, Status: "up", ResponseMs: 120, Message: "ok", Metrics: "{}", CheckedAt: now.Add(-2 * time.Hour)},
		{CheckID: checkID, Status: "up", ResponseMs: 100, Message: "ok", Metrics: "{}", CheckedAt: now.Add(-10 * time.Minute)},
	} {
		r := result
		if err := st.SaveResult(&r); err != nil {
			t.Fatal(err)
		}
	}

	resp, err := client.Get(ts.URL + "/api/dashboard/status")
	if err != nil {
		t.Fatal(err)
	}
	targets := parseJSONArray(t, resp)
	var dashboardTarget map[string]any
	for _, target := range targets {
		if target["id"] == targetID {
			dashboardTarget = target
			break
		}
	}
	if dashboardTarget == nil {
		t.Fatal("expected dashboard target in response")
	}
	checks, ok := dashboardTarget["checks"].([]any)
	if !ok || len(checks) != 1 {
		t.Fatal("expected one dashboard check")
	}
	check, ok := checks[0].(map[string]any)
	if !ok {
		t.Fatal("expected dashboard check object")
	}
	if check["last_status"] != "up" {
		t.Fatalf("expected last_status up, got %v", check["last_status"])
	}
	if check["uptime_90d_pct"].(float64) <= 0 {
		t.Fatalf("expected positive uptime percentage, got %v", check["uptime_90d_pct"])
	}

	resp, err = client.Get(ts.URL + "/api/dashboard/history/" + checkID + "?range=4h")
	if err != nil {
		t.Fatal(err)
	}
	recentResults := parseJSONArray(t, resp)
	if len(recentResults) != 2 {
		t.Fatalf("expected 2 recent results, got %d", len(recentResults))
	}

	resp, err = client.Get(ts.URL + "/api/dashboard/history/" + checkID)
	if err != nil {
		t.Fatal(err)
	}
	daily := parseJSONArray(t, resp)
	if len(daily) < 2 {
		t.Fatalf("expected at least 2 daily uptime entries, got %d", len(daily))
	}

	resp, err = client.Get(ts.URL + "/api/sla/history")
	if err != nil {
		t.Fatal(err)
	}
	sla := parseJSON(t, resp)
	categories, ok := sla["categories"].([]any)
	if !ok {
		t.Fatal("expected categories array")
	}
	var networkFound bool
	for _, raw := range categories {
		category, ok := raw.(map[string]any)
		if !ok || category["name"] != "Network" {
			continue
		}
		targets, ok := category["targets"].([]any)
		if !ok {
			t.Fatal("expected targets array in network category")
		}
		for _, rawTarget := range targets {
			target, ok := rawTarget.(map[string]any)
			if ok && target["id"] == targetID {
				networkFound = true
				break
			}
		}
	}
	if !networkFound {
		t.Fatal("expected target in Network SLA category")
	}
}

func TestSOCPublicStatusAndHistory(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "socadmin", "testpassword12345", "admin")
	client := loginAs(t, ts, "socadmin", "testpassword12345")

	_, checkID := createTargetViaAPI(t, client, ts.URL, "soc-target", "soc.example.com", "Network", "http")
	if err := st.SaveResult(&store.CheckResult{
		CheckID:    checkID,
		Status:     "up",
		ResponseMs: 42,
		Message:    "ok",
		Metrics:    "{}",
		CheckedAt:  time.Now().Add(-15 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(ts.URL + "/api/soc/status")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401 when soc_public is disabled, got %d", resp.StatusCode)
	}

	resp = doJSON(t, client, http.MethodPut, ts.URL+"/api/settings", map[string]string{
		"soc_public": "true",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, err = http.Get(ts.URL + "/api/soc/status")
	if err != nil {
		t.Fatal(err)
	}
	targets := parseJSONArray(t, resp)
	if len(targets) == 0 {
		t.Fatal("expected public SOC status to return targets")
	}

	resp, err = http.Get(ts.URL + "/api/soc/history/" + checkID + "?range=4h")
	if err != nil {
		t.Fatal(err)
	}
	history := parseJSONArray(t, resp)
	if len(history) != 1 {
		t.Fatalf("expected 1 SOC history result, got %d", len(history))
	}
}

func TestCreateTargetInvalidConfig(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "cfgadmin", "testpassword12345", "admin")
	client := loginAs(t, ts, "cfgadmin", "testpassword12345")

	payload := map[string]any{
		"name":     "Bad Config Target",
		"host":     "badconfig.example.com",
		"category": "Network",
		"operator": "AND",
		"conditions": []map[string]any{
			{
				"check_type":      "http",
				"check_name":      "HTTP Check",
				"config":          "{invalid json",
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
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for invalid config JSON, got %d", resp.StatusCode)
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
