package checker

import (
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

// A-082: a target Host is a name or IP, optionally followed by a path. The
// refusals live in testdata/host_rules.json, shared with the API tests and the
// target form (frontend/tests/condition-rules.test.mjs).
func TestHostError(t *testing.T) {
	var table struct {
		Cases []struct {
			Host  string `json:"host"`
			Error string `json:"error"`
		} `json:"cases"`
	}
	data, err := os.ReadFile("testdata/host_rules.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &table); err != nil {
		t.Fatal(err)
	}
	if len(table.Cases) == 0 {
		t.Fatal("testdata/host_rules.json has no cases")
	}
	for _, c := range table.Cases {
		if got := HostError(c.Host); got != c.Error {
			t.Errorf("HostError(%q) = %q, want %q", c.Host, got, c.Error)
		}
	}
}

func TestSplitHost(t *testing.T) {
	cases := []struct{ host, name, path string }{
		{"example.com", "example.com", ""},
		{"example.com/", "example.com", "/"},
		{"www.somehost.com/health", "www.somehost.com", "/health"},
		{"somehost.com/health/", "somehost.com", "/health/"},
		{"::1/health", "::1", "/health"},
		{"10.0.0.5", "10.0.0.5", ""},
		{"/health", "", "/health"},
		{"", "", ""},
	}
	for _, c := range cases {
		name, path := SplitHost(c.host)
		if name != c.name || path != c.path {
			t.Errorf("SplitHost(%q) = %q, %q; want %q, %q", c.host, name, path, c.name, c.path)
		}
	}
}

// HTTP and Page Hash request the Host's path (with the check's port); an
// Endpoint other than empty or "/" goes after it.
func TestHostPathReachesHTTPChecks(t *testing.T) {
	cases := []struct {
		hostPath, endpoint, wantPath string
	}{
		{"/health", "/", "/health"},
		{"/health/", "/", "/health/"},
		{"/app", "status", "/app/status"},
		{"/", "/", "/"},
	}
	for _, checkType := range []string{"http", "page_hash"} {
		for _, tc := range cases {
			t.Run(checkType+tc.hostPath+"+"+tc.endpoint, func(t *testing.T) {
				var mu sync.Mutex
				var gotPath string
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					gotPath = r.URL.Path
					mu.Unlock()
					w.WriteHeader(200)
				}))
				defer srv.Close()

				u, _ := url.Parse(srv.URL)
				cfg, _ := json.Marshal(map[string]any{
					"scheme":   "http",
					"port":     jsonPort(u.Port()),
					"endpoint": tc.endpoint,
				})
				r := Run(checkType, u.Hostname()+tc.hostPath, string(cfg))
				if r.Status != "up" {
					t.Fatalf("expected up, got %s (%s)", r.Status, r.Message)
				}
				mu.Lock()
				defer mu.Unlock()
				if gotPath != tc.wantPath {
					t.Fatalf("server saw path %q, want %q", gotPath, tc.wantPath)
				}
			})
		}
	}
}

// TCP connects to the Host's name only.
func TestHostPathTCPUsesName(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	cfg, _ := json.Marshal(map[string]any{"port": jsonPort(port)})

	r := Run("tcp", "127.0.0.1/health", string(cfg))
	if r.Status != "up" {
		t.Fatalf("expected up, got %s (%s)", r.Status, r.Message)
	}
	if want := net.JoinHostPort("127.0.0.1", port); r.Metrics["addr"] != want {
		t.Fatalf("metrics addr %v, want %s", r.Metrics["addr"], want)
	}
}

// TLS dials the Host's name and sends it as SNI, without the path.
func TestHostPathTLSUsesName(t *testing.T) {
	var mu sync.Mutex
	var gotSNI string
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.TLS = &tls.Config{GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		mu.Lock()
		gotSNI = hello.ServerName
		mu.Unlock()
		return nil, nil
	}}
	srv.StartTLS()
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	cfg, _ := json.Marshal(map[string]any{"port": jsonPort(u.Port())})

	r := Run("tls_cert", "localhost/health", string(cfg))
	if r.Status != "up" {
		t.Fatalf("expected up, got %s (%s)", r.Status, r.Message)
	}
	mu.Lock()
	defer mu.Unlock()
	if gotSNI != "localhost" {
		t.Fatalf("server saw SNI %q, want %q", gotSNI, "localhost")
	}
}

// A DNS check with an empty Query asks for the Host's name, without the path.
func TestHostPathDNSQueriesName(t *testing.T) {
	var mu sync.Mutex
	var gotName string
	ns := startDNSServer(t, func(q dnsmessage.Question) ([]dnsmessage.Resource, dnsmessage.RCode) {
		mu.Lock()
		gotName = q.Name.String()
		mu.Unlock()
		return []dnsmessage.Resource{{
			Header: dnsmessage.ResourceHeader{Name: q.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 60},
			Body:   &dnsmessage.AResource{A: [4]byte{127, 0, 0, 9}},
		}}, dnsmessage.RCodeSuccess
	})
	cfg, _ := json.Marshal(map[string]any{"record_type": "A", "nameserver": ns})

	r := Run("dns", "example.test/health", string(cfg))
	if r.Status != "up" {
		t.Fatalf("expected up, got %s (%s)", r.Status, r.Message)
	}
	mu.Lock()
	defer mu.Unlock()
	if gotName != "example.test." {
		t.Fatalf("server was asked for %q, want %q", gotName, "example.test.")
	}
}
