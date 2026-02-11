package api

import (
	"net/http"
	"strings"

	"github.com/bekci/internal/store"
)

var validCheckTypes = map[string]bool{
	"http": true, "tcp": true, "ping": true,
	"dns": true, "page_hash": true, "tls_cert": true,
}

func (s *Server) handleListChecks(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	checks, err := s.store.ListChecksByTarget(targetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list checks")
		return
	}
	writeJSON(w, http.StatusOK, checks)
}

func (s *Server) handleCreateCheck(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")

	// Verify target exists
	target, err := s.store.GetTarget(targetID)
	if err != nil || target == nil {
		writeError(w, http.StatusBadRequest, "target not found")
		return
	}

	var req struct {
		Type      string `json:"type"`
		Name      string `json:"name"`
		Config    string `json:"config"`
		IntervalS int    `json:"interval_s"`
		Enabled   *bool  `json:"enabled"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || req.Type == "" {
		writeError(w, http.StatusBadRequest, "name and type are required")
		return
	}
	if !validCheckTypes[req.Type] {
		writeError(w, http.StatusBadRequest, "invalid check type")
		return
	}
	if req.Config == "" {
		req.Config = "{}"
	}
	if req.IntervalS <= 0 {
		req.IntervalS = 300
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	c := &store.Check{
		TargetID:  targetID,
		Type:      req.Type,
		Name:      req.Name,
		Config:    req.Config,
		IntervalS: req.IntervalS,
		Enabled:   enabled,
	}
	if err := s.store.CreateCheck(c); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create check")
		return
	}
	if s.scheduler != nil {
		s.scheduler.Reload()
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name      string `json:"name"`
		Config    string `json:"config"`
		IntervalS int    `json:"interval_s"`
		Enabled   *bool  `json:"enabled"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Config == "" {
		req.Config = "{}"
	}
	if req.IntervalS <= 0 {
		req.IntervalS = 300
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	if err := s.store.UpdateCheck(id, req.Name, req.Config, req.IntervalS, enabled); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "check not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update check")
		return
	}
	if s.scheduler != nil {
		s.scheduler.Reload()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteCheck(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.DeleteCheck(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "check not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete check")
		return
	}
	if s.scheduler != nil {
		s.scheduler.Reload()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRunCheckNow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Verify check exists
	check, err := s.store.GetCheck(id)
	if err != nil || check == nil {
		writeError(w, http.StatusNotFound, "check not found")
		return
	}

	if s.scheduler != nil {
		s.scheduler.RunNow(id)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "queued"})
}

func (s *Server) handleCheckResults(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	results, err := s.store.GetRecentResults(id, 24) // last 24 hours
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get results")
		return
	}
	writeJSON(w, http.StatusOK, results)
}
