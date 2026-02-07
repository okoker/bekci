package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Global      GlobalConfig   `yaml:"global"`
	Resend      ResendConfig   `yaml:"resend"`
	SSHDefaults SSHConfig      `yaml:"ssh_defaults"`
	Projects    []Project      `yaml:"projects"`
}

type GlobalConfig struct {
	CheckInterval   time.Duration `yaml:"check_interval"`
	AlertCooldown   time.Duration `yaml:"alert_cooldown"`
	HistoryDays     int           `yaml:"history_days"`
	RestartAttempts int           `yaml:"restart_attempts"`
	RestartDelay    time.Duration `yaml:"restart_delay"`
	WebPort         int           `yaml:"web_port"`
	DBPath          string        `yaml:"db_path"`
	LogLevel        string        `yaml:"log_level"`
	LogPath         string        `yaml:"log_path"`
}

type ResendConfig struct {
	APIKey string   `yaml:"api_key"`
	From   string   `yaml:"from"`
	To     []string `yaml:"to"`
}

type SSHConfig struct {
	KeyPath string        `yaml:"key_path"`
	User    string        `yaml:"user"`
	Timeout time.Duration `yaml:"timeout"`
}

type Project struct {
	Name     string    `yaml:"name"`
	Services []Service `yaml:"services"`
}

type Service struct {
	Name          string        `yaml:"name"`
	URL           string        `yaml:"url"`
	Check         CheckConfig   `yaml:"check"`
	Restart       RestartConfig `yaml:"restart"`
	CheckInterval time.Duration `yaml:"check_interval"`
}

type CheckConfig struct {
	Type           string `yaml:"type"` // https, process, ssh_process, ssh_command
	Endpoint       string `yaml:"endpoint"`
	ExpectStatus   int    `yaml:"expect_status"`
	Timeout        time.Duration `yaml:"timeout"`
	FollowRedirect bool   `yaml:"follow_redirect"`
	SkipTLSVerify  bool   `yaml:"skip_tls_verify"`
	// For process checks
	ProcessName string `yaml:"name"`
	ArgsContain string `yaml:"args_contain"`
	// For SSH checks
	Host    string `yaml:"host"`
	Command string `yaml:"command"`
	KeyPath string `yaml:"key_path"`
	User    string `yaml:"user"`
}

type RestartConfig struct {
	Type      string `yaml:"type"` // local, ssh, docker, none
	Enabled   *bool  `yaml:"enabled"`
	Command   string `yaml:"command"`
	Container string `yaml:"container"`
	Host      string `yaml:"host"`
	KeyPath   string `yaml:"key_path"`
	User      string `yaml:"user"`
}

// Load reads and parses the config file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	applyDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

func applyDefaults(cfg *Config) {
	// Global defaults
	if cfg.Global.CheckInterval == 0 {
		cfg.Global.CheckInterval = 5 * time.Minute
	}
	if cfg.Global.AlertCooldown == 0 {
		cfg.Global.AlertCooldown = 30 * time.Minute
	}
	if cfg.Global.HistoryDays == 0 {
		cfg.Global.HistoryDays = 90
	}
	if cfg.Global.RestartAttempts == 0 {
		cfg.Global.RestartAttempts = 3
	}
	if cfg.Global.RestartDelay == 0 {
		cfg.Global.RestartDelay = 15 * time.Second
	}
	if cfg.Global.WebPort == 0 {
		cfg.Global.WebPort = 65000
	}
	if cfg.Global.DBPath == "" {
		cfg.Global.DBPath = "bekci.db"
	}
	if cfg.Global.LogLevel == "" {
		cfg.Global.LogLevel = "warn"
	}
	if cfg.Global.LogPath == "" {
		cfg.Global.LogPath = "bekci.log"
	}

	// SSH defaults
	if cfg.SSHDefaults.KeyPath == "" {
		home, _ := os.UserHomeDir()
		cfg.SSHDefaults.KeyPath = filepath.Join(home, ".ssh", "id_rsa")
	} else {
		cfg.SSHDefaults.KeyPath = expandPath(cfg.SSHDefaults.KeyPath)
	}
	if cfg.SSHDefaults.User == "" {
		cfg.SSHDefaults.User = "root"
	}
	if cfg.SSHDefaults.Timeout == 0 {
		cfg.SSHDefaults.Timeout = 10 * time.Second
	}

	// Service defaults
	for i := range cfg.Projects {
		for j := range cfg.Projects[i].Services {
			svc := &cfg.Projects[i].Services[j]

			if svc.CheckInterval == 0 {
				svc.CheckInterval = cfg.Global.CheckInterval
			}

			// Check defaults
			if svc.Check.ExpectStatus == 0 && svc.Check.Type == "https" {
				svc.Check.ExpectStatus = 200
			}
			if svc.Check.Timeout == 0 {
				svc.Check.Timeout = 10 * time.Second
			}
			if svc.Check.KeyPath != "" {
				svc.Check.KeyPath = expandPath(svc.Check.KeyPath)
			}

			// Restart defaults
			if svc.Restart.Type == "" {
				svc.Restart.Type = "none"
			}
			if svc.Restart.Enabled == nil {
				enabled := svc.Restart.Type != "none"
				svc.Restart.Enabled = &enabled
			}
			if svc.Restart.KeyPath != "" {
				svc.Restart.KeyPath = expandPath(svc.Restart.KeyPath)
			}
		}
	}
}

