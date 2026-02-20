package scheduler

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/bekci/internal/checker"
	"github.com/bekci/internal/store"
)

// RuleEvaluator evaluates rules after a check result is saved.
type RuleEvaluator interface {
	Evaluate(checkID string)
}

type Scheduler struct {
	store     *store.Store
	engine    RuleEvaluator
	timers    map[string]*time.Timer   // check_id → timer
	intervals map[string]time.Duration // check_id → current interval
	checkMu   map[string]*sync.Mutex   // per-check mutex to prevent concurrent runs
	mu        sync.Mutex
	eventCh   chan string // check_id for immediate run
	ctx       context.Context
	cancel    context.CancelFunc
}

// SetEngine sets the rule evaluator called after each check result.
func (s *Scheduler) SetEngine(e RuleEvaluator) {
	s.engine = e
}

func New(st *store.Store) *Scheduler {
	return &Scheduler{
		store:     st,
		timers:    make(map[string]*time.Timer),
		intervals: make(map[string]time.Duration),
		checkMu:   make(map[string]*sync.Mutex),
		eventCh:   make(chan string, 100),
	}
}

// Start loads all enabled checks and begins scheduling.
func (s *Scheduler) Start(ctx context.Context) {
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.loadChecks()

	// Safety-net poll: every 60s, reload checks
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				s.loadChecks()
			}
		}
	}()

	// Event channel: immediate runs
	go func() {
		for {
			select {
			case <-s.ctx.Done():
				return
			case checkID := <-s.eventCh:
				go s.runCheck(checkID)
			}
		}
	}()

	slog.Info("Scheduler started")
}

// Stop cancels all timers and the scheduler context.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, t := range s.timers {
		t.Stop()
		delete(s.timers, id)
		delete(s.intervals, id)
		delete(s.checkMu, id)
	}
	slog.Info("Scheduler stopped")
}

// Reload re-reads the DB and updates timers.
func (s *Scheduler) Reload() {
	s.loadChecks()
}

// RunNow sends a check ID to the event channel for immediate execution.
func (s *Scheduler) RunNow(checkID string) {
	select {
	case s.eventCh <- checkID:
	default:
		slog.Warn("RunNow: event channel full, dropping", "check_id", checkID)
	}
}

func (s *Scheduler) loadChecks() {
	checks, err := s.store.ListAllEnabledChecks()
	if err != nil {
		slog.Error("Scheduler: failed to load checks", "error", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Build set of active check IDs
	activeIDs := make(map[string]bool)
	for _, ec := range checks {
		activeIDs[ec.ID] = true
	}

	// Remove timers for checks no longer active
	for id, t := range s.timers {
		if !activeIDs[id] {
			t.Stop()
			delete(s.timers, id)
			delete(s.intervals, id)
			slog.Debug("Scheduler: removed check", "check_id", id)
		}
	}

	// Add new checks or reschedule if interval changed
	for _, ec := range checks {
		interval := time.Duration(ec.IntervalS) * time.Second
		if interval < 10*time.Second {
			interval = 10 * time.Second
		}
		if _, exists := s.timers[ec.ID]; !exists {
			s.scheduleCheck(ec)
		} else if stored := s.intervals[ec.ID]; stored != interval {
			s.timers[ec.ID].Stop()
			delete(s.timers, ec.ID)
			delete(s.intervals, ec.ID)
			s.scheduleCheck(ec)
			slog.Info("Scheduler: interval changed, rescheduled",
				"check_id", ec.ID, "old", stored, "new", interval)
		}
	}
}

func (s *Scheduler) scheduleCheck(ec store.EnabledCheck) {
	// Ensure per-check mutex exists
	if _, ok := s.checkMu[ec.ID]; !ok {
		s.checkMu[ec.ID] = &sync.Mutex{}
	}

	interval := time.Duration(ec.IntervalS) * time.Second
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}

	// Schedule first run after a short delay (stagger checks)
	checkID := ec.ID
	timer := time.AfterFunc(5*time.Second, func() {
		s.runAndReschedule(checkID, interval)
	})
	s.timers[checkID] = timer
	s.intervals[checkID] = interval
	slog.Debug("Scheduler: scheduled check", "check_id", checkID, "interval", interval)
}

func (s *Scheduler) runAndReschedule(checkID string, interval time.Duration) {
	if s.ctx.Err() != nil {
		return
	}
	s.runCheck(checkID)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return
	}
	timer := time.AfterFunc(interval, func() {
		s.runAndReschedule(checkID, interval)
	})
	s.timers[checkID] = timer
}

func (s *Scheduler) runCheck(checkID string) {
	// Per-check mutex prevents concurrent runs
	s.mu.Lock()
	mu, ok := s.checkMu[checkID]
	s.mu.Unlock()
	if !ok {
		mu = &sync.Mutex{}
		s.mu.Lock()
		s.checkMu[checkID] = mu
		s.mu.Unlock()
	}
	if !mu.TryLock() {
		return // already running
	}
	defer mu.Unlock()

	// Load check + target from DB
	check, err := s.store.GetCheck(checkID)
	if err != nil || check == nil {
		slog.Debug("Scheduler: check not found", "check_id", checkID)
		return
	}

	target, err := s.store.GetTarget(check.TargetID)
	if err != nil || target == nil {
		slog.Debug("Scheduler: target not found", "target_id", check.TargetID)
		return
	}

	// Run the check
	result := checker.Run(check.Type, target.Host, check.Config)

	// Serialize metrics
	metricsJSON, err := json.Marshal(result.Metrics)
	if err != nil {
		slog.Error("Scheduler: failed to marshal metrics", "check_id", checkID, "error", err)
	}

	// Save result
	cr := &store.CheckResult{
		CheckID:    checkID,
		Status:     result.Status,
		ResponseMs: result.ResponseMs,
		Message:    result.Message,
		Metrics:    string(metricsJSON),
		CheckedAt:  time.Now(),
	}
	if err := s.store.SaveResult(cr); err != nil {
		slog.Error("Scheduler: failed to save result", "check_id", checkID, "error", err)
	} else if s.engine != nil {
		go s.engine.Evaluate(checkID)
	}

	slog.Debug("Check completed", "check_id", checkID, "type", check.Type, "status", result.Status,
		"response_ms", result.ResponseMs)
}
