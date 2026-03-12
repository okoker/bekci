package alerter

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/bekci/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func createAlertUser(t *testing.T, s *store.Store, username, email, phone string) *store.User {
	t.Helper()
	now := time.Now()
	u := &store.User{
		ID:           uuid.New().String(),
		Username:     username,
		Email:        email,
		Phone:        phone,
		PasswordHash: "hash",
		Role:         "viewer",
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.CreateUser(u); err != nil {
		t.Fatal(err)
	}
	return u
}

func createAlertTarget(t *testing.T, s *store.Store, name string) *store.Target {
	t.Helper()
	target := &store.Target{
		Name:               name,
		Host:               name + ".example.com",
		Enabled:            true,
		PreferredCheckType: "http",
		Operator:           "AND",
		Category:           "Network",
	}
	conds := []store.TargetCondition{
		{
			CheckType:     "http",
			CheckName:     name + " HTTP",
			Config:        "{}",
			IntervalS:     60,
			Field:         "status",
			Comparator:    "eq",
			Value:         "down",
			FailCount:     1,
			FailWindow:    0,
			GroupOperator: "AND",
		},
	}
	if err := s.CreateTargetWithConditions(target, conds, ""); err != nil {
		t.Fatal(err)
	}
	if target.RuleID == nil {
		t.Fatal("expected target rule id to be populated")
	}
	return target
}

func configureSignalAlerting(t *testing.T, s *store.Store, apiURL string) {
	t.Helper()
	if err := s.SetSettings(map[string]string{
		"alert_method":    "signal",
		"signal_api_url":  apiURL,
		"signal_number":   "+1000",
		"signal_username": "user",
		"signal_password": "pass",
	}); err != nil {
		t.Fatal(err)
	}
}

func configureEmailAlerting(t *testing.T, s *store.Store, apiKey, from string) {
	t.Helper()
	if err := s.SetSettings(map[string]string{
		"alert_method":     "email",
		"resend_api_key":   apiKey,
		"alert_from_email": from,
	}); err != nil {
		t.Fatal(err)
	}
}

func withResendURL(t *testing.T, url string) {
	t.Helper()
	prev := resendAPIURL
	resendAPIURL = url
	t.Cleanup(func() {
		resendAPIURL = prev
	})
}

func listAlertHistory(t *testing.T, s *store.Store) ([]store.AlertHistoryItem, int) {
	t.Helper()
	entries, total, err := s.ListAlertHistory(50, 0)
	if err != nil {
		t.Fatal(err)
	}
	return entries, total
}

func TestRenderEmailAlert(t *testing.T) {
	subj, body := RenderEmailAlert("MyServer", "10.0.0.1", "unhealthy", nil, time.Now())
	if !strings.Contains(subj, "[ALERT]") {
		t.Fatalf("unhealthy subject missing [ALERT]: %s", subj)
	}
	if !strings.Contains(subj, "DOWN") {
		t.Fatalf("unhealthy subject missing DOWN: %s", subj)
	}
	if !strings.Contains(body, "#dc2626") {
		t.Fatalf("unhealthy body missing red color #dc2626")
	}

	subj, body = RenderEmailAlert("MyServer", "10.0.0.1", "healthy", nil, time.Now())
	if !strings.Contains(subj, "[RECOVERED]") {
		t.Fatalf("healthy subject missing [RECOVERED]: %s", subj)
	}
	if !strings.Contains(subj, "UP") {
		t.Fatalf("healthy subject missing UP: %s", subj)
	}
	if !strings.Contains(body, "#16a34a") {
		t.Fatalf("healthy body missing green color #16a34a")
	}
}

func TestRenderSignalAlert(t *testing.T) {
	msg := RenderSignalAlert("MyServer", "10.0.0.1", "unhealthy", nil, time.Now())
	if !strings.Contains(msg, "ALERT") {
		t.Fatalf("unhealthy message missing ALERT: %s", msg)
	}
	if !strings.Contains(msg, "DOWN") {
		t.Fatalf("unhealthy message missing DOWN: %s", msg)
	}

	msg = RenderSignalAlert("MyServer", "10.0.0.1", "healthy", nil, time.Now())
	if !strings.Contains(msg, "RECOVERED") {
		t.Fatalf("healthy message missing RECOVERED: %s", msg)
	}
	if !strings.Contains(msg, "UP") {
		t.Fatalf("healthy message missing UP: %s", msg)
	}
}

func TestSendSignalSuccess(t *testing.T) {
	var gotReq *http.Request
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReq = r
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	err := SendSignal(srv.URL, "user", "pass", "+1234", []string{"+5678"}, "test message")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if gotReq.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("expected Content-Type application/json, got: %s", gotReq.Header.Get("Content-Type"))
	}

	u, p, ok := gotReq.BasicAuth()
	if !ok {
		t.Fatalf("expected Basic Auth to be set")
	}
	if u != "user" || p != "pass" {
		t.Fatalf("expected Basic Auth user/pass, got %s/%s", u, p)
	}

	var payload map[string]any
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("failed to unmarshal request body: %v", err)
	}
	if payload["message"] != "test message" {
		t.Fatalf("expected message 'test message', got: %v", payload["message"])
	}
	if payload["number"] != "+1234" {
		t.Fatalf("expected number '+1234', got: %v", payload["number"])
	}
}

func TestSendSignalFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	err := SendSignal(srv.URL, "user", "pass", "+1234", []string{"+5678"}, "test")
	if err == nil {
		t.Fatalf("expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("expected error to contain '400', got: %v", err)
	}
}

func TestSendEmailSuccess(t *testing.T) {
	var gotReq *http.Request
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReq = r
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"msg_123"}`))
	}))
	defer srv.Close()

	withResendURL(t, srv.URL)

	err := SendEmail("api-key", "from@example.com", []string{"to@example.com"}, "subject", "<b>hello</b>")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if gotReq.Header.Get("Authorization") != "Bearer api-key" {
		t.Fatalf("expected Authorization header, got %q", gotReq.Header.Get("Authorization"))
	}
	if gotReq.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", gotReq.Header.Get("Content-Type"))
	}

	var payload map[string]any
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("unmarshal email payload: %v", err)
	}
	if payload["from"] != "from@example.com" {
		t.Fatalf("expected from@example.com, got %v", payload["from"])
	}
	if payload["subject"] != "subject" {
		t.Fatalf("expected subject, got %v", payload["subject"])
	}
	if payload["html"] != "<b>hello</b>" {
		t.Fatalf("expected html body, got %v", payload["html"])
	}
}

func TestSendEmailFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("resend broke"))
	}))
	defer srv.Close()

	withResendURL(t, srv.URL)

	err := SendEmail("api-key", "from@example.com", []string{"to@example.com"}, "subject", "<b>hello</b>")
	if err == nil {
		t.Fatal("expected SendEmail to fail")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected error to contain status code, got %v", err)
	}
}

func TestSendTestEmailNotConfigured(t *testing.T) {
	s := newTestStore(t)
	svc := New(s)

	err := svc.SendTestEmail("test@example.com")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got: %v", err)
	}
}

func TestSendTestSignalNotConfigured(t *testing.T) {
	s := newTestStore(t)
	svc := New(s)

	err := svc.SendTestSignal("+1234")
	if !errors.Is(err, ErrSignalNotConfigured) {
		t.Fatalf("expected ErrSignalNotConfigured, got: %v", err)
	}
}

func TestSendTestEmailSuccess(t *testing.T) {
	s := newTestStore(t)
	svc := New(s)

	var gotBody []byte
	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"msg_123"}`))
	}))
	defer srv.Close()

	withResendURL(t, srv.URL)
	configureEmailAlerting(t, s, "email-key", "alerts@example.com")

	if err := svc.SendTestEmail("test@example.com"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if hitCount != 1 {
		t.Fatalf("expected 1 email request, got %d", hitCount)
	}

	var payload map[string]any
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("unmarshal email payload: %v", err)
	}
	to, ok := payload["to"].([]any)
	if !ok || len(to) != 1 || to[0] != "test@example.com" {
		t.Fatalf("unexpected to payload: %#v", payload["to"])
	}
	if !strings.Contains(payload["subject"].(string), "Test Email") {
		t.Fatalf("expected test email subject, got %v", payload["subject"])
	}
}

