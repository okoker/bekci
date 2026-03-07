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
