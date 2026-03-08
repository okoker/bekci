package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/bekci/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNewScheduler(t *testing.T) {
	st := newTestStore(t)
	s := New(st)
	if s == nil {
		t.Fatal("expected non-nil scheduler")
	}
	if len(s.timers) != 0 {
		t.Fatalf("expected empty timers map, got %d", len(s.timers))
	}
	if s.eventCh == nil {
		t.Fatal("expected non-nil eventCh")
	}
}

func TestRunNowChannel(t *testing.T) {
	st := newTestStore(t)
	s := New(st)
	s.RunNow("check-123")
	select {
	case id := <-s.eventCh:
		if id != "check-123" {
			t.Fatalf("got %s, want check-123", id)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestStartStop(t *testing.T) {
	st := newTestStore(t)
	s := New(st)
	s.Start(context.Background())
	time.Sleep(100 * time.Millisecond)
	s.Stop()
}

// --- Test helpers ---

type mockEvaluator struct {
	mu    sync.Mutex
	calls []string
}

func (m *mockEvaluator) Evaluate(checkID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, checkID)
}

func startHTTPServer(t *testing.T) (string, int) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	}))
	t.Cleanup(ts.Close)
	host, portStr, _ := net.SplitHostPort(ts.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)
	return host, port
}

func createHTTPTarget(t *testing.T, st *store.Store, host string, port int) string {
	t.Helper()
	tgt := &store.Target{
		Name: "http-test", Host: host,
		Enabled: true, Operator: "AND", Category: "Network",
	}
	config := fmt.Sprintf(`{"scheme":"http","port":%d,"endpoint":"/"}`, port)
	conds := []store.TargetCondition{{
		CheckType: "http", CheckName: "HTTP", Config: config,
		IntervalS: 300, Field: "status", Comparator: "eq", Value: "down",
		FailCount: 1, FailWindow: 0, GroupOperator: "AND",
	}}
	if err := st.CreateTargetWithConditions(tgt, conds, ""); err != nil {
		t.Fatal(err)
	}
	checks, err := st.ListChecksByTarget(tgt.ID)
	if err != nil || len(checks) == 0 {
		t.Fatal("no checks created")
	}
	return checks[0].ID
}

// --- Scheduler tests ---

// T-SC4: loadChecks only schedules enabled+unpaused targets
func TestLoadChecksFiltersDisabledAndPaused(t *testing.T) {
	st := newTestStore(t)
	s := New(st)

	conds := []store.TargetCondition{{
		CheckType: "http", CheckName: "HTTP", Config: "{}",
		IntervalS: 60, Field: "status", Comparator: "eq", Value: "down",
		FailCount: 1, FailWindow: 0, GroupOperator: "AND",
	}}

	// Target 1: enabled + unpaused → should be scheduled
	tgt1 := &store.Target{
		Name: "active", Host: "active.example.com",
		Enabled: true, Operator: "AND", Category: "Network",
	}
	if err := st.CreateTargetWithConditions(tgt1, conds, ""); err != nil {
		t.Fatal(err)
	}

	// Target 2: disabled → should NOT be scheduled
	tgt2 := &store.Target{
		Name: "disabled", Host: "disabled.example.com",
		Enabled: false, Operator: "AND", Category: "Network",
	}
	if err := st.CreateTargetWithConditions(tgt2, conds, ""); err != nil {
		t.Fatal(err)
	}

	// Target 3: paused → should NOT be scheduled
	tgt3 := &store.Target{
		Name: "paused", Host: "paused.example.com",
		Enabled: true, Operator: "AND", Category: "Network",
	}
	if err := st.CreateTargetWithConditions(tgt3, conds, ""); err != nil {
		t.Fatal(err)
	}
	if err := st.PauseTarget(tgt3.ID); err != nil {
		t.Fatal(err)
	}

	s.loadChecks()

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.timers) != 1 {
		t.Fatalf("expected 1 timer (active only), got %d", len(s.timers))
	}
}

