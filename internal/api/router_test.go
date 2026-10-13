package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/bekci/internal/alerter"
	"github.com/bekci/internal/auth"
	"github.com/bekci/internal/store"
)

// T-P47: API paths with no route (unknown path or wrong method) get a JSON 404,
// not the SPA's index.html with 200 (review item D-26). Frontend routes and
// static assets still come from the SPA.
func TestUnknownAPIPathsReturnJSON(t *testing.T) {
	st, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	spa := fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html><title>Bekci</title>")},
		"assets/app.js": {Data: []byte("export const ok = true;\n")},
	}
	srv := New(st, auth.New(st, "test-secret-key-for-testing"), nil, alerter.New(st), "test", spa, "", "", "", t.TempDir())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { ts.Close(); srv.Close() })

	do := func(method, path string) (int, string, string) {
		t.Helper()
		req, err := http.NewRequest(method, ts.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, resp.Header.Get("Content-Type"), string(body)
	}

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/nope"},
		{http.MethodGet, "/api/v1/nope"}, // no token: still a 404, not an HTML page
		{http.MethodDelete, "/api/login"},
		{http.MethodGet, "/api"},
		{http.MethodGet, "/api/targets/"},
	} {
		code, ctype, body := do(tc.method, tc.path)
		if code != http.StatusNotFound {
			t.Errorf("%s %s: status %d, want 404", tc.method, tc.path, code)
		}
		if !strings.HasPrefix(ctype, "application/json") {
			t.Errorf("%s %s: Content-Type %q, want application/json", tc.method, tc.path, ctype)
		}
		var e map[string]string
		if err := json.Unmarshal([]byte(body), &e); err != nil || e["error"] == "" {
			t.Errorf("%s %s: body %q, want a JSON error", tc.method, tc.path, body)
		}
	}

	for _, path := range []string{"/", "/dashboard", "/apiary", "/assets/app.js"} {
		code, _, body := do(http.MethodGet, path)
		if code != http.StatusOK {
			t.Errorf("GET %s: status %d, want 200", path, code)
		}
		if path != "/assets/app.js" && !strings.Contains(body, "<title>Bekci</title>") {
			t.Errorf("GET %s: body %q, want index.html", path, body)
		}
	}

	if code, _, _ := do(http.MethodGet, "/api/health"); code != http.StatusOK {
		t.Errorf("GET /api/health: status %d, want 200", code)
	}
}
