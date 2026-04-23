package api

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bekci/internal/store"
)

// v1HostSnapshot is the single-target response shape for GET /api/v1/hosts.
// Purpose: give a remote consumer a point-in-time view of the host's
// preferred check without round-tripping the richer target detail API.
type v1HostSnapshot struct {
	TargetID  string       `json:"target_id"`
	Name      string       `json:"name"`
	Host      string       `json:"host"`
	Project   string       `json:"project"`
	Location  string       `json:"location"`
	Contacts  string       `json:"contacts"`
	Notes     string       `json:"notes"`
	Tags      []string     `json:"tags"`
	LastCheck v1LastCheck  `json:"last_check"`
}

type v1LastCheck struct {
	Status     string  `json:"status"` // "up" | "down" | "none"
	CheckType  string  `json:"check_type,omitempty"`
	CheckedAt  *string `json:"checked_at,omitempty"`
	Message    string  `json:"message,omitempty"`
	ResponseMs int64   `json:"response_ms,omitempty"`
}

// handleV1Hosts returns an array of targets whose host field
// case-insensitively matches the ?host= query param. Each element carries
// the preferred check's last result plus project/location/contacts/
// notes/tags. Returns {"targets":[]} when no match.
//
// Always returns an array (even for a single match) so machine consumers
// have one branch-free code path.
func (s *Server) handleV1Hosts(w http.ResponseWriter, r *http.Request) {
	host := strings.TrimSpace(r.URL.Query().Get("host"))
	if host == "" {
		writeError(w, http.StatusBadRequest, "query param 'host' is required")
		return
	}

	all, err := s.store.ListTargets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list targets")
		return
	}

	// Case-insensitive exact match on targets.host.
	var matched []*store.Target
	for i := range all {
		if strings.EqualFold(strings.TrimSpace(all[i].Host), host) {
			matched = append(matched, &all[i])
		}
	}

	snapshots := make([]v1HostSnapshot, 0, len(matched))
	for _, t := range matched {
		tags, _ := s.store.ListTagsByTarget(t.ID)
		snap := v1HostSnapshot{
			TargetID: t.ID,
			Name:     t.Name,
			Host:     t.Host,
			Project:  deref(t.Project),
			Location: deref(t.Location),
			Contacts: deref(t.Contacts),
			Notes:    deref(t.Notes),
			Tags:     tags,
		}
		if snap.Tags == nil {
			snap.Tags = []string{}
		}

		// Pick the preferred check, fall back to first check.
		checks, _ := s.store.ListChecksByTarget(t.ID)
		var preferred *int // index into checks
		for i := range checks {
			if checks[i].Type == t.PreferredCheckType {
				idx := i
				preferred = &idx
				break
			}
		}
		if preferred == nil && len(checks) > 0 {
			idx := 0
			preferred = &idx
		}

		if preferred == nil {
			snap.LastCheck = v1LastCheck{Status: "none"}
		} else {
			c := checks[*preferred]
			res, _ := s.store.GetLastResult(c.ID)
			if res == nil {
				snap.LastCheck = v1LastCheck{Status: "none", CheckType: c.Type}
			} else {
				ts := res.CheckedAt.UTC().Format(time.RFC3339)
				snap.LastCheck = v1LastCheck{
					Status:     res.Status,
					CheckType:  c.Type,
					CheckedAt:  &ts,
					Message:    res.Message,
					ResponseMs: res.ResponseMs,
				}
			}
		}

		snapshots = append(snapshots, snap)
	}

	// Lightweight audit trail: who hit this, what they asked, how many matched.
	tokName, _ := r.Context().Value(ctxAPITokenName).(string)
	slog.Info("v1.hosts query",
		"token", tokName,
		"host", host,
		"matched", len(snapshots),
		"ip", clientIP(r),
	)

	writeJSON(w, http.StatusOK, map[string]any{"targets": snapshots})
}

// deref unwraps a *string, returning "" when nil — keeps the JSON shape
// predictable (always strings, never null for these fields).
func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
