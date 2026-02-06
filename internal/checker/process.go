package checker

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/bekci/internal/config"
)

func (c *Checker) checkProcess(svc *config.Service) *Result {
	start := time.Now()

	processName := svc.Check.ProcessName
	argsContain := svc.Check.ArgsContain

	// Use pgrep to find processes
	var cmd *exec.Cmd
	if argsContain != "" {
		// Search by full command line
		cmd = exec.Command("pgrep", "-f", argsContain)
	} else {
		// Search by process name
		cmd = exec.Command("pgrep", processName)
	}

	output, err := cmd.Output()
	responseMs := measureTime(start)

	if err != nil {
		// pgrep returns exit code 1 if no processes found
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return resultDown(fmt.Sprintf("process %q not found", processName), responseMs)
		}
		return resultDown(fmt.Sprintf("pgrep error: %v", err), responseMs)
	}

	// Check if we got PIDs
	pids := strings.TrimSpace(string(output))
	if pids == "" {
		return resultDown(fmt.Sprintf("process %q not found", processName), responseMs)
	}

	return resultUp(0, responseMs)
}
