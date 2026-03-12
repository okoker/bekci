package checker

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func TestRunUnknownType(t *testing.T) {
	r := Run("foobar", "localhost", "{}")
	if r.Status != "down" {
		t.Fatalf("expected status down, got %s", r.Status)
	}
	if !strings.Contains(r.Message, "unknown check type") {
		t.Fatalf("expected message to contain 'unknown check type', got %s", r.Message)
	}
}

func TestHTTPCheckSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	host := u.Hostname()
	port := u.Port()

	cfg, _ := json.Marshal(map[string]any{
		"scheme":   "http",
		"port":     jsonPort(port),
		"endpoint": "/",
	})

	r := Run("http", host, string(cfg))
	if r.Status != "up" {
		t.Fatalf("expected status up, got %s (message: %s)", r.Status, r.Message)
	}
	if r.ResponseMs < 0 {
		t.Fatalf("expected ResponseMs >= 0, got %d", r.ResponseMs)
	}
}

func TestHTTPCheckWrongStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	host := u.Hostname()
	port := u.Port()

	cfg, _ := json.Marshal(map[string]any{
		"scheme":        "http",
		"port":          jsonPort(port),
		"endpoint":      "/",
		"expect_status": 200,
	})

	r := Run("http", host, string(cfg))
	if r.Status != "down" {
		t.Fatalf("expected status down, got %s (message: %s)", r.Status, r.Message)
	}
}

func TestTCPCheckSuccess(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())

	cfg, _ := json.Marshal(map[string]any{
		"port": jsonPort(portStr),
	})

	r := Run("tcp", "127.0.0.1", string(cfg))
	if r.Status != "up" {
		t.Fatalf("expected status up, got %s (message: %s)", r.Status, r.Message)
	}
}

func TestTLSCertCheck(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	host := u.Hostname()
	port := u.Port()

	cfg, _ := json.Marshal(map[string]any{
		"port": jsonPort(port),
	})

	r := Run("tls_cert", host, string(cfg))
	if r.Status != "up" {
		t.Fatalf("expected status up, got %s (message: %s)", r.Status, r.Message)
	}
	dl, ok := r.Metrics["days_left"]
	if !ok {
		t.Fatal("expected metrics to contain days_left")
	}
	switch dl.(type) {
	case int, float64:
	default:
		t.Fatalf("expected days_left to be numeric, got %T", dl)
	}
}

func TestPageHashCheckNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	host := u.Hostname()
	port := u.Port()

	cfg, _ := json.Marshal(map[string]any{
		"scheme":   "http",
		"port":     jsonPort(port),
		"endpoint": "/",
	})

	r := Run("page_hash", host, string(cfg))
	if r.Status != "down" {
		t.Fatalf("expected status down, got %s (message: %s)", r.Status, r.Message)
	}
	if !strings.Contains(r.Message, "404") {
		t.Fatalf("expected message to contain '404', got %s", r.Message)
	}
}

func TestDNSCheckSuccess(t *testing.T) {
	ns := startDNSServer(t, func(q dnsmessage.Question) ([]dnsmessage.Resource, dnsmessage.RCode) {
		if q.Type != dnsmessage.TypeA {
			return nil, dnsmessage.RCodeSuccess
		}
		return []dnsmessage.Resource{
			{
				Header: dnsmessage.ResourceHeader{
					Name:  q.Name,
					Type:  dnsmessage.TypeA,
					Class: dnsmessage.ClassINET,
					TTL:   60,
				},
				Body: &dnsmessage.AResource{A: [4]byte{127, 0, 0, 9}},
			},
		}, dnsmessage.RCodeSuccess
	})

	cfg, _ := json.Marshal(map[string]any{
		"query":       "example.test",
		"record_type": "A",
		"nameserver":  ns,
	})

	r := Run("dns", "ignored", string(cfg))
	if r.Status != "up" {
		t.Fatalf("expected status up, got %s (message: %s)", r.Status, r.Message)
	}
	if resolved, ok := r.Metrics["resolved"].([]string); !ok || len(resolved) != 1 || resolved[0] != "127.0.0.9" {
		t.Fatalf("expected resolved A record in metrics, got %#v", r.Metrics["resolved"])
	}
}

