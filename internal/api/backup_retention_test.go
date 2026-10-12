package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bekci/internal/alerter"
	"github.com/bekci/internal/auth"
	"github.com/bekci/internal/store"
)

const (
	oldBackup1    = "bekci-full-20200101-000000.tar.gz"
	oldBackup2    = "bekci-full-20200101-000001.tar.gz"
	autoSavedName = "bekci-full-20300101-000000.tar.gz"
)

// setupBackupServer returns a server whose backup directory already holds the
// given indexed backups, with backup_max_copies set to maxCopies.
func setupBackupServer(t *testing.T, maxCopies string, existing ...string) (*Server, string) {
	t.Helper()
	st, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.SetSettings(map[string]string{"backup_max_copies": maxCopies}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	var entries []backupMeta
	for _, name := range existing {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("archive "+name), 0600); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, backupMeta{Filename: name, CreatedAt: "2020-01-01T00:00:00Z"})
	}
	if err := saveBackupIndex(dir, entries); err != nil {
		t.Fatal(err)
	}

	srv := New(st, auth.New(st, "test-secret-key-for-testing"), nil, alerter.New(st), "test", nil, "", "", "", dir)
	t.Cleanup(srv.Close)
	return srv, dir
}

func saveViaEndpoint(srv *Server) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	srv.handleSaveFullBackup(rec, httptest.NewRequest(http.MethodPost, "/api/backup/full/save", strings.NewReader("{}")))
	return rec
}

func indexedBackups(dir string) []string {
	var names []string
	for _, e := range loadBackupIndex(dir) {
		names = append(names, e.Filename)
	}
	return names
}

// T-P32: a failed write must never cost an existing backup (SR-07).
func TestFullBackupSaveKeepsOldCopyWhenWriteFails(t *testing.T) {
	for _, tc := range []struct {
		name string
		save func(t *testing.T, srv *Server, dir string)
	}{
		{"save endpoint", func(t *testing.T, srv *Server, dir string) {
			// The endpoint names the file after the current second; a directory
			// at every name it could pick makes the write fail.
			now := time.Now()
			for i := -1; i <= 30; i++ {
				name := "bekci-full-" + now.Add(time.Duration(i)*time.Second).Format("20060102-150405") + ".tar.gz"
				if err := os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if rec := saveViaEndpoint(srv); rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500", rec.Code)
			}
		}},
		{"auto-save copy", func(t *testing.T, srv *Server, dir string) {
			if err := os.Mkdir(filepath.Join(dir, autoSavedName), 0700); err != nil {
				t.Fatal(err)
			}
			srv.autoSaveBackupCopy([]byte("new archive"), autoSavedName, false)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, dir := setupBackupServer(t, "1", oldBackup1)

			tc.save(t, srv, dir)

			data, err := os.ReadFile(filepath.Join(dir, oldBackup1))
			if err != nil || string(data) != "archive "+oldBackup1 {
				t.Fatalf("existing backup lost after failed save: %q, %v", data, err)
			}
			if got, want := indexedBackups(dir), []string{oldBackup1}; !slices.Equal(got, want) {
				t.Fatalf("index = %v, want %v", got, want)
			}
		})
	}
}

// T-P33: after a successful save, exactly the newest backup_max_copies remain.
func TestFullBackupSavePrunesOldestAfterWrite(t *testing.T) {
	for _, tc := range []struct {
		name string
		save func(t *testing.T, srv *Server) string // returns the new backup's filename
	}{
		{"save endpoint", func(t *testing.T, srv *Server) string {
			rec := saveViaEndpoint(srv)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
			}
			var resp map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			return resp["filename"]
		}},
		{"auto-save copy", func(t *testing.T, srv *Server) string {
			srv.autoSaveBackupCopy([]byte("new archive"), autoSavedName, false)
			return autoSavedName
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, dir := setupBackupServer(t, "2", oldBackup1, oldBackup2)

			newName := tc.save(t, srv)

			want := []string{oldBackup2, newName}
			if got := indexedBackups(dir); !slices.Equal(got, want) {
				t.Fatalf("index = %v, want %v", got, want)
			}
			for _, name := range want {
				if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
					t.Errorf("%s missing: %v", name, err)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, oldBackup1)); !os.IsNotExist(err) {
				t.Errorf("oldest backup %s not pruned: %v", oldBackup1, err)
			}
		})
	}
}
