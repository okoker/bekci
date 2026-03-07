package engine

import (
	"sync"
	"testing"
	"time"

	"github.com/bekci/internal/store"
	"github.com/google/uuid"
)

type dispatchCall struct {
	ruleID, oldState, newState string
}

type mockDispatcher struct {
	mu    sync.Mutex
	calls []dispatchCall
}

func (m *mockDispatcher) Dispatch(ruleID, oldState, newState string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, dispatchCall{ruleID, oldState, newState})
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := store.New(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func setupTarget(t *testing.T, st *store.Store, field, comparator, value string, failCount, failWindow int) (checkID, ruleID string) {
	t.Helper()
	tgt := &store.Target{
		ID: uuid.New().String(), Name: "test", Host: "example.com",
		Enabled: true, Operator: "AND", Category: "Network",
	}
	conds := []store.TargetCondition{{
		CheckType: "http", CheckName: "HTTP", Config: "{}",
		IntervalS: 300, Field: field, Comparator: comparator, Value: value,
		FailCount: failCount, FailWindow: failWindow, ConditionGroup: 0, GroupOperator: "AND",
	}}
	if err := st.CreateTargetWithConditions(tgt, conds, ""); err != nil {
		t.Fatal(err)
	}
	checks, err := st.ListChecksByTarget(tgt.ID)
	if err != nil || len(checks) == 0 {
		t.Fatal("no checks created")
	}
	target, err := st.GetTarget(tgt.ID)
	if err != nil || target == nil || target.RuleID == nil {
		t.Fatal("no rule created")
	}
	return checks[0].ID, *target.RuleID
}

func saveResult(t *testing.T, st *store.Store, checkID, status string) {
	t.Helper()
	if err := st.SaveResult(&store.CheckResult{
		CheckID: checkID, Status: status, ResponseMs: 100,
		Message: "", Metrics: "{}", CheckedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluateStatusUp(t *testing.T) {
	st := newTestStore(t)
	eng := New(st)
	checkID, ruleID := setupTarget(t, st, "status", "eq", "down", 1, 0)
	saveResult(t, st, checkID, "up")
	eng.Evaluate(checkID)
	state, err := st.GetRuleState(ruleID)
	if err != nil {
		t.Fatal(err)
	}
	if state == nil || state.CurrentState != "healthy" {
		t.Fatalf("expected healthy, got %v", state)
	}
}

func TestEvaluateStatusDown(t *testing.T) {
	st := newTestStore(t)
	eng := New(st)
	checkID, ruleID := setupTarget(t, st, "status", "eq", "down", 1, 0)
	saveResult(t, st, checkID, "down")
	eng.Evaluate(checkID)
	state, err := st.GetRuleState(ruleID)
	if err != nil {
		t.Fatal(err)
	}
	if state == nil || state.CurrentState != "unhealthy" {
		t.Fatalf("expected unhealthy, got %v", state)
	}
}

func TestEvaluateStateTransition(t *testing.T) {
	st := newTestStore(t)
	eng := New(st)
	md := &mockDispatcher{}
	eng.SetDispatcher(md)
	checkID, ruleID := setupTarget(t, st, "status", "eq", "down", 1, 0)

	saveResult(t, st, checkID, "up")
	eng.Evaluate(checkID)
	state, err := st.GetRuleState(ruleID)
	if err != nil {
		t.Fatal(err)
	}
	if state == nil || state.CurrentState != "healthy" {
		t.Fatalf("expected healthy, got %v", state)
	}

	saveResult(t, st, checkID, "down")
	eng.Evaluate(checkID)
	time.Sleep(100 * time.Millisecond)
	state, err = st.GetRuleState(ruleID)
	if err != nil {
		t.Fatal(err)
	}
	if state == nil || state.CurrentState != "unhealthy" {
		t.Fatalf("expected unhealthy, got %v", state)
	}

	md.mu.Lock()
	defer md.mu.Unlock()
	if len(md.calls) != 1 {
		t.Fatalf("expected 1 dispatch call, got %d", len(md.calls))
	}
	if md.calls[0].oldState != "healthy" || md.calls[0].newState != "unhealthy" {
		t.Fatalf("expected healthy->unhealthy, got %s->%s", md.calls[0].oldState, md.calls[0].newState)
	}
}

func TestEvaluateNoStateChange(t *testing.T) {
	st := newTestStore(t)
	eng := New(st)
	md := &mockDispatcher{}
	eng.SetDispatcher(md)
	checkID, _ := setupTarget(t, st, "status", "eq", "down", 1, 0)

	saveResult(t, st, checkID, "down")
	eng.Evaluate(checkID)
	time.Sleep(100 * time.Millisecond)

	saveResult(t, st, checkID, "down")
	eng.Evaluate(checkID)
	time.Sleep(100 * time.Millisecond)

	md.mu.Lock()
	defer md.mu.Unlock()
	if len(md.calls) != 1 {
		t.Fatalf("expected 1 dispatch call, got %d", len(md.calls))
	}
}

func TestEvaluateConditionGroupAND(t *testing.T) {
	st := newTestStore(t)
	eng := New(st)
	tgt := &store.Target{
		ID: uuid.New().String(), Name: "test-and", Host: "example.com",
		Enabled: true, Operator: "AND", Category: "Network",
	}
	conds := []store.TargetCondition{
		{CheckType: "http", CheckName: "HTTP1", Config: "{}", IntervalS: 300, Field: "status", Comparator: "eq", Value: "down", FailCount: 1, FailWindow: 0, ConditionGroup: 0, GroupOperator: "AND"},
		{CheckType: "tcp", CheckName: "TCP1", Config: `{"port":80}`, IntervalS: 300, Field: "status", Comparator: "eq", Value: "down", FailCount: 1, FailWindow: 0, ConditionGroup: 0, GroupOperator: "AND"},
	}
	if err := st.CreateTargetWithConditions(tgt, conds, ""); err != nil {
		t.Fatal(err)
	}
	checks, err := st.ListChecksByTarget(tgt.ID)
	if err != nil || len(checks) < 2 {
		t.Fatal("expected 2 checks")
	}
	target, err := st.GetTarget(tgt.ID)
	if err != nil || target == nil || target.RuleID == nil {
		t.Fatal("no rule created")
	}
	ruleID := *target.RuleID

	var httpCheckID, tcpCheckID string
	for _, c := range checks {
		if c.Type == "http" {
			httpCheckID = c.ID
		} else {
			tcpCheckID = c.ID
		}
	}

	saveResult(t, st, httpCheckID, "down")
	saveResult(t, st, tcpCheckID, "up")
	eng.Evaluate(httpCheckID)

	state, err := st.GetRuleState(ruleID)
	if err != nil {
		t.Fatal(err)
	}
	if state == nil || state.CurrentState != "healthy" {
		t.Fatalf("expected healthy, got %v", state)
	}
}

func TestEvaluateConditionGroupOR(t *testing.T) {
	st := newTestStore(t)
	eng := New(st)
	tgt := &store.Target{
		ID: uuid.New().String(), Name: "test-or", Host: "example.com",
		Enabled: true, Operator: "AND", Category: "Network",
	}
	conds := []store.TargetCondition{
		{CheckType: "http", CheckName: "HTTP1", Config: "{}", IntervalS: 300, Field: "status", Comparator: "eq", Value: "down", FailCount: 1, FailWindow: 0, ConditionGroup: 0, GroupOperator: "AND"},
		{CheckType: "tcp", CheckName: "TCP1", Config: `{"port":80}`, IntervalS: 300, Field: "status", Comparator: "eq", Value: "down", FailCount: 1, FailWindow: 0, ConditionGroup: 1, GroupOperator: "AND"},
	}
	if err := st.CreateTargetWithConditions(tgt, conds, ""); err != nil {
		t.Fatal(err)
	}
	checks, err := st.ListChecksByTarget(tgt.ID)
	if err != nil || len(checks) < 2 {
		t.Fatal("expected 2 checks")
	}
	target, err := st.GetTarget(tgt.ID)
	if err != nil || target == nil || target.RuleID == nil {
		t.Fatal("no rule created")
	}
	ruleID := *target.RuleID

	var httpCheckID, tcpCheckID string
	for _, c := range checks {
		if c.Type == "http" {
			httpCheckID = c.ID
		} else {
			tcpCheckID = c.ID
		}
	}

	saveResult(t, st, httpCheckID, "down")
	saveResult(t, st, tcpCheckID, "up")
	eng.Evaluate(httpCheckID)

	state, err := st.GetRuleState(ruleID)
	if err != nil {
		t.Fatal(err)
	}
	if state == nil || state.CurrentState != "unhealthy" {
		t.Fatalf("expected unhealthy, got %v", state)
	}
}

func TestEvaluateFailWindow(t *testing.T) {
	st := newTestStore(t)
	eng := New(st)
	checkID, ruleID := setupTarget(t, st, "status", "eq", "down", 3, 300)
	saveResult(t, st, checkID, "down")
	saveResult(t, st, checkID, "down")
	eng.Evaluate(checkID)
	state, err := st.GetRuleState(ruleID)
	if err != nil {
		t.Fatal(err)
	}
	if state == nil || state.CurrentState != "healthy" {
		t.Fatalf("expected healthy, got %v", state)
	}
}