func TestDNSCheckExpectedValueMismatch(t *testing.T) {
	ns := startDNSServer(t, func(q dnsmessage.Question) ([]dnsmessage.Resource, dnsmessage.RCode) {
		return []dnsmessage.Resource{
			{
				Header: dnsmessage.ResourceHeader{
					Name:  q.Name,
					Type:  dnsmessage.TypeA,
					Class: dnsmessage.ClassINET,
					TTL:   60,
				},
				Body: &dnsmessage.AResource{A: [4]byte{127, 0, 0, 9}},
			},
		}, dnsmessage.RCodeSuccess
	})

	cfg, _ := json.Marshal(map[string]any{
		"query":        "example.test",
		"record_type":  "A",
		"expect_value": "127.0.0.10",
		"nameserver":   ns,
	})

	r := Run("dns", "ignored", string(cfg))
	if r.Status != "down" {
		t.Fatalf("expected status down, got %s (message: %s)", r.Status, r.Message)
	}
	if !strings.Contains(r.Message, "expected 127.0.0.10") {
		t.Fatalf("expected mismatch message, got %s", r.Message)
	}
}

func TestDNSCheckCNAMESuccess(t *testing.T) {
	ns := startDNSServer(t, func(q dnsmessage.Question) ([]dnsmessage.Resource, dnsmessage.RCode) {
		if q.Type != dnsmessage.TypeCNAME {
			return nil, dnsmessage.RCodeSuccess
		}
		return []dnsmessage.Resource{
			{
				Header: dnsmessage.ResourceHeader{
					Name:  q.Name,
					Type:  dnsmessage.TypeCNAME,
					Class: dnsmessage.ClassINET,
					TTL:   60,
				},
				Body: &dnsmessage.CNAMEResource{CNAME: dnsmessage.MustNewName("alias.example.test.")},
			},
		}, dnsmessage.RCodeSuccess
	})

	cfg, _ := json.Marshal(map[string]any{
		"query":        "example.test",
		"record_type":  "CNAME",
		"expect_value": "alias.example.test",
		"nameserver":   ns,
	})

	r := Run("dns", "ignored", string(cfg))
	if r.Status != "up" {
		t.Fatalf("expected status up, got %s (message: %s)", r.Status, r.Message)
	}
	if !strings.Contains(r.Message, "alias.example.test.") {
		t.Fatalf("expected resolved cname in message, got %s", r.Message)
	}
}

func TestDNSCheckUnsupportedRecordType(t *testing.T) {
	cfg, _ := json.Marshal(map[string]any{
		"record_type": "TXT",
	})

	r := Run("dns", "example.test", string(cfg))
	if r.Status != "down" {
		t.Fatalf("expected status down, got %s", r.Status)
	}
	if !strings.Contains(r.Message, "unsupported record type") {
		t.Fatalf("expected unsupported record type message, got %s", r.Message)
	}
}

func TestPingCheckInvalidHost(t *testing.T) {
	r := Run("ping", "not a valid host name", "{}")
	if r.Status != "down" {
		t.Fatalf("expected status down, got %s", r.Status)
	}
	if !strings.Contains(r.Message, "pinger init:") {
		t.Fatalf("expected pinger init error, got %s", r.Message)
	}
}

func TestPingCheckLoopback(t *testing.T) {
	cfg, _ := json.Marshal(map[string]any{
		"count":     1,
		"timeout_s": 1,
	})

	r := Run("ping", "127.0.0.1", string(cfg))
	if r.Status == "down" {
		if strings.Contains(strings.ToLower(r.Message), "operation not permitted") ||
			strings.Contains(strings.ToLower(r.Message), "permission denied") ||
			strings.Contains(strings.ToLower(r.Message), "protocol not available") ||
			strings.Contains(strings.ToLower(r.Message), "protocol wrong type for socket") {
			t.Skipf("loopback ping not permitted in this environment: %s", r.Message)
		}
		t.Fatalf("expected loopback ping to succeed, got down (message: %s)", r.Message)
	}
	if got := r.Metrics["packets_recv"]; got == nil {
		t.Fatalf("expected ping metrics to include packets_recv, got %#v", r.Metrics)
	}
}

