package checker

import (
	"fmt"
	"time"

	"github.com/bekci/internal/config"
)

type Result struct {
	Status     string // "up" or "down"
	StatusCode int
	ResponseMs int64
	Error      string
}

type Checker struct {
	sshDefaults config.SSHConfig
}

func New(sshDefaults config.SSHConfig) *Checker {
	return &Checker{sshDefaults: sshDefaults}
}

// Check performs a health check based on the service configuration
func (c *Checker) Check(svc *config.Service) *Result {
	switch svc.Check.Type {
	case "https":
		return c.checkHTTPS(svc)
	case "tcp":
		return c.checkTCP(svc)
	case "process":
		return c.checkProcess(svc)
	case "ssh_process":
		return c.checkSSHProcess(svc)
	case "ssh_command":
		return c.checkSSHCommand(svc)
	default:
		return &Result{
			Status: "down",
			Error:  fmt.Sprintf("unknown check type: %s", svc.Check.Type),
		}
	}
}

func resultUp(statusCode int, responseMs int64) *Result {
	return &Result{
		Status:     "up",
		StatusCode: statusCode,
		ResponseMs: responseMs,
	}
}

func resultDown(err string, responseMs int64) *Result {
	return &Result{
		Status:     "down",
		Error:      err,
		ResponseMs: responseMs,
	}
}

func measureTime(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}
