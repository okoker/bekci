package api

import (
	"fmt"
	"net/http"
	"strconv"
)

func (s *Server) handleListRecipients(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Verify target exists
	t, err := s.store.GetTarget(id)
	if err != nil || t == nil {
		writeError(w, http.StatusNotFound, "target not found")
		return
	}

	recipients, err := s.store.ListTargetRecipients(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list recipients")
		return
	}
	writeJSON(w, http.StatusOK, recipients)
}

func (s *Server) handleSetRecipients(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req struct {
		UserIDs []string `json:"user_ids"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Verify target exists
	t, err := s.store.GetTarget(id)
	if err != nil || t == nil {
		writeError(w, http.StatusNotFound, "target not found")
		return
	}

	if err := s.store.SetTargetRecipients(id, req.UserIDs); err != nil {
		s.audit(r, "set_alert_recipients", "target", id, "failed", "failure")
		writeError(w, http.StatusInternalServerError, "failed to set recipients")
		return
	}

	s.audit(r, "set_alert_recipients", "target", id, fmt.Sprintf("user_ids=%v count=%d", req.UserIDs, len(req.UserIDs)), "success")
	writeJSON(w, http.StatusOK, map[string]string{"message": "recipients updated"})
}

func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 50
	}
	offset := (page - 1) * limit

	entries, total, err := s.store.ListAlertHistory(limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list alerts")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
		"total":   total,
	})
}

func (s *Server) handleTestEmail(w http.ResponseWriter, r *http.Request) {
	if s.alerter == nil {
		s.audit(r, "test_email", "settings", "", "alerter unavailable", "failure")
		writeError(w, http.StatusServiceUnavailable, "alerter not initialized")
		return
	}

	claims := getClaims(r)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	user, err := s.store.GetUserByID(claims.Subject)
	if err != nil || user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if user.Email == "" {
		s.audit(r, "test_email", "settings", "", "no email configured", "failure")
		writeError(w, http.StatusBadRequest, "your account has no email address configured")
		return
	}

	if err := s.alerter.SendTestEmail(user.Email); err != nil {
		s.audit(r, "test_email", "settings", "", "to="+user.Email+" error="+err.Error(), "failure")
		writeError(w, http.StatusInternalServerError, "failed to send test email: "+err.Error())
		return
	}

	s.audit(r, "test_email", "settings", "", "to="+user.Email, "success")
	writeJSON(w, http.StatusOK, map[string]string{"message": "test email sent to " + user.Email})
}

func (s *Server) handleTestSignal(w http.ResponseWriter, r *http.Request) {
	if s.alerter == nil {
		s.audit(r, "test_signal", "settings", "", "alerter unavailable", "failure")
		writeError(w, http.StatusServiceUnavailable, "alerter not initialized")
		return
	}

	claims := getClaims(r)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		Phone string `json:"phone"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Phone == "" {
		s.audit(r, "test_signal", "settings", "", "no phone provided", "failure")
		writeError(w, http.StatusBadRequest, "phone number is required")
		return
	}

	if err := s.alerter.SendTestSignal(req.Phone); err != nil {
		s.audit(r, "test_signal", "settings", "", "to="+req.Phone+" error="+err.Error(), "failure")
		writeError(w, http.StatusInternalServerError, "failed to send test signal: "+err.Error())
		return
	}

	s.audit(r, "test_signal", "settings", "", "to="+req.Phone, "success")
	writeJSON(w, http.StatusOK, map[string]string{"message": "test signal sent to " + req.Phone})
}
