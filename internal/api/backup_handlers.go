package api

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/bekci/internal/alerter"
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
	filename := fmt.Sprintf("bekci-config-%s.json", time.Now().Format("20060102-150405"))
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
			slog.Error("Restore: invalid form data", "error", err)
			writeError(w, http.StatusBadRequest, "invalid form data")
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			slog.Error("Restore: missing file field", "error", err)
			writeError(w, http.StatusBadRequest, "missing file field")
			return
		}
		defer file.Close()
		if err := json.NewDecoder(file).Decode(&data); err != nil {
			slog.Error("Restore: invalid JSON in file", "error", err)
			writeError(w, http.StatusBadRequest, "invalid JSON in uploaded file")
			return
		}
	} else {
		// Raw JSON body (including no Content-Type header)
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			slog.Error("Restore: invalid JSON body", "error", err)
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
	}

	// Validate backup format
	if data.Version != 1 && data.Version != 2 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported backup version: %d", data.Version))
		return
	}
	currentSchema, err := s.store.SchemaVersion()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read current schema version")
		return
	}
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

// buildFullBackupArchive creates a full backup archive (tar.gz), optionally encrypted.
// Returns the archive data, filename, and any error.
func (s *Server) buildFullBackupArchive(encrypt bool, passphrase string) ([]byte, string, error) {
	// Create temp file for SQLite backup
	tmpDB, err := os.CreateTemp("", "bekci-fullbackup-*.db")
	if err != nil {
		return nil, "", fmt.Errorf("creating temp file: %w", err)
	}
	tmpDBPath := tmpDB.Name()
	tmpDB.Close()
	defer os.Remove(tmpDBPath)

	// Perform SQLite online backup to temp file
	if err := s.store.FullBackup(tmpDBPath); err != nil {
		return nil, "", fmt.Errorf("SQLite backup: %w", err)
	}

	// Build tar.gz archive: bekci.db + config.yaml
	tmpArchive, err := os.CreateTemp("", "bekci-fullbackup-*.tar.gz")
	if err != nil {
		return nil, "", fmt.Errorf("creating temp archive: %w", err)
	}
	tmpArchivePath := tmpArchive.Name()
	defer os.Remove(tmpArchivePath)

	gzw := gzip.NewWriter(tmpArchive)
	tw := tar.NewWriter(gzw)

	if err := addFileToTar(tw, tmpDBPath, "bekci.db"); err != nil {
		tw.Close()
		gzw.Close()
		tmpArchive.Close()
		return nil, "", fmt.Errorf("adding DB to archive: %w", err)
	}

	if s.configPath != "" {
		if err := addFileToTar(tw, s.configPath, "config.yaml"); err != nil {
			slog.Warn("Could not include config.yaml in backup", "error", err)
		}
	}

	tw.Close()
	gzw.Close()
	tmpArchive.Close()

	archiveData, err := os.ReadFile(tmpArchivePath)
	if err != nil {
		return nil, "", fmt.Errorf("reading archive: %w", err)
	}

	ext := ".tar.gz"
	if encrypt {
		archiveData, err = crypto.Encrypt(archiveData, passphrase)
		if err != nil {
			return nil, "", fmt.Errorf("encrypting: %w", err)
		}
		ext = ".tar.gz.enc"
	}

	filename := fmt.Sprintf("bekci-full-%s%s", time.Now().Format("20060102-150405"), ext)
	return archiveData, filename, nil
}

func (s *Server) handleFullBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Encrypt    bool   `json:"encrypt"`
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	encrypt := req.Encrypt
	passphrase := req.Passphrase

	if encrypt && len(passphrase) < 8 {
		writeError(w, http.StatusBadRequest, "passphrase must be at least 8 characters")
		return
	}

	archiveData, filename, err := s.buildFullBackupArchive(encrypt, passphrase)
	if err != nil {
		slog.Error("Full backup failed", "error", err)
		s.audit(r, "export_full_backup", "backup", "", err.Error(), "failure")
		writeError(w, http.StatusInternalServerError, "backup failed")
		return
	}

	// Keep a server copy before sending, so it is already listed when the download
	// arrives and is kept even if the download does not get through
	if _, err := s.saveBackupCopy(archiveData, filename, encrypt); err != nil {
		slog.Warn("Auto-save backup copy failed", "filename", filename, "error", err)
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(archiveData)))
	detail := fmt.Sprintf("encrypted=%v size=%d", encrypt, len(archiveData))
	if _, err := w.Write(archiveData); err != nil {
		slog.Error("Failed to send full backup", "filename", filename, "error", err)
		s.audit(r, "export_full_backup", "backup", "", detail+" error="+err.Error(), "failure")
	} else {
		s.audit(r, "export_full_backup", "backup", "", detail, "success")
	}
}

