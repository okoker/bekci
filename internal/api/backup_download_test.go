package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/golang-jwt/jwt/v5"

	"github.com/bekci/internal/alerter"
	"github.com/bekci/internal/auth"
	"github.com/bekci/internal/store"
)

// setupDownloadServer runs the full middleware chain on a real HTTP server (the
// server, unlike httptest.ResponseRecorder, enforces the declared Content-Length)
// and returns an admin client and the backup directory.
func setupDownloadServer(t *testing.T) (*httptest.Server, *http.Client, *store.Store, string) {
	t.Helper()
	st, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	createUser(t, st, "admin", "adminpass1234", "admin")

	dir := t.TempDir()
	srv := New(st, auth.New(st, "test-secret-key-for-testing"), nil, alerter.New(st), "test", nil, "", "", "", dir)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { ts.Close(); srv.Close() })
	return ts, loginAs(t, ts, "admin", "adminpass1234"), st, dir
}

// browserRequest sets Accept-Encoding the way a browser does. Setting it by hand
// also stops Go's transport from transparently decoding the body, so the test
// sees exactly the bytes and headers on the wire.
func browserRequest(t *testing.T, method, url, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	return req
}

// readWholeBody fails the test if the body is cut short of its declared length.
func readWholeBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v (got %d bytes, Content-Length %q, Content-Encoding %q)",
			err, len(body), resp.Header.Get("Content-Length"), resp.Header.Get("Content-Encoding"))
	}
	if cl := resp.Header.Get("Content-Length"); cl != strconv.Itoa(len(body)) {
		t.Fatalf("Content-Length %q but body is %d bytes", cl, len(body))
	}
	return body
}

// A browser always sends Accept-Encoding: gzip. The full backup must still reach
// it complete, untouched and as a readable tar.gz holding the database.
func TestFullBackupDownloadSurvivesGzipAcceptEncoding(t *testing.T) {
	ts, client, _, _ := setupDownloadServer(t)

	resp, err := client.Do(browserRequest(t, http.MethodPost, ts.URL+"/api/backup/full", "{}"))
	if err != nil {
		t.Fatal(err)
	}
	body := readWholeBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "" {
		t.Fatalf("archive was re-encoded with Content-Encoding %q", enc)
	}
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("body is not a gzip archive: %v", err)
	}
	hdr, err := tar.NewReader(gz).Next()
	if err != nil {
		t.Fatalf("reading tar: %v", err)
	}
	if hdr.Name != "bekci.db" {
		t.Fatalf("first archive entry %q, want bekci.db", hdr.Name)
	}
}

// Downloading a saved copy must return exactly the bytes on disk. Random content
// does not compress, as with a real tar.gz.
func TestSavedBackupDownloadSurvivesGzipAcceptEncoding(t *testing.T) {
	ts, client, _, dir := setupDownloadServer(t)

	const name = "bekci-full-20300101-000000.tar.gz"
	want := make([]byte, 256*1024)
	if _, err := rand.Read(want); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), want, 0600); err != nil {
		t.Fatal(err)
	}
	if err := saveBackupIndex(dir, []backupMeta{{Filename: name, Size: int64(len(want))}}); err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(browserRequest(t, http.MethodGet, ts.URL+"/api/backup/full/saved/"+name, ""))
	if err != nil {
		t.Fatal(err)
	}
	got := readWholeBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "" {
		t.Fatalf("archive was re-encoded with Content-Encoding %q", enc)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("downloaded %d bytes differ from the %d bytes on disk", len(got), len(want))
	}
}

// failingWriter stands in for a client connection that breaks mid-download.
type failingWriter struct{ header http.Header }

func (w *failingWriter) Header() http.Header       { return w.header }
func (w *failingWriter) WriteHeader(int)           {}
func (w *failingWriter) Write([]byte) (int, error) { return 0, errors.New("connection reset by peer") }

// The audit log is a security record: when the archive cannot be sent, the
// export must be recorded as a failure, never as a success.
func TestFullBackupExportAuditsSendFailure(t *testing.T) {
	st, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	admin := createUserWithContact(t, st, "admin", "adminpass1234", "admin", "", "")
	srv := New(st, auth.New(st, "test-secret-key-for-testing"), nil, alerter.New(st), "test", nil, "", "", "", t.TempDir())
	t.Cleanup(srv.Close)

	req := httptest.NewRequest(http.MethodPost, "/api/backup/full", strings.NewReader("{}"))
	claims := &auth.Claims{Role: "admin", RegisteredClaims: jwt.RegisteredClaims{Subject: admin.ID}}
	req = req.WithContext(context.WithValue(req.Context(), ctxClaims, claims))
	srv.handleFullBackup(&failingWriter{header: http.Header{}}, req)

	entries, _, err := st.ListAuditEntries(10, 0, "export_full_backup")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d export_full_backup audit entries, want 1", len(entries))
	}
	if entries[0].Status != "failure" {
		t.Fatalf("audit status %q for a download that never reached the client, want failure (detail %q)",
			entries[0].Status, entries[0].Detail)
	}
}

