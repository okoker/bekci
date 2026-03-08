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

// --- saveResultFull helper for custom metrics/response_ms ---

func saveResultFull(t *testing.T, st *store.Store, checkID, status, metrics string, responseMs int64) {
	t.Helper()
	if err := st.SaveResult(&store.CheckResult{
		CheckID: checkID, Status: status, ResponseMs: responseMs,
		Message: "", Metrics: metrics, CheckedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
}

// --- Numeric comparators ---

// T-E8: response_ms gt 500, responseMs=600 → unhealthy
func TestResponseMsGt(t *testing.T) {
	st := newTestStore(t)
	eng := New(st)
	checkID, ruleID := setupTarget(t, st, "response_ms", "gt", "500", 1, 0)
	saveResultFull(t, st, checkID, "up", "{}", 600)
	eng.Evaluate(checkID)
	state, err := st.GetRuleState(ruleID)
	if err != nil {
		t.Fatal(err)
	}
	if state == nil || state.CurrentState != "unhealthy" {
		t.Fatalf("expected unhealthy, got %v", state)
	}
}

// T-E9: response_ms gt 500, responseMs=100 → healthy (not triggered)
func TestResponseMsGtNotTriggered(t *testing.T) {
	st := newTestStore(t)
	eng := New(st)
	checkID, ruleID := setupTarget(t, st, "response_ms", "gt", "500", 1, 0)
	saveResultFull(t, st, checkID, "up", "{}", 100)
	eng.Evaluate(checkID)
	state, err := st.GetRuleState(ruleID)
	if err != nil {
		t.Fatal(err)
	}
	if state == nil || state.CurrentState != "healthy" {
		t.Fatalf("expected healthy, got %v", state)
	}
}

// T-E10: response_ms lt 50, responseMs=30 → unhealthy
func TestComparatorLt(t *testing.T) {
	st := newTestStore(t)
	eng := New(st)
	checkID, ruleID := setupTarget(t, st, "response_ms", "lt", "50", 1, 0)
	saveResultFull(t, st, checkID, "up", "{}", 30)
	eng.Evaluate(checkID)
	state, err := st.GetRuleState(ruleID)
	if err != nil {
		t.Fatal(err)
	}
	if state == nil || state.CurrentState != "unhealthy" {
		t.Fatalf("expected unhealthy, got %v", state)
	}
}

// T-E11: response_ms gte 100, exactly 100ms → unhealthy (boundary)
func TestComparatorGteBoundary(t *testing.T) {
	st := newTestStore(t)
	eng := New(st)
	checkID, ruleID := setupTarget(t, st, "response_ms", "gte", "100", 1, 0)
	saveResultFull(t, st, checkID, "up", "{}", 100)
	eng.Evaluate(checkID)
	state, err := st.GetRuleState(ruleID)
	if err != nil {
		t.Fatal(err)
	}
	if state == nil || state.CurrentState != "unhealthy" {
		t.Fatalf("expected unhealthy, got %v", state)
	}
}

// T-E12: response_ms lte 100, exactly 100ms → unhealthy (boundary)
func TestComparatorLteBoundary(t *testing.T) {
	st := newTestStore(t)
	eng := New(st)
	checkID, ruleID := setupTarget(t, st, "response_ms", "lte", "100", 1, 0)
	saveResultFull(t, st, checkID, "up", "{}", 100)
	eng.Evaluate(checkID)
	state, err := st.GetRuleState(ruleID)
	if err != nil {
		t.Fatal(err)
	}
	if state == nil || state.CurrentState != "unhealthy" {
		t.Fatalf("expected unhealthy, got %v", state)
	}
}

// T-E13: status neq up, result "down" → unhealthy
func TestComparatorNeq(t *testing.T) {
	st := newTestStore(t)
	eng := New(st)
	checkID, ruleID := setupTarget(t, st, "status", "neq", "up", 1, 0)
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

// --- Field extraction ---

// T-E14: metrics.packet_loss gt 50, metrics={"packet_loss":75} → unhealthy
func TestMetricsFieldExtraction(t *testing.T) {
	st := newTestStore(t)
	eng := New(st)
	checkID, ruleID := setupTarget(t, st, "metrics.packet_loss", "gt", "50", 1, 0)
	saveResultFull(t, st, checkID, "up", `{"packet_loss":75}`, 100)
	eng.Evaluate(checkID)
	state, err := st.GetRuleState(ruleID)
	if err != nil {
		t.Fatal(err)
	}
	if state == nil || state.CurrentState != "unhealthy" {
		t.Fatalf("expected unhealthy, got %v", state)
	}
}

// T-E15: metrics.packet_loss gt 50, malformed JSON → healthy (silent fail)
func TestMetricsMalformedJSON(t *testing.T) {
	st := newTestStore(t)
	eng := New(st)
	checkID, ruleID := setupTarget(t, st, "metrics.packet_loss", "gt", "50", 1, 0)
	saveResultFull(t, st, checkID, "up", `"not json"`, 100)
	eng.Evaluate(checkID)
	state, err := st.GetRuleState(ruleID)
	if err != nil {
		t.Fatal(err)
	}
	if state == nil || state.CurrentState != "healthy" {
		t.Fatalf("expected healthy, got %v", state)
	}
}

// T-E16: metrics.nonexistent gt 50, key missing → healthy (silent fail)
func TestMetricsMissingKey(t *testing.T) {
	st := newTestStore(t)
	eng := New(st)
	checkID, ruleID := setupTarget(t, st, "metrics.nonexistent", "gt", "50", 1, 0)
	saveResultFull(t, st, checkID, "up", `{"packet_loss":75}`, 100)
	eng.Evaluate(checkID)
	state, err := st.GetRuleState(ruleID)
	if err != nil {
		t.Fatal(err)
	}
	if state == nil || state.CurrentState != "healthy" {
		t.Fatalf("expected healthy, got %v", state)
	}
}

// T-E17: unknown field gt 100 → healthy (silent fail)
func TestUnknownFieldGtComparator(t *testing.T) {
	st := newTestStore(t)
	eng := New(st)
	checkID, ruleID := setupTarget(t, st, "bogus_field", "gt", "100", 1, 0)
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

// --- Fail window edge cases ---

// T-E18: fail_count=3, fail_window=3600, 3 consecutive "down" → unhealthy
func TestFailWindowMatchingStreak(t *testing.T) {
	st := newTestStore(t)
	eng := New(st)
	checkID, ruleID := setupTarget(t, st, "status", "eq", "down", 3, 3600)
	saveResult(t, st, checkID, "down")
	saveResult(t, st, checkID, "down")
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

// T-E19: fail_count=3, fail_window=3600, down/up/down/down → healthy (streak broken)
func TestFailWindowBrokenStreak(t *testing.T) {
	st := newTestStore(t)
	eng := New(st)
	checkID, ruleID := setupTarget(t, st, "status", "eq", "down", 3, 3600)
	// Save chronologically: down, up, down, down
	// DESC order: down(newest), down, up, down(oldest)
	// Streak from newest: down, down → breaks at up → streak=2 < fail_count=3
	saveResult(t, st, checkID, "down")
	time.Sleep(time.Millisecond)
	saveResult(t, st, checkID, "up")
	time.Sleep(time.Millisecond)
	saveResult(t, st, checkID, "down")
	time.Sleep(time.Millisecond)
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

// T-E20: fail_count=1, fail_window=300, no results → healthy (empty)
func TestFailWindowEmptyResults(t *testing.T) {
	st := newTestStore(t)
	eng := New(st)
	checkID, ruleID := setupTarget(t, st, "status", "eq", "down", 1, 300)
	// No results saved — brand new check
	eng.Evaluate(checkID)
	state, err := st.GetRuleState(ruleID)
	if err != nil {
		t.Fatal(err)
	}
	if state == nil || state.CurrentState != "healthy" {
		t.Fatalf("expected healthy, got %v", state)
	}
}

// --- State transitions ---

// T-E21: unhealthy → save "up" → healthy + Dispatch(unhealthy→healthy)
func TestRecoveryTransition(t *testing.T) {
	st := newTestStore(t)
	eng := New(st)
	md := &mockDispatcher{}
	eng.SetDispatcher(md)
	checkID, ruleID := setupTarget(t, st, "status", "eq", "down", 1, 0)

	// Make it unhealthy first
	saveResult(t, st, checkID, "down")
	eng.Evaluate(checkID)
	time.Sleep(100 * time.Millisecond)
	state, err := st.GetRuleState(ruleID)
	if err != nil {
		t.Fatal(err)
	}
	if state == nil || state.CurrentState != "unhealthy" {
		t.Fatalf("expected unhealthy, got %v", state)
	}

	// Now recover
	saveResult(t, st, checkID, "up")
	eng.Evaluate(checkID)
	time.Sleep(100 * time.Millisecond)
	state, err = st.GetRuleState(ruleID)
	if err != nil {
		t.Fatal(err)
	}
	if state == nil || state.CurrentState != "healthy" {
		t.Fatalf("expected healthy after recovery, got %v", state)
	}

	// Verify dispatch: 2 calls — healthy→unhealthy, then unhealthy→healthy
	md.mu.Lock()
	defer md.mu.Unlock()
	if len(md.calls) != 2 {
		t.Fatalf("expected 2 dispatch calls, got %d", len(md.calls))
	}
	if md.calls[1].oldState != "unhealthy" || md.calls[1].newState != "healthy" {
		t.Fatalf("expected unhealthy→healthy, got %s→%s", md.calls[1].oldState, md.calls[1].newState)
	}
}
