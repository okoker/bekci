package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bekci/internal/auth"
)

type contextKey string

const (
	ctxClaims contextKey = "claims"
)

// getClaims extracts auth claims from request context.
func getClaims(r *http.Request) *auth.Claims {
	if c, ok := r.Context().Value(ctxClaims).(*auth.Claims); ok {
		return c
	}
	return nil
}

// requireAuth validates the JWT Bearer token and adds claims to context.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing or invalid authorization header")
			return
		}
		tokenStr := strings.TrimPrefix(header, "Bearer ")

		claims, err := s.auth.ValidateToken(tokenStr)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		// Check user is still active
		user, err := s.store.GetUserByID(claims.Subject)
		if err != nil || user == nil || user.Status != "active" {
			writeError(w, http.StatusUnauthorized, "account not active")
			return
		}

		ctx := context.WithValue(r.Context(), ctxClaims, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireRole checks that the authenticated user has the required role.
func requireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := getClaims(r)
			if claims == nil {
				writeError(w, http.StatusUnauthorized, "not authenticated")
				return
			}
			for _, role := range roles {
				if claims.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}
			writeError(w, http.StatusForbidden, "insufficient permissions")
		})
	}
}

// loggingMiddleware logs HTTP requests.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		slog.Debug("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration", time.Since(start).String(),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// corsMiddleware adds CORS headers for API routes.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
