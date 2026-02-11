package api

import (
	"net/http"
	"strconv"
)

// Known settings with their validation rules.
var knownSettings = map[string]bool{
	"session_timeout_hours":  true,
	"history_days":           true,
	"default_check_interval": true,
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.GetAllSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate: only known keys, must be positive integers
	for key, val := range req {
		if !knownSettings[key] {
			writeError(w, http.StatusBadRequest, "unknown setting: "+key)
			return
		}
		n, err := strconv.Atoi(val)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "setting "+key+" must be a positive integer")
			return
		}
	}

	if err := s.store.SetSettings(req); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "settings updated"})
}
