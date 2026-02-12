package api

import (
	"net/http"
)

func (s *Server) handleListChecks(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	checks, err := s.store.ListChecksByTarget(targetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list checks")
		return
	}
	writeJSON(w, http.StatusOK, checks)
}

func (s *Server) handleRunCheckNow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	check, err := s.store.GetCheck(id)
	if err != nil || check == nil {
		writeError(w, http.StatusNotFound, "check not found")
		return
	}

	if s.scheduler != nil {
		s.scheduler.RunNow(id)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "queued"})
}

func (s *Server) handleCheckResults(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	results, err := s.store.GetRecentResults(id, 24)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get results")
		return
	}
	writeJSON(w, http.StatusOK, results)
}