// saveBackupCopy stores a backup archive in s.backupDir and prunes the oldest
// copies beyond backup_max_copies. Used by Save to server, the download
// auto-save and scheduled backups. It never overwrites an existing file, and on
// error the directory and index are left as they were. Returns the truncated
// SHA256 recorded in the index.
func (s *Server) saveBackupCopy(data []byte, filename string, encrypted bool) (string, error) {
	maxStr, _ := s.store.GetSetting("backup_max_copies")
	maxCopies, _ := strconv.Atoi(maxStr)
	if maxCopies <= 0 {
		maxCopies = 5
	}

	s.backupMu.Lock()
	defer s.backupMu.Unlock()

	if err := os.MkdirAll(s.backupDir, 0700); err != nil {
		return "", fmt.Errorf("creating backup directory: %w", err)
	}

	entries := loadBackupIndex(s.backupDir)

	// O_EXCL: a name that already exists (same-second save) fails instead of overwriting
	destPath := filepath.Join(s.backupDir, filename)
	f, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", fmt.Errorf("creating backup file: %w", err)
	}
	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(destPath)
		return "", fmt.Errorf("writing backup file: %w", err)
	}

	// Compute SHA256 (store truncated to 16 hex chars — enough for integrity check)
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])[:16]

	entries = append(entries, backupMeta{
		Filename:  filename,
		SHA256:    hashStr,
		Size:      int64(len(data)),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Encrypted: encrypted,
	})

	// Drop the oldest copies beyond the limit from the index first and delete
	// their files only once the new index is saved
	var pruned []backupMeta
	if len(entries) > maxCopies {
		pruned = entries[:len(entries)-maxCopies]
		entries = entries[len(entries)-maxCopies:]
	}
	if err := saveBackupIndex(s.backupDir, entries); err != nil {
		os.Remove(destPath)
		return "", fmt.Errorf("updating backup index: %w", err)
	}
	for _, old := range pruned {
		if err := os.Remove(filepath.Join(s.backupDir, old.Filename)); err != nil {
			slog.Warn("Failed to prune old backup", "filename", old.Filename, "error", err)
		}
	}

	slog.Info("Saved backup copy", "filename", filename, "size", len(data))
	return hashStr, nil
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

// ── Server-side backup storage ──

type backupMeta struct {
	Filename  string `json:"filename"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"created_at"`
	Encrypted bool   `json:"encrypted"`
}

var validBackupFilename = regexp.MustCompile(`^bekci-full-\d{8}-\d{6}\.tar\.gz(\.enc)?$`)

func loadBackupIndex(backupDir string) []backupMeta {
	data, err := os.ReadFile(filepath.Join(backupDir, "index.json"))
	if err != nil {
		return []backupMeta{}
	}
	var entries []backupMeta
	if err := json.Unmarshal(data, &entries); err != nil {
		return []backupMeta{}
	}
	// Filter out entries whose files no longer exist on disk
	valid := make([]backupMeta, 0, len(entries))
	for _, e := range entries {
		path := filepath.Join(backupDir, e.Filename)
		if _, err := os.Stat(path); err == nil {
			valid = append(valid, e)
		}
	}
	return valid
}

// saveBackupIndex replaces index.json via a temp file and rename, so a crash or
// a concurrent reader never sees a partial index. Writers hold s.backupMu.
func saveBackupIndex(backupDir string, entries []backupMeta) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(backupDir, "index.json")
	if err := os.WriteFile(path+".tmp", data, 0600); err != nil {
		os.Remove(path + ".tmp")
		return err
	}
	return os.Rename(path+".tmp", path)
}

func (s *Server) handleSaveFullBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Encrypt    bool   `json:"encrypt"`
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	encrypt := req.Encrypt
	passphrase := req.Passphrase

	if encrypt && len(passphrase) < 8 {
		writeError(w, http.StatusBadRequest, "passphrase must be at least 8 characters")
		return
	}

	archiveData, filename, err := s.buildFullBackupArchive(encrypt, passphrase)
	if err != nil {
		slog.Error("Full backup save failed", "error", err)
		s.audit(r, "save_full_backup", "backup", "", err.Error(), "failure")
		writeError(w, http.StatusInternalServerError, "backup failed")
		return
	}

	hashStr, err := s.saveBackupCopy(archiveData, filename, encrypt)
	if err != nil {
		slog.Error("Full backup save failed", "error", err, "path", s.backupDir)
		s.audit(r, "save_full_backup", "backup", "", err.Error(), "failure")
		writeError(w, http.StatusInternalServerError, "backup failed")
		return
	}

	s.audit(r, "save_full_backup", "backup", "", fmt.Sprintf("encrypted=%v size=%d file=%s", encrypt, len(archiveData), filename), "success")
	writeJSON(w, http.StatusOK, map[string]string{"message": "backup saved", "filename": filename, "sha256": hashStr})
}