func TestDispatchNoRecipients(t *testing.T) {
	s := newTestStore(t)
	target := createAlertTarget(t, s, "norecipients")
	svc := New(s)

	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		w.WriteHeader(200)
	}))
	defer srv.Close()

	configureSignalAlerting(t, s, srv.URL)
	svc.Dispatch(*target.RuleID, "healthy", "unhealthy")

	if hitCount != 0 {
		t.Fatalf("expected no signal requests, got %d", hitCount)
	}
	_, total := listAlertHistory(t, s)
	if total != 0 {
		t.Fatalf("expected no alert history entries, got %d", total)
	}
}

func TestDispatchRespectsCooldown(t *testing.T) {
	s := newTestStore(t)
	target := createAlertTarget(t, s, "cooldown")
	recipient := createAlertUser(t, s, "cooldown-user", "", "+441111")
	svc := New(s)

	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		w.WriteHeader(200)
	}))
	defer srv.Close()

	configureSignalAlerting(t, s, srv.URL)
	if err := s.SetSettings(map[string]string{"alert_cooldown_s": "3600"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTargetRecipients(target.ID, []string{recipient.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.LogAlert(target.ID, *target.RuleID, recipient.ID, "firing", "prior alert"); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch(*target.RuleID, "healthy", "unhealthy")

	if hitCount != 0 {
		t.Fatalf("expected cooldown to suppress signal request, got %d", hitCount)
	}
	_, total := listAlertHistory(t, s)
	if total != 1 {
		t.Fatalf("expected existing alert history only, got %d", total)
	}
}

func TestDispatchSignalSuccessLogsAlert(t *testing.T) {
	s := newTestStore(t)
	target := createAlertTarget(t, s, "dispatch-success")
	recipient := createAlertUser(t, s, "signal-user", "", "+442222")
	svc := New(s)

	var gotBody []byte
	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	configureSignalAlerting(t, s, srv.URL)
	if err := s.SetTargetRecipients(target.ID, []string{recipient.ID}); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch(*target.RuleID, "healthy", "unhealthy")

	if hitCount != 1 {
		t.Fatalf("expected 1 signal request, got %d", hitCount)
	}

	var payload map[string]any
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("unmarshal signal payload: %v", err)
	}
	if payload["number"] != "+1000" {
		t.Fatalf("expected sender number +1000, got %v", payload["number"])
	}
	recipients, ok := payload["recipients"].([]any)
	if !ok || len(recipients) != 1 || recipients[0] != "+442222" {
		t.Fatalf("unexpected recipients payload: %#v", payload["recipients"])
	}
	if !strings.Contains(payload["message"].(string), "ALERT") {
		t.Fatalf("expected ALERT message, got %v", payload["message"])
	}

	entries, total := listAlertHistory(t, s)
	if total != 1 {
		t.Fatalf("expected 1 alert history entry, got %d", total)
	}
	if entries[0].AlertType != "firing" {
		t.Fatalf("expected firing alert type, got %q", entries[0].AlertType)
	}
	if !strings.HasPrefix(entries[0].Message, "[Signal] ") {
		t.Fatalf("expected signal summary, got %q", entries[0].Message)
	}
}

func TestDispatchSignalFailureStillLogsAlert(t *testing.T) {
	s := newTestStore(t)
	target := createAlertTarget(t, s, "dispatch-failure")
	recipient := createAlertUser(t, s, "failure-user", "", "+443333")
	svc := New(s)

	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		w.WriteHeader(400)
		w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	configureSignalAlerting(t, s, srv.URL)
	if err := s.SetTargetRecipients(target.ID, []string{recipient.ID}); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch(*target.RuleID, "healthy", "unhealthy")

	if hitCount != 1 {
		t.Fatalf("expected 1 signal request, got %d", hitCount)
	}
	entries, total := listAlertHistory(t, s)
	if total != 1 {
		t.Fatalf("expected failed send to still log alert, got %d entries", total)
	}
	if entries[0].AlertType != "firing" {
		t.Fatalf("expected firing alert type, got %q", entries[0].AlertType)
	}
}

func TestDispatchRecoveryBypassesCooldown(t *testing.T) {
	s := newTestStore(t)
	target := createAlertTarget(t, s, "dispatch-recovery")
	recipient := createAlertUser(t, s, "recovery-user", "", "+444444")
	svc := New(s)

	var gotBody []byte
	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	configureSignalAlerting(t, s, srv.URL)
	if err := s.SetSettings(map[string]string{"alert_cooldown_s": "3600"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTargetRecipients(target.ID, []string{recipient.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.LogAlert(target.ID, *target.RuleID, recipient.ID, "firing", "prior alert"); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch(*target.RuleID, "unhealthy", "healthy")

	if hitCount != 1 {
		t.Fatalf("expected recovery to bypass cooldown and send, got %d requests", hitCount)
	}

	var payload map[string]any
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("unmarshal signal payload: %v", err)
	}
	if !strings.Contains(payload["message"].(string), "RECOVERED") {
		t.Fatalf("expected RECOVERED message, got %v", payload["message"])
	}

	entries, total := listAlertHistory(t, s)
	if total != 2 {
		t.Fatalf("expected prior alert plus recovery alert, got %d", total)
	}
	var foundRecovery bool
	for _, entry := range entries {
		if entry.AlertType == "recovery" {
			foundRecovery = true
		}
	}
	if !foundRecovery {
		t.Fatal("expected recovery alert entry to be logged")
	}
}

func TestDispatchEmailSuccessLogsAlert(t *testing.T) {
	s := newTestStore(t)
	target := createAlertTarget(t, s, "dispatch-email")
	recipient := createAlertUser(t, s, "email-user", "email-user@example.com", "")
	svc := New(s)

	var gotBody []byte
	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"msg_123"}`))
	}))
	defer srv.Close()

	withResendURL(t, srv.URL)
	configureEmailAlerting(t, s, "email-key", "alerts@example.com")
	if err := s.SetTargetRecipients(target.ID, []string{recipient.ID}); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch(*target.RuleID, "healthy", "unhealthy")

	if hitCount != 1 {
		t.Fatalf("expected 1 email request, got %d", hitCount)
	}

	var payload map[string]any
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("unmarshal email payload: %v", err)
	}
	if payload["from"] != "alerts@example.com" {
		t.Fatalf("expected alerts@example.com sender, got %v", payload["from"])
	}
	if !strings.Contains(payload["subject"].(string), "[ALERT]") {
		t.Fatalf("expected alert subject, got %v", payload["subject"])
	}

	entries, total := listAlertHistory(t, s)
	if total != 1 {
		t.Fatalf("expected 1 alert history entry, got %d", total)
	}
	if entries[0].AlertType != "firing" {
		t.Fatalf("expected firing alert type, got %q", entries[0].AlertType)
	}
	if !strings.Contains(entries[0].Message, "[ALERT]") {
		t.Fatalf("expected email alert subject logged, got %q", entries[0].Message)
	}
}

func TestCheckRealertsDisabled(t *testing.T) {
	s := newTestStore(t)
	target := createAlertTarget(t, s, "realert-disabled")
	recipient := createAlertUser(t, s, "disabled-user", "", "+445555")
	svc := New(s)

	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		w.WriteHeader(200)
	}))
	defer srv.Close()

	configureSignalAlerting(t, s, srv.URL)
	if err := s.SetTargetRecipients(target.ID, []string{recipient.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateRuleState(*target.RuleID, "unhealthy"); err != nil {
		t.Fatal(err)
	}
	if err := s.LogAlert(target.ID, *target.RuleID, recipient.ID, "firing", "prior alert"); err != nil {
		t.Fatal(err)
	}

	svc.CheckRealerts()

	if hitCount != 0 {
		t.Fatalf("expected disabled re-alerting to send nothing, got %d requests", hitCount)
	}
	_, total := listAlertHistory(t, s)
	if total != 1 {
		t.Fatalf("expected original alert history only, got %d", total)
	}
}

func TestCheckRealertsSkipsRecentAlert(t *testing.T) {
	s := newTestStore(t)
	target := createAlertTarget(t, s, "realert-recent")
	recipient := createAlertUser(t, s, "recent-user", "", "+446666")
	svc := New(s)

	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		w.WriteHeader(200)
	}))
	defer srv.Close()

	configureSignalAlerting(t, s, srv.URL)
	if err := s.SetSettings(map[string]string{"alert_realert_s": "3600"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTargetRecipients(target.ID, []string{recipient.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateRuleState(*target.RuleID, "unhealthy"); err != nil {
		t.Fatal(err)
	}
	if err := s.LogAlert(target.ID, *target.RuleID, recipient.ID, "firing", "recent alert"); err != nil {
		t.Fatal(err)
	}

	svc.CheckRealerts()

	if hitCount != 0 {
		t.Fatalf("expected recent alert to suppress re-alerting, got %d requests", hitCount)
	}
	_, total := listAlertHistory(t, s)
	if total != 1 {
		t.Fatalf("expected original alert history only, got %d", total)
	}
}

func TestCheckRealertsSignalSuccess(t *testing.T) {
	s := newTestStore(t)
	target := createAlertTarget(t, s, "realert-success")
	recipient := createAlertUser(t, s, "realert-user", "", "+447777")
	svc := New(s)

	var gotBody []byte
	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	configureSignalAlerting(t, s, srv.URL)
	if err := s.SetSettings(map[string]string{"alert_realert_s": "1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTargetRecipients(target.ID, []string{recipient.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateRuleState(*target.RuleID, "unhealthy"); err != nil {
		t.Fatal(err)
	}
	if err := s.LogAlert(target.ID, *target.RuleID, recipient.ID, "firing", "prior alert"); err != nil {
		t.Fatal(err)
	}

	time.Sleep(1100 * time.Millisecond)
	svc.CheckRealerts()

	if hitCount != 1 {
		t.Fatalf("expected 1 re-alert signal request, got %d", hitCount)
	}

	var payload map[string]any
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("unmarshal signal payload: %v", err)
	}
	if !strings.Contains(payload["message"].(string), "RE-ALERT") {
		t.Fatalf("expected RE-ALERT message, got %v", payload["message"])
	}

	entries, total := listAlertHistory(t, s)
	if total != 2 {
		t.Fatalf("expected original alert plus re-alert, got %d", total)
	}
	var foundRealert bool
	for _, entry := range entries {
		if entry.AlertType == "re-alert" && strings.HasPrefix(entry.Message, "[Signal RE-ALERT]") {
			foundRealert = true
		}
	}
	if !foundRealert {
		t.Fatal("expected re-alert entry to be logged")
	}
}

func TestCheckRealertsEmailSuccess(t *testing.T) {
	s := newTestStore(t)
	target := createAlertTarget(t, s, "realert-email")
	recipient := createAlertUser(t, s, "realert-email-user", "realert@example.com", "")
	svc := New(s)

	var gotBody []byte
	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"msg_123"}`))
	}))
	defer srv.Close()

	withResendURL(t, srv.URL)
	configureEmailAlerting(t, s, "email-key", "alerts@example.com")
	if err := s.SetSettings(map[string]string{"alert_realert_s": "1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTargetRecipients(target.ID, []string{recipient.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateRuleState(*target.RuleID, "unhealthy"); err != nil {
		t.Fatal(err)
	}
	if err := s.LogAlert(target.ID, *target.RuleID, recipient.ID, "firing", "prior alert"); err != nil {
		t.Fatal(err)
	}

	time.Sleep(1100 * time.Millisecond)
	svc.CheckRealerts()

	if hitCount != 1 {
		t.Fatalf("expected 1 re-alert email request, got %d", hitCount)
	}

	var payload map[string]any
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("unmarshal email payload: %v", err)
	}
	if !strings.Contains(payload["subject"].(string), "[RE-ALERT]") {
		t.Fatalf("expected re-alert email subject, got %v", payload["subject"])
	}

	entries, total := listAlertHistory(t, s)
	if total != 2 {
		t.Fatalf("expected original alert plus re-alert, got %d", total)
	}
	var foundRealert bool
	for _, entry := range entries {
		if entry.AlertType == "re-alert" && strings.Contains(entry.Message, "[RE-ALERT]") {
			foundRealert = true
		}
	}
	if !foundRealert {
		t.Fatal("expected re-alert email entry to be logged")
	}
}

