package api

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"time"

	"github.com/bekci/internal/crypto"
	"github.com/bekci/internal/store"
)

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.ExportBackup(s.version)
	if err != nil {
		slog.Error("Backup export failed", "error", err)
		s.audit(r, "export_backup", "backup", "", err.Error(), "failure")
		writeError(w, http.StatusInternalServerError, "backup failed")
		return
	}

	s.audit(r, "export_backup", "backup", "", "", "success")
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

	// Support both multipart form upload (frontend) and raw JSON (API/curl)
	isMultipart := false
	if ct := r.Header.Get("Content-Type"); ct != "" {
		mediaType, _, _ := mime.ParseMediaType(ct)
		if mediaType != "application/json" {
			isMultipart = true
		}
	}

	if isMultipart {
		// Multipart form: parse and read the "file" field
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			writeError(w, http.StatusBadRequest, "invalid form data: "+err.Error())
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "missing file field: "+err.Error())
			return
		}
		defer file.Close()
		if err := json.NewDecoder(file).Decode(&data); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON in uploaded file: "+err.Error())
			return
		}
	} else {
		// Raw JSON body (including no Content-Type header)
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
	}

	// Validate backup format
	if data.Version != 1 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported backup version: %d", data.Version))
		return
	}
	currentSchema := s.store.SchemaVersion()
	if data.SchemaVersion > currentSchema {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("backup schema version %d is newer than this server supports (%d)", data.SchemaVersion, currentSchema))
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
		s.audit(r, "restore_backup", "backup", "", "restore failed", "failure")
		writeError(w, http.StatusInternalServerError, "restore failed")
		return
	}

	// Reload scheduler to pick up new targets/checks
	s.scheduler.Reload()

	s.audit(r, "restore_backup", "backup", "", "from="+data.AppVersion, "success")
	slog.Warn("Database restored from backup", "backup_created", data.CreatedAt, "app_version", data.AppVersion)
	writeJSON(w, http.StatusOK, map[string]string{"message": "restore successful"})
}

func (s *Server) handleFullBackup(w http.ResponseWriter, r *http.Request) {
	encrypt := r.URL.Query().Get("encrypt") == "true"
	passphrase := r.URL.Query().Get("passphrase")

	if encrypt && len(passphrase) < 8 {
		writeError(w, http.StatusBadRequest, "passphrase must be at least 8 characters")
		return
	}

	// Create temp file for SQLite backup
	tmpDB, err := os.CreateTemp("", "bekci-fullbackup-*.db")
	if err != nil {
		slog.Error("Failed to create temp file for full backup", "error", err)
		writeError(w, http.StatusInternalServerError, "backup failed")
		return
	}
	tmpDBPath := tmpDB.Name()
	tmpDB.Close()
	defer os.Remove(tmpDBPath)

	// Perform SQLite online backup to temp file
	if err := s.store.FullBackup(tmpDBPath); err != nil {
		slog.Error("SQLite full backup failed", "error", err)
		s.audit(r, "export_full_backup", "backup", "", err.Error(), "failure")
		writeError(w, http.StatusInternalServerError, "backup failed")
		return
	}

	// Build tar.gz archive: bekci.db + config.yaml
	tmpArchive, err := os.CreateTemp("", "bekci-fullbackup-*.tar.gz")
	if err != nil {
		slog.Error("Failed to create temp archive", "error", err)
		writeError(w, http.StatusInternalServerError, "backup failed")
		return
	}
	tmpArchivePath := tmpArchive.Name()
	defer os.Remove(tmpArchivePath)

	gzw := gzip.NewWriter(tmpArchive)
	tw := tar.NewWriter(gzw)

	// Add bekci.db
	if err := addFileToTar(tw, tmpDBPath, "bekci.db"); err != nil {
		tw.Close()
		gzw.Close()
		tmpArchive.Close()
		slog.Error("Failed to add DB to archive", "error", err)
		writeError(w, http.StatusInternalServerError, "backup failed")
		return
	}

	// Add config.yaml (if it exists)
	if s.configPath != "" {
		if err := addFileToTar(tw, s.configPath, "config.yaml"); err != nil {
			slog.Warn("Could not include config.yaml in backup", "error", err)
			// Non-fatal — config may not exist (env-only setup)
		}
	}

	tw.Close()
	gzw.Close()
	tmpArchive.Close()

	// Read the archive
	archiveData, err := os.ReadFile(tmpArchivePath)
	if err != nil {
		slog.Error("Failed to read archive", "error", err)
		writeError(w, http.StatusInternalServerError, "backup failed")
		return
	}

	// Optionally encrypt
	ext := ".tar.gz"
	if encrypt {
		archiveData, err = crypto.Encrypt(archiveData, passphrase)
		if err != nil {
			slog.Error("Failed to encrypt backup", "error", err)
			writeError(w, http.StatusInternalServerError, "encryption failed")
			return
		}
		ext = ".tar.gz.enc"
	}

	s.audit(r, "export_full_backup", "backup", "", fmt.Sprintf("encrypted=%v size=%d", encrypt, len(archiveData)), "success")

	filename := fmt.Sprintf("bekci-full-%s%s", time.Now().Format("20060102-150405"), ext)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(archiveData)))
	w.Write(archiveData)
}

// addFileToTar adds a file from disk into a tar archive with the given name.
func addFileToTar(tw *tar.Writer, filePath, archiveName string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	hdr := &tar.Header{
		Name:    archiveName,
		Size:    info.Size(),
		Mode:    0600,
		ModTime: info.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}

	_, err = io.Copy(tw, f)
	return err
}

func (s *Server) handleGeneratePassphrase(w http.ResponseWriter, r *http.Request) {
	passphrase, err := crypto.GeneratePassphrase(4)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate passphrase")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"passphrase": passphrase})
}
