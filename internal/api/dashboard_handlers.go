package api

import (
	"net/http"
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
	Checks             []dashboardCheck `json:"checks"`
}

func (s *Server) handleDashboardStatus(w http.ResponseWriter, r *http.Request) {
	targets, err := s.store.ListTargets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list targets")
		return
	}

	var result []dashboardTarget
	for _, t := range targets {
		dt := dashboardTarget{
			ID:                 t.ID,
			Name:               t.Name,
			Host:               t.Host,
			PreferredCheckType: t.PreferredCheckType,
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

			// Get last result
			last, err := s.store.GetLastResult(c.ID)
			if err == nil && last != nil {
				dc.LastStatus = last.Status
				dc.LastMessage = last.Message
				dc.ResponseMs = last.ResponseMs
			}

			// Get 90-day uptime
			pct, err := s.store.GetUptimePercent(c.ID, 90)
			if err == nil {
				dc.Uptime90d = pct
			}

			dt.Checks = append(dt.Checks, dc)
		}
		if dt.Checks == nil {
			dt.Checks = []dashboardCheck{}
		}

		result = append(result, dt)
	}

	if result == nil {
		result = []dashboardTarget{}
	}

	writeJSON(w, http.StatusOK, result)
}

// SOC handlers — delegate to same logic as dashboard, with conditional auth.
func (s *Server) handleSocStatus(w http.ResponseWriter, r *http.Request) {
	s.handleDashboardStatus(w, r)
}

func (s *Server) handleSocHistory(w http.ResponseWriter, r *http.Request) {
	s.handleCheckHistory(w, r)
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
