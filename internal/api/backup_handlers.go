package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/bekci/internal/store"
)

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.ExportBackup(s.version)
	if err != nil {
		slog.Error("Backup export failed", "error", err)
		writeError(w, http.StatusInternalServerError, "backup failed")
		return
	}

	filename := fmt.Sprintf("bekci-backup-%s.json", time.Now().Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	json.NewEncoder(w).Encode(data)
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	// 10MB limit (config-only backups are small, but be generous)
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	defer r.Body.Close()

	var data store.BackupData
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Validate backup format
	if data.Version != 1 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported backup version: %d", data.Version))
		return
	}
	if data.SchemaVersion > 8 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("backup schema version %d is newer than this server supports (8)", data.SchemaVersion))
		return
	}

	// Ensure at least one active admin exists in the backup
	hasActiveAdmin := false
	for _, u := range data.Users {
		if u.Role == "admin" && u.Status == "active" {
			hasActiveAdmin = true
			break
		}
	}
	if !hasActiveAdmin {
		writeError(w, http.StatusBadRequest, "backup must contain at least one active admin user")
		return
	}

	// Perform restore
	if err := s.store.RestoreBackup(&data); err != nil {
		slog.Error("Backup restore failed", "error", err)
		writeError(w, http.StatusInternalServerError, "restore failed: "+err.Error())
		return
	}

	// Reload scheduler to pick up new targets/checks
	s.scheduler.Reload()

	s.audit(r, "restore_backup", "backup", "", "from="+data.AppVersion, "success")
	slog.Warn("Database restored from backup", "backup_created", data.CreatedAt, "app_version", data.AppVersion)
	writeJSON(w, http.StatusOK, map[string]string{"message": "restore successful"})
}