func TestSendTestSignalSuccess(t *testing.T) {
	s := newTestStore(t)
	svc := New(s)

	var gotBody []byte
	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	configureSignalAlerting(t, s, srv.URL)

	if err := svc.SendTestSignal("+448888"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if hitCount != 1 {
		t.Fatalf("expected 1 signal request, got %d", hitCount)
	}

	var payload map[string]any
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("unmarshal signal payload: %v", err)
	}
	recipients, ok := payload["recipients"].([]any)
	if !ok || len(recipients) != 1 || recipients[0] != "+448888" {
		t.Fatalf("unexpected recipients payload: %#v", payload["recipients"])
	}
	if !strings.Contains(payload["message"].(string), "Test Signal") {
		t.Fatalf("expected test signal message, got %v", payload["message"])
	}
}

func TestSendWebhookSuccess(t *testing.T) {
	var gotReq *http.Request
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReq = r
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	payload := WebhookPayload{
		Event:         "firing",
		Target:        "test-server",
		TargetAddress: "10.0.0.1",
		Category:      "Network",
		Message:       "Test alert",
		FailingChecks: []FailingCheck{
			{Type: "ping", Detail: "100% packet loss"},
		},
		Timestamp: "2026-03-12T14:30:00Z",
	}

	auth := WebhookAuth{Type: "bearer", BearerToken: "my-token"}
	err := SendWebhook(srv.URL, auth, false, payload)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if gotReq.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("expected Content-Type application/json, got: %s", gotReq.Header.Get("Content-Type"))
	}
	if gotReq.Header.Get("Authorization") != "Bearer my-token" {
		t.Fatalf("expected Bearer auth header, got: %s", gotReq.Header.Get("Authorization"))
	}

	var got WebhookPayload
	if err := json.Unmarshal(gotBody, &got); err != nil {
		t.Fatalf("unmarshal webhook body: %v", err)
	}
	if got.Event != "firing" {
		t.Fatalf("expected event=firing, got %q", got.Event)
	}
	if got.Target != "test-server" {
		t.Fatalf("expected target=test-server, got %q", got.Target)
	}
	if len(got.FailingChecks) != 1 || got.FailingChecks[0].Type != "ping" {
		t.Fatalf("unexpected failing_checks: %+v", got.FailingChecks)
	}
}

