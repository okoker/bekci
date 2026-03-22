package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/bekci/internal/alerter"
	"github.com/bekci/internal/api"
	"github.com/bekci/internal/auth"
	"github.com/bekci/internal/config"
	"github.com/bekci/internal/engine"
	"github.com/bekci/internal/scheduler"
	"github.com/bekci/internal/store"
)

var version = "2.0.0-dev"

//go:embed all:frontend_dist
var frontendFS embed.FS

func main() {
	// Subcommand detection — must run before flag.Parse()
	if len(os.Args) > 1 && os.Args[1] == "restore-full" {
		runRestoreFull()
		return
	}

	var (
		configPath  = flag.String("config", "config.yaml", "path to config file")
		showVersion = flag.Bool("version", false, "show version")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("bekci version %s\n", version)
		os.Exit(0)
	}

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Setup logging
	logFile, err := os.OpenFile(cfg.Logging.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		log.Fatalf("Failed to open log file %s: %v", cfg.Logging.Path, err)
	}
	defer logFile.Close()

	level := config.ParseLogLevel(cfg.Logging.Level)
	writer := io.MultiWriter(os.Stderr, logFile)
	handler := slog.NewTextHandler(writer, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))

	// Initialize store
	db, err := store.New(cfg.Server.DBPath)
	if err != nil {
		slog.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// First-boot: create initial admin if no users exist
	count, err := db.CountUsers()
	if err != nil {
		slog.Error("Failed to count users", "error", err)
		os.Exit(1)
	}
	if count == 0 {
		hash, err := auth.HashPassword(cfg.InitAdmin.Password)
		if err != nil {
			slog.Error("Failed to hash admin password", "error", err)
			os.Exit(1)
		}
		now := time.Now()
		admin := &store.User{
			ID:           uuid.New().String(),
			Username:     cfg.InitAdmin.Username,
			Email:        "",
			PasswordHash: hash,
			Role:         "admin",
			Status:       "active",
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := db.CreateUser(admin); err != nil {
			slog.Error("Failed to create initial admin", "error", err)
			os.Exit(1)
		}
		slog.Warn("Created initial admin user — change the password after first login",
			"username", cfg.InitAdmin.Username)
	}

	// Check shutdown marker — detect unclean restarts
	shutdownMarker := filepath.Join(filepath.Dir(cfg.Server.DBPath), ".shutdown_clean")
	if _, err := os.Stat(shutdownMarker); err == nil {
		// Marker exists — clean restart
		os.Remove(shutdownMarker)
	} else if os.IsNotExist(err) {
		// No marker — unclean restart (only log if DB already existed, not first boot)
		if count > 0 {
			slog.Warn("UNCLEAN RESTART DETECTED — Bekci was not shut down cleanly")
			entries := []string{
				"SYSTEM RESTART DETECTED — Bekci was not shut down cleanly",
				"Previous instance may have crashed or been killed",
				"Check system logs (journalctl -u bekci) for details",
				"Dashboard data during the outage period may be incomplete",
			}
			for _, detail := range entries {
				db.CreateAuditEntry(&store.AuditEntry{
					UserID:       "system",
					Username:     "system",
					Action:       "unclean_restart",
					ResourceType: "system",
					Detail:       detail,
					Status:       "failure",
				})
			}
			// Send system alert to configured recipients
			go alerter.SendSystemAlert(db,
				"Bekci restarted unexpectedly",
				"Previous instance did not shut down cleanly. Check audit log for details.",
			)
		}
	} else {
		// Unexpected error (permission denied, etc.) — log but don't assume unclean
		slog.Warn("Could not check shutdown marker", "path", shutdownMarker, "error", err)
	}

	// Initialize auth service
	authSvc := auth.New(db, cfg.Auth.JWTSecret)

	// Initialize scheduler + rules engine + alerter
	sched := scheduler.New(db)
	eng := engine.New(db)
	alertSvc := alerter.New(db)
	eng.SetDispatcher(alertSvc)
	sched.SetEngine(eng)

	// Get embedded frontend (may be empty during dev)
	var spa fs.FS
	if sub, err := fs.Sub(frontendFS, "frontend_dist"); err == nil {
		if _, err := fs.Stat(sub, "index.html"); err == nil {
			spa = sub
		}
	}

	// Create API server
	apiServer := api.New(db, authSvc, sched, alertSvc, version, spa, cfg.Server.CORSOrigin, cfg.Server.DBPath, *configPath, cfg.Server.BackupDir)

	// Setup HTTP server
	httpServer := &http.Server{
		Addr:         net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port)),
		Handler:      apiServer.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start scheduler
	go sched.Start(ctx)

	// Scheduler stale monitor — independent goroutine checks heartbeat every 60s
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				lt := sched.LastTick()
				if lt.IsZero() {
					continue
				}
				if time.Since(lt) > 120*time.Second {
					staleSec := int(time.Since(lt).Seconds())
					slog.Warn("Scheduler stale", "last_tick_seconds_ago", staleSec)
					db.CreateAuditEntry(&store.AuditEntry{
						UserID:       "system",
						Username:     "system",
						Action:       "scheduler_stale",
						ResourceType: "system",
						Detail:       fmt.Sprintf("Scheduler stale — no tick for %ds", staleSec),
						Status:       "failure",
					})
				}
			}
		}
	}()

	// Hourly cleanup: sessions + old results
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if purged, err := db.PurgeExpiredSessions(); err != nil {
					slog.Error("Session cleanup error", "error", err)
				} else if purged > 0 {
					slog.Info("Purged expired sessions", "count", purged)
				}

				// Purge old check results based on history_days setting
				historyDays := 3
				if v, err := db.GetSetting("history_days"); err == nil && v != "" {
					if d, err := strconv.Atoi(v); err == nil && d > 0 {
						historyDays = d
					}
				}
				if purged, err := db.PurgeOldResults(historyDays); err != nil {
					slog.Error("Results cleanup error", "error", err)
				} else if purged > 0 {
					slog.Info("Purged old check results", "count", purged, "older_than_days", historyDays)
				}
			}
		}
	}()

	// Daily cleanup: audit log rotation (also runs once at startup)
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		purgeAuditAndAlerts := func() {
			retentionDays := 91
			if v, err := db.GetSetting("audit_retention_days"); err == nil && v != "" {
				if d, err := strconv.Atoi(v); err == nil && d > 0 {
					retentionDays = d
				}
			}
			if purged, err := db.PurgeOldAuditEntries(retentionDays); err != nil {
				slog.Error("Audit log cleanup error", "error", err)
			} else if purged > 0 {
				slog.Info("Purged old audit entries", "count", purged, "older_than_days", retentionDays)
			}

			// Purge old alert history (reuse audit_retention_days)
			if purged, err := db.PurgeOldAlertHistory(retentionDays); err != nil {
				slog.Error("Alert history cleanup error", "error", err)
			} else if purged > 0 {
				slog.Info("Purged old alert history", "count", purged, "older_than_days", retentionDays)
			}
		}

		purgeAuditAndAlerts() // run once at startup
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				purgeAuditAndAlerts()
			}
		}
	}()

	// Re-alert ticker: check for still-firing rules every 60s
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				alertSvc.CheckRealerts()
			}
		}
	}()

	// Start HTTP server
	go func() {
		slog.Warn("Bekci started",
			"version", version,
			"addr", fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
			"db", cfg.Server.DBPath,
			"log_file", cfg.Logging.Path,
			"log_level", cfg.Logging.Level,
		)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	slog.Warn("Shutting down...")

	// Write clean shutdown marker early — before HTTP drain which could hit Docker's stop timeout
	if err := os.WriteFile(shutdownMarker, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0600); err != nil {
		slog.Error("Failed to write shutdown marker", "error", err)
	}

	sched.Stop()
	apiServer.Close()
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}
	slog.Warn("Shutdown complete")
}
