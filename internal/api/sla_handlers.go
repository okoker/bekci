package api

import (
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/bekci/internal/store"
)

type slaDailyUptime struct {
	Date      string  `json:"date"`
	UptimePct float64 `json:"uptime_pct"`
}

type slaTarget struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	DailyUptime []slaDailyUptime `json:"daily_uptime"`
}

type slaCategory struct {
	Name         string      `json:"name"`
	SLAThreshold float64     `json:"sla_threshold"`
	Targets      []slaTarget `json:"targets"`
}

type slaPauseStats struct {
	Count         int `json:"count"`
	AffectedHosts int `json:"affected_hosts"`
}

type slaHistoryResponse struct {
	Categories []slaCategory `json:"categories"`
	PauseStats slaPauseStats `json:"pause_stats"`
}

func (s *Server) handleSLAHistory(w http.ResponseWriter, r *http.Request) {
	targets, err := s.store.ListTargets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list targets")
		return
	}

	// Load categories from DB
	cats, err := s.store.ListTagOptions("category")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load categories")
		return
	}

	// Load SLA thresholds dynamically
	allSettings, err := s.store.GetAllSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}
	slaThresholds := categorySLAThresholds(cats, allSettings)

	// Group targets by category (skip disabled targets)
	catTargets := make(map[string][]slaTarget)
	for _, t := range targets {
		if !t.Enabled {
			continue
		}
		// Find preferred check
		checks, err := s.store.ListChecksByTarget(t.ID)
		if err != nil {
			slog.Warn("SLA: failed to list checks for target", "target_id", t.ID, "error", err)
			continue
		}
		if len(checks) == 0 {
			continue
		}

		var checkID string
		for _, c := range checks {
			if c.Type == t.PreferredCheckType {
				checkID = c.ID
				break
			}
		}
		if checkID == "" {
			checkID = checks[0].ID
		}

		// Get 90-day daily uptime
		uptimes, err := s.store.GetDailyUptime(checkID, 90)
		if err != nil {
			slog.Warn("SLA: failed to get daily uptime", "check_id", checkID, "target_id", t.ID, "error", err)
			continue
		}

		daily := make([]slaDailyUptime, len(uptimes))
		for i, u := range uptimes {
			daily[i] = slaDailyUptime{Date: u.Date, UptimePct: u.UptimePct}
		}

		st := slaTarget{
			ID:          t.ID,
			Name:        t.Name,
			DailyUptime: daily,
		}

		cat := t.Category
		if cat == "" {
			cat = "Other"
		}
		catTargets[cat] = append(catTargets[cat], st)
	}

	// Pause stats for current calendar month
	pauseCount, pauseHosts, _ := s.store.GetMonthlyPauseStats()

	// Sort: alphabetical, "Other" last
	sort.Slice(cats, func(i, j int) bool {
		if cats[i].Value == "Other" {
			return false
		}
		if cats[j].Value == "Other" {
			return true
		}
		return cats[i].Value < cats[j].Value
	})

	resp := slaHistoryResponse{
		PauseStats: slaPauseStats{Count: pauseCount, AffectedHosts: pauseHosts},
	}
	for _, cat := range cats {
		resp.Categories = append(resp.Categories, slaCategory{
			Name:         cat.Value,
			SLAThreshold: slaThresholds[cat.Value],
			Targets:      orEmptyTargets(catTargets[cat.Value]),
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// defaultSLAThreshold applies to a category with no saved sla_<category>
// setting, e.g. one created after install. 0 still disables a category.
const defaultSLAThreshold = 99.9

// categorySLAThresholds maps each category to its SLA threshold from settings.
// A category without a saved setting gets defaultSLAThreshold; a saved value
// that does not parse is left out, so that category has no SLA.
func categorySLAThresholds(cats []store.TagOption, settings map[string]string) map[string]float64 {
	thresholds := make(map[string]float64, len(cats))
	for _, cat := range cats {
		v, ok := settings[store.CategoryToSLAKey(cat.Value)]
		if !ok {
			thresholds[cat.Value] = defaultSLAThreshold
			continue
		}
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			thresholds[cat.Value] = f
		}
	}
	return thresholds
}

func orEmptyTargets(t []slaTarget) []slaTarget {
	if t == nil {
		return []slaTarget{}
	}
	return t
}
