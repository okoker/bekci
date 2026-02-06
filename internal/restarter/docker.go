package restarter

import (
	"fmt"
	"os/exec"

	"github.com/bekci/internal/config"
)

func (r *Restarter) restartDocker(svc *config.Service) *Result {
	var cmd *exec.Cmd

	if svc.Restart.Command != "" {
		// Custom docker command
		cmd = exec.Command("sh", "-c", svc.Restart.Command)
	} else if svc.Restart.Container != "" {
		// Standard docker restart
		cmd = exec.Command("docker", "restart", svc.Restart.Container)
	} else {
		return &Result{
			Success: false,
			Error:   "no container or command specified for docker restart",
		}
	}

	output, err := cmd.CombinedOutput()

	if err != nil {
		return &Result{
			Success: false,
			Output:  string(output),
			Error:   fmt.Sprintf("docker restart failed: %v", err),
		}
	}

	return &Result{
		Success: true,
		Output:  string(output),
	}
}
