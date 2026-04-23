package api

import (
	"net/http"
	"strings"
)

// handleListAPITokens returns every token (active + revoked). Plaintext
// never leaves the store — callers see only metadata and the short
// `prefix` for identification.
func (s *Server) handleListAPITokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := s.store.ListAPITokens()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list api tokens")
		return
	}
	writeJSON(w, http.StatusOK, tokens)
}

// handleCreateAPIToken mints a new token and returns its plaintext ONCE.
// The admin UI must display the plaintext immediately because it cannot
// be retrieved later.
func (s *Server) handleCreateAPIToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = maxLen(strings.TrimSpace(req.Name), 80)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	claims := getClaims(r)
	creator := ""
	if claims != nil {
		creator = claims.Subject
	}

	tok, plaintext, err := s.store.CreateAPIToken(req.Name, creator)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create api token")
		return
	}
	s.audit(r, "api_token_create", "api_token", tok.ID, "name="+tok.Name, "success")

	writeJSON(w, http.StatusCreated, map[string]any{
		"token": tok,
		// Plaintext is visible ONLY in this response.
		"plaintext": plaintext,
	})
}

// handleRevokeAPIToken soft-deletes a token. Idempotent.
func (s *Server) handleRevokeAPIToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "token id required")
		return
	}
	if err := s.store.RevokeAPIToken(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to revoke api token")
		return
	}
	s.audit(r, "api_token_revoke", "api_token", id, "", "success")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
