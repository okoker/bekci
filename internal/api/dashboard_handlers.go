package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type dashboardCheck struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Enabled     bool    `json:"enabled"`
	IntervalS   int     `json:"interval_s"`
	LastStatus  string  `json:"last_status"`
	LastMessage string  `json:"last_message"`
	ResponseMs  int64   `json:"response_ms"`
	Uptime90d   float64 `json:"uptime_90d_pct"`
}

type dashboardTarget struct {
	ID                 string           `json:"id"`
	Name               string           `json:"name"`
	Host               string           `json:"host"`
	PreferredCheckType string           `json:"preferred_check_type"`
	State              string           `json:"state"`
	Paused             bool             `json:"paused"`
	PausedAt           *string          `json:"paused_at,omitempty"`
	Category           string           `json:"category"`
	SLAStatus          string           `json:"sla_status"`
	SLATarget          float64          `json:"sla_target"`
	Checks             []dashboardCheck `json:"checks"`
}

// categoryToSLAKey maps target category names to settings keys.
var categoryToSLAKey = map[string]string{
	"Network":           "sla_network",
	"Security":          "sla_security",
	"Physical Security": "sla_physical_security",
	"Key Services":      "sla_key_services",
	"Other":             "sla_other",
}

func (s *Server) buildDashboardTargets() ([]dashboardTarget, error) {
	targets, err := s.store.ListTargets()
	if err != nil {
		return nil, err
	}

	// Load SLA thresholds once
	allSettings, err := s.store.GetAllSettings()
	if err != nil {
		return nil, fmt.Errorf("failed to load settings: %w", err)
	}
	slaThresholds := make(map[string]float64)
	for cat, key := range categoryToSLAKey {
		if v, ok := allSettings[key]; ok {
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				slaThresholds[cat] = f
			}
		}
	}

	// Batch: all rule states, checks, and results in 3 queries (replaces N+1 pattern)
	ruleStates, err := s.store.ListAllRuleStates()
	if err != nil {
		return nil, fmt.Errorf("failed to load rule states: %w", err)
	}

	allChecks, err := s.store.ListAllChecks()
	if err != nil {
		return nil, fmt.Errorf("failed to load checks: %w", err)
	}

	checkSummaries, err := s.store.GetBatchLastResultAndUptime()
	if err != nil {
		return nil, fmt.Errorf("failed to load check summaries: %w", err)
	}

	var result []dashboardTarget
	for _, t := range targets {
		// Hide disabled targets from dashboard/SOC
		if !t.Enabled {
			continue
		}

		dt := dashboardTarget{
			ID:                 t.ID,
			Name:               t.Name,
			Host:               t.Host,
			PreferredCheckType: t.PreferredCheckType,
			Category:           t.Category,
		}

		// Paused targets show as "paused" state
		if t.PausedAt != nil {
			dt.Paused = true
			ps := t.PausedAt.Format("2006-01-02T15:04:05Z")
			dt.PausedAt = &ps
			dt.State = "paused"
		} else if t.RuleID != nil {
			if rs, ok := ruleStates[*t.RuleID]; ok {
				dt.State = rs.CurrentState
			}
		}

		for _, c := range allChecks[t.ID] {
			dc := dashboardCheck{
				ID:        c.ID,
				Name:      c.Name,
				Type:      c.Type,
				Enabled:   c.Enabled,
				IntervalS: c.IntervalS,
			}

			if summary, ok := checkSummaries[c.ID]; ok {
				dc.LastStatus = summary.Status
				dc.LastMessage = summary.Message
				dc.ResponseMs = summary.ResponseMs
				dc.Uptime90d = summary.Uptime90d
			}

			dt.Checks = append(dt.Checks, dc)
		}
		if dt.Checks == nil {
			dt.Checks = []dashboardCheck{}
		}

		// Compute SLA status from preferred check's 90d uptime vs category threshold
		if threshold, ok := slaThresholds[t.Category]; ok && threshold > 0 {
			dt.SLATarget = threshold
			// Find preferred check's uptime
			var prefUptime float64 = -1
			for _, c := range dt.Checks {
				if c.Type == t.PreferredCheckType {
					prefUptime = c.Uptime90d
					break
				}
			}
			if prefUptime < 0 && len(dt.Checks) > 0 {
				prefUptime = dt.Checks[0].Uptime90d
			}
			if prefUptime >= 0 {
				if prefUptime >= threshold {
					dt.SLAStatus = "healthy"
				} else {
					dt.SLAStatus = "unhealthy"
				}
			}
		}

		result = append(result, dt)
	}

	if result == nil {
		result = []dashboardTarget{}
	}
	return result, nil
}

func (s *Server) handleDashboardStatus(w http.ResponseWriter, r *http.Request) {
	targets, err := s.buildDashboardTargets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list targets")
		return
	}
	writeJSON(w, http.StatusOK, targets)
}

// SOC handlers — returns same flat []dashboardTarget.
func (s *Server) handleSocStatus(w http.ResponseWriter, r *http.Request) {
	targets, err := s.buildDashboardTargets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list targets")
		return
	}
	writeJSON(w, http.StatusOK, targets)
}

func (s *Server) handleCheckHistory(w http.ResponseWriter, r *http.Request) {
	checkID := r.PathValue("checkId")
	rangeParam := r.URL.Query().Get("range")

	switch rangeParam {
	case "4h":
		results, err := s.store.GetRecentResultsSlim(checkID, 4)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to get results")
			return
		}
		writeJSON(w, http.StatusOK, results)
	default: // "90d" or empty
		uptimes, err := s.store.GetDailyUptime(checkID, 90)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to get uptime")
			return
		}
		writeJSON(w, http.StatusOK, uptimes)
	}
}
