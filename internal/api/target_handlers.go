package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/bekci/internal/store"
)

// normalizeFailWindow auto-sets fail_window when fail_count > 1 and validates bounds.
func normalizeFailWindow(failCount, failWindow, intervalS int) int {
	if failCount <= 1 {
		return failWindow // no change needed for single-check mode
	}
	// If fail_count > 1 but no fail_window, default to interval_s
	if failWindow <= 0 {
		failWindow = intervalS
	}
	// Clamp: min = interval_s, max = 1800
	if failWindow < intervalS {
		failWindow = intervalS
	}
	if failWindow > 1800 {
		failWindow = 1800
	}
	return failWindow
}

var validOperators = map[string]bool{"AND": true, "OR": true}
var validCategories = map[string]bool{
	"Network": true, "Security": true, "Physical Security": true,
	"Key Services": true, "Other": true,
}
var validCheckTypes = map[string]bool{
	"http": true, "tcp": true, "ping": true,
	"dns": true, "page_hash": true, "tls_cert": true,
}

type targetRequest struct {
	Name               string                   `json:"name"`
	Host               string                   `json:"host"`
	Description        string                   `json:"description"`
	Enabled            *bool                    `json:"enabled"`
	Operator           string                   `json:"operator"`
	Category           string                   `json:"category"`
	PreferredCheckType string                   `json:"preferred_check_type"`
	Conditions         []targetConditionRequest `json:"conditions"`
}

