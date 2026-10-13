package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/bekci/internal/store"
)

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	group := r.URL.Query().Get("group")
	if group != "project" && group != "location" && group != "category" && group != "tag" {
		writeError(w, http.StatusBadRequest, "group must be 'project', 'location', 'category', or 'tag'")
		return
	}
	tags, err := s.store.ListTagOptions(group)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tags")
		return
	}
	if group == "category" {
		// Clients use the server's key instead of deriving it: lowercasing
		// "İ" differs between Go and JavaScript.
		type categoryOption struct {
			store.TagOption
			SLAKey string `json:"sla_key"`
		}
		cats := make([]categoryOption, len(tags))
		for i, t := range tags {
			cats[i] = categoryOption{t, store.CategoryToSLAKey(t.Value)}
		}
		writeJSON(w, http.StatusOK, cats)
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
	req.Value = maxLen(strings.TrimSpace(req.Value), 50)
	if req.Group != "project" && req.Group != "location" && req.Group != "category" && req.Group != "tag" {
		writeError(w, http.StatusBadRequest, "group must be 'project', 'location', 'category', or 'tag'")
		return
	}
	if req.Value == "" {
		writeError(w, http.StatusBadRequest, "value is required")
		return
	}
	tag, err := s.store.CreateTagOption(req.Group, req.Value)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "already exists") {
			s.audit(r, "create_tag", "tag", "", "group="+req.Group+" value="+req.Value+" duplicate", "failure")
			writeError(w, http.StatusConflict, "tag value already exists")
			return
		}
		s.audit(r, "create_tag", "tag", "", "group="+req.Group+" value="+req.Value+" failed", "failure")
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
		if catErr, ok := err.(*store.CategoryInUseError); ok {
			s.audit(r, "delete_tag", "tag", idStr, fmt.Sprintf("category in use by %d targets", len(catErr.Targets)), "failure")
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":   catErr.Error(),
				"targets": catErr.Targets,
			})
			return
		}
		if strings.Contains(err.Error(), "not found") {
			s.audit(r, "delete_tag", "tag", idStr, "not found", "failure")
			writeError(w, http.StatusNotFound, "tag not found")
			return
		}
		if strings.Contains(err.Error(), "cannot delete") {
			s.audit(r, "delete_tag", "tag", idStr, err.Error(), "failure")
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.audit(r, "delete_tag", "tag", idStr, "failed", "failure")
		writeError(w, http.StatusInternalServerError, "failed to delete tag")
		return
	}
	s.audit(r, "delete_tag", "tag", idStr, "", "success")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRenameTag(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tag id")
		return
	}
	var req struct {
		Value string `json:"value"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Value = maxLen(strings.TrimSpace(req.Value), 50)
	if req.Value == "" {
		writeError(w, http.StatusBadRequest, "value is required")
		return
	}
	if err := s.store.RenameTagOption(id, req.Value); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.audit(r, "rename_tag", "tag", idStr, "not found", "failure")
			writeError(w, http.StatusNotFound, "tag not found")
			return
		}
		if strings.Contains(err.Error(), "already exists") {
			s.audit(r, "rename_tag", "tag", idStr, "new_value="+req.Value+" duplicate", "failure")
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		if strings.Contains(err.Error(), "cannot rename") {
			s.audit(r, "rename_tag", "tag", idStr, err.Error(), "failure")
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.audit(r, "rename_tag", "tag", idStr, "new_value="+req.Value+" failed", "failure")
		writeError(w, http.StatusInternalServerError, "failed to rename tag")
		return
	}
	s.audit(r, "rename_tag", "tag", idStr, "new_value="+req.Value, "success")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
