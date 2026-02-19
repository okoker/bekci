package api

import (
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
	allSettings, _ := s.store.GetAllSettings()
	slaThresholds := make(map[string]float64)
	for cat, key := range categoryToSLAKey {
		if v, ok := allSettings[key]; ok {
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				slaThresholds[cat] = f
			}
		}
	}

	var result []dashboardTarget
	for _, t := range targets {
		dt := dashboardTarget{
			ID:                 t.ID,
			Name:               t.Name,
			Host:               t.Host,
			PreferredCheckType: t.PreferredCheckType,
			Category:           t.Category,
		}

		// Per-target health from rule_states
		if t.RuleID != nil {
			rs, err := s.store.GetRuleState(*t.RuleID)
			if err == nil && rs != nil {
				dt.State = rs.CurrentState
			}
		}

		checks, err := s.store.ListChecksByTarget(t.ID)
		if err != nil {
			continue
		}

		for _, c := range checks {
			dc := dashboardCheck{
				ID:        c.ID,
				Name:      c.Name,
				Type:      c.Type,
				Enabled:   c.Enabled,
				IntervalS: c.IntervalS,
			}

			last, err := s.store.GetLastResult(c.ID)
			if err == nil && last != nil {
				dc.LastStatus = last.Status
				dc.LastMessage = last.Message
				dc.ResponseMs = last.ResponseMs
			}

			pct, err := s.store.GetUptimePercent(c.ID, 90)
			if err == nil {
				dc.Uptime90d = pct
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
		results, err := s.store.GetRecentResults(checkID, 4)
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
