package api

import (
	"io/fs"
	"net/http"
	"sync"
	"time"

	"github.com/bekci/internal/alerter"
	"github.com/bekci/internal/auth"
	"github.com/bekci/internal/scheduler"
	"github.com/bekci/internal/store"
)

type cachedSetting struct {
	mu    sync.Mutex
	value string
	until time.Time
}

type Server struct {
	store          *store.Store
	auth           *auth.Service
	scheduler      *scheduler.Scheduler
	alerter        *alerter.AlertService
	version        string
	spa            fs.FS // embedded frontend/dist
	corsOrigin     string
	dbPath         string
	loginLimiter   *loginLimiter
	socPublicCache cachedSetting
}

// New creates a new API server.
func New(st *store.Store, authSvc *auth.Service, sched *scheduler.Scheduler, alertSvc *alerter.AlertService, version string, spa fs.FS, corsOrigin string, dbPath string) *Server {
	return &Server{
		store:        st,
		auth:         authSvc,
		scheduler:    sched,
		alerter:      alertSvc,
		version:      version,
		spa:          spa,
		corsOrigin:   corsOrigin,
		dbPath:       dbPath,
		loginLimiter: newLoginLimiter(),
	}
}

// Close releases resources held by the server (e.g. background goroutines).
func (s *Server) Close() {
	s.loginLimiter.Stop()
}

// cachedSocPublic returns the soc_public setting value, using a 30s cache.
func (s *Server) cachedSocPublic() string {
	s.socPublicCache.mu.Lock()
	defer s.socPublicCache.mu.Unlock()
	if time.Now().Before(s.socPublicCache.until) {
		return s.socPublicCache.value
	}
	val, _ := s.store.GetSetting("soc_public")
	s.socPublicCache.value = val
	s.socPublicCache.until = time.Now().Add(30 * time.Second)
	return val
}

// invalidateSocPublicCache forces the next socAuth call to re-read from DB.
func (s *Server) invalidateSocPublicCache() {
	s.socPublicCache.mu.Lock()
	s.socPublicCache.until = time.Time{}
	s.socPublicCache.mu.Unlock()
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

	// User management — admin only (except list, which operators need for recipient picker)
	adminAuth := func(h http.HandlerFunc) http.Handler {
		return s.requireAuth(requireRole("admin")(http.HandlerFunc(h)))
	}
	mux.Handle("POST /api/users", adminAuth(s.handleCreateUser))
	mux.Handle("GET /api/users/{id}", adminAuth(s.handleGetUser))
	mux.Handle("PUT /api/users/{id}", adminAuth(s.handleUpdateUser))
	mux.Handle("PUT /api/users/{id}/suspend", adminAuth(s.handleSuspendUser))
	mux.Handle("PUT /api/users/{id}/password", adminAuth(s.handleResetPassword))

	// Backup & Restore — admin only
	mux.Handle("GET /api/backup", adminAuth(s.handleBackup))
	mux.Handle("POST /api/backup/restore", adminAuth(s.handleRestore))

	// Fail2Ban status — admin only
	mux.Handle("GET /api/fail2ban/status", adminAuth(s.handleFail2BanStatus))

	// Auth helpers for monitoring routes
	anyAuth := func(h http.HandlerFunc) http.Handler {
		return s.requireAuth(http.HandlerFunc(h))
	}
	opAuth := func(h http.HandlerFunc) http.Handler {
		return s.requireAuth(requireRole("admin", "operator")(http.HandlerFunc(h)))
	}

	// User list — operators need this for recipient picker
	mux.Handle("GET /api/users", opAuth(s.handleListUsers))

	// System health
	mux.Handle("GET /api/system/health", anyAuth(s.handleSystemHealth))

	// Audit log
	mux.Handle("GET /api/audit-log", opAuth(s.handleListAuditLogs))

	// Targets
	mux.Handle("GET /api/targets", anyAuth(s.handleListTargets))
	mux.Handle("POST /api/targets", opAuth(s.handleCreateTarget))
	mux.Handle("GET /api/targets/{id}", anyAuth(s.handleGetTarget))
	mux.Handle("PUT /api/targets/{id}", opAuth(s.handleUpdateTarget))
	mux.Handle("DELETE /api/targets/{id}", opAuth(s.handleDeleteTarget))

	// Target alert recipients
	mux.Handle("GET /api/targets/{id}/recipients", anyAuth(s.handleListRecipients))
	mux.Handle("PUT /api/targets/{id}/recipients", opAuth(s.handleSetRecipients))

	// Checks (read-only + run)
	mux.Handle("GET /api/targets/{id}/checks", anyAuth(s.handleListChecks))
	mux.Handle("POST /api/checks/{id}/run", opAuth(s.handleRunCheckNow))
	mux.Handle("GET /api/checks/{id}/results", anyAuth(s.handleCheckResults))

	// Dashboard
	mux.Handle("GET /api/dashboard/status", anyAuth(s.handleDashboardStatus))
	mux.Handle("GET /api/dashboard/history/{checkId}", anyAuth(s.handleCheckHistory))

	// Alerts
	mux.Handle("GET /api/alerts", anyAuth(s.handleListAlerts))
	mux.Handle("POST /api/settings/test-email", adminAuth(s.handleTestEmail))

	// SLA
	mux.Handle("GET /api/sla/history", anyAuth(s.handleSLAHistory))

	// SOC — conditional auth based on soc_public setting
	mux.Handle("GET /api/soc/status", s.socAuth(http.HandlerFunc(s.handleSocStatus)))
	mux.Handle("GET /api/soc/history/{checkId}", s.socAuth(http.HandlerFunc(s.handleCheckHistory)))

	// SPA handler — serve frontend for all non-API routes
	mux.Handle("/", s.spaHandler())

	// Wrap everything with CORS + logging
	return recoveryMiddleware(loggingMiddleware(corsMiddleware(s.corsOrigin)(mux)))
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
