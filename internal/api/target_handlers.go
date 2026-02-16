package api

import (
	"net/http"
	"strings"

	"github.com/bekci/internal/store"
)

var validOperators = map[string]bool{"AND": true, "OR": true}
var validCategories = map[string]bool{
	"ISP": true, "Router/Switch": true, "FW/WAF": true, "VPN": true,
	"SIEM/Logging": true, "PAM/DAM": true, "Security Other": true,
	"IT Server": true, "Other": true,
}
var validCheckTypes = map[string]bool{
	"http": true, "tcp": true, "ping": true,
	"dns": true, "page_hash": true, "tls_cert": true,
}

type targetRequest struct {
	Name        string                   `json:"name"`
	Host        string                   `json:"host"`
	Description string                   `json:"description"`
	Enabled     *bool                    `json:"enabled"`
	Operator    string                   `json:"operator"`
	Category    string                   `json:"category"`
	Conditions  []targetConditionRequest `json:"conditions"`
}

type targetConditionRequest struct {
	CheckID    string `json:"check_id"`
	CheckType  string `json:"check_type"`
	CheckName  string `json:"check_name"`
	Config     string `json:"config"`
	IntervalS  int    `json:"interval_s"`
	Field      string `json:"field"`
	Comparator string `json:"comparator"`
	Value      string `json:"value"`
	FailCount  int    `json:"fail_count"`
	FailWindow int    `json:"fail_window"`
}

func (s *Server) handleListTargets(w http.ResponseWriter, r *http.Request) {
	targets, err := s.store.ListTargetSummaries()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list targets")
		return
	}
	writeJSON(w, http.StatusOK, targets)
}

func (s *Server) handleCreateTarget(w http.ResponseWriter, r *http.Request) {
	var req targetRequest
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
	if req.Operator == "" {
		req.Operator = "AND"
	}
	if !validOperators[req.Operator] {
		writeError(w, http.StatusBadRequest, "operator must be AND or OR")
		return
	}
	if req.Category == "" {
		req.Category = "Other"
	}
	if !validCategories[req.Category] {
		writeError(w, http.StatusBadRequest, "invalid category")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// Validate conditions
	conds := make([]store.TargetCondition, 0, len(req.Conditions))
	for _, c := range req.Conditions {
		if c.CheckType == "" || !validCheckTypes[c.CheckType] {
			writeError(w, http.StatusBadRequest, "invalid check_type in condition")
			return
		}
		if c.CheckName == "" {
			writeError(w, http.StatusBadRequest, "check_name required in condition")
			return
		}
		if c.Config == "" {
			c.Config = "{}"
		}
		if c.IntervalS <= 0 {
			c.IntervalS = 300
		}
		if c.Value == "" {
			c.Value = "down"
		}
		conds = append(conds, store.TargetCondition{
			CheckID:    c.CheckID,
			CheckType:  c.CheckType,
			CheckName:  c.CheckName,
			Config:     c.Config,
			IntervalS:  c.IntervalS,
			Field:      c.Field,
			Comparator: c.Comparator,
			Value:      c.Value,
			FailCount:  c.FailCount,
			FailWindow: c.FailWindow,
		})
	}

	t := &store.Target{
		Name:     req.Name,
		Host:     req.Host,
		Description: req.Description,
		Enabled:  enabled,
		Operator: req.Operator,
		Category: req.Category,
	}

	if err := s.store.CreateTargetWithConditions(t, conds); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "target name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create target")
		return
	}

	if s.scheduler != nil {
		s.scheduler.Reload()
	}

	// Return full detail
	detail, err := s.store.GetTargetDetail(t.ID)
	if err != nil || detail == nil {
		writeJSON(w, http.StatusCreated, t)
		return
	}
	writeJSON(w, http.StatusCreated, detail)
}

func (s *Server) handleGetTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	detail, err := s.store.GetTargetDetail(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get target")
		return
	}
	if detail == nil {
		writeError(w, http.StatusNotFound, "target not found")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleUpdateTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req targetRequest
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
	if req.Operator == "" {
		req.Operator = "AND"
	}
	if !validOperators[req.Operator] {
		writeError(w, http.StatusBadRequest, "operator must be AND or OR")
		return
	}
	if req.Category == "" {
		req.Category = "Other"
	}
	if !validCategories[req.Category] {
		writeError(w, http.StatusBadRequest, "invalid category")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// Validate conditions
	conds := make([]store.TargetCondition, 0, len(req.Conditions))
	for _, c := range req.Conditions {
		if c.CheckType == "" || !validCheckTypes[c.CheckType] {
			writeError(w, http.StatusBadRequest, "invalid check_type in condition")
			return
		}
		if c.CheckName == "" {
			writeError(w, http.StatusBadRequest, "check_name required in condition")
			return
		}
		if c.Config == "" {
			c.Config = "{}"
		}
		if c.IntervalS <= 0 {
			c.IntervalS = 300
		}
		if c.Value == "" {
			c.Value = "down"
		}
		conds = append(conds, store.TargetCondition{
			CheckID:    c.CheckID,
			CheckType:  c.CheckType,
			CheckName:  c.CheckName,
			Config:     c.Config,
			IntervalS:  c.IntervalS,
			Field:      c.Field,
			Comparator: c.Comparator,
			Value:      c.Value,
			FailCount:  c.FailCount,
			FailWindow: c.FailWindow,
		})
	}

	if err := s.store.UpdateTargetWithConditions(id, req.Name, req.Host, req.Description, enabled, req.Operator, req.Category, conds); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "target not found")
			return
		}
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "target name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update target")
		return
	}

	if s.scheduler != nil {
		s.scheduler.Reload()
	}

	// Return full detail
	detail, err := s.store.GetTargetDetail(id)
	if err != nil || detail == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	writeJSON(w, http.StatusOK, detail)
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
