package main

import (
	"context"
	"flag"
	"fmt"
	"log"
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

	// Initialize store
	db, err := store.New(cfg.Global.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
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
			log.Printf("Web server error: %v", err)
		}
	}()

	log.Printf("Bekci v%s started on port %d", version, cfg.Global.WebPort)

	// Wait for shutdown signal
	<-sigCh
	log.Println("Shutting down...")

	// Cancel context to stop scheduler
	cancel()

	// Shutdown web server
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Printf("Web server shutdown error: %v", err)
	}

	log.Println("Shutdown complete")
}
