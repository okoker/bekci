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
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/bekci/internal/api"
	"github.com/bekci/internal/auth"
	"github.com/bekci/internal/config"
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
		if cfg.InitAdmin.Password == "" {
			slog.Error("No users exist and init_admin.password is empty. Set BEKCI_ADMIN_PASSWORD env var or init_admin.password in config.")
			os.Exit(1)
		}
		if len(cfg.InitAdmin.Password) < 8 {
			slog.Error("init_admin.password must be at least 8 characters")
			os.Exit(1)
		}
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
		slog.Warn("Created initial admin user", "username", cfg.InitAdmin.Username)
	}

	// Initialize auth service
	authSvc := auth.New(db, cfg.Auth.JWTSecret)

	// Get embedded frontend (may be empty during dev)
	var spa fs.FS
	if sub, err := fs.Sub(frontendFS, "frontend_dist"); err == nil {
		if _, err := fs.Stat(sub, "index.html"); err == nil {
			spa = sub
		}
	}

	// Create API server
	apiServer := api.New(db, authSvc, version, spa)

	// Setup HTTP server
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      apiServer.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Hourly session cleanup
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
			}
		}
	}()

	// Start HTTP server
	go func() {
		slog.Warn("Bekci started", "version", version, "port", cfg.Server.Port)
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
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}
	slog.Warn("Shutdown complete")
}