func TestSendWebhookNoToken(t *testing.T) {
	var gotReq *http.Request

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReq = r
		w.WriteHeader(200)
	}))
	defer srv.Close()

	payload := WebhookPayload{Event: "firing", Target: "t", Timestamp: "now"}
	err := SendWebhook(srv.URL, WebhookAuth{}, false, payload)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if gotReq.Header.Get("Authorization") != "" {
		t.Fatalf("expected no Authorization header when no auth, got: %s", gotReq.Header.Get("Authorization"))
	}
}

func TestSendWebhookFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	payload := WebhookPayload{Event: "firing", Target: "t", Timestamp: "now"}
	err := SendWebhook(srv.URL, WebhookAuth{Type: "bearer", BearerToken: "token"}, false, payload)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected error to contain '500', got: %v", err)
	}
}

func TestSendWebhookConnectionRefused(t *testing.T) {
	payload := WebhookPayload{Event: "firing", Target: "t", Timestamp: "now"}
	err := SendWebhook("http://127.0.0.1:1", WebhookAuth{}, false, payload)
	if err == nil {
		t.Fatal("expected error for connection refused")
	}
}

// T-AL31: SendWebhook with Basic Auth sends correct Authorization header
func TestSendWebhookBasicAuth(t *testing.T) {
	var gotReq *http.Request

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReq = r
		w.WriteHeader(200)
	}))
	defer srv.Close()

	payload := WebhookPayload{Event: "firing", Target: "t", Timestamp: "now"}
	auth := WebhookAuth{Type: "basic", BasicUsername: "xsoar", BasicPassword: "s3cret"}
	err := SendWebhook(srv.URL, auth, false, payload)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	user, pass, ok := gotReq.BasicAuth()
	if !ok {
		t.Fatal("expected Basic auth header to be present")
	}
	if user != "xsoar" || pass != "s3cret" {
		t.Fatalf("expected xsoar:s3cret, got %s:%s", user, pass)
	}
}

