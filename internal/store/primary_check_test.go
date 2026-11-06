package store

import (
	"sort"
	"testing"
)

// T-S33: PrimaryCheck picks the check that stands for a target in SLA, dashboard
// uptime and /api/v1/hosts: the first check of the preferred type in list order,
// otherwise the first check; nil when the target has no checks (backlog A-062).
func TestPrimaryCheck(t *testing.T) {
	checks := []Check{
		{ID: "1", Type: "ping", Name: "a-ping"},
		{ID: "2", Type: "http", Name: "b-slow"},
		{ID: "3", Type: "http", Name: "c-page"},
	}
	for _, tc := range []struct{ preferred, want string }{
		{"http", "2"}, // two of the type: only the first counts
		{"ping", "1"},
		{"dns", "1"}, // no check of the type: the first check
		{"", "1"},
	} {
		got := PrimaryCheck(checks, tc.preferred)
		if got == nil || got.ID != tc.want {
			t.Errorf("preferred %q: got %+v, want check %s", tc.preferred, got, tc.want)
		}
	}
	if got := PrimaryCheck(nil, "http"); got != nil {
		t.Errorf("no checks: got %+v, want nil", got)
	}
}

// T-S34: checks with the same name are listed in id order by both list queries,
// so every caller of PrimaryCheck agrees on which one comes first (backlog A-062).
func TestCheckListsOrderEqualNamesByID(t *testing.T) {
	s := newTestStore(t)
	tgt := &Target{Name: "dup", Host: "dup.example.com", Enabled: true, Operator: "AND", Category: "Other"}
	conds := []TargetCondition{{CheckType: "http", CheckName: "HTTPS Check", Config: "{}", IntervalS: 60,
		Field: "status", Comparator: "eq", Value: "down", FailCount: 1, GroupOperator: "AND"}}
	if err := s.CreateTargetWithConditions(tgt, conds, ""); err != nil {
		t.Fatal(err)
	}
	// Two more checks with the same name, inserted with their ids out of order.
	for _, id := range []string{"zzz-check", "aaa-check"} {
		if _, err := s.db.Exec(`INSERT INTO checks (id, target_id, type, name) VALUES (?, ?, 'http', 'HTTPS Check')`, id, tgt.ID); err != nil {
			t.Fatal(err)
		}
	}

	byTarget, err := s.ListChecksByTarget(tgt.ID)
	if err != nil {
		t.Fatal(err)
	}
	all, err := s.ListAllChecks()
	if err != nil {
		t.Fatal(err)
	}
	idsOf := func(cs []Check) []string {
		var ids []string
		for _, c := range cs {
			ids = append(ids, c.ID)
		}
		return ids
	}
	want := idsOf(byTarget)
	sort.Strings(want)
	if got := idsOf(byTarget); !equalStrings(got, want) {
		t.Errorf("ListChecksByTarget: %v, want id order %v", got, want)
	}
	if got := idsOf(all[tgt.ID]); !equalStrings(got, want) {
		t.Errorf("ListAllChecks: %v, want id order %v", got, want)
	}
}
