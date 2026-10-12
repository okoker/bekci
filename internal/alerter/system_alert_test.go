package alerter

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestSendSystemAlertEmailProvider(t *testing.T) {
	for _, tc := range []struct {
		name       string
		provider   string
		wantSMTP   int32
		wantResend int32
	}{
		{name: "ms365", provider: "ms365", wantSMTP: 1},
		{name: "resend", provider: "resend", wantResend: 1},
		{name: "default", provider: "", wantResend: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := newTestStore(t)
			user := createAlertUser(t, st, "recipient", "recipient@example.com", "")

			var smtpCalls, resendCalls atomic.Int32
			smtpServer, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			smtpDone := make(chan struct{})
			go func() {
				defer close(smtpDone)
				conn, err := smtpServer.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				smtpCalls.Add(1)
				// A rejected SMTP greeting proves routing and ensures SMTP failure
				// does not fall back to the configured Resend account.
				_, _ = io.WriteString(conn, "421 Test SMTP service unavailable\r\n")
			}()
			t.Cleanup(func() {
				smtpServer.Close()
				<-smtpDone
			})
			host, port, err := net.SplitHostPort(smtpServer.Addr().String())
			if err != nil {
				t.Fatal(err)
			}

			resendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resendCalls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"system-alert"}`)
			}))
			t.Cleanup(resendServer.Close)
			withResendURL(t, resendServer.URL)

			// Keep both providers configured so the selected provider alone
			// determines which transport is used.
			if err := st.SetSettings(map[string]string{
				"alert_method":        "email",
				"email_provider":      tc.provider,
				"alert_from_email":    "alerts@example.com",
				"resend_api_key":      "test-api-key",
				"smtp_host":           host,
				"smtp_port":           port,
				"smtp_username":       "test-user",
				"smtp_password":       "test-password",
				"system_alert_admins": "false",
				"system_alert_users":  user.ID,
			}); err != nil {
				t.Fatal(err)
			}

			SendSystemAlert(st, "Bekci restarted unexpectedly", "Test restart notification")

			if got := smtpCalls.Load(); got != tc.wantSMTP {
				t.Errorf("SMTP calls = %d, want %d", got, tc.wantSMTP)
			}
			if got := resendCalls.Load(); got != tc.wantResend {
				t.Errorf("Resend calls = %d, want %d", got, tc.wantResend)
			}
		})
	}
}
