package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bekci/internal/crypto"

	"gopkg.in/yaml.v3"
)

func runRestoreFull() {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("=============================================================")
	fmt.Println("  Bekci Full Restore")
	fmt.Println("=============================================================")
	fmt.Println()
	fmt.Println("This will REPLACE the current database and optionally the")
	fmt.Println("config file. The Bekci service should be STOPPED before")
	fmt.Println("proceeding.")
	fmt.Println()

	// Get archive path
	var archivePath string
	if len(os.Args) > 2 {
		archivePath = os.Args[2]
	} else {
		fmt.Print("Archive path: ")
		if !scanner.Scan() {
			fmt.Println("Aborted.")
			os.Exit(1)
		}
		archivePath = strings.TrimSpace(scanner.Text())
	}

	if archivePath == "" {
		fmt.Println("Error: no archive path provided.")
		os.Exit(1)
	}

	// Read archive
	data, err := os.ReadFile(archivePath)
	if err != nil {
		fmt.Printf("Error reading archive: %v\n", err)
		os.Exit(1)
	}

	// Detect encrypted archive
	isEncrypted := strings.HasSuffix(archivePath, ".enc")
	if isEncrypted {
		fmt.Print("Passphrase: ")
		if !scanner.Scan() {
			fmt.Println("Aborted.")
			os.Exit(1)
		}
		passphrase := strings.TrimSpace(scanner.Text())

		data, err = crypto.Decrypt(data, passphrase)
		if err != nil {
			fmt.Printf("Decryption failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Decrypted successfully.")
	}

	// Extract tar.gz to temp dir
	tmpDir, err := os.MkdirTemp("", "bekci-restore-*")
	if err != nil {
		fmt.Printf("Error creating temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	if err := extractTarGz(data, tmpDir); err != nil {
		fmt.Printf("Error extracting archive: %v\n", err)
		os.Exit(1)
	}

	// Check extracted files
	dbPath := filepath.Join(tmpDir, "bekci.db")
	configPath := filepath.Join(tmpDir, "config.yaml")

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Println("Error: archive does not contain bekci.db")
		os.Exit(1)
	}

	hasConfig := true
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		hasConfig = false
	}

	// Show bundled config if present
	var bundledConfig map[string]interface{}
	if hasConfig {
		configData, _ := os.ReadFile(configPath)
		fmt.Println()
		fmt.Println("--- Bundled config.yaml ---")
		fmt.Println(string(configData))
		fmt.Println("---------------------------")
		yaml.Unmarshal(configData, &bundledConfig)
	}

	// Ask about config usage
	useConfig := false
	if hasConfig {
		fmt.Print("\nUse bundled config.yaml? (Y/n): ")
		if scanner.Scan() {
			answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
			useConfig = answer == "" || answer == "y" || answer == "yes"
		}
	}

	// If not using bundled config, run interactive wizard
	var wizardConfig *wizardResult
	if !useConfig {
		wizardConfig = runConfigWizard(scanner, bundledConfig)
	}

	// Get destination paths
	fmt.Println()
	destDBPath := promptWithDefault(scanner, "Destination DB path", "/var/lib/bekci/bekci.db")
	destConfigPath := promptWithDefault(scanner, "Destination config path", "/etc/bekci/config.yaml")

	// Summary
	fmt.Println()
	fmt.Println("=============================================================")
	fmt.Println("  Restore Summary")
	fmt.Println("=============================================================")
	fmt.Printf("  Database:  %s -> %s\n", filepath.Base(archivePath), destDBPath)
	if useConfig {
		fmt.Printf("  Config:    bundled config.yaml -> %s\n", destConfigPath)
	} else if wizardConfig != nil {
		fmt.Printf("  Config:    custom (wizard) -> %s\n", destConfigPath)
	} else {
		fmt.Println("  Config:    not changed")
	}
	fmt.Println("=============================================================")
	fmt.Println()
	fmt.Print("Proceed? (y/N): ")
	if !scanner.Scan() {
		fmt.Println("Aborted.")
		os.Exit(1)
	}
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	if answer != "y" && answer != "yes" {
		fmt.Println("Aborted.")
		os.Exit(0)
	}

	// Perform restore
	fmt.Println()

	// Copy database
	if err := copyFile(dbPath, destDBPath); err != nil {
		fmt.Printf("Error copying database: %v\n", err)
		os.Exit(1)
	}
	os.Chmod(destDBPath, 0600)
	fmt.Printf("Database restored to %s\n", destDBPath)

	// Write config
	if useConfig && hasConfig {
		if err := copyFile(configPath, destConfigPath); err != nil {
			fmt.Printf("Error copying config: %v\n", err)
			os.Exit(1)
		}
		os.Chmod(destConfigPath, 0600)
		fmt.Printf("Config restored to %s\n", destConfigPath)
	} else if wizardConfig != nil {
		configYAML := wizardConfig.toYAML()
		if err := os.WriteFile(destConfigPath, []byte(configYAML), 0600); err != nil {
			fmt.Printf("Error writing config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Config written to %s\n", destConfigPath)
	}

	fmt.Println()
	fmt.Println("Restore complete. Start the service with:")
	fmt.Println("  sudo systemctl start bekci")
}

type wizardResult struct {
	Port      string
	DBPath    string
	LogLevel  string
	LogPath   string
	Username  string
	Password  string
}

func (w *wizardResult) toYAML() string {
	var sb strings.Builder
	sb.WriteString("server:\n")
	sb.WriteString(fmt.Sprintf("  port: %s\n", w.Port))
	sb.WriteString(fmt.Sprintf("  db_path: %s\n", w.DBPath))
	sb.WriteString("\nlogging:\n")
	sb.WriteString(fmt.Sprintf("  level: %s\n", w.LogLevel))
	sb.WriteString(fmt.Sprintf("  path: %s\n", w.LogPath))
	sb.WriteString("\ninit_admin:\n")
	sb.WriteString(fmt.Sprintf("  username: %s\n", w.Username))
	sb.WriteString(fmt.Sprintf("  password: %s\n", w.Password))
	return sb.String()
}

func runConfigWizard(scanner *bufio.Scanner, bundled map[string]interface{}) *wizardResult {
	fmt.Println()
	fmt.Println("--- Config Wizard ---")
	fmt.Println("Press Enter to keep the default value shown in [brackets].")
	fmt.Println()

	// Try to extract defaults from bundled config
	defPort := "65000"
	defDB := "/var/lib/bekci/bekci.db"
	defLogLevel := "warn"
	defLogPath := "/var/log/bekci/bekci.log"
	defUser := "admin"
	defPass := ""

	if bundled != nil {
		if srv, ok := bundled["server"].(map[string]interface{}); ok {
			if p, ok := srv["port"]; ok {
				defPort = fmt.Sprintf("%v", p)
			}
			if d, ok := srv["db_path"].(string); ok {
				defDB = d
			}
		}
		if log, ok := bundled["logging"].(map[string]interface{}); ok {
			if l, ok := log["level"].(string); ok {
				defLogLevel = l
			}
			if p, ok := log["path"].(string); ok {
				defLogPath = p
			}
		}
		if admin, ok := bundled["init_admin"].(map[string]interface{}); ok {
			if u, ok := admin["username"].(string); ok {
				defUser = u
			}
		}
	}

	return &wizardResult{
		Port:     promptWithDefault(scanner, "Server port", defPort),
		DBPath:   promptWithDefault(scanner, "Database path", defDB),
		LogLevel: promptWithDefault(scanner, "Log level (debug/info/warn/error)", defLogLevel),
		LogPath:  promptWithDefault(scanner, "Log file path", defLogPath),
		Username: promptWithDefault(scanner, "Admin username", defUser),
		Password: promptWithDefault(scanner, "Admin password", defPass),
	}
}

func promptWithDefault(scanner *bufio.Scanner, label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("  %s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("  %s: ", label)
	}
	if !scanner.Scan() {
		return defaultVal
	}
	val := strings.TrimSpace(scanner.Text())
	if val == "" {
		return defaultVal
	}
	return val
}

func extractTarGz(data []byte, destDir string) error {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}

		// Sanitize path — only allow base filenames
		name := filepath.Base(hdr.Name)
		destPath := filepath.Join(destDir, name)

		f, err := os.Create(destPath)
		if err != nil {
			return fmt.Errorf("creating %s: %w", name, err)
		}

		// Limit copy to header size to prevent decompression bombs
		if _, err := io.Copy(f, io.LimitReader(tr, hdr.Size)); err != nil {
			f.Close()
			return fmt.Errorf("extracting %s: %w", name, err)
		}
		f.Close()
	}
	return nil
}

func copyFile(src, dst string) error {
	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
