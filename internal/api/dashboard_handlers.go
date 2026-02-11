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
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Host      string           `json:"host"`
	ProjectID string           `json:"project_id"`
	Checks    []dashboardCheck `json:"checks"`
}

type dashboardProject struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Targets []dashboardTarget `json:"targets"`
}

func (s *Server) handleDashboardStatus(w http.ResponseWriter, r *http.Request) {
	projects, err := s.store.ListProjects()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}

	var result []dashboardProject
	for _, p := range projects {
		dp := dashboardProject{ID: p.ID, Name: p.Name}

		targets, err := s.store.ListTargets(p.ID)
		if err != nil {
			continue
		}

		for _, t := range targets {
			dt := dashboardTarget{
				ID:        t.ID,
				Name:      t.Name,
				Host:      t.Host,
				ProjectID: t.ProjectID,
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

			dp.Targets = append(dp.Targets, dt)
		}
		if dp.Targets == nil {
			dp.Targets = []dashboardTarget{}
		}

		result = append(result, dp)
	}

	if result == nil {
		result = []dashboardProject{}
	}

	writeJSON(w, http.StatusOK, result)
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
