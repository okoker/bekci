package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/bekci/internal/auth"
	"github.com/bekci/internal/store"
)

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	if users == nil {
		users = []store.User{}
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)
	req.Role = strings.TrimSpace(req.Role)

	if req.Username == "" || req.Password == "" || req.Role == "" {
		writeError(w, http.StatusBadRequest, "username, password, and role are required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if req.Role != "admin" && req.Role != "operator" && req.Role != "viewer" {
		writeError(w, http.StatusBadRequest, "role must be admin, operator, or viewer")
		return
	}

	existing, _ := s.store.GetUserByUsername(req.Username)
	if existing != nil {
		writeError(w, http.StatusConflict, "username already exists")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "password hashing failed")
		return
	}

	now := time.Now()
	user := &store.User{
		ID:           uuid.New().String(),
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: hash,
		Role:         req.Role,
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.store.CreateUser(user); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":       user.ID,
		"username": user.Username,
		"email":    user.Email,
		"role":     user.Role,
		"status":   user.Status,
	})
}

func (s *Server) handleGetUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	user, err := s.store.GetUserByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":         user.ID,
		"username":   user.Username,
		"email":      user.Email,
		"role":       user.Role,
		"status":     user.Status,
		"created_at": user.CreatedAt,
		"updated_at": user.UpdatedAt,
	})
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := s.store.GetUserByID(id)
	if err != nil || user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	role := req.Role
	if role == "" {
		role = user.Role
	}
	if role != "admin" && role != "operator" && role != "viewer" {
		writeError(w, http.StatusBadRequest, "role must be admin, operator, or viewer")
		return
	}

	// Last admin protection: prevent demoting the last active admin
	if user.Role == "admin" && role != "admin" {
		count, err := s.store.CountActiveAdmins()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if count <= 1 {
			writeError(w, http.StatusConflict, "cannot demote the last active admin")
			return
		}
	}

	email := req.Email
	if email == "" {
		email = user.Email
	}

	if err := s.store.UpdateUser(id, email, role); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "user updated"})
}

func (s *Server) handleSuspendUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Suspended bool `json:"suspended"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := s.store.GetUserByID(id)
	if err != nil || user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	// Last admin protection: prevent suspending the last active admin
	if req.Suspended && user.Role == "admin" {
		count, err := s.store.CountActiveAdmins()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if count <= 1 {
			writeError(w, http.StatusConflict, "cannot suspend the last active admin")
			return
		}
	}

	if err := s.store.SuspendUser(id, req.Suspended); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	// Kill all sessions when suspending
	if req.Suspended {
		s.store.DeleteUserSessions(id)
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "user status updated"})
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	user, err := s.store.GetUserByID(id)
	if err != nil || user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "password hashing failed")
		return
	}
	if err := s.store.UpdateUserPassword(id, hash); err != nil {
		writeError(w, http.StatusInternalServerError, "password reset failed")
		return
	}

	// Kill all sessions so user must re-login
	s.store.DeleteUserSessions(id)

	writeJSON(w, http.StatusOK, map[string]string{"message": "password reset"})
}
