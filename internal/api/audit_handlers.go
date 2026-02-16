package api

import (
	"net/http"
	"strconv"
)

func (s *Server) handleListAuditLogs(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 50
	}
	offset := (page - 1) * limit

	entries, total, err := s.store.ListAuditEntries(limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load audit log")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
		"total":   total,
		"page":    page,
		"limit":   limit,
	})
}
