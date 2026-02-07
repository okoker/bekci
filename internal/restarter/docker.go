package restarter

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	"github.com/bekci/internal/config"
)

func (r *Restarter) restartDocker(svc *config.Service) *Result {
	ctx, cancel := context.WithTimeout(context.Background(), restartTimeout)
	defer cancel()

	var cmd *exec.Cmd

	if svc.Restart.Command != "" {
		// Custom docker command
		cmd = exec.CommandContext(ctx, "sh", "-c", svc.Restart.Command)
	} else if svc.Restart.Container != "" {
		// Standard docker restart
		cmd = exec.CommandContext(ctx, "docker", "restart", svc.Restart.Container)
	} else {
		return &Result{
			Success: false,
			Error:   "no container or command specified for docker restart",
		}
	}

	cmd.Stdin = nil
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Start(); err != nil {
		return &Result{
			Success: false,
			Error:   fmt.Sprintf("docker restart failed: %v", err),
		}
	}

	err := cmd.Wait()

	if ctx.Err() == context.DeadlineExceeded && svc.Restart.Command != "" && isBackgroundCommand(svc.Restart.Command) {
		return &Result{
			Success: true,
			Output:  buf.String(),
		}
	}

	if err != nil {
		return &Result{
			Success: false,
			Output:  buf.String(),
			Error:   fmt.Sprintf("docker restart failed: %v", err),
		}
	}

	return &Result{
		Success: true,
		Output:  buf.String(),
	}
}
