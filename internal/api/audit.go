package api

import (
	"log/slog"
	"net/http"

	"github.com/bekci/internal/store"
)

// audit logs an action performed by the authenticated user.
// Extracts user info from JWT claims and client IP from request.
// Never fails the caller — errors are logged only.
func (s *Server) audit(r *http.Request, action, resourceType, resourceID, detail, status string) {
	claims := getClaims(r)
	if claims == nil {
		return
	}

	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}

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
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}

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
