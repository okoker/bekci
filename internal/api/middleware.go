package api

import (
	"compress/gzip"
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bekci/internal/auth"
)

type contextKey string

const (
	ctxClaims       contextKey = "claims"
	ctxUserRef      contextKey = "userRef"
	ctxAPITokenName contextKey = "apiTokenName"
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

// requireAPIToken validates an `Authorization: Bearer bk_…` header against
// the api_tokens table and attaches the token record to the request
// context. Used on /api/v1/* endpoints intended for machine consumers.
// Enforces the admin-configurable per-token rate limit after auth —
// rejects with 429 + Retry-After once a token exceeds the cap within a
// 60-second window.
func (s *Server) requireAPIToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			w.Header().Set("WWW-Authenticate", `Bearer realm="bekci-api"`)
			writeError(w, http.StatusUnauthorized, "bearer token required")
			return
		}
		plaintext := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		tok, err := s.store.AuthenticateAPIToken(plaintext)
		if err != nil || tok == nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="bekci-api", error="invalid_token"`)
			writeError(w, http.StatusUnauthorized, "invalid or revoked token")
			return
		}

		// Rate limit check — flat N req/min per token, admin-tunable.
		if s.v1RateLimiter != nil {
			limit := s.cachedAPIRateLimit()
			ok, retryAfter := s.v1RateLimiter.Allow(tok.ID, limit)
			if !ok {
				slog.Warn("v1 rate limit exceeded",
					"token", tok.Name,
					"ip", clientIP(r),
					"path", r.URL.Path,
					"limit_per_min", limit,
				)
				s.auditAPIToken(r, tok.ID, tok.Name, "api_rate_limit_exceeded",
					"path="+r.URL.Path+" limit_per_min="+strconv.Itoa(limit), "failure")
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				writeJSON(w, http.StatusTooManyRequests, map[string]any{
					"error":         "rate limit exceeded",
					"retry_after_s": retryAfter,
					"limit_per_min": limit,
				})
				return
			}
		}

		ctx := context.WithValue(r.Context(), ctxAPITokenName, tok.Name)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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

// gzipMiddleware compresses responses for clients that accept gzip.
func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		gz := gzip.NewWriter(w)
		defer gz.Close()
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length")
		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, Writer: gz}, r)
	})
}

type gzipResponseWriter struct {
	http.ResponseWriter
	Writer *gzip.Writer
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	return g.Writer.Write(b)
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
