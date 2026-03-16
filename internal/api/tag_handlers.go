package api

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	group := r.URL.Query().Get("group")
	if group != "project" && group != "location" {
		writeError(w, http.StatusBadRequest, "group must be 'project' or 'location'")
		return
	}
	tags, err := s.store.ListTagOptions(group)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tags")
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

func (s *Server) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Group string `json:"group"`
		Value string `json:"value"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Value = strings.TrimSpace(req.Value)
	if req.Group != "project" && req.Group != "location" {
		writeError(w, http.StatusBadRequest, "group must be 'project' or 'location'")
		return
	}
	if req.Value == "" {
		writeError(w, http.StatusBadRequest, "value is required")
		return
	}
	tag, err := s.store.CreateTagOption(req.Group, req.Value)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "tag value already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create tag")
		return
	}
	s.audit(r, "create_tag", "tag", strconv.Itoa(tag.ID), "group="+req.Group+" value="+req.Value, "success")
	writeJSON(w, http.StatusCreated, tag)
}

func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tag id")
		return
	}
	if err := s.store.DeleteTagOption(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "tag not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete tag")
		return
	}
	s.audit(r, "delete_tag", "tag", idStr, "", "success")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
