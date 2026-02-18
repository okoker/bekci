package api

import (
	"log/slog"
	"net/http"
	"strings"

	authpkg "github.com/bekci/internal/auth"
)

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	ip := clientIP(r)

	token, user, err := s.auth.Login(req.Username, req.Password, ip)
	if err != nil {
		slog.Warn("Login failed", "username", req.Username, "ip", ip)
		s.auditLogin(r, "", req.Username, "login_failed", err.Error(), "failure")
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	s.auditLogin(r, user.ID, user.Username, "login", "", "success")
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user": map[string]any{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
			"role":     user.Role,
		},
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	claims := getClaims(r)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if err := s.auth.Logout(claims.SessionID); err != nil {
		writeError(w, http.StatusInternalServerError, "logout failed")
		return
	}
	s.audit(r, "logout", "session", "", "", "success")
	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	claims := getClaims(r)
	user, err := s.store.GetUserByID(claims.Subject)
	if err != nil || user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":       user.ID,
		"username": user.Username,
		"email":    user.Email,
		"role":     user.Role,
		"status":   user.Status,
	})
}

func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	claims := getClaims(r)
	var req struct {
		Email string `json:"email"`
		Phone string `json:"phone"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := s.store.GetUserByID(claims.Subject)
	if err != nil || user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	if err := s.store.UpdateUser(user.ID, req.Email, req.Phone, user.Role); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	s.audit(r, "update_profile", "user", user.ID, "email="+req.Email, "success")
	writeJSON(w, http.StatusOK, map[string]string{"message": "profile updated"})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	claims := getClaims(r)
	var req struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.New) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	user, err := s.store.GetUserByID(claims.Subject)
	if err != nil || user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if !authpkg.CheckPassword(user.PasswordHash, req.Current) {
		s.audit(r, "change_password_failed", "user", user.ID, "incorrect current password", "failure")
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	hash, err := authpkg.HashPassword(req.New)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "password hashing failed")
		return
	}
	if err := s.store.UpdateUserPassword(user.ID, hash); err != nil {
		writeError(w, http.StatusInternalServerError, "password update failed")
		return
	}
	// Invalidate all other sessions
	s.store.DeleteUserSessionsExcept(user.ID, claims.SessionID)
	s.audit(r, "change_password", "user", user.ID, "", "success")
	writeJSON(w, http.StatusOK, map[string]string{"message": "password changed"})
}
