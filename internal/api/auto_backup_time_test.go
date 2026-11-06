package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bekci/internal/alerter"
	"github.com/bekci/internal/auth"
	"github.com/bekci/internal/store"
)

// T-P57: backlog A-067 — the auto-backup time is a whole hour. Anything else
// was saved without a word: 25:00 then never ran, abc ran at midnight.
func TestAutoBackupTimeValidation(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "admin2", "testpassword12345", "admin")
	client := loginAs(t, ts, "admin2", "testpassword12345")

	put := func(v string) int {
		body, _ := json.Marshal(map[string]string{"auto_backup_time": v})
		resp, err := client.Do(putReq(t, ts.URL+"/api/settings", body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	for _, v := range []string{"00:00", "03:00", "23:00", ""} {
		if code := put(v); code != 200 {
			t.Errorf("%q: got %d, want 200", v, code)
		}
		if got, _ := st.GetSetting("auto_backup_time"); got != v {
			t.Errorf("%q: stored %q", v, got)
		}
	}

	if code := put("22:00"); code != 200 {
		t.Fatalf("22:00: got %d", code)
	}
	for _, v := range []string{"25:00", "24:00", "abc", "3pm", "03:45", "3:00", " 03:00", "03:00 ", "03:60"} {
		if code := put(v); code != 400 {
			t.Errorf("%q: got %d, want 400", v, code)
		}
	}
	if got, _ := st.GetSetting("auto_backup_time"); got != "22:00" {
		t.Errorf("a rejected value changed the stored time: %q", got)
	}
}

// T-P58: backlog A-067 — when the hourly check makes a scheduled backup. The
// schedule counts calendar days: an exact 7 x 24 hours made "weekly" every 8
// days, because the check a week later falls the seconds the backup took
// short of 7 days and the next check in that hour is the next day.
func TestScheduledBackupHourAndCalendarDays(t *testing.T) {
	zone := time.FixedZone("UTC+3", 3*3600)
	now := time.Date(2026, 11, 9, 3, 0, 30, 0, zone) // Monday, 03:00:30 local
	at := func(day, hour, min, sec int) time.Time { return time.Date(2026, 11, day, hour, min, sec, 0, zone) }

	cases := []struct {
		name, schedule, hour string
		last                 time.Time // newest saved backup; zero = none
		want                 bool
	}{
		{"weekly, last one a week ago took 15s to save", "weekly", "03:00", at(2, 3, 0, 45), true},
		{"weekly, last one 6 days ago", "weekly", "03:00", at(3, 3, 0, 0), false},
		{"weekly, 7 days ago in UTC but the 3rd here", "weekly", "03:00", time.Date(2026, 11, 2, 22, 0, 0, 0, time.UTC), false},
		{"weekly, Save to server last Monday afternoon", "weekly", "03:00", at(2, 14, 0, 0), true},
		{"10 days, last one 10 days ago at the same check", "10days", "03:00", time.Date(2026, 10, 30, 3, 0, 45, 0, zone), true},
		{"10 days, last one 9 days ago", "10days", "03:00", time.Date(2026, 10, 31, 3, 0, 0, 0, zone), false},
		{"monthly, last one 30 days ago at the same check", "monthly", "03:00", time.Date(2026, 10, 10, 3, 0, 45, 0, zone), true},
		{"monthly, last one 29 days ago", "monthly", "03:00", time.Date(2026, 10, 11, 3, 0, 0, 0, zone), false},
		{"another hour", "weekly", "04:00", time.Time{}, false},
		{"schedule off", "off", "03:00", time.Time{}, false},
		{"empty time is 03:00", "weekly", "", time.Time{}, true},
		{"25:00 saved before validation is 03:00", "weekly", "25:00", time.Time{}, true},
		{"abc saved before validation is 03:00", "weekly", "abc", time.Time{}, true},
		{"03:45 saved before validation runs in its hour", "weekly", "03:45", time.Time{}, true},
		{"4:30 saved before validation runs in its hour", "weekly", "4:30", time.Time{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, err := store.New(t.TempDir() + "/test.db")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { st.Close() })
			dir := t.TempDir()
			before := 0
			if !c.last.IsZero() {
				if err := os.WriteFile(filepath.Join(dir, oldBackup1), []byte("x"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := saveBackupIndex(dir, []backupMeta{{Filename: oldBackup1, CreatedAt: c.last.UTC().Format(time.RFC3339)}}); err != nil {
					t.Fatal(err)
				}
				before = 1
			}
			if err := st.SetSettings(map[string]string{"auto_backup_schedule": c.schedule, "auto_backup_time": c.hour}); err != nil {
				t.Fatal(err)
			}
			srv := New(st, auth.New(st, "test-secret-key-for-testing"), nil, alerter.New(st), "test", nil, "", "", "", dir)
			t.Cleanup(srv.Close)

			srv.runScheduledBackupAt(now)

			if made := len(loadBackupIndex(dir)) > before; made != c.want {
				t.Errorf("backup made = %v, want %v", made, c.want)
			}
		})
	}
}

// T-P59: backlog A-067 — System Health shows the hour the backup runs in, not
// a stored value the server cannot read.
func TestSystemInfoBackupTimeIsTheHourUsed(t *testing.T) {
	ts, st := setupTestServer(t)
	createUser(t, st, "adminsys", "testpassword12345", "admin")
	admin := loginAs(t, ts, "adminsys", "testpassword12345")

	for stored, want := range map[string]string{"": "03:00", "22:00": "22:00", "25:00": "03:00", "abc": "03:00", "04:30": "04:00"} {
		if err := st.SetSettings(map[string]string{"auto_backup_time": stored}); err != nil {
			t.Fatal(err)
		}
		resp, err := admin.Get(ts.URL + "/api/system/info")
		if err != nil {
			t.Fatal(err)
		}
		backup, _ := parseJSON(t, resp)["backup"].(map[string]any)
		if backup["time"] != want {
			t.Errorf("stored %q: backup.time = %v, want %q", stored, backup["time"], want)
		}
	}
}
