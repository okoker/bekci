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
	encrypt := r.URL.Query().Get("encrypt") == "true"
	passphrase := r.URL.Query().Get("passphrase")

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

	s.audit(r, "export_full_backup", "backup", "", fmt.Sprintf("encrypted=%v size=%d", encrypt, len(archiveData)), "success")

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

func saveBackupIndex(backupDir string, entries []backupMeta) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(backupDir, "index.json"), data, 0600)
}

func (s *Server) handleSaveFullBackup(w http.ResponseWriter, r *http.Request) {
	encrypt := r.URL.Query().Get("encrypt") == "true"
	passphrase := r.URL.Query().Get("passphrase")

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

	// Ensure backup directory exists
	if err := os.MkdirAll(s.backupDir, 0755); err != nil {
		slog.Error("Failed to create backup directory", "error", err, "path", s.backupDir)
		writeError(w, http.StatusInternalServerError, "backup failed")
		return
	}

	// Write file
	destPath := filepath.Join(s.backupDir, filename)
	if err := os.WriteFile(destPath, archiveData, 0600); err != nil {
		slog.Error("Failed to write backup file", "error", err)
		writeError(w, http.StatusInternalServerError, "backup failed")
		return
	}

	// Compute SHA256
	hash := sha256.Sum256(archiveData)
	hashStr := hex.EncodeToString(hash[:])

	// Update index
	entries := loadBackupIndex(s.backupDir)
	entries = append(entries, backupMeta{
		Filename:  filename,
		SHA256:    hashStr,
		Size:      int64(len(archiveData)),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Encrypted: encrypt,
	})
	if err := saveBackupIndex(s.backupDir, entries); err != nil {
		slog.Warn("Failed to update backup index", "error", err)
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
	absBackupDir, _ := filepath.Abs(s.backupDir)
	absPath, _ := filepath.Abs(fullPath)
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

	s.audit(r, "download_saved_backup", "backup", "", filename, "success")

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	io.Copy(w, f)
}

func (s *Server) handleDeleteSavedBackup(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")
	if !validBackupFilename.MatchString(filename) {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}

	fullPath := filepath.Join(s.backupDir, filename)

	// Safety: ensure resolved path is within backup dir
	absBackupDir, _ := filepath.Abs(s.backupDir)
	absPath, _ := filepath.Abs(fullPath)
	if absPath != filepath.Join(absBackupDir, filename) {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}

	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "backup not found")
			return
		}
		slog.Error("Failed to delete backup", "error", err)
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
