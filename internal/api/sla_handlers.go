package api

import (
	"net/http"
	"strconv"
	"strings"
)

// Fixed category ordering for SLA page.
var slaCategories = []string{"Network", "Security", "Physical Security", "Key Services", "Other"}

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

type slaHistoryResponse struct {
	Categories []slaCategory `json:"categories"`
}

func (s *Server) handleSLAHistory(w http.ResponseWriter, r *http.Request) {
	targets, err := s.store.ListTargets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list targets")
		return
	}

	// Load SLA thresholds
	allSettings, _ := s.store.GetAllSettings()
	slaThresholds := make(map[string]float64)
	for cat, key := range categoryToSLAKey {
		if v, ok := allSettings[key]; ok {
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				slaThresholds[cat] = f
			}
		}
	}

	// Group targets by category
	catTargets := make(map[string][]slaTarget)
	for _, t := range targets {
		// Find preferred check
		checks, err := s.store.ListChecksByTarget(t.ID)
		if err != nil || len(checks) == 0 {
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

	// Build ordered response: known categories first, then any unknown
	resp := slaHistoryResponse{}
	seen := make(map[string]bool)

	for _, cat := range slaCategories {
		seen[cat] = true
		resp.Categories = append(resp.Categories, slaCategory{
			Name:         cat,
			SLAThreshold: slaThresholds[cat],
			Targets:      orEmptyTargets(catTargets[cat]),
		})
	}

	// Append any unknown categories
	for cat, tgts := range catTargets {
		if !seen[cat] {
			resp.Categories = append(resp.Categories, slaCategory{
				Name:         cat,
				SLAThreshold: slaThresholds[cat],
				Targets:      tgts,
			})
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func orEmptyTargets(t []slaTarget) []slaTarget {
	if t == nil {
		return []slaTarget{}
	}
	return t
}
