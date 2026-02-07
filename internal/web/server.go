package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/bekci/internal/config"
	"github.com/bekci/internal/scheduler"
	"github.com/bekci/internal/store"
)

//go:embed templates/*
var templatesFS embed.FS

type Server struct {
	port      int
	store     *store.Store
	config    *config.Config
	scheduler *scheduler.Scheduler
	server    *http.Server
	tmpl      *template.Template
}

func New(port int, s *store.Store, cfg *config.Config, sched *scheduler.Scheduler) *Server {
	srv := &Server{
		port:      port,
		store:     s,
		config:    cfg,
		scheduler: sched,
	}

	// Parse templates
	var err error
	srv.tmpl, err = template.New("").Funcs(template.FuncMap{
		"formatDate": formatDate,
		"uptimeColor": uptimeColor,
		"statusClass": statusClass,
	}).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		slog.Error("Could not parse templates", "error", err)
	}

	return srv
}

func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Routes
	mux.HandleFunc("/", s.handleStatus)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/status", s.handleAPIStatus)
	mux.HandleFunc("/api/check-now", s.handleCheckNow)

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	slog.Info("Web server starting", "port", s.port)
	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

func formatDate(t time.Time) string {
	return t.Format("02/01/2006")
}

func uptimeColor(pct float64) string {
	if pct < 0 {
		return "#e5e7eb" // gray - no data
	}
	if pct >= 100 {
		return "#22c55e" // green
	}
	if pct >= 95 {
		return "#eab308" // yellow
	}
	if pct >= 80 {
		return "#f97316" // orange
	}
	return "#ef4444" // red
}

func statusClass(status string) string {
	switch status {
	case "up":
		return "operational"
	case "down":
		return "down"
	default:
		return "unknown"
	}
}
