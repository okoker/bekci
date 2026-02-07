package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/bekci/internal/alerter"
	"github.com/bekci/internal/checker"
	"github.com/bekci/internal/config"
	"github.com/bekci/internal/restarter"
	"github.com/bekci/internal/scheduler"
	"github.com/bekci/internal/store"
	"github.com/bekci/internal/web"
)

var version = "dev"

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

	// Detect run mode: if parent is PID 1 (launchd), we're running as a service
	logPath := cfg.Global.LogPath
	if os.Getppid() == 1 {
		logPath = "/var/log/bekci.log"
	}

	// Setup logging
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file %s: %v", logPath, err)
	}
	defer logFile.Close()

	level := config.ParseLogLevel(cfg.Global.LogLevel)
	writer := io.MultiWriter(os.Stderr, logFile)
	handler := slog.NewTextHandler(writer, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))

	// Initialize store
	db, err := store.New(cfg.Global.DBPath)
	if err != nil {
		slog.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Start purge routine for old entries
	go db.StartPurgeRoutine(cfg.Global.HistoryDays)

	// Initialize alerter
	alert := alerter.New(cfg.Resend, cfg.Global.AlertCooldown)

	// Initialize checker registry
	chk := checker.New(cfg.SSHDefaults)

	// Initialize restarter registry
	rst := restarter.New(cfg.SSHDefaults)

	// Create scheduler
	sched := scheduler.New(cfg, db, chk, rst, alert)

	// Create web server
	srv := web.New(cfg.Global.WebPort, db, cfg, sched)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start scheduler
	go sched.Start(ctx)

	// Start web server
	go func() {
		if err := srv.Start(); err != nil {
			slog.Error("Web server error", "error", err)
		}
	}()

	slog.Warn("Bekci started", "version", version, "port", cfg.Global.WebPort)

	// Wait for shutdown signal
	<-sigCh
	slog.Warn("Shutting down...")

	// Cancel context to stop scheduler
	cancel()

	// Shutdown web server
	if err := srv.Shutdown(context.Background()); err != nil {
		slog.Error("Web server shutdown error", "error", err)
	}

	slog.Warn("Shutdown complete")
}
