package scheduler

import (
	"context"
	"log"
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
	log.Println("Running initial health checks...")
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

			log.Printf("Started monitoring %s (interval: %v)", serviceKey, svc.CheckInterval)
		}
	}

	log.Println("Scheduler ready")

	// Wait for context cancellation
	<-ctx.Done()

	// Stop all tickers
	s.mu.Lock()
	for _, ticker := range s.tickers {
		ticker.Stop()
	}
	s.mu.Unlock()

	wg.Wait()
	log.Println("Scheduler stopped")
}

// CheckAllNow triggers an immediate check of all services
func (s *Scheduler) CheckAllNow() {
	log.Println("Manual check triggered")
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
		log.Printf("Error saving check result for %s: %v", serviceKey, err)
	}

	// Handle status transitions
	if result.Status == "down" {
		failures++

		// Log the failure
		log.Printf("[DOWN] %s: %s (failure #%d)", serviceKey, result.Error, failures)

		// Send alert on first failure (or after recovery)
		if prevStatus != "down" {
			if err := s.alerter.SendDownAlert(projectName, svc.Name, result.Error); err != nil {
				log.Printf("Error sending alert for %s: %v", serviceKey, err)
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
		if svc.Restart.Type != "none" && svc.Restart.Type != "" {
			log.Printf("Attempting restart for %s...", serviceKey)
			restartResult := s.restarter.Restart(svc, s.cfg.Global.RestartAttempts, s.cfg.Global.RestartDelay)
			if restartResult.Success {
				log.Printf("Restart successful for %s: %s", serviceKey, restartResult.Output)
			} else {
				log.Printf("Restart failed for %s: %s", serviceKey, restartResult.Error)
			}
		}
	} else if result.Status == "up" {
		// Service is up
		if prevStatus == "down" {
			// Recovery!
			downtime := time.Since(lastChange)
			log.Printf("[RECOVERY] %s is back up after %v", serviceKey, downtime)

			// Send recovery alert
			if err := s.alerter.SendRecoveryAlert(projectName, svc.Name, downtime); err != nil {
				log.Printf("Error sending recovery alert for %s: %v", serviceKey, err)
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
		log.Printf("Error updating service state for %s: %v", serviceKey, err)
	}
}
