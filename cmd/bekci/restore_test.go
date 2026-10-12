package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func openWALDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1) // keep PRAGMAs on the connection that writes
	return db
}

func mustExec(t *testing.T, db *sql.DB, stmts ...string) {
	t.Helper()
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

func copyTestFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func readMarker(t *testing.T, path string) string {
	t.Helper()
	db := openWALDB(t, path)
	defer db.Close()
	var v string
	if err := db.QueryRow("SELECT v FROM marker").Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// T-RS1: restoring over a database that a crash left with a hot WAL yields the
// snapshot, not the crashed database's un-checkpointed pages (SR-02).
func TestReplaceDatabaseDiscardsStaleWAL(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "live.db")
	snapshot := filepath.Join(dir, "snapshot.db")
	crashed := filepath.Join(dir, "crashed", "bekci.db")
	probe := filepath.Join(dir, "probe", "bekci.db")

	db := openWALDB(t, live)
	mustExec(t, db, "CREATE TABLE marker(v TEXT)", "INSERT INTO marker VALUES('backed-up')")
	db.Close() // clean close checkpoints and removes the WAL
	copyTestFile(t, live, snapshot)

	// Commit a change that lives only in the WAL, then capture the files as a
	// crash would leave them (twice: one copy to restore over, one to probe).
	db = openWALDB(t, live)
	mustExec(t, db, "PRAGMA wal_autocheckpoint=0", "UPDATE marker SET v='stale-wal'")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		copyTestFile(t, live+suffix, crashed+suffix)
		copyTestFile(t, live+suffix, probe+suffix)
	}
	db.Close()
	if got := readMarker(t, probe); got != "stale-wal" {
		t.Fatalf("crash image marker = %q; fixture did not leave a hot WAL", got)
	}

	if err := replaceDatabase(snapshot, crashed); err != nil {
		t.Fatal(err)
	}
	if got := readMarker(t, crashed); got != "backed-up" {
		t.Fatalf("restored marker = %q, want %q", got, "backed-up")
	}
}

// T-RS2: if an old side file cannot be removed, the restore stops before
// touching the existing database.
func TestReplaceDatabaseStopsWhenSideFileCannotBeRemoved(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "bekci.db")
	src := filepath.Join(dir, "snapshot.db")
	if err := os.WriteFile(dst, []byte("existing database"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	// A non-empty directory where the WAL would be cannot be removed.
	if err := os.MkdirAll(filepath.Join(dst+"-wal", "blocker"), 0700); err != nil {
		t.Fatal(err)
	}

	if err := replaceDatabase(src, dst); err == nil {
		t.Fatal("expected an error when a side file cannot be removed")
	}
	if data, _ := os.ReadFile(dst); string(data) != "existing database" {
		t.Fatalf("existing database changed: %q", data)
	}
}

// T-RS3: restoring onto a new server (no database or side files yet) works.
func TestReplaceDatabaseFreshDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "snapshot.db")
	dst := filepath.Join(dir, "new", "bekci.db")
	if err := os.WriteFile(src, []byte("snapshot"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := replaceDatabase(src, dst); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(dst); string(data) != "snapshot" {
		t.Fatalf("restored database = %q, want %q", data, "snapshot")
	}
}
