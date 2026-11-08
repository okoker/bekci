package checker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

// A-073: the Endpoint gets exactly one leading "/" whatever was typed, so a
// stored "health" or "//health" reaches /health instead of a broken host name.
func TestEndpointGetsOneLeadingSlash(t *testing.T) {
	cases := []struct {
		endpoint  string
		wantPath  string
		wantQuery string
	}{
		{"health", "/health", ""},
		{"/health", "/health", ""},
		{"//health", "/health", ""},
		{"", "/", ""},
		{"?x=1", "/", "x=1"},
		{"health/x", "/health/x", ""},
	}
	for _, checkType := range []string{"http", "page_hash"} {
		for _, tc := range cases {
			t.Run(checkType+"/"+tc.endpoint, func(t *testing.T) {
				var mu sync.Mutex
				var gotPath, gotQuery string
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
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
				r := Run(checkType, u.Hostname(), string(cfg))
				if r.Status != "up" {
					t.Fatalf("expected up, got %s (%s)", r.Status, r.Message)
				}
				mu.Lock()
				defer mu.Unlock()
				if gotPath != tc.wantPath || gotQuery != tc.wantQuery {
					t.Fatalf("endpoint %q: server saw path %q query %q, want path %q query %q",
						tc.endpoint, gotPath, gotQuery, tc.wantPath, tc.wantQuery)
				}
			})
		}
	}
}

func TestBuildHTTPURL(t *testing.T) {
	cases := []struct {
		scheme, host string
		port         int
		endpoint     string
		want         string
	}{
		{"https", "example.com", 0, "health", "https://example.com/health"},
		{"https", "example.com", 0, "/health", "https://example.com/health"},
		{"https", "example.com", 0, "///health", "https://example.com/health"},
		{"https", "example.com", 0, "", "https://example.com"},
		{"http", "example.com", 8080, "?x=1", "http://example.com:8080/?x=1"},
		{"http", "::1", 8080, "health", "http://[::1]:8080/health"},
		{"http", "::1", 0, "health", "http://[::1]/health"},
		{"https", "example.com/", 0, "health", "https://example.com/health"},
	}
	for _, tc := range cases {
		if got := buildHTTPURL(tc.scheme, tc.host, tc.port, tc.endpoint); got != tc.want {
			t.Errorf("buildHTTPURL(%q,%q,%d,%q) = %q, want %q", tc.scheme, tc.host, tc.port, tc.endpoint, got, tc.want)
		}
	}
}