func (s *Server) handleListSavedBackups(w http.ResponseWriter, r *http.Request) {
	entries := loadBackupIndex(s.backupDir)
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleDownloadSavedBackup(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")
	if !validBackupFilename.MatchString(filename) {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}

	fullPath := filepath.Join(s.backupDir, filename)

	// Safety: ensure resolved path is within backup dir
	absBackupDir, err := filepath.Abs(s.backupDir)
	if err != nil {
		slog.Error("Failed to resolve backup dir", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		slog.Error("Failed to resolve backup path", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if absPath != filepath.Join(absBackupDir, filename) {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}

	f, err := os.Open(fullPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read backup")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	if _, err := io.Copy(w, f); err != nil {
		slog.Error("Failed to stream backup file", "filename", filename, "error", err)
		s.audit(r, "download_saved_backup", "backup", "", filename+" error="+err.Error(), "failure")
		return
	}
	s.audit(r, "download_saved_backup", "backup", "", filename, "success")
}

func (s *Server) handleDeleteSavedBackup(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")
	if !validBackupFilename.MatchString(filename) {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}

	fullPath := filepath.Join(s.backupDir, filename)

	// Safety: ensure resolved path is within backup dir
	absBackupDir, err := filepath.Abs(s.backupDir)
	if err != nil {
		slog.Error("Failed to resolve backup dir", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		slog.Error("Failed to resolve backup path", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if absPath != filepath.Join(absBackupDir, filename) {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}

	s.backupMu.Lock()
	defer s.backupMu.Unlock()

	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			s.audit(r, "delete_saved_backup", "backup", "", filename+" not found", "failure")
			writeError(w, http.StatusNotFound, "backup not found")
			return
		}
		slog.Error("Failed to delete backup", "error", err)
		s.audit(r, "delete_saved_backup", "backup", "", filename+" error="+err.Error(), "failure")
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}

	// Update index — remove entry
	entries := loadBackupIndex(s.backupDir)
	filtered := make([]backupMeta, 0, len(entries))
	for _, e := range entries {
		if e.Filename != filename {
			filtered = append(filtered, e)
		}
	}
	if err := saveBackupIndex(s.backupDir, filtered); err != nil {
		slog.Warn("Failed to update backup index", "error", err)
	}

	s.audit(r, "delete_saved_backup", "backup", "", filename, "success")
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// RunScheduledBackup creates an unencrypted full backup if the schedule is due.
// Called from the hourly maintenance goroutine in main.go.
func (s *Server) RunScheduledBackup() {
	schedule, _ := s.store.GetSetting("auto_backup_schedule")
	if schedule == "" || schedule == "off" {
		return
	}

	timeStr, _ := s.store.GetSetting("auto_backup_time")
	if timeStr == "" {
		timeStr = "03:00"
	}

	// Parse target hour:minute
	var targetHour, targetMin int
	fmt.Sscanf(timeStr, "%d:%d", &targetHour, &targetMin)

	now := time.Now()

	// Only proceed if current hour matches target hour (checked hourly)
	if now.Hour() != targetHour {
		return
	}

	// Check if a backup was already made today
	entries := loadBackupIndex(s.backupDir)
	if len(entries) > 0 {
		lastEntry := entries[len(entries)-1]
		if lastTime, err := time.Parse(time.RFC3339, lastEntry.CreatedAt); err == nil {
			daysSince := int(now.Sub(lastTime).Hours() / 24)
			switch schedule {
			case "weekly":
				if daysSince < 7 {
					return
				}
			case "10days":
				if daysSince < 10 {
					return
				}
			case "monthly":
				if daysSince < 30 {
					return
				}
			}
		}
	}

	// Audit: starting
	s.store.CreateAuditEntry(&store.AuditEntry{
		UserID:       "system",
		Username:     "system",
		Action:       "auto_backup",
		ResourceType: "backup",
		Detail:       "Starting scheduled backup",
		Status:       "success",
	})

	// Build backup (unencrypted) and store it (respects max_copies)
	archiveData, filename, err := s.buildFullBackupArchive(false, "")
	if err == nil {
		_, err = s.saveBackupCopy(archiveData, filename, false)
	}
	if err != nil {
		slog.Error("Scheduled backup failed", "error", err)
		s.store.CreateAuditEntry(&store.AuditEntry{
			UserID:       "system",
			Username:     "system",
			Action:       "auto_backup",
			ResourceType: "backup",
			Detail:       "Backup failed: " + err.Error(),
			Status:       "failure",
		})
		alerter.SendSystemAlert(s.store,
			"Bekci scheduled backup failed",
			"The scheduled backup could not be completed. Check audit log for details.",
		)
		return
	}

	// Audit: completed
	s.store.CreateAuditEntry(&store.AuditEntry{
		UserID:       "system",
		Username:     "system",
		Action:       "auto_backup",
		ResourceType: "backup",
		Detail:       fmt.Sprintf("Backup completed: %s (%d bytes)", filename, len(archiveData)),
		Status:       "success",
	})

	slog.Info("Scheduled backup completed", "filename", filename, "size", len(archiveData))
}