type targetConditionRequest struct {
	CheckID        string `json:"check_id"`
	CheckType      string `json:"check_type"`
	CheckName      string `json:"check_name"`
	Config         string `json:"config"`
	IntervalS      int    `json:"interval_s"`
	Field          string `json:"field"`
	Comparator     string `json:"comparator"`
	Value          string `json:"value"`
	FailCount      int    `json:"fail_count"`
	FailWindow     int    `json:"fail_window"`
	ConditionGroup int    `json:"condition_group"`
	GroupOperator  string `json:"group_operator"`
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
	if err := readJSON(w, r, &req); err != nil {
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
	if len(req.Conditions) == 0 {
		s.audit(r, "create_target", "target", "", "name="+req.Name+" no conditions", "failure")
		writeError(w, http.StatusBadRequest, "at least one condition is required")
		return
	}
	conds := make([]store.TargetCondition, 0, len(req.Conditions))
	for _, c := range req.Conditions {
		if c.CheckType == "" || !validCheckTypes[c.CheckType] {
			s.audit(r, "create_target", "target", "", "name="+req.Name+" invalid check_type="+c.CheckType, "failure")
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
		groupOp := c.GroupOperator
		if groupOp == "" {
			groupOp = req.Operator // backward compat: inherit target-level operator
		}
		if !validOperators[groupOp] {
			writeError(w, http.StatusBadRequest, "group_operator must be AND or OR")
			return
		}
		failCount := c.FailCount
		if failCount <= 0 {
			failCount = 1
		}
		failWindow := normalizeFailWindow(failCount, c.FailWindow, c.IntervalS)
		conds = append(conds, store.TargetCondition{
			CheckID:        c.CheckID,
			CheckType:      c.CheckType,
			CheckName:      c.CheckName,
			Config:         c.Config,
			IntervalS:      c.IntervalS,
			Field:          c.Field,
			Comparator:     c.Comparator,
			Value:          c.Value,
			FailCount:      failCount,
			FailWindow:     failWindow,
			ConditionGroup: c.ConditionGroup,
			GroupOperator:  groupOp,
		})
	}

	t := &store.Target{
		Name:               req.Name,
		Host:               req.Host,
		Description:        req.Description,
		Enabled:            enabled,
		Operator:           req.Operator,
		Category:           req.Category,
		PreferredCheckType: req.PreferredCheckType,
	}

	creatorID := ""
	if claims := getClaims(r); claims != nil {
		creatorID = claims.Subject
	}

	if err := s.store.CreateTargetWithConditions(t, conds, creatorID); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			s.audit(r, "create_target", "target", "", "name="+t.Name+" duplicate", "failure")
			writeError(w, http.StatusConflict, "target name already exists")
			return
		}
		s.audit(r, "create_target", "target", "", "name="+t.Name+" failed", "failure")
		writeError(w, http.StatusInternalServerError, "failed to create target")
		return
	}

	if s.scheduler != nil {
		s.scheduler.Reload()
	}

	s.audit(r, "create_target", "target", t.ID, fmt.Sprintf("name=%s host=%s category=%s conditions=%d", t.Name, req.Host, req.Category, len(conds)), "success")

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
	if err := readJSON(w, r, &req); err != nil {
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
	if len(req.Conditions) == 0 {
		s.audit(r, "update_target", "target", id, "name="+req.Name+" no conditions", "failure")
		writeError(w, http.StatusBadRequest, "at least one condition is required")
		return
	}
	conds := make([]store.TargetCondition, 0, len(req.Conditions))
	for _, c := range req.Conditions {
		if c.CheckType == "" || !validCheckTypes[c.CheckType] {
			s.audit(r, "update_target", "target", id, "name="+req.Name+" invalid check_type="+c.CheckType, "failure")
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
		groupOp := c.GroupOperator
		if groupOp == "" {
			groupOp = req.Operator
		}
		if !validOperators[groupOp] {
			writeError(w, http.StatusBadRequest, "group_operator must be AND or OR")
			return
		}
		failCount := c.FailCount
		if failCount <= 0 {
			failCount = 1
		}
		failWindow := normalizeFailWindow(failCount, c.FailWindow, c.IntervalS)
		conds = append(conds, store.TargetCondition{
			CheckID:        c.CheckID,
			CheckType:      c.CheckType,
			CheckName:      c.CheckName,
			Config:         c.Config,
			IntervalS:      c.IntervalS,
			Field:          c.Field,
			Comparator:     c.Comparator,
			Value:          c.Value,
			FailCount:      failCount,
			FailWindow:     failWindow,
			ConditionGroup: c.ConditionGroup,
			GroupOperator:  groupOp,
		})
	}

	if err := s.store.UpdateTargetWithConditions(id, req.Name, req.Host, req.Description, enabled, req.Operator, req.Category, req.PreferredCheckType, conds); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.audit(r, "update_target", "target", id, "not found", "failure")
			writeError(w, http.StatusNotFound, "target not found")
			return
		}
		if strings.Contains(err.Error(), "UNIQUE") {
			s.audit(r, "update_target", "target", id, "name="+req.Name+" duplicate", "failure")
			writeError(w, http.StatusConflict, "target name already exists")
			return
		}
		s.audit(r, "update_target", "target", id, "name="+req.Name+" failed", "failure")
		writeError(w, http.StatusInternalServerError, "failed to update target")
		return
	}

	if s.scheduler != nil {
		s.scheduler.Reload()
	}

	s.audit(r, "update_target", "target", id, fmt.Sprintf("name=%s host=%s conditions=%d", req.Name, req.Host, len(conds)), "success")

	// Return full detail
	detail, err := s.store.GetTargetDetail(id)
	if err != nil || detail == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handlePauseTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.PauseTarget(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.audit(r, "pause_target", "target", id, "not found", "failure")
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if strings.Contains(err.Error(), "already paused") {
			s.audit(r, "pause_target", "target", id, "already paused", "failure")
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.audit(r, "pause_target", "target", id, "failed", "failure")
		writeError(w, http.StatusInternalServerError, "failed to pause target")
		return
	}
	if s.scheduler != nil {
		s.scheduler.Reload()
	}
	s.audit(r, "pause_target", "target", id, "paused", "success")
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (s *Server) handleUnpauseTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.UnpauseTarget(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.audit(r, "unpause_target", "target", id, "not found", "failure")
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if strings.Contains(err.Error(), "not paused") {
			s.audit(r, "unpause_target", "target", id, "not paused", "failure")
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.audit(r, "unpause_target", "target", id, "failed", "failure")
		writeError(w, http.StatusInternalServerError, "failed to unpause target")
		return
	}
	if s.scheduler != nil {
		s.scheduler.Reload()
	}
	// Run all checks immediately after unpause
	checks, err := s.store.ListChecksByTarget(id)
	if err == nil && s.scheduler != nil {
		for _, c := range checks {
			s.scheduler.RunNow(c.ID)
		}
	}
	s.audit(r, "unpause_target", "target", id, "unpaused", "success")
	writeJSON(w, http.StatusOK, map[string]string{"status": "unpaused"})
}

func (s *Server) handleDeleteTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.DeleteTarget(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.audit(r, "delete_target", "target", id, "not found", "failure")
			writeError(w, http.StatusNotFound, "target not found")
			return
		}
		s.audit(r, "delete_target", "target", id, "failed", "failure")
		writeError(w, http.StatusInternalServerError, "failed to delete target")
		return
	}
	if s.scheduler != nil {
		s.scheduler.Reload()
	}
	s.audit(r, "delete_target", "target", id, "", "success")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
