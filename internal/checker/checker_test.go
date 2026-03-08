package checker

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
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

func jsonPort(s string) int {
	var p int
	fmt.Sscanf(s, "%d", &p)
	return p
}
