package api

import (
	"net/http"
	"strings"

	"github.com/bekci/internal/store"
)

func (s *Server) handleListTargets(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	targets, err := s.store.ListTargets(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list targets")
		return
	}
	writeJSON(w, http.StatusOK, targets)
}

func (s *Server) handleCreateTarget(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID   string `json:"project_id"`
		Name        string `json:"name"`
		Host        string `json:"host"`
		Description string `json:"description"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Host = strings.TrimSpace(req.Host)
	if req.Name == "" || req.Host == "" || req.ProjectID == "" {
		writeError(w, http.StatusBadRequest, "project_id, name, and host are required")
		return
	}

	// Verify project exists
	proj, err := s.store.GetProject(req.ProjectID)
	if err != nil || proj == nil {
		writeError(w, http.StatusBadRequest, "project not found")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	t := &store.Target{
		ProjectID:   req.ProjectID,
		Name:        req.Name,
		Host:        req.Host,
		Description: req.Description,
		Enabled:     enabled,
	}
	if err := s.store.CreateTarget(t); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "target name already exists in this project")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create target")
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleGetTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tw, err := s.store.GetTargetWithChecks(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get target")
		return
	}
	if tw == nil {
		writeError(w, http.StatusNotFound, "target not found")
		return
	}
	writeJSON(w, http.StatusOK, tw)
}

func (s *Server) handleUpdateTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name        string `json:"name"`
		Host        string `json:"host"`
		Description string `json:"description"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Host = strings.TrimSpace(req.Host)
	if req.Name == "" || req.Host == "" {
		writeError(w, http.StatusBadRequest, "name and host are required")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	if err := s.store.UpdateTarget(id, req.Name, req.Host, req.Description, enabled); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "target not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update target")
		return
	}
	if s.scheduler != nil {
		s.scheduler.Reload()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.DeleteTarget(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "target not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete target")
		return
	}
	if s.scheduler != nil {
		s.scheduler.Reload()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