// Saved-backup downloads are audited as well; a copy that never reached the
// client must not be recorded as downloaded.
func TestSavedBackupDownloadAuditsSendFailure(t *testing.T) {
	st, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	admin := createUserWithContact(t, st, "admin", "adminpass1234", "admin", "", "")
	dir := t.TempDir()
	const name = "bekci-full-20300101-000000.tar.gz"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("archive"), 0600); err != nil {
		t.Fatal(err)
	}
	srv := New(st, auth.New(st, "test-secret-key-for-testing"), nil, alerter.New(st), "test", nil, "", "", "", dir)
	t.Cleanup(srv.Close)

	req := httptest.NewRequest(http.MethodGet, "/api/backup/full/saved/"+name, nil)
	req.SetPathValue("filename", name)
	claims := &auth.Claims{Role: "admin", RegisteredClaims: jwt.RegisteredClaims{Subject: admin.ID}}
	req = req.WithContext(context.WithValue(req.Context(), ctxClaims, claims))
	srv.handleDownloadSavedBackup(&failingWriter{header: http.Header{}}, req)

	entries, _, err := st.ListAuditEntries(10, 0, "download_saved_backup")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d download_saved_backup audit entries, want 1", len(entries))
	}
	if entries[0].Status != "failure" {
		t.Fatalf("audit status %q for a download that never reached the client, want failure (detail %q)",
			entries[0].Status, entries[0].Detail)
	}
}

// Guard: API JSON must stay gzip-compressed for clients that accept it
// (REQUIREMENTS decision #25).
func TestJSONResponsesStillGzipped(t *testing.T) {
	ts, client, _, _ := setupDownloadServer(t)

	resp, err := client.Do(browserRequest(t, http.MethodGet, ts.URL+"/api/me", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if enc := resp.Header.Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("Content-Encoding %q, want gzip", enc)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("body is not gzip: %v", err)
	}
	var me map[string]any
	if err := json.NewDecoder(gz).Decode(&me); err != nil {
		t.Fatalf("decoding JSON: %v", err)
	}
	if me["username"] != "admin" {
		t.Fatalf("username %v, want admin", me["username"])
	}
}

// Guard: the embedded UI's script files must stay compressed and arrive whole.
// http.FileServer declares the uncompressed Content-Length unless the response is
// already marked as encoded, so the middleware must replace it when compressing.
func TestSPAAssetsStillGzipped(t *testing.T) {
	st, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	script := []byte(strings.Repeat("export const status = 'healthy';\n", 200))
	spa := fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html><title>Bekci</title>")},
		"assets/app.js": {Data: script},
	}
	srv := New(st, auth.New(st, "test-secret-key-for-testing"), nil, alerter.New(st), "test", spa, "", "", "", t.TempDir())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { ts.Close(); srv.Close() })

	resp, err := http.DefaultClient.Do(browserRequest(t, http.MethodGet, ts.URL+"/assets/app.js", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v (got %d bytes, Content-Length %q)", err, len(raw), resp.Header.Get("Content-Length"))
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("Content-Encoding %q, want gzip", enc)
	}
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("body is not gzip: %v", err)
	}
	got, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("decompressing: %v", err)
	}
	if !bytes.Equal(got, script) {
		t.Fatalf("decompressed %d bytes, want the %d-byte script", len(got), len(script))
	}
}

// indexAtFirstWrite records which backups are listed at the moment the download
// body starts, i.e. the earliest the browser can ask for the saved list.
type indexAtFirstWrite struct {
	header http.Header
	dir    string
	listed []string
	wrote  bool
}

func (w *indexAtFirstWrite) Header() http.Header { return w.header }
func (w *indexAtFirstWrite) WriteHeader(int)     {}
func (w *indexAtFirstWrite) Write(b []byte) (int, error) {
	if !w.wrote {
		w.wrote = true
		w.listed = indexedBackups(w.dir)
	}
	return len(b), nil
}

// The server copy of a download must be listed before the download is sent, so
// the page's list refresh shows it and it exists even if the download fails.
func TestFullBackupDownloadSavesCopyBeforeSending(t *testing.T) {
	srv, dir := setupBackupServer(t, "5")
	w := &indexAtFirstWrite{header: http.Header{}, dir: dir}

	srv.handleFullBackup(w, httptest.NewRequest(http.MethodPost, "/api/backup/full", strings.NewReader("{}")))

	disposition := w.header.Get("Content-Disposition")
	name := strings.TrimSuffix(strings.TrimPrefix(disposition, `attachment; filename="`), `"`)
	if !w.wrote || !strings.HasPrefix(name, "bekci-full-") {
		t.Fatalf("no download sent (wrote=%v, Content-Disposition %q)", w.wrote, disposition)
	}
	if len(w.listed) != 1 || w.listed[0] != name {
		t.Fatalf("saved list when the download started: %v, want [%s]", w.listed, name)
	}
}