// T-SC5: loadChecks removes timers for targets that become inactive
func TestLoadChecksRemovesStaleTimers(t *testing.T) {
	st := newTestStore(t)
	s := New(st)

	tgt := &store.Target{
		Name: "stale", Host: "stale.example.com",
		Enabled: true, Operator: "AND", Category: "Network",
	}
	conds := []store.TargetCondition{{
		CheckType: "http", CheckName: "HTTP", Config: "{}",
		IntervalS: 60, Field: "status", Comparator: "eq", Value: "down",
		FailCount: 1, FailWindow: 0, GroupOperator: "AND",
	}}
	if err := st.CreateTargetWithConditions(tgt, conds, ""); err != nil {
		t.Fatal(err)
	}

	s.loadChecks()
	s.mu.Lock()
	if len(s.timers) != 1 {
		s.mu.Unlock()
		t.Fatalf("expected 1 timer, got %d", len(s.timers))
	}
	s.mu.Unlock()

	// Pause target → loadChecks should remove stale timer
	if err := st.PauseTarget(tgt.ID); err != nil {
		t.Fatal(err)
	}

	s.loadChecks()

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.timers) != 0 {
		t.Fatalf("expected 0 timers after pause, got %d", len(s.timers))
	}
}

// T-SC6: runCheck saves result to DB
func TestRunCheckSavesResult(t *testing.T) {
	st := newTestStore(t)
	s := New(st)
	host, port := startHTTPServer(t)
	checkID := createHTTPTarget(t, st, host, port)

	s.runCheck(checkID)

	result, err := st.GetLastResult(checkID)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected result to be saved")
	}
	if result.Status != "up" {
		t.Fatalf("expected status up, got %s", result.Status)
	}
}

// T-SC7: runCheck calls engine.Evaluate after saving result
func TestRunCheckCallsEngine(t *testing.T) {
	st := newTestStore(t)
	s := New(st)
	me := &mockEvaluator{}
	s.SetEngine(me)
	host, port := startHTTPServer(t)
	checkID := createHTTPTarget(t, st, host, port)

	s.runCheck(checkID)
	time.Sleep(100 * time.Millisecond) // wait for async goroutine

	me.mu.Lock()
	defer me.mu.Unlock()
	if len(me.calls) != 1 {
		t.Fatalf("expected 1 Evaluate call, got %d", len(me.calls))
	}
	if me.calls[0] != checkID {
		t.Fatalf("expected Evaluate(%s), got Evaluate(%s)", checkID, me.calls[0])
	}
}

// T-SC8: runCheck auto-captures baseline hash for page_hash checks
func TestRunCheckBaselineHash(t *testing.T) {
	st := newTestStore(t)
	s := New(st)
	host, port := startHTTPServer(t)

	tgt := &store.Target{
		Name: "hash-test", Host: host,
		Enabled: true, Operator: "AND", Category: "Network",
	}
	config := fmt.Sprintf(`{"scheme":"http","port":%d,"endpoint":"/"}`, port)
	conds := []store.TargetCondition{{
		CheckType: "page_hash", CheckName: "PageHash", Config: config,
		IntervalS: 300, Field: "status", Comparator: "eq", Value: "down",
		FailCount: 1, FailWindow: 0, GroupOperator: "AND",
	}}
	if err := st.CreateTargetWithConditions(tgt, conds, ""); err != nil {
		t.Fatal(err)
	}
	checks, err := st.ListChecksByTarget(tgt.ID)
	if err != nil || len(checks) == 0 {
		t.Fatal("no checks created")
	}
	checkID := checks[0].ID

	s.runCheck(checkID)

	// Verify baseline_hash was persisted in check config
	check, err := st.GetCheck(checkID)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(check.Config), &cfg); err != nil {
		t.Fatal(err)
	}
	hash, ok := cfg["baseline_hash"].(string)
	if !ok || hash == "" {
		t.Fatalf("expected baseline_hash in config, got %v", cfg)
	}
}
