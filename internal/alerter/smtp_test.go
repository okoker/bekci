package alerter

import (
	"io"
	"mime"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestBuildSMTPMessageSubject(t *testing.T) {
	for _, tc := range []struct {
		name, subject, want string
	}{
		{"ascii", "[ALERT] router is DOWN", "[ALERT] router is DOWN"},
		{"unicode", "[ALERT] İstanbul – edge 🦉 is DOWN", "[ALERT] İstanbul – edge 🦉 is DOWN"},
		{"cr", "router\rX-Injected: yes", "router X-Injected: yes"},
		{"lf", "router\nX-Injected: yes", "router X-Injected: yes"},
		{"crlf", "router\r\nReply-To: injected@example.invalid", "router  Reply-To: injected@example.invalid"},
		{"body", "router\r\nContent-Type: text/plain\r\n\r\nFAKE BODY", "router  Content-Type: text/plain    FAKE BODY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const body = "<html><body>Original alert &amp; details</body></html>"
			message := buildSMTPMessage("bekci@example.invalid", []string{"one@example.invalid", "two@example.invalid"}, tc.subject, body)
			msg := assertSMTPMessage(t, message, body)
			encoded := msg.Header.Get("Subject")
			for _, c := range encoded {
				if c > 127 || c == '\r' || c == '\n' {
					t.Fatalf("subject is not a single ASCII header value: %q", encoded)
				}
			}
			decoded, err := new(mime.WordDecoder).DecodeHeader(encoded)
			if err != nil || decoded != tc.want {
				t.Fatalf("decoded subject = %q, error = %v; want %q", decoded, err, tc.want)
			}
		})
	}
}

func TestBuildSMTPMessageRenderedAlert(t *testing.T) {
	for _, state := range []string{"unhealthy", "healthy"} {
		t.Run(state, func(t *testing.T) {
			subject, body := RenderEmailAlert("router\r\nX-Injected: yes\r\n\r\nFAKE BODY", "192.0.2.1", state, nil, time.Now(), nil)
			message := buildSMTPMessage("bekci@example.invalid", []string{"one@example.invalid", "two@example.invalid"}, subject, body)
			assertSMTPMessage(t, message, body)
		})
	}
}

func assertSMTPMessage(t *testing.T, message, wantBody string) *mail.Message {
	t.Helper()
	msg, err := mail.ReadMessage(strings.NewReader(message))
	if err != nil {
		t.Fatal(err)
	}
	wantHeaders := map[string]string{
		"From":         "bekci@example.invalid",
		"To":           "one@example.invalid, two@example.invalid",
		"Mime-Version": "1.0",
		"Content-Type": "text/html; charset=utf-8",
	}
	if len(msg.Header) != len(wantHeaders)+1 || len(msg.Header["Subject"]) != 1 {
		t.Fatalf("unexpected message headers: %#v", msg.Header)
	}
	for key, value := range wantHeaders {
		if values := msg.Header[key]; len(values) != 1 || values[0] != value {
			t.Fatalf("%s = %q; want %q", key, values, value)
		}
	}
	body, err := io.ReadAll(msg.Body)
	if err != nil || string(body) != wantBody {
		t.Fatalf("body changed: %q, error = %v; want %q", body, err, wantBody)
	}
	return msg
}