func TestPageHashCheckBaselineCapture(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	host := u.Hostname()
	port := u.Port()

	cfg, _ := json.Marshal(map[string]any{
		"scheme":   "http",
		"port":     jsonPort(port),
		"endpoint": "/",
	})

	r := Run("page_hash", host, string(cfg))
	if r.Status != "up" {
		t.Fatalf("expected status up, got %s (message: %s)", r.Status, r.Message)
	}
	if r.Message != "baseline hash captured" {
		t.Fatalf("expected baseline capture message, got %s", r.Message)
	}
	if captured, ok := r.Metrics["baseline_captured"].(bool); !ok || !captured {
		t.Fatalf("expected baseline_captured=true, got %#v", r.Metrics["baseline_captured"])
	}
}

func TestPageHashCheckHashMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	host := u.Hostname()
	port := u.Port()
	sum := sha256.Sum256([]byte("different"))

	cfg, _ := json.Marshal(map[string]any{
		"scheme":        "http",
		"port":          jsonPort(port),
		"endpoint":      "/",
		"baseline_hash": fmt.Sprintf("%x", sum),
	})

	r := Run("page_hash", host, string(cfg))
	if r.Status != "down" {
		t.Fatalf("expected status down, got %s (message: %s)", r.Status, r.Message)
	}
	if r.Message != "hash mismatch" {
		t.Fatalf("expected hash mismatch message, got %s", r.Message)
	}
}

func TestSNMPv2cTimeout(t *testing.T) {
	// Connect to a non-routable address to trigger timeout
	cfg, _ := json.Marshal(map[string]any{
		"port":      161,
		"timeout_s": 1,
		"community": "public",
	})
	r := Run("snmp_v2c", "192.0.2.1", string(cfg))
	if r.Status != "down" {
		t.Fatalf("expected down for non-routable host, got %s", r.Status)
	}
}

func TestSNMPv3MissingUsername(t *testing.T) {
	cfg, _ := json.Marshal(map[string]any{
		"port":      161,
		"timeout_s": 1,
		"username":  "",
	})
	r := Run("snmp_v3", "192.0.2.1", string(cfg))
	if r.Status != "down" {
		t.Fatalf("expected down for missing username, got %s", r.Status)
	}
	if !strings.Contains(r.Message, "not configured") {
		t.Fatalf("expected 'not configured' message, got: %s", r.Message)
	}
}

func TestSNMPv3Timeout(t *testing.T) {
	cfg, _ := json.Marshal(map[string]any{
		"port":            161,
		"timeout_s":       1,
		"username":        "testuser",
		"security_level":  "authPriv",
		"auth_protocol":   "SHA",
		"auth_passphrase": "testpass12345678",
		"privacy_protocol":   "AES",
		"privacy_passphrase": "testpass12345678",
	})
	r := Run("snmp_v3", "192.0.2.1", string(cfg))
	if r.Status != "down" {
		t.Fatalf("expected down for non-routable host, got %s", r.Status)
	}
}

func TestSNMPDispatcherRouting(t *testing.T) {
	for _, ct := range []string{"snmp_v2c", "snmp_v3"} {
		r := Run(ct, "192.0.2.1", `{"port":161,"timeout_s":1,"community":"public","username":"u","auth_passphrase":"testpass12345678","privacy_passphrase":"testpass12345678"}`)
		if strings.Contains(r.Message, "unknown check type") {
			t.Fatalf("check type %s was not routed by dispatcher", ct)
		}
	}
}

func jsonPort(s string) int {
	var p int
	fmt.Sscanf(s, "%d", &p)
	return p
}

func startDNSServer(t *testing.T, handler func(q dnsmessage.Question) ([]dnsmessage.Resource, dnsmessage.RCode)) string {
	t.Helper()

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() })

	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}

			var parser dnsmessage.Parser
			hdr, err := parser.Start(buf[:n])
			if err != nil {
				continue
			}

			q, err := parser.Question()
			if err != nil {
				continue
			}

			answers, rcode := handler(q)
			msg := dnsmessage.Message{
				Header: dnsmessage.Header{
					ID:            hdr.ID,
					Response:      true,
					Authoritative: true,
					RCode:         rcode,
				},
				Questions: []dnsmessage.Question{q},
				Answers:   answers,
			}

			wire, err := msg.Pack()
			if err != nil {
				continue
			}
			_, _ = pc.WriteTo(wire, addr)
		}
	}()

	return pc.LocalAddr().String()
}
