package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Known settings with their validation rules.
var knownSettings = map[string]bool{
	"session_timeout_hours":  true,
	"history_days":           true,
	"audit_retention_days":   true,
	"soc_public":             true,
	// Alerting settings
	"alert_method":     true,
	"resend_api_key":   true,
	"alert_from_email": true,
	"alert_cooldown_s": true,
	"alert_realert_s":  true,
	// SLA thresholds (per category, float 0–100)
	"sla_network":           true,
	"sla_security":          true,
	"sla_physical_security": true,
	"sla_key_services":      true,
	"sla_other":             true,
}

// Boolean settings that accept "true"/"false" instead of positive integers.
var boolSettings = map[string]bool{
	"soc_public": true,
}

// String settings that accept arbitrary text (not validated as positive integers).
var stringSettings = map[string]bool{
	"alert_method":     true,
	"resend_api_key":   true,
	"alert_from_email": true,
}

// Zero-allowed integer settings (allow 0 as a valid value, e.g. to disable re-alerting).
var zeroAllowedSettings = map[string]bool{
	"alert_cooldown_s": true,
	"alert_realert_s":  true,
}

// Upper bounds for integer settings.
var maxSettings = map[string]int{
	"session_timeout_hours":  8760,  // 1 year
	"history_days":           3650,  // 10 years
	"audit_retention_days":   3650,  // 10 years
	"alert_cooldown_s":       86400, // 1 day
	"alert_realert_s":        86400, // 1 day
}

// Float settings validated as 0–100 range (SLA thresholds).
var floatSettings = map[string]bool{
	"sla_network":           true,
	"sla_security":          true,
	"sla_physical_security": true,
	"sla_key_services":      true,
	"sla_other":             true,
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.GetAllSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}
	// Mask sensitive values — show "configured" or empty
	if v, ok := settings["resend_api_key"]; ok && v != "" {
		settings["resend_api_key"] = "••••••••"
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate: only known keys, type-appropriate values
	for key, val := range req {
		if !knownSettings[key] {
			writeError(w, http.StatusBadRequest, "unknown setting: "+key)
			return
		}
		if boolSettings[key] {
			if val != "true" && val != "false" {
				writeError(w, http.StatusBadRequest, "setting "+key+" must be 'true' or 'false'")
				return
			}
		} else if floatSettings[key] {
			// Normalize: strip trailing zeros/dot for clean storage
			f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
			if err != nil || f < 0 || f > 100 {
				writeError(w, http.StatusBadRequest, "setting "+key+" must be a number between 0 and 100")
				return
			}
		} else if stringSettings[key] {
			// Accept any string (including empty for clearing API keys)
			continue
		} else if zeroAllowedSettings[key] {
			n, err := strconv.Atoi(val)
			if err != nil || n < 0 {
				writeError(w, http.StatusBadRequest, "setting "+key+" must be a non-negative integer")
				return
			}
			if mx, ok := maxSettings[key]; ok && n > mx {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("setting %s must be at most %d", key, mx))
				return
			}
		} else {
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				writeError(w, http.StatusBadRequest, "setting "+key+" must be a positive integer")
				return
			}
			if mx, ok := maxSettings[key]; ok && n > mx {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("setting %s must be at most %d", key, mx))
				return
			}
		}
	}

	// Don't overwrite API key with the masked value
	if v, ok := req["resend_api_key"]; ok && v == "••••••••" {
		delete(req, "resend_api_key")
	}

	if err := s.store.SetSettings(req); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update settings")
		return
	}
	s.audit(r, "update_settings", "settings", "", "", "success")
	writeJSON(w, http.StatusOK, map[string]string{"message": "settings updated"})
}
