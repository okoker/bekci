package api

import (
	"io/fs"
	"net/http"

	"github.com/bekci/internal/auth"
	"github.com/bekci/internal/scheduler"
	"github.com/bekci/internal/store"
)

type Server struct {
	store     *store.Store
	auth      *auth.Service
	scheduler *scheduler.Scheduler
	version   string
	spa       fs.FS // embedded frontend/dist
}

// New creates a new API server.
func New(st *store.Store, authSvc *auth.Service, sched *scheduler.Scheduler, version string, spa fs.FS) *Server {
	return &Server{
		store:     st,
		auth:      authSvc,
		scheduler: sched,
		version:   version,
		spa:       spa,
	}
}

// Handler returns the root HTTP handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Public API routes
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("GET /api/health", s.handleHealth)

	// Authenticated routes — self-service
	mux.Handle("POST /api/logout", s.requireAuth(http.HandlerFunc(s.handleLogout)))
	mux.Handle("GET /api/me", s.requireAuth(http.HandlerFunc(s.handleGetMe)))
	mux.Handle("PUT /api/me", s.requireAuth(http.HandlerFunc(s.handleUpdateMe)))
	mux.Handle("PUT /api/me/password", s.requireAuth(http.HandlerFunc(s.handleChangePassword)))

	// Settings — any authenticated user can view, admin can update
	mux.Handle("GET /api/settings", s.requireAuth(http.HandlerFunc(s.handleGetSettings)))
	mux.Handle("PUT /api/settings", s.requireAuth(requireRole("admin")(http.HandlerFunc(s.handleUpdateSettings))))

	// User management — admin only
	adminAuth := func(h http.HandlerFunc) http.Handler {
		return s.requireAuth(requireRole("admin")(http.HandlerFunc(h)))
	}
	mux.Handle("GET /api/users", adminAuth(s.handleListUsers))
	mux.Handle("POST /api/users", adminAuth(s.handleCreateUser))
	mux.Handle("GET /api/users/{id}", adminAuth(s.handleGetUser))
	mux.Handle("PUT /api/users/{id}", adminAuth(s.handleUpdateUser))
	mux.Handle("PUT /api/users/{id}/suspend", adminAuth(s.handleSuspendUser))
	mux.Handle("PUT /api/users/{id}/password", adminAuth(s.handleResetPassword))

	// Auth helpers for monitoring routes
	anyAuth := func(h http.HandlerFunc) http.Handler {
		return s.requireAuth(http.HandlerFunc(h))
	}
	opAuth := func(h http.HandlerFunc) http.Handler {
		return s.requireAuth(requireRole("admin", "operator")(http.HandlerFunc(h)))
	}

	// Targets
	mux.Handle("GET /api/targets", anyAuth(s.handleListTargets))
	mux.Handle("POST /api/targets", opAuth(s.handleCreateTarget))
	mux.Handle("GET /api/targets/{id}", anyAuth(s.handleGetTarget))
	mux.Handle("PUT /api/targets/{id}", opAuth(s.handleUpdateTarget))
	mux.Handle("DELETE /api/targets/{id}", opAuth(s.handleDeleteTarget))

	// Checks
	mux.Handle("GET /api/targets/{id}/checks", anyAuth(s.handleListChecks))
	mux.Handle("POST /api/targets/{id}/checks", opAuth(s.handleCreateCheck))
	mux.Handle("PUT /api/checks/{id}", opAuth(s.handleUpdateCheck))
	mux.Handle("DELETE /api/checks/{id}", opAuth(s.handleDeleteCheck))
	mux.Handle("POST /api/checks/{id}/run", opAuth(s.handleRunCheckNow))
	mux.Handle("GET /api/checks/{id}/results", anyAuth(s.handleCheckResults))

	// Dashboard
	mux.Handle("GET /api/dashboard/status", anyAuth(s.handleDashboardStatus))
	mux.Handle("GET /api/dashboard/history/{checkId}", anyAuth(s.handleCheckHistory))

	// SOC — conditional auth based on soc_public setting
	mux.Handle("GET /api/soc/status", s.socAuth(http.HandlerFunc(s.handleSocStatus)))
	mux.Handle("GET /api/soc/history/{checkId}", s.socAuth(http.HandlerFunc(s.handleSocHistory)))

	// SPA handler — serve frontend for all non-API routes
	mux.Handle("/", s.spaHandler())

	// Wrap everything with CORS + logging
	return loggingMiddleware(corsMiddleware(mux))
}

// spaHandler serves the embedded Vue SPA with index.html fallback.
func (s *Server) spaHandler() http.Handler {
	if s.spa == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusNotFound, "frontend not built")
		})
	}

	fileServer := http.FileServer(http.FS(s.spa))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to serve static file
		path := r.URL.Path
		if path == "/" {
			path = "index.html"
		} else {
			path = path[1:] // strip leading /
		}

		// Check if file exists
		f, err := s.spa.Open(path)
		if err != nil {
			// File not found — serve index.html for Vue Router
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		f.Close()

		fileServer.ServeHTTP(w, r)
	})
}
