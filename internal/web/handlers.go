package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bekci/internal/config"
)

// ProjectStatus represents a project's status for the API/template
type ProjectStatus struct {
	Name     string          `json:"name"`
	Services []ServiceStatus `json:"services"`
}

// ServiceStatus represents a service's status for the API/template
type ServiceStatus struct {
	Name          string       `json:"name"`
	CheckTarget   string       `json:"check_target"`
	Status        string       `json:"status"`
	UptimePct     float64      `json:"uptime_pct"`
	LastCheck     time.Time    `json:"last_check"`
	LastError     string       `json:"last_error,omitempty"`
	ResponseMs    int64        `json:"response_ms"`
	DailyHistory  []DayStatus  `json:"daily_history"`
}

// DayStatus represents a single day's status
type DayStatus struct {
	Date      string  `json:"date"`
	UptimePct float64 `json:"uptime_pct"`
	Color     string  `json:"color"`
}

// handleHealth is the self-check endpoint
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleAPIStatus returns JSON status for all services
func (s *Server) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.getStatus()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleCheckNow triggers an immediate check of all services
func (s *Server) handleCheckNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.scheduler.CheckAllNow()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "triggered"})
}

// handleStatus renders the HTML status page
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	status, err := s.getStatus()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		Projects      []ProjectStatus
		LastUpdated   string
		CheckInterval string
	}{
		Projects:      status,
		LastUpdated:   time.Now().Format("02/01/2006 15:04:05"),
		CheckInterval: s.config.Global.CheckInterval.String(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "status.html", data); err != nil {
		slog.Error("Template error", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// getStatus builds the status data for all projects/services
func (s *Server) getStatus() ([]ProjectStatus, error) {
	var projects []ProjectStatus

	states, err := s.store.GetAllServiceStates()
	if err != nil {
		return nil, err
	}

	for _, proj := range s.config.Projects {
		ps := ProjectStatus{
			Name:     proj.Name,
			Services: make([]ServiceStatus, 0, len(proj.Services)),
		}

		for _, svc := range proj.Services {
			serviceKey := config.GetServiceKey(proj.Name, svc.Name)

			ss := ServiceStatus{
				Name:        svc.Name,
				CheckTarget: checkTarget(&svc),
				Status:      "unknown",
			}

			// Get current status
			if status, ok := states[serviceKey]; ok {
				ss.Status = status
			}

			// Get uptime percentage
			uptime, _ := s.store.GetOverallUptime(serviceKey, 90)
			ss.UptimePct = uptime

			// Get recent check for response time and last error
			recent, _ := s.store.GetRecentChecks(serviceKey, 1)
			if len(recent) > 0 {
				ss.LastCheck = recent[0].CheckedAt
				ss.ResponseMs = recent[0].ResponseMs
				if recent[0].Status == "down" {
					ss.LastError = recent[0].Error
				}
			}

			// Get daily history (90 days)
			dailyStats, _ := s.store.GetDailyStats(serviceKey, 90)
			ss.DailyHistory = make([]DayStatus, len(dailyStats))
			for i, ds := range dailyStats {
				ss.DailyHistory[i] = DayStatus{
					Date:      ds.Date.Format("02/01/2006"),
					UptimePct: ds.UptimePct,
					Color:     uptimeColor(ds.UptimePct),
				}
			}

			ps.Services = append(ps.Services, ss)
		}

		projects = append(projects, ps)
	}

	return projects, nil
}

// checkTarget builds a display string for the service's check endpoint
func checkTarget(svc *config.Service) string {
	switch svc.Check.Type {
	case "https":
		url := svc.URL
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			url = "https://" + url
		}
		if svc.Check.Endpoint != "" {
			url = strings.TrimSuffix(url, "/") + svc.Check.Endpoint
		}
		return url
	case "tcp":
		return fmt.Sprintf("tcp://%s", svc.URL)
	case "process":
		return fmt.Sprintf("process:%s", svc.Check.ProcessName)
	case "ssh_process":
		return fmt.Sprintf("ssh://%s process:%s", svc.Check.Host, svc.Check.ProcessName)
	case "ssh_command":
		// Only show base command name, strip arguments to avoid leaking credentials
		cmd := svc.Check.Command
		if i := strings.IndexByte(cmd, ' '); i > 0 {
			cmd = cmd[:i]
		}
		return fmt.Sprintf("ssh://%s $ %s", svc.Check.Host, cmd)
	default:
		return ""
	}
}
