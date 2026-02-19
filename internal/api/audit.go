package api

import (
	"log/slog"
	"net"
	"net/http"

	"github.com/bekci/internal/store"
)

// clientIP returns the real client IP from the request.
// Behind a trusted reverse proxy (detected by loopback RemoteAddr), it reads
// X-Real-IP which nginx sets to $remote_addr. For direct connections, it
// uses RemoteAddr. The port is always stripped.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		if real := r.Header.Get("X-Real-IP"); real != "" {
			if parsed := net.ParseIP(real); parsed != nil {
				return parsed.String()
			}
		}
	}
	return host
}

// audit logs an action performed by the authenticated user.
// Extracts user info from JWT claims and client IP from request.
// Never fails the caller — errors are logged only.
func (s *Server) audit(r *http.Request, action, resourceType, resourceID, detail, status string) {
	claims := getClaims(r)
	if claims == nil {
		return
	}

	ip := clientIP(r)

	// Resolve username from store (claims don't carry it)
	username := claims.Subject
	if user, err := s.store.GetUserByID(claims.Subject); err == nil && user != nil {
		username = user.Username
	}

	entry := &store.AuditEntry{
		UserID:       claims.Subject,
		Username:     username,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Detail:       detail,
		IPAddress:    ip,
		Status:       status,
	}
	if err := s.store.CreateAuditEntry(entry); err != nil {
		slog.Error("Failed to create audit entry", "action", action, "error", err)
	}
}

// auditLogin logs login-related events where JWT claims are not yet available.
func (s *Server) auditLogin(r *http.Request, userID, username, action, detail, status string) {
	ip := clientIP(r)

	entry := &store.AuditEntry{
		UserID:       userID,
		Username:     username,
		Action:       action,
		ResourceType: "session",
		Detail:       detail,
		IPAddress:    ip,
		Status:       status,
	}
	if err := s.store.CreateAuditEntry(entry); err != nil {
		slog.Error("Failed to create audit entry", "action", action, "error", err)
	}
}