func validate(cfg *Config) error {
	if len(cfg.Projects) == 0 {
		return fmt.Errorf("no projects defined")
	}

	for i, proj := range cfg.Projects {
		if proj.Name == "" {
			return fmt.Errorf("project %d: name is required", i)
		}
		if len(proj.Services) == 0 {
			return fmt.Errorf("project %q: no services defined", proj.Name)
		}

		for j, svc := range proj.Services {
			if svc.Name == "" {
				return fmt.Errorf("project %q, service %d: name is required", proj.Name, j)
			}

			// Validate check type
			switch svc.Check.Type {
			case "https":
				if svc.URL == "" {
					return fmt.Errorf("service %q: url is required for https check", svc.Name)
				}
			case "tcp":
				if svc.URL == "" {
					return fmt.Errorf("service %q: url (host:port) is required for tcp check", svc.Name)
				}
			case "process":
				if svc.Check.ProcessName == "" {
					return fmt.Errorf("service %q: process name is required for process check", svc.Name)
				}
			case "ssh_process":
				if svc.Check.Host == "" {
					return fmt.Errorf("service %q: host is required for ssh_process check", svc.Name)
				}
				if svc.Check.ProcessName == "" {
					return fmt.Errorf("service %q: process name is required for ssh_process check", svc.Name)
				}
			case "ssh_command":
				if svc.Check.Host == "" {
					return fmt.Errorf("service %q: host is required for ssh_command check", svc.Name)
				}
				if svc.Check.Command == "" {
					return fmt.Errorf("service %q: command is required for ssh_command check", svc.Name)
				}
			case "":
				return fmt.Errorf("service %q: check type is required", svc.Name)
			default:
				return fmt.Errorf("service %q: unknown check type %q", svc.Name, svc.Check.Type)
			}

			// Validate restart type
			switch svc.Restart.Type {
			case "local":
				if svc.Restart.Command == "" {
					return fmt.Errorf("service %q: command is required for local restart", svc.Name)
				}
			case "ssh":
				if svc.Restart.Host == "" {
					return fmt.Errorf("service %q: host is required for ssh restart", svc.Name)
				}
				if svc.Restart.Command == "" {
					return fmt.Errorf("service %q: command is required for ssh restart", svc.Name)
				}
			case "docker":
				if svc.Restart.Container == "" && svc.Restart.Command == "" {
					return fmt.Errorf("service %q: container or command is required for docker restart", svc.Name)
				}
			case "none", "":
				// OK
			default:
				return fmt.Errorf("service %q: unknown restart type %q", svc.Name, svc.Restart.Type)
			}
		}
	}

	return nil
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

// GetServiceKey returns a unique identifier for a service
func GetServiceKey(projectName, serviceName string) string {
	return fmt.Sprintf("%s/%s", projectName, serviceName)
}

// ParseLogLevel converts a log level string to slog.Level
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
