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
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/google/uuid"

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

	// Initialize auth service
	authSvc := auth.New(db, cfg.Auth.JWTSecret)

	// Initialize scheduler + rules engine
	sched := scheduler.New(db)
	eng := engine.New(db)
	sched.SetEngine(eng)

	// Get embedded frontend (may be empty during dev)
	var spa fs.FS
	if sub, err := fs.Sub(frontendFS, "frontend_dist"); err == nil {
		if _, err := fs.Stat(sub, "index.html"); err == nil {
			spa = sub
		}
	}

	// Create API server
	apiServer := api.New(db, authSvc, sched, version, spa, cfg.Server.CORSOrigin)

	// Setup HTTP server
	httpServer := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
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
				historyDays := 90
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

	// Daily cleanup: audit log rotation
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
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
			}
		}
	}()

	// Start HTTP server
	go func() {
		slog.Warn("Bekci started", "version", version, "addr", fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port))
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
	sched.Stop()
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}
	slog.Warn("Shutdown complete")
}
