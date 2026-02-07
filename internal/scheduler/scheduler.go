package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/bekci/internal/alerter"
	"github.com/bekci/internal/checker"
	"github.com/bekci/internal/config"
	"github.com/bekci/internal/restarter"
	"github.com/bekci/internal/store"
)

type Scheduler struct {
	cfg       *config.Config
	store     *store.Store
	checker   *checker.Checker
	restarter *restarter.Restarter
	alerter   *alerter.Alerter

	tickers map[string]*time.Ticker
	mu      sync.Mutex
}

func New(cfg *config.Config, s *store.Store, c *checker.Checker, r *restarter.Restarter, a *alerter.Alerter) *Scheduler {
	return &Scheduler{
		cfg:       cfg,
		store:     s,
		checker:   c,
		restarter: r,
		alerter:   a,
		tickers:   make(map[string]*time.Ticker),
	}
}

// Start begins checking all services
func (s *Scheduler) Start(ctx context.Context) {
	var wg sync.WaitGroup

	// Check all services immediately on startup
	slog.Info("Running initial health checks...")
	for _, proj := range s.cfg.Projects {
		for i := range proj.Services {
			svc := &proj.Services[i]
			s.checkService(proj.Name, svc)
		}
	}

	// Start per-service tickers
	for _, proj := range s.cfg.Projects {
		for i := range proj.Services {
			svc := &proj.Services[i]
			serviceKey := config.GetServiceKey(proj.Name, svc.Name)

			ticker := time.NewTicker(svc.CheckInterval)
			s.mu.Lock()
			s.tickers[serviceKey] = ticker
			s.mu.Unlock()

			wg.Add(1)
			go func(projName string, svc *config.Service, ticker *time.Ticker) {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						ticker.Stop()
						return
					case <-ticker.C:
						s.checkService(projName, svc)
					}
				}
			}(proj.Name, svc, ticker)

			slog.Info("Started monitoring", "service", serviceKey, "interval", svc.CheckInterval)
		}
	}

	slog.Info("Scheduler ready")

	// Wait for context cancellation
	<-ctx.Done()

	// Stop all tickers
	s.mu.Lock()
	for _, ticker := range s.tickers {
		ticker.Stop()
	}
	s.mu.Unlock()

	wg.Wait()
	slog.Info("Scheduler stopped")
}

// CheckAllNow triggers an immediate check of all services
func (s *Scheduler) CheckAllNow() {
	slog.Info("Manual check triggered")
	for _, proj := range s.cfg.Projects {
		for i := range proj.Services {
			svc := &proj.Services[i]
			go s.checkService(proj.Name, svc)
		}
	}
}

func (s *Scheduler) checkService(projectName string, svc *config.Service) {
	serviceKey := config.GetServiceKey(projectName, svc.Name)

	// Perform check
	result := s.checker.Check(svc)

	// Get previous state
	prevStatus, lastChange, failures, _ := s.store.GetServiceState(serviceKey)

	// Save result
	checkResult := &store.CheckResult{
		ServiceKey: serviceKey,
		Status:     result.Status,
		StatusCode: result.StatusCode,
		ResponseMs: result.ResponseMs,
		Error:      result.Error,
		CheckedAt:  time.Now(),
	}
	if err := s.store.SaveCheckResult(checkResult); err != nil {
		slog.Error("Error saving check result", "service", serviceKey, "error", err)
	}

	// Handle status transitions
	if result.Status == "down" {
		failures++

		// Log the failure
		slog.Warn("Service down", "service", serviceKey, "error", result.Error, "failures", failures)

		// Send alert on first failure (or after recovery)
		if prevStatus != "down" {
			if err := s.alerter.SendDownAlert(projectName, svc.Name, result.Error); err != nil {
				slog.Error("Error sending alert", "service", serviceKey, "error", err)
			}

			// Save alert record
			s.store.SaveAlert(&store.Alert{
				ServiceKey: serviceKey,
				AlertType:  "down",
				Message:    result.Error,
				SentAt:     time.Now(),
			})
		}

		// Attempt restart
		if svc.Restart.Enabled != nil && *svc.Restart.Enabled {
			slog.Warn("Attempting restart", "service", serviceKey)
			restartResult := s.restarter.Restart(svc, s.cfg.Global.RestartAttempts, s.cfg.Global.RestartDelay)
			if restartResult.Success {
				slog.Warn("Restart successful", "service", serviceKey, "output", restartResult.Output)
			} else {
				slog.Error("Restart failed", "service", serviceKey, "error", restartResult.Error)
			}
		}
	} else if result.Status == "up" {
		// Service is up
		if prevStatus == "down" {
			// Recovery!
			downtime := time.Since(lastChange)
			slog.Warn("Service recovered", "service", serviceKey, "downtime", downtime)

			// Send recovery alert
			if err := s.alerter.SendRecoveryAlert(projectName, svc.Name, downtime); err != nil {
				slog.Error("Error sending recovery alert", "service", serviceKey, "error", err)
			}

			// Save alert record
			s.store.SaveAlert(&store.Alert{
				ServiceKey: serviceKey,
				AlertType:  "recovery",
				Message:    "Service recovered",
				SentAt:     time.Now(),
			})

			// Clear cooldown so next failure triggers immediate alert
			s.alerter.ClearCooldown(projectName, svc.Name)
		}
		failures = 0
	}

	// Update service state
	if err := s.store.UpdateServiceState(serviceKey, result.Status, failures); err != nil {
		slog.Error("Error updating service state", "service", serviceKey, "error", err)
	}
}
