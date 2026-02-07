package restarter

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bekci/internal/config"
)

type Result struct {
	Success bool
	Output  string
	Error   string
}

type Restarter struct {
	sshDefaults config.SSHConfig
}

func New(sshDefaults config.SSHConfig) *Restarter {
	return &Restarter{sshDefaults: sshDefaults}
}

// Restart attempts to restart a service with retries
func (r *Restarter) Restart(svc *config.Service, attempts int, delay time.Duration) *Result {
	if svc.Restart.Type == "none" || svc.Restart.Type == "" {
		return &Result{Success: false, Error: "restart disabled (type=none)"}
	}
	if svc.Restart.Enabled != nil && !*svc.Restart.Enabled {
		return &Result{Success: false, Error: "restart disabled (enabled=false)"}
	}

	var lastResult *Result

	for i := 0; i < attempts; i++ {
		if i > 0 {
			slog.Warn("Restart retry", "attempt", i+1, "of", attempts, "service", svc.Name, "delay", delay)
			time.Sleep(delay)
		}

		lastResult = r.doRestart(svc)
		if lastResult.Success {
			return lastResult
		}

		slog.Warn("Restart attempt failed", "attempt", i+1, "service", svc.Name, "error", lastResult.Error)
	}

	return lastResult
}

// isBackgroundCommand checks if a command is intended to run in the background (ends with &)
func isBackgroundCommand(cmd string) bool {
	return strings.HasSuffix(strings.TrimSpace(cmd), "&")
}

func (r *Restarter) doRestart(svc *config.Service) *Result {
	switch svc.Restart.Type {
	case "local":
		return r.restartLocal(svc)
	case "ssh":
		return r.restartSSH(svc)
	case "docker":
		return r.restartDocker(svc)
	default:
		return &Result{
			Success: false,
			Error:   fmt.Sprintf("unknown restart type: %s", svc.Restart.Type),
		}
	}
}