func configureWebhookAlerting(t *testing.T, s *store.Store, webhookURL string) {
	t.Helper()
	if err := s.SetSettings(map[string]string{
		"webhook_enabled": "true",
		"webhook_url":     webhookURL,
	}); err != nil {
		t.Fatal(err)
	}
}

func configureWebhookAlertingWithToken(t *testing.T, s *store.Store, webhookURL, token string) {
	t.Helper()
	if err := s.SetSettings(map[string]string{
		"webhook_enabled":      "true",
		"webhook_url":          webhookURL,
		"webhook_auth_type":    "bearer",
		"webhook_bearer_token": token,
	}); err != nil {
		t.Fatal(err)
	}
}

func configureWebhookAlertingWithBasicAuth(t *testing.T, s *store.Store, webhookURL, user, pass string) {
	t.Helper()
	if err := s.SetSettings(map[string]string{
		"webhook_enabled":        "true",
		"webhook_url":            webhookURL,
		"webhook_auth_type":      "basic",
		"webhook_basic_username": user,
		"webhook_basic_password": pass,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDispatchWebhookFiring(t *testing.T) {
	s := newTestStore(t)
	target := createAlertTarget(t, s, "webhook-fire")
	recipient := createAlertUser(t, s, "wh-user", "wh@example.com", "")
	svc := New(s)

	var gotBody []byte
	var gotReq *http.Request
	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		gotReq = r
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	configureWebhookAlertingWithToken(t, s, srv.URL, "secret-token")
	if err := s.SetTargetRecipients(target.ID, []string{recipient.ID}); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch(*target.RuleID, "healthy", "unhealthy")

	if hitCount != 1 {
		t.Fatalf("expected 1 webhook request, got %d", hitCount)
	}

	if gotReq.Header.Get("Authorization") != "Bearer secret-token" {
		t.Fatalf("expected Bearer auth, got: %s", gotReq.Header.Get("Authorization"))
	}

	var payload WebhookPayload
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("unmarshal webhook payload: %v", err)
	}
	if payload.Event != "firing" {
		t.Fatalf("expected event=firing, got %q", payload.Event)
	}
	if payload.Target != "webhook-fire" {
		t.Fatalf("expected target=webhook-fire, got %q", payload.Target)
	}
	if payload.Category != "Network" {
		t.Fatalf("expected category=Network, got %q", payload.Category)
	}

	entries, total := listAlertHistory(t, s)
	if total < 1 {
		t.Fatalf("expected at least 1 alert history entry, got %d", total)
	}
	var foundWebhook bool
	for _, e := range entries {
		if strings.HasPrefix(e.Message, "[Webhook] ") {
			foundWebhook = true
		}
	}
	if !foundWebhook {
		t.Fatal("expected webhook alert history entry")
	}
}

func TestDispatchWebhookRecovery(t *testing.T) {
	s := newTestStore(t)
	target := createAlertTarget(t, s, "webhook-recovery")
	recipient := createAlertUser(t, s, "wh-rec-user", "wh-rec@example.com", "")
	svc := New(s)

	var gotBody []byte
	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	configureWebhookAlerting(t, s, srv.URL)
	if err := s.SetTargetRecipients(target.ID, []string{recipient.ID}); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch(*target.RuleID, "unhealthy", "healthy")

	if hitCount != 1 {
		t.Fatalf("expected 1 webhook request, got %d", hitCount)
	}

	var payload WebhookPayload
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("unmarshal webhook payload: %v", err)
	}
	if payload.Event != "recovery" {
		t.Fatalf("expected event=recovery, got %q", payload.Event)
	}
}

func TestDispatchWebhookDisabled(t *testing.T) {
	s := newTestStore(t)
	target := createAlertTarget(t, s, "webhook-disabled")
	recipient := createAlertUser(t, s, "wh-disabled-user", "wh-dis@example.com", "")
	svc := New(s)

	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// webhook_enabled defaults to empty/false
	if err := s.SetSettings(map[string]string{"webhook_url": srv.URL}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTargetRecipients(target.ID, []string{recipient.ID}); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch(*target.RuleID, "healthy", "unhealthy")

	if hitCount != 0 {
		t.Fatalf("expected 0 webhook requests when disabled, got %d", hitCount)
	}
}

func TestDispatchWebhookFailureSetsLastError(t *testing.T) {
	s := newTestStore(t)
	target := createAlertTarget(t, s, "webhook-fail")
	recipient := createAlertUser(t, s, "wh-fail-user", "wh-fail@example.com", "")
	svc := New(s)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("server error"))
	}))
	defer srv.Close()

	configureWebhookAlerting(t, s, srv.URL)
	if err := s.SetTargetRecipients(target.ID, []string{recipient.ID}); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch(*target.RuleID, "healthy", "unhealthy")

	lastErr, _ := s.GetSetting("webhook_last_error")
	if lastErr == "" {
		t.Fatal("expected webhook_last_error to be set after failure")
	}
}

