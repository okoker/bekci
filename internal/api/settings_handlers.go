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
	// Signal settings
	"signal_api_url":  true,
	"signal_number":   true,
	"signal_username": true,
	"signal_password": true,
	// Webhook settings
	"webhook_enabled":        true,
	"webhook_url":            true,
	"webhook_auth_type":      true,
	"webhook_bearer_token":   true,
	"webhook_basic_username": true,
	"webhook_basic_password": true,
	"webhook_skip_tls":       true,
	"webhook_last_error":     true,
	"webhook_last_success":   true,
	// SLA thresholds (per category, float 0–100)
	"sla_network":           true,
	"sla_security":          true,
	"sla_physical_security": true,
	"sla_key_services":      true,
	"sla_other":             true,
}

// Boolean settings that accept "true"/"false" instead of positive integers.
var boolSettings = map[string]bool{
	"soc_public":      true,
	"webhook_enabled": true,
	"webhook_skip_tls": true,
}

// String settings that accept arbitrary text (not validated as positive integers).
var stringSettings = map[string]bool{
	"alert_method":     true,
	"resend_api_key":   true,
	"alert_from_email": true,
	"signal_api_url":   true,
	"signal_number":    true,
	"signal_username":      true,
	"signal_password":      true,
	"webhook_url":            true,
	"webhook_auth_type":      true,
	"webhook_bearer_token":   true,
	"webhook_basic_username": true,
	"webhook_basic_password": true,
	"webhook_last_error":     true,
	"webhook_last_success":   true,
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
	// Strip stale/unknown keys so frontend never sees them
	for key := range settings {
		if !knownSettings[key] {
			delete(settings, key)
		}
	}
	// Mask sensitive values — show "configured" or empty
	if v, ok := settings["resend_api_key"]; ok && v != "" {
		settings["resend_api_key"] = "••••••••"
	}
	if v, ok := settings["signal_password"]; ok && v != "" {
		settings["signal_password"] = "••••••••"
	}
	if v, ok := settings["webhook_bearer_token"]; ok && v != "" {
		settings["webhook_bearer_token"] = "••••••••"
	}
	if v, ok := settings["webhook_basic_password"]; ok && v != "" {
		settings["webhook_basic_password"] = "••••••••"
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
			s.audit(r, "update_settings", "settings", "", "unknown key="+key, "failure")
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
		} else if key == "alert_method" {
			if val != "" && val != "email" && val != "signal" && val != "email+signal" {
				writeError(w, http.StatusBadRequest, "alert_method must be '', 'email', 'signal', or 'email+signal'")
				return
			}
		} else if key == "webhook_url" {
			if val != "" && !strings.HasPrefix(val, "http://") && !strings.HasPrefix(val, "https://") {
				writeError(w, http.StatusBadRequest, "webhook_url must start with http:// or https://")
				return
			}
		} else if key == "webhook_auth_type" {
			if val != "" && val != "bearer" && val != "basic" {
				writeError(w, http.StatusBadRequest, "webhook_auth_type must be '', 'bearer', or 'basic'")
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

	// Don't overwrite masked values
	if v, ok := req["resend_api_key"]; ok && v == "••••••••" {
		delete(req, "resend_api_key")
	}
	if v, ok := req["signal_password"]; ok && v == "••••••••" {
		delete(req, "signal_password")
	}
	if v, ok := req["webhook_bearer_token"]; ok && v == "••••••••" {
		delete(req, "webhook_bearer_token")
	}
	if v, ok := req["webhook_basic_password"]; ok && v == "••••••••" {
		delete(req, "webhook_basic_password")
	}

	if err := s.store.SetSettings(req); err != nil {
		s.audit(r, "update_settings", "settings", "", "failed", "failure")
		writeError(w, http.StatusInternalServerError, "failed to update settings")
		return
	}
	if _, ok := req["soc_public"]; ok {
		s.invalidateSocPublicCache()
	}
	s.audit(r, "update_settings", "settings", "", "", "success")
	writeJSON(w, http.StatusOK, map[string]string{"message": "settings updated"})
}
