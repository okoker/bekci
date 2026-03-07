package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/bekci/internal/auth"
)

type contextKey string

const (
	ctxClaims  contextKey = "claims"
	ctxUserRef contextKey = "userRef"
)

// userRef is a mutable holder so outer middleware (logging) can read user info
// set by inner middleware (auth). Go contexts are immutable, but a pointer stored
// in the original context lets inner layers write back to the outer scope.
type userRef struct {
	ID string
}

// getClaims extracts auth claims from request context.
func getClaims(r *http.Request) *auth.Claims {
	if c, ok := r.Context().Value(ctxClaims).(*auth.Claims); ok {
		return c
	}
	return nil
}

// requireAuth validates the JWT from the HttpOnly cookie and adds claims to context.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("token")
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		tokenStr := cookie.Value

		claims, err := s.auth.ValidateToken(tokenStr)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		// Check user is still active and revalidate role from DB
		user, err := s.store.GetUserByID(claims.Subject)
		if err != nil || user == nil || user.Status != "active" {
			writeError(w, http.StatusUnauthorized, "account not active")
			return
		}
		claims.Role = user.Role

		// Write user ID back to the mutable holder so outer middleware can read it
		if ref, ok := r.Context().Value(ctxUserRef).(*userRef); ok {
			ref.ID = claims.Subject
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

// socAuth conditionally requires auth based on the soc_public setting.
// If soc_public is "true", allow anonymous access; otherwise require Bearer token.
func (s *Server) socAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cachedSocPublic() == "true" {
			next.ServeHTTP(w, r)
			return
		}
		// Fall through to standard auth
		s.requireAuth(next).ServeHTTP(w, r)
	})
}

// recoveryMiddleware catches panics so a single bad request cannot kill the process.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("Panic recovered", "error", err, "method", r.Method, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs HTTP requests.
// All requests are logged at DEBUG level. 5xx responses are also logged at WARN
// with IP and user context to aid production troubleshooting.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ref := &userRef{}
		r = r.WithContext(context.WithValue(r.Context(), ctxUserRef, ref))
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		duration := time.Since(start).String()

		slog.Debug("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration", duration,
		)

		if sw.status >= 500 {
			slog.Warn("HTTP 5xx",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"duration", duration,
				"ip", clientIP(r),
				"user_id", ref.ID,
			)
		}
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

// corsMiddleware adds CORS headers only when a specific origin is configured.
// When origin is empty (production with embedded SPA), no CORS headers are sent.
func corsMiddleware(origin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if origin == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Credentials", "true")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
