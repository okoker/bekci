package restarter

import (
	"os/exec"

	"github.com/bekci/internal/config"
)

func (r *Restarter) restartLocal(svc *config.Service) *Result {
	cmd := exec.Command("sh", "-c", svc.Restart.Command)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return &Result{
			Success: false,
			Output:  string(output),
			Error:   err.Error(),
		}
	}

	return &Result{
		Success: true,
		Output:  string(output),
	}
}