func TestSendTestWebhookNotConfigured(t *testing.T) {
	s := newTestStore(t)
	svc := New(s)

	err := svc.SendTestWebhook()
	if !errors.Is(err, ErrWebhookNotConfigured) {
		t.Fatalf("expected ErrWebhookNotConfigured, got: %v", err)
	}
}

func TestSendTestWebhookSuccess(t *testing.T) {
	s := newTestStore(t)
	svc := New(s)

	var gotBody []byte
	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	configureWebhookAlerting(t, s, srv.URL)

	if err := svc.SendTestWebhook(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if hitCount != 1 {
		t.Fatalf("expected 1 webhook request, got %d", hitCount)
	}

	var payload WebhookPayload
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Event != "test" {
		t.Fatalf("expected event=test, got %q", payload.Event)
	}
}

// T-AL32: Dispatch with Basic Auth sends correct credentials
func TestDispatchWebhookBasicAuth(t *testing.T) {
	s := newTestStore(t)
	target := createAlertTarget(t, s, "webhook-basic")
	recipient := createAlertUser(t, s, "wh-basic-user", "wh-basic@example.com", "")
	svc := New(s)

	var gotReq *http.Request
	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		gotReq = r
		w.WriteHeader(200)
	}))
	defer srv.Close()

	configureWebhookAlertingWithBasicAuth(t, s, srv.URL, "xsoar-user", "xsoar-pass")
	if err := s.SetTargetRecipients(target.ID, []string{recipient.ID}); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch(*target.RuleID, "healthy", "unhealthy")

	if hitCount != 1 {
		t.Fatalf("expected 1 webhook request, got %d", hitCount)
	}

	user, pass, ok := gotReq.BasicAuth()
	if !ok {
		t.Fatal("expected Basic auth header in dispatch")
	}
	if user != "xsoar-user" || pass != "xsoar-pass" {
		t.Fatalf("expected xsoar-user:xsoar-pass, got %s:%s", user, pass)
	}
}

// T-AL33: SendTestWebhook with Basic Auth sends correct credentials
func TestSendTestWebhookBasicAuth(t *testing.T) {
	s := newTestStore(t)
	svc := New(s)

	var gotReq *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReq = r
		w.WriteHeader(200)
	}))
	defer srv.Close()

	configureWebhookAlertingWithBasicAuth(t, s, srv.URL, "test-user", "test-pass")

	if err := svc.SendTestWebhook(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	user, pass, ok := gotReq.BasicAuth()
	if !ok {
		t.Fatal("expected Basic auth header in test webhook")
	}
	if user != "test-user" || pass != "test-pass" {
		t.Fatalf("expected test-user:test-pass, got %s:%s", user, pass)
	}
}
