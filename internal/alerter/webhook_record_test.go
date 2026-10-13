package alerter

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bekci/internal/store"
)

// webhookReceiver answers every POST with status and returns its URL and the
// raw body of the last payload received.
func webhookReceiver(t *testing.T, status int) (string, func() []byte) {
	t.Helper()
	var mu sync.Mutex
	var last []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		last = body
		mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func() []byte {
		mu.Lock()
		defer mu.Unlock()
		return last
	}
}

// webhookAudits returns the webhook_dispatch audit entries as "resource|detail|status".
func webhookAudits(t *testing.T, st *store.Store) []string {
	t.Helper()
	entries, _, err := st.ListAuditEntries(100, 0, "webhook_dispatch")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if e.Action == "webhook_dispatch" {
			out = append(out, e.ResourceID+"|"+e.Detail+"|"+e.Status)
		}
	}
	return out
}

// T-AL46: a system alert's webhook carries a timestamp and empty arrays (not
// null), and records its outcome like every other delivery: last success or
// error setting plus a webhook_dispatch audit entry (SR-20, review item D-32).
func TestSendSystemAlertWebhookRecorded(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		st := newTestStore(t)
		url, body := webhookReceiver(t, http.StatusOK)
		configureWebhookAlerting(t, st, url)
		if err := st.SetSettings(map[string]string{"system_alert_admins": "false", "webhook_last_error": "old error"}); err != nil {
			t.Fatal(err)
		}

		SendSystemAlert(st, "Bekci restarted unexpectedly", "Test restart notification")

		raw := body()
		var p map[string]any
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatalf("payload %q: %v", raw, err)
		}
		if p["event"] != "system_alert" || p["target"] != "Bekci" {
			t.Errorf("event/target = %v/%v, want system_alert/Bekci", p["event"], p["target"])
		}
		if ts, _ := p["timestamp"].(string); ts == "" {
			t.Errorf("timestamp empty")
		} else if _, err := time.Parse(time.RFC3339, ts); err != nil {
			t.Errorf("timestamp %q is not RFC3339: %v", ts, err)
		}
		for _, field := range []string{"tags", "failing_checks"} {
			if arr, ok := p[field].([]any); !ok || len(arr) != 0 {
				t.Errorf("%s = %v, want []", field, p[field])
			}
		}
		if v, _ := st.GetSetting("webhook_last_success"); v == "" {
			t.Errorf("webhook_last_success not set")
		}
		if v, _ := st.GetSetting("webhook_last_error"); v != "" {
			t.Errorf("webhook_last_error = %q, want cleared", v)
		}
		if got := webhookAudits(t, st); len(got) != 1 || got[0] != "Bekci|system_alert|success" {
			t.Errorf("webhook audits = %v, want [Bekci|system_alert|success]", got)
		}
	})

	t.Run("failure", func(t *testing.T) {
		st := newTestStore(t)
		url, _ := webhookReceiver(t, http.StatusInternalServerError)
		configureWebhookAlerting(t, st, url)
		if err := st.SetSettings(map[string]string{"system_alert_admins": "false"}); err != nil {
			t.Fatal(err)
		}

		SendSystemAlert(st, "Bekci restarted unexpectedly", "Test restart notification")

		if v, _ := st.GetSetting("webhook_last_error"); v == "" {
			t.Errorf("webhook_last_error not set after a 500")
		}
		got := webhookAudits(t, st)
		if len(got) != 1 || !strings.HasPrefix(got[0], "Bekci|system_alert — error: ") || !strings.HasSuffix(got[0], "|failure") {
			t.Errorf("webhook audits = %v, want one Bekci system_alert failure", got)
		}
	})
}

// T-AL47: the Settings "Test" webhook updates the delivery status the page
// re-reads right after it (SR-20, review item D-32).
func TestSendTestWebhookRecordsStatus(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		st := newTestStore(t)
		url, _ := webhookReceiver(t, http.StatusOK)
		configureWebhookAlerting(t, st, url)
		if err := st.SetSettings(map[string]string{"webhook_last_error": "old error"}); err != nil {
			t.Fatal(err)
		}

		if err := New(st).SendTestWebhook(); err != nil {
			t.Fatal(err)
		}
		if v, _ := st.GetSetting("webhook_last_success"); v == "" {
			t.Errorf("webhook_last_success not set")
		}
		if v, _ := st.GetSetting("webhook_last_error"); v != "" {
			t.Errorf("webhook_last_error = %q, want cleared", v)
		}
		if got := webhookAudits(t, st); len(got) != 1 || got[0] != "Bekci Test|test|success" {
			t.Errorf("webhook audits = %v, want [Bekci Test|test|success]", got)
		}
	})

	t.Run("failure", func(t *testing.T) {
		st := newTestStore(t)
		url, _ := webhookReceiver(t, http.StatusInternalServerError)
		configureWebhookAlerting(t, st, url)

		if err := New(st).SendTestWebhook(); err == nil {
			t.Fatal("expected an error for a 500 response")
		}
		if v, _ := st.GetSetting("webhook_last_error"); v == "" {
			t.Errorf("webhook_last_error not set after a 500")
		}
		if v, _ := st.GetSetting("webhook_last_success"); v != "" {
			t.Errorf("webhook_last_success = %q, want unset", v)
		}
	})
}
