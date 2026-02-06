package restarter

import (
	"fmt"
	"log"
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
		return &Result{Success: false, Error: "restart disabled"}
	}

	var lastResult *Result

	for i := 0; i < attempts; i++ {
		if i > 0 {
			log.Printf("Restart attempt %d/%d for %s after %v delay", i+1, attempts, svc.Name, delay)
			time.Sleep(delay)
		}

		lastResult = r.doRestart(svc)
		if lastResult.Success {
			return lastResult
		}

		log.Printf("Restart attempt %d failed for %s: %s", i+1, svc.Name, lastResult.Error)
	}

	return lastResult
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
