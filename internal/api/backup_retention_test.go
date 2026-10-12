package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bekci/internal/alerter"
	"github.com/bekci/internal/auth"
	"github.com/bekci/internal/store"
	"github.com/google/uuid"
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
		{"saveBackupCopy", func(t *testing.T, srv *Server, dir string) {
			if err := os.Mkdir(filepath.Join(dir, autoSavedName), 0700); err != nil {
				t.Fatal(err)
			}
			if _, err := srv.saveBackupCopy([]byte("new archive"), autoSavedName, false); err == nil {
				t.Fatal("save reported success after a failed write")
			}
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
		{"saveBackupCopy", func(t *testing.T, srv *Server) string {
			if _, err := srv.saveBackupCopy([]byte("new archive"), autoSavedName, false); err != nil {
				t.Fatal(err)
			}
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

// T-P34: a scheduled backup that cannot be stored is audited and alerted as a failure (SR-08).
func TestScheduledBackupReportsStorageFailure(t *testing.T) {
	st, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	// An ordinary file where the backup directory should be makes every save fail.
	notADir := filepath.Join(t.TempDir(), "backups")
	if err := os.WriteFile(notADir, nil, 0600); err != nil {
		t.Fatal(err)
	}

	var alerts atomic.Int32
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { alerts.Add(1) }))
	t.Cleanup(hook.Close)
	now := time.Now()
	ops := &store.User{ID: uuid.New().String(), Username: "ops", PasswordHash: "hash", Role: "viewer", Status: "active", CreatedAt: now, UpdatedAt: now}
	if err := st.CreateUser(ops); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSettings(map[string]string{
		"auto_backup_schedule": "weekly",
		"system_alert_admins":  "false",
		"system_alert_users":   ops.ID,
		"webhook_enabled":      "true",
		"webhook_url":          hook.URL,
	}); err != nil {
		t.Fatal(err)
	}

	srv := New(st, auth.New(st, "test-secret-key-for-testing"), nil, alerter.New(st), "test", nil, "", "", "", notADir)
	t.Cleanup(srv.Close)

	// The schedule only fires in its configured hour; run again if the hour turned mid-run.
	for {
		hour := time.Now().Hour()
		if err := st.SetSettings(map[string]string{"auto_backup_time": fmt.Sprintf("%02d:00", hour)}); err != nil {
			t.Fatal(err)
		}
		srv.RunScheduledBackup()
		if time.Now().Hour() == hour {
			break
		}
	}

	entries, _, err := st.ListAuditEntries(100, 0, "auto_backup")
	if err != nil {
		t.Fatal(err)
	}
	failed := false
	for _, e := range entries {
		if e.Status == "failure" {
			failed = true
		}
		if strings.HasPrefix(e.Detail, "Backup completed") {
			t.Errorf("success audited after a failed save: %q", e.Detail)
		}
	}
	if !failed {
		t.Error("no failure audit entry for the failed save")
	}
	if alerts.Load() == 0 {
		t.Error("no system alert sent for the failed save")
	}
}

// T-P35: a save never overwrites an existing backup of the same name (SR-09).
func TestBackupSaveRefusesExistingFilename(t *testing.T) {
	srv, dir := setupBackupServer(t, "5", oldBackup1)

	if _, err := srv.saveBackupCopy([]byte("first"), autoSavedName, false); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.saveBackupCopy([]byte("second"), autoSavedName, false); err == nil {
		t.Error("second save of the same filename reported success")
	}

	if data, err := os.ReadFile(filepath.Join(dir, autoSavedName)); err != nil || string(data) != "first" {
		t.Fatalf("first backup overwritten: %q, %v", data, err)
	}
	if got, want := indexedBackups(dir), []string{oldBackup1, autoSavedName}; !slices.Equal(got, want) {
		t.Fatalf("index = %v, want %v", got, want)
	}
}

// T-P36: concurrent saves and deletes never lose index entries (SR-09).
func TestBackupConcurrentSavesAndDeletesKeepIndex(t *testing.T) {
	const n = 20
	var existing, added []string
	for i := range n {
		existing = append(existing, fmt.Sprintf("bekci-full-20200101-0000%02d.tar.gz", i))
		added = append(added, fmt.Sprintf("bekci-full-20300101-0000%02d.tar.gz", i))
	}
	srv, dir := setupBackupServer(t, "100", existing...)

	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			if _, err := srv.saveBackupCopy([]byte("archive"), added[i], false); err != nil {
				t.Errorf("save %s: %v", added[i], err)
			}
		})
		wg.Go(func() {
			req := httptest.NewRequest(http.MethodDelete, "/api/backup/full/saved/"+existing[i], nil)
			req.SetPathValue("filename", existing[i])
			rec := httptest.NewRecorder()
			srv.handleDeleteSavedBackup(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("delete %s: status %d", existing[i], rec.Code)
			}
		})
	}
	wg.Wait()

	got := indexedBackups(dir)
	slices.Sort(got)
	if !slices.Equal(got, added) {
		t.Fatalf("index = %v, want %v", got, added)
	}
}

// T-P37: an index write failure is reported and leaves backups and index as they were (SR-08).
func TestBackupSaveIndexFailureChangesNothing(t *testing.T) {
	srv, dir := setupBackupServer(t, "1", oldBackup1)
	// A directory where the temporary index goes makes the index write fail.
	if err := os.Mkdir(filepath.Join(dir, "index.json.tmp"), 0700); err != nil {
		t.Fatal(err)
	}

	if _, err := srv.saveBackupCopy([]byte("new archive"), autoSavedName, false); err == nil {
		t.Fatal("save reported success after a failed index write")
	}

	if _, err := os.Stat(filepath.Join(dir, oldBackup1)); err != nil {
		t.Errorf("existing backup lost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, autoSavedName)); !os.IsNotExist(err) {
		t.Errorf("unindexed new backup left on disk: %v", err)
	}
	if got, want := indexedBackups(dir), []string{oldBackup1}; !slices.Equal(got, want) {
		t.Fatalf("index = %v, want %v", got, want)
	}
}
