package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

func makeUser(username, role string) *User {
	now := time.Now()
	return &User{
		ID:           uuid.New().String(),
		Username:     username,
		Email:        username + "@test.com",
		Phone:        "+1234567890",
		PasswordHash: "hash123",
		Role:         role,
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func makeTarget(name string) (*Target, []TargetCondition) {
	t := &Target{
		Name:               name,
		Host:               name + ".example.com",
		Description:        "test target",
		Enabled:            true,
		PreferredCheckType: "http",
		Operator:           "AND",
		Category:           "Other",
	}
	conds := []TargetCondition{
		{
			CheckType:     "http",
			CheckName:     name + " HTTP",
			Config:        "{}",
			IntervalS:     60,
			Field:         "status",
			Comparator:    "eq",
			Value:         "down",
			FailCount:     1,
			FailWindow:    0,
			GroupOperator: "AND",
		},
		{
			CheckType:     "tcp",
			CheckName:     name + " TCP",
			Config:        `{"port":80}`,
			IntervalS:     120,
			Field:         "status",
			Comparator:    "eq",
			Value:         "down",
			FailCount:     2,
			FailWindow:    300,
			GroupOperator: "AND",
		},
	}
	return t, conds
}

func TestCreateAndGetUser(t *testing.T) {
	s := newTestStore(t)
	u := makeUser("alice", "admin")

	if err := s.CreateUser(u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	got, err := s.GetUserByID(u.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got == nil {
		t.Fatal("GetUserByID returned nil")
	}
	if got.Username != "alice" {
		t.Fatalf("username = %q, want alice", got.Username)
	}
	if got.Email != "alice@test.com" {
		t.Fatalf("email = %q, want alice@test.com", got.Email)
	}
	if got.Role != "admin" {
		t.Fatalf("role = %q, want admin", got.Role)
	}
	if got.Status != "active" {
		t.Fatalf("status = %q, want active", got.Status)
	}

	got2, err := s.GetUserByUsername("alice")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if got2 == nil {
		t.Fatal("GetUserByUsername returned nil")
	}
	if got2.ID != u.ID {
		t.Fatalf("ID mismatch: %s != %s", got2.ID, u.ID)
	}
}

func TestListUsers(t *testing.T) {
	s := newTestStore(t)
	for _, name := range []string{"user1", "user2", "user3"} {
		if err := s.CreateUser(makeUser(name, "viewer")); err != nil {
			t.Fatalf("CreateUser(%s): %v", name, err)
		}
	}

	users, err := s.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 3 {
		t.Fatalf("len = %d, want 3", len(users))
	}
}

func TestUpdateUser(t *testing.T) {
	s := newTestStore(t)
	u := makeUser("bob", "viewer")
	if err := s.CreateUser(u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if err := s.UpdateUser(u.ID, "bob@new.com", "+9999", "operator"); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	got, err := s.GetUserByID(u.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got.Email != "bob@new.com" {
		t.Fatalf("email = %q, want bob@new.com", got.Email)
	}
	if got.Phone != "+9999" {
		t.Fatalf("phone = %q, want +9999", got.Phone)
	}
	if got.Role != "operator" {
		t.Fatalf("role = %q, want operator", got.Role)
	}
}

func TestSuspendUser(t *testing.T) {
	s := newTestStore(t)
	u := makeUser("carol", "viewer")
	if err := s.CreateUser(u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if err := s.SuspendUser(u.ID, true); err != nil {
		t.Fatalf("SuspendUser(true): %v", err)
	}
	got, _ := s.GetUserByID(u.ID)
	if got.Status != "suspended" {
		t.Fatalf("status = %q, want suspended", got.Status)
	}

	if err := s.SuspendUser(u.ID, false); err != nil {
		t.Fatalf("SuspendUser(false): %v", err)
	}
	got, _ = s.GetUserByID(u.ID)
	if got.Status != "active" {
		t.Fatalf("status = %q, want active", got.Status)
	}
}

func TestCountActiveAdmins(t *testing.T) {
	s := newTestStore(t)
	if err := s.CreateUser(makeUser("adm1", "admin")); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateUser(makeUser("adm2", "admin")); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateUser(makeUser("op1", "operator")); err != nil {
		t.Fatal(err)
	}

	count, err := s.CountActiveAdmins()
	if err != nil {
		t.Fatalf("CountActiveAdmins: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}

func TestSessionCRUD(t *testing.T) {
	s := newTestStore(t)
	u := makeUser("sessuser", "viewer")
	if err := s.CreateUser(u); err != nil {
		t.Fatal(err)
	}

	sess := &Session{
		ID:        uuid.New().String(),
		UserID:    u.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		IPAddress: "127.0.0.1",
		CreatedAt: time.Now(),
	}
	if err := s.CreateSession(sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	got, err := s.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got == nil {
		t.Fatal("GetSession returned nil")
	}
	if got.UserID != u.ID {
		t.Fatalf("UserID = %q, want %q", got.UserID, u.ID)
	}
	if got.IPAddress != "127.0.0.1" {
		t.Fatalf("IPAddress = %q, want 127.0.0.1", got.IPAddress)
	}

	if err := s.DeleteSession(sess.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	got, err = s.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("GetSession after delete: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestPurgeExpiredSessions(t *testing.T) {
	s := newTestStore(t)
	u := makeUser("purgeuser", "viewer")
	if err := s.CreateUser(u); err != nil {
		t.Fatal(err)
	}

	expired := &Session{
		ID:        uuid.New().String(),
		UserID:    u.ID,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
		IPAddress: "1.1.1.1",
		CreatedAt: time.Now().Add(-2 * time.Hour),
	}
	valid := &Session{
		ID:        uuid.New().String(),
		UserID:    u.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		IPAddress: "2.2.2.2",
		CreatedAt: time.Now(),
	}
	if err := s.CreateSession(expired); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(valid); err != nil {
		t.Fatal(err)
	}

	n, err := s.PurgeExpiredSessions()
	if err != nil {
		t.Fatalf("PurgeExpiredSessions: %v", err)
	}
	if n != 1 {
		t.Fatalf("purged = %d, want 1", n)
	}

	got, _ := s.GetSession(valid.ID)
	if got == nil {
		t.Fatal("valid session was purged")
	}
	got, _ = s.GetSession(expired.ID)
	if got != nil {
		t.Fatal("expired session still exists")
	}
}

func TestDeleteUserSessions(t *testing.T) {
	s := newTestStore(t)
	u := makeUser("delsessuser", "viewer")
	if err := s.CreateUser(u); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		sess := &Session{
			ID:        uuid.New().String(),
			UserID:    u.ID,
			ExpiresAt: time.Now().Add(time.Hour),
			IPAddress: "10.0.0.1",
			CreatedAt: time.Now(),
		}
		if err := s.CreateSession(sess); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.DeleteUserSessions(u.ID); err != nil {
		t.Fatalf("DeleteUserSessions: %v", err)
	}

	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, u.ID).Scan(&count)
	if count != 0 {
		t.Fatalf("remaining sessions = %d, want 0", count)
	}
}

func TestSettings(t *testing.T) {
	s := newTestStore(t)

	kv := map[string]string{
		"test_key1": "val1",
		"test_key2": "val2",
		"test_key3": "val3",
	}
	if err := s.SetSettings(kv); err != nil {
		t.Fatalf("SetSettings: %v", err)
	}

	for k, want := range kv {
		got, err := s.GetSetting(k)
		if err != nil {
			t.Fatalf("GetSetting(%s): %v", k, err)
		}
		if got != want {
			t.Fatalf("GetSetting(%s) = %q, want %q", k, got, want)
		}
	}

	all, err := s.GetAllSettings()
	if err != nil {
		t.Fatalf("GetAllSettings: %v", err)
	}
	for k, want := range kv {
		if all[k] != want {
			t.Fatalf("GetAllSettings[%s] = %q, want %q", k, all[k], want)
		}
	}
}

func TestCreateTargetWithConditions(t *testing.T) {
	s := newTestStore(t)
	tgt, conds := makeTarget("srv1")

	if err := s.CreateTargetWithConditions(tgt, conds, ""); err != nil {
		t.Fatalf("CreateTargetWithConditions: %v", err)
	}

	got, err := s.GetTarget(tgt.ID)
	if err != nil {
		t.Fatalf("GetTarget: %v", err)
	}
	if got == nil {
		t.Fatal("GetTarget returned nil")
	}
	if got.Name != "srv1" {
		t.Fatalf("name = %q, want srv1", got.Name)
	}

	checks, err := s.ListChecksByTarget(tgt.ID)
	if err != nil {
		t.Fatalf("ListChecksByTarget: %v", err)
	}
	if len(checks) != 2 {
		t.Fatalf("checks len = %d, want 2", len(checks))
	}

	types := map[string]bool{}
	for _, c := range checks {
		types[c.Type] = true
	}
	if !types["http"] || !types["tcp"] {
		t.Fatalf("check types = %v, want http and tcp", types)
	}

	if got.RuleID == nil {
		t.Fatal("RuleID is nil, want non-nil")
	}
}

func TestDeleteTarget(t *testing.T) {
	s := newTestStore(t)
	tgt, conds := makeTarget("del-tgt")
	if err := s.CreateTargetWithConditions(tgt, conds, ""); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteTarget(tgt.ID); err != nil {
		t.Fatalf("DeleteTarget: %v", err)
	}

	got, err := s.GetTarget(tgt.ID)
	if err != nil {
		t.Fatalf("GetTarget: %v", err)
	}
	if got != nil {
		t.Fatal("target still exists after delete")
	}

	targets, err := s.ListTargets()
	if err != nil {
		t.Fatal(err)
	}
	for _, tg := range targets {
		if tg.ID == tgt.ID {
			t.Fatal("deleted target found in ListTargets")
		}
	}
}

func TestSaveAndQueryResults(t *testing.T) {
	s := newTestStore(t)
	tgt, conds := makeTarget("res-tgt")
	if err := s.CreateTargetWithConditions(tgt, conds[:1], ""); err != nil {
		t.Fatal(err)
	}
	checks, _ := s.ListChecksByTarget(tgt.ID)
	if len(checks) == 0 {
		t.Fatal("no checks created")
	}
	checkID := checks[0].ID

	now := time.Now()
	for i := 0; i < 5; i++ {
		r := &CheckResult{
			CheckID:    checkID,
			Status:     "up",
			ResponseMs: int64(100 + i),
			Message:    fmt.Sprintf("ok-%d", i),
			Metrics:    "{}",
			CheckedAt:  now.Add(time.Duration(-i) * time.Minute),
		}
		if err := s.SaveResult(r); err != nil {
			t.Fatalf("SaveResult: %v", err)
		}
	}

	results, err := s.GetRecentResults(checkID, 24)
	if err != nil {
		t.Fatalf("GetRecentResults: %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("results len = %d, want 5", len(results))
	}
	for i := 1; i < len(results); i++ {
		if results[i].CheckedAt.Before(results[i-1].CheckedAt) {
			t.Fatalf("results not in ASC order at index %d", i)
		}
	}
}

func TestGetUptimePercent(t *testing.T) {
	s := newTestStore(t)
	tgt, conds := makeTarget("uptime-tgt")
	if err := s.CreateTargetWithConditions(tgt, conds[:1], ""); err != nil {
		t.Fatal(err)
	}
	checks, _ := s.ListChecksByTarget(tgt.ID)
	checkID := checks[0].ID

	now := time.Now()
	for i := 0; i < 10; i++ {
		status := "up"
		if i >= 7 {
			status = "down"
		}
		r := &CheckResult{
			CheckID:    checkID,
			Status:     status,
			ResponseMs: 50,
			Message:    "",
			Metrics:    "{}",
			CheckedAt:  now.Add(time.Duration(-i) * time.Minute),
		}
		if err := s.SaveResult(r); err != nil {
			t.Fatal(err)
		}
	}

	pct, err := s.GetUptimePercent(checkID, 1)
	if err != nil {
		t.Fatalf("GetUptimePercent: %v", err)
	}
	if pct != 70.0 {
		t.Fatalf("uptime = %.2f, want 70.00", pct)
	}
}

func TestPurgeOldResults(t *testing.T) {
	s := newTestStore(t)
	tgt, conds := makeTarget("purge-res-tgt")
	if err := s.CreateTargetWithConditions(tgt, conds[:1], ""); err != nil {
		t.Fatal(err)
	}
	checks, _ := s.ListChecksByTarget(tgt.ID)
	checkID := checks[0].ID

	now := time.Now()
	for i := 0; i < 2; i++ {
		r := &CheckResult{
			CheckID:   checkID,
			Status:    "up",
			Message:   "",
			Metrics:   "{}",
			CheckedAt: now.Add(-40 * 24 * time.Hour),
		}
		if err := s.SaveResult(r); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		r := &CheckResult{
			CheckID:   checkID,
			Status:    "up",
			Message:   "",
			Metrics:   "{}",
			CheckedAt: now,
		}
		if err := s.SaveResult(r); err != nil {
			t.Fatal(err)
		}
	}

	purged, err := s.PurgeOldResults(30)
	if err != nil {
		t.Fatalf("PurgeOldResults: %v", err)
	}
	if purged != 2 {
		t.Fatalf("purged = %d, want 2", purged)
	}

	var remaining int
	s.db.QueryRow(`SELECT COUNT(*) FROM check_results WHERE check_id = ?`, checkID).Scan(&remaining)
	if remaining != 2 {
		t.Fatalf("remaining = %d, want 2", remaining)
	}
}

func TestAuditEntries(t *testing.T) {
	s := newTestStore(t)

	for i := 0; i < 5; i++ {
		e := &AuditEntry{
			UserID:       "uid1",
			Username:     "admin",
			Action:       fmt.Sprintf("action_%d", i),
			ResourceType: "target",
			ResourceID:   "tid1",
			Detail:       "",
			IPAddress:    "10.0.0.1",
			Status:       "success",
		}
		if err := s.CreateAuditEntry(e); err != nil {
			t.Fatalf("CreateAuditEntry: %v", err)
		}
	}

	entries, total, err := s.ListAuditEntries(2, 0)
	if err != nil {
		t.Fatalf("ListAuditEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries len = %d, want 2", len(entries))
	}
	if total != 5 {
		t.Fatalf("total = %d, want 5", total)
	}

	purged, err := s.PurgeOldAuditEntries(0)
	if err != nil {
		t.Fatalf("PurgeOldAuditEntries: %v", err)
	}
	if purged != 5 {
		t.Fatalf("purged = %d, want 5", purged)
	}
}

func TestAlertRecipients(t *testing.T) {
	s := newTestStore(t)
	tgt, conds := makeTarget("alert-tgt")
	if err := s.CreateTargetWithConditions(tgt, conds[:1], ""); err != nil {
		t.Fatal(err)
	}

	u1 := makeUser("recip1", "operator")
	u2 := makeUser("recip2", "operator")
	if err := s.CreateUser(u1); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateUser(u2); err != nil {
		t.Fatal(err)
	}

	if err := s.SetTargetRecipients(tgt.ID, []string{u1.ID, u2.ID}); err != nil {
		t.Fatalf("SetTargetRecipients: %v", err)
	}
	recipients, err := s.ListTargetRecipients(tgt.ID)
	if err != nil {
		t.Fatalf("ListTargetRecipients: %v", err)
	}
	if len(recipients) != 2 {
		t.Fatalf("recipients len = %d, want 2", len(recipients))
	}

	if err := s.SetTargetRecipients(tgt.ID, []string{u1.ID}); err != nil {
		t.Fatal(err)
	}
	recipients, _ = s.ListTargetRecipients(tgt.ID)
	if len(recipients) != 1 {
		t.Fatalf("recipients len = %d, want 1", len(recipients))
	}
}

func TestAlertHistory(t *testing.T) {
	s := newTestStore(t)
	tgt, conds := makeTarget("hist-tgt")
	if err := s.CreateTargetWithConditions(tgt, conds[:1], ""); err != nil {
		t.Fatal(err)
	}
	ruleID := ""
	if tgt.RuleID != nil {
		ruleID = *tgt.RuleID
	}

	u := makeUser("alertuser", "admin")
	if err := s.CreateUser(u); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		if err := s.LogAlert(tgt.ID, ruleID, u.ID, "firing", fmt.Sprintf("alert %d", i)); err != nil {
			t.Fatalf("LogAlert: %v", err)
		}
	}

	items, total, err := s.ListAlertHistory(10, 0)
	if err != nil {
		t.Fatalf("ListAlertHistory: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("items len = %d, want 3", len(items))
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}

	purged, err := s.PurgeOldAlertHistory(0)
	if err != nil {
		t.Fatalf("PurgeOldAlertHistory: %v", err)
	}
	if purged != 3 {
		t.Fatalf("purged = %d, want 3", purged)
	}
}

func TestCooldownIgnoresRecoveryAlerts(t *testing.T) {
	s := newTestStore(t)
	tgt, conds := makeTarget("cooldown-tgt")
	if err := s.CreateTargetWithConditions(tgt, conds[:1], ""); err != nil {
		t.Fatal(err)
	}
	ruleID := ""
	if tgt.RuleID != nil {
		ruleID = *tgt.RuleID
	}

	u := makeUser("cooldownuser", "admin")
	if err := s.CreateUser(u); err != nil {
		t.Fatal(err)
	}

	// Log a firing alert
	if err := s.LogAlert(tgt.ID, ruleID, u.ID, "firing", "host is down"); err != nil {
		t.Fatalf("LogAlert(firing): %v", err)
	}

	// Record the firing alert time
	firingTime, err := s.GetLastAlertTime(ruleID)
	if err != nil {
		t.Fatalf("GetLastAlertTime after firing: %v", err)
	}
	if firingTime.IsZero() {
		t.Fatal("expected non-zero firing time")
	}

	// Log a recovery alert after the firing alert
	// Small sleep to ensure different timestamp
	time.Sleep(50 * time.Millisecond)
	if err := s.LogAlert(tgt.ID, ruleID, u.ID, "recovery", "host recovered"); err != nil {
		t.Fatalf("LogAlert(recovery): %v", err)
	}

	// GetLastAlertTime should return the firing time, not the recovery time
	got, err := s.GetLastAlertTime(ruleID)
	if err != nil {
		t.Fatalf("GetLastAlertTime after recovery: %v", err)
	}
	if !got.Equal(firingTime) {
		t.Fatalf("GetLastAlertTime = %v, want %v (firing time); recovery alert should be ignored", got, firingTime)
	}
}

func TestBackupExportRestore(t *testing.T) {
	s1 := newTestStore(t)

	u := makeUser("backupuser", "admin")
	if err := s1.CreateUser(u); err != nil {
		t.Fatal(err)
	}

	tgt, conds := makeTarget("bak-tgt")
	if err := s1.CreateTargetWithConditions(tgt, conds, ""); err != nil {
		t.Fatal(err)
	}

	if err := s1.SetSettings(map[string]string{"custom_key": "custom_val"}); err != nil {
		t.Fatal(err)
	}

	backup, err := s1.ExportBackup("1.0.0-test")
	if err != nil {
		t.Fatalf("ExportBackup: %v", err)
	}

	s2 := newTestStore(t)
	if err := s2.RestoreBackup(backup); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}

	got, err := s2.GetUserByUsername("backupuser")
	if err != nil {
		t.Fatalf("GetUserByUsername on restored: %v", err)
	}
	if got == nil {
		t.Fatal("user not found in restored store")
	}
	if got.ID != u.ID {
		t.Fatalf("restored user ID = %q, want %q", got.ID, u.ID)
	}

	val, err := s2.GetSetting("custom_key")
	if err != nil {
		t.Fatalf("GetSetting on restored: %v", err)
	}
	if val != "custom_val" {
		t.Fatalf("restored setting = %q, want custom_val", val)
	}

	restoredTarget, err := s2.GetTarget(tgt.ID)
	if err != nil {
		t.Fatalf("GetTarget on restored: %v", err)
	}
	if restoredTarget == nil {
		t.Fatal("target not found in restored store")
	}
	if restoredTarget.Name != "bak-tgt" {
		t.Fatalf("restored target name = %q, want bak-tgt", restoredTarget.Name)
	}
}

func TestRecipientsExcludesSuspendedUsers(t *testing.T) {
	s := newTestStore(t)

	uA := makeUser("recipA", "operator")
	uB := makeUser("recipB", "operator")
	if err := s.CreateUser(uA); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateUser(uB); err != nil {
		t.Fatal(err)
	}

	tgt, conds := makeTarget("suspend-tgt")
	if err := s.CreateTargetWithConditions(tgt, conds[:1], ""); err != nil {
		t.Fatal(err)
	}

	if err := s.SetTargetRecipients(tgt.ID, []string{uA.ID, uB.ID}); err != nil {
		t.Fatalf("SetTargetRecipients: %v", err)
	}

	recipients, err := s.ListTargetRecipients(tgt.ID)
	if err != nil {
		t.Fatalf("ListTargetRecipients (both active): %v", err)
	}
	if len(recipients) != 2 {
		t.Fatalf("recipients len = %d, want 2", len(recipients))
	}

	if err := s.SuspendUser(uB.ID, true); err != nil {
		t.Fatalf("SuspendUser: %v", err)
	}

	recipients, err = s.ListTargetRecipients(tgt.ID)
	if err != nil {
		t.Fatalf("ListTargetRecipients (one suspended): %v", err)
	}
	if len(recipients) != 1 {
		t.Fatalf("recipients len = %d, want 1 (suspended user should be excluded)", len(recipients))
	}
	if recipients[0].ID != uA.ID {
		t.Fatalf("remaining recipient = %q, want %q", recipients[0].ID, uA.ID)
	}
}

func TestGetFiringRulesIgnoresPausedDisabled(t *testing.T) {
	s := newTestStore(t)
	tgt, conds := makeTarget("firing-tgt")
	if err := s.CreateTargetWithConditions(tgt, conds[:1], ""); err != nil {
		t.Fatal(err)
	}
	if tgt.RuleID == nil {
		t.Fatal("RuleID is nil after CreateTargetWithConditions")
	}
	ruleID := *tgt.RuleID

	// Set the rule_state to 'unhealthy' (CreateTargetWithConditions already inserts 'healthy')
	if _, err := s.db.Exec(`UPDATE rule_states SET current_state = 'unhealthy', last_change = CURRENT_TIMESTAMP WHERE rule_id = ?`, ruleID); err != nil {
		t.Fatalf("update rule_state: %v", err)
	}

	// Active + enabled target should appear
	rules, err := s.GetFiringRules()
	if err != nil {
		t.Fatalf("GetFiringRules (active): %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("firing rules len = %d, want 1", len(rules))
	}
	if rules[0].RuleID != ruleID || rules[0].TargetID != tgt.ID {
		t.Fatalf("unexpected rule: %+v", rules[0])
	}

	// Pause the target — should no longer appear
	if err := s.PauseTarget(tgt.ID); err != nil {
		t.Fatalf("PauseTarget: %v", err)
	}
	rules, err = s.GetFiringRules()
	if err != nil {
		t.Fatalf("GetFiringRules (paused): %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("firing rules len = %d, want 0 (paused target should be excluded)", len(rules))
	}

	// Unpause — should appear again
	if err := s.UnpauseTarget(tgt.ID); err != nil {
		t.Fatalf("UnpauseTarget: %v", err)
	}
	rules, err = s.GetFiringRules()
	if err != nil {
		t.Fatalf("GetFiringRules (unpaused): %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("firing rules len = %d, want 1 after unpause", len(rules))
	}

	// Disable the target — should no longer appear
	if _, err := s.db.Exec(`UPDATE targets SET enabled = 0 WHERE id = ?`, tgt.ID); err != nil {
		t.Fatalf("disable target: %v", err)
	}
	rules, err = s.GetFiringRules()
	if err != nil {
		t.Fatalf("GetFiringRules (disabled): %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("firing rules len = %d, want 0 (disabled target should be excluded)", len(rules))
	}
}
