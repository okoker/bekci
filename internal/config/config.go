package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Auth      AuthConfig      `yaml:"auth"`
	Logging   LoggingConfig   `yaml:"logging"`
	InitAdmin InitAdminConfig `yaml:"init_admin"`
}

type ServerConfig struct {
	Port   int    `yaml:"port"`
	DBPath string `yaml:"db_path"`
}

type AuthConfig struct {
	JWTSecret string `yaml:"jwt_secret"`
}

type LoggingConfig struct {
	Level string `yaml:"level"`
	Path  string `yaml:"path"`
}

type InitAdminConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// Load reads config from YAML file, applies env overrides and defaults, then validates.
func Load(path string) (*Config, error) {
	cfg := &Config{}

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
		// Config file is optional — env vars can provide everything
	} else {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config: %w", err)
		}
	}

	applyEnvOverrides(cfg)
	applyDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("BEKCI_JWT_SECRET"); v != "" {
		cfg.Auth.JWTSecret = v
	}
	if v := os.Getenv("BEKCI_ADMIN_PASSWORD"); v != "" {
		cfg.InitAdmin.Password = v
	}
	if v := os.Getenv("BEKCI_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		}
	}
	if v := os.Getenv("BEKCI_DB_PATH"); v != "" {
		cfg.Server.DBPath = v
	}
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 65000
	}
	if cfg.Server.DBPath == "" {
		cfg.Server.DBPath = "bekci.db"
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "warn"
	}
	if cfg.Logging.Path == "" {
		cfg.Logging.Path = "bekci.log"
	}
	if cfg.InitAdmin.Username == "" {
		cfg.InitAdmin.Username = "admin"
	}
	if cfg.InitAdmin.Password == "" {
		cfg.InitAdmin.Password = "admin1234"
	}

	// Auto-generate JWT secret if not provided — persist next to DB so it survives restarts
	if cfg.Auth.JWTSecret == "" {
		cfg.Auth.JWTSecret = loadOrGenerateSecret(cfg.Server.DBPath)
	}
}

// loadOrGenerateSecret reads a persisted secret from <db_dir>/.jwt_secret,
// or generates a new one and saves it.
func loadOrGenerateSecret(dbPath string) string {
	secretPath := filepath.Join(filepath.Dir(dbPath), ".jwt_secret")

	data, err := os.ReadFile(secretPath)
	if err == nil && len(strings.TrimSpace(string(data))) > 0 {
		return strings.TrimSpace(string(data))
	}

	// Generate random 32-byte secret
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Fallback — should never happen
		return "bekci-fallback-secret-change-me"
	}
	secret := hex.EncodeToString(b)

	if err := os.WriteFile(secretPath, []byte(secret+"\n"), 0600); err != nil {
		slog.Warn("Could not persist JWT secret", "path", secretPath, "error", err)
	} else {
		slog.Warn("Generated and persisted JWT secret", "path", secretPath)
	}

	return secret
}

func validate(cfg *Config) error {
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}

// ParseLogLevel converts a log level string to slog.Level.
func ParseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelWarn
	}
}
