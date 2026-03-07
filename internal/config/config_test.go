package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	cfgPath := filepath.Join(tmp, "config.yaml")

	yaml := "server:\n  db_path: " + dbPath + "\n"
	if err := os.WriteFile(cfgPath, []byte(yaml), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Server.Port != 65000 {
		t.Fatalf("port = %d, want 65000", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Fatalf("host = %q, want %q", cfg.Server.Host, "0.0.0.0")
	}
	if cfg.Logging.Level != "warn" {
		t.Fatalf("log level = %q, want %q", cfg.Logging.Level, "warn")
	}
	if cfg.InitAdmin.Username != "admin" {
		t.Fatalf("username = %q, want %q", cfg.InitAdmin.Username, "admin")
	}
	if cfg.InitAdmin.Password != "admin1234" {
		t.Fatalf("password = %q, want %q", cfg.InitAdmin.Password, "admin1234")
	}
	if cfg.Auth.JWTSecret == "" {
		t.Fatalf("jwt secret was not auto-generated")
	}
}

func TestEnvOverrides(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	cfgPath := filepath.Join(tmp, "config.yaml")

	yaml := "server:\n  db_path: " + dbPath + "\n"
	if err := os.WriteFile(cfgPath, []byte(yaml), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("BEKCI_PORT", "9999")
	t.Setenv("BEKCI_HOST", "127.0.0.1")
	t.Setenv("BEKCI_LOG_LEVEL", "debug")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Server.Port != 9999 {
		t.Fatalf("port = %d, want 9999", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Fatalf("host = %q, want %q", cfg.Server.Host, "127.0.0.1")
	}
	if cfg.Logging.Level != "debug" {
		t.Fatalf("log level = %q, want %q", cfg.Logging.Level, "debug")
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelWarn},
		{"", slog.LevelWarn},
	}
	for _, tt := range tests {
		got := ParseLogLevel(tt.input)
		if got != tt.want {
			t.Fatalf("ParseLogLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
