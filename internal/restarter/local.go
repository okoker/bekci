package restarter

import (
	"bytes"
	"context"
	"os/exec"
	"time"

	"github.com/bekci/internal/config"
)

const restartTimeout = 30 * time.Second

func (r *Restarter) restartLocal(svc *config.Service) *Result {
	ctx, cancel := context.WithTimeout(context.Background(), restartTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", svc.Restart.Command)
	// Detach stdin/stdout/stderr so background processes don't hold pipes
	cmd.Stdin = nil
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Start(); err != nil {
		return &Result{
			Success: false,
			Error:   err.Error(),
		}
	}

	err := cmd.Wait()

	// Timeout with background command (ends with &) is expected success
	if ctx.Err() == context.DeadlineExceeded && isBackgroundCommand(svc.Restart.Command) {
		return &Result{
			Success: true,
			Output:  buf.String(),
		}
	}

	if err != nil {
		return &Result{
			Success: false,
			Output:  buf.String(),
			Error:   err.Error(),
		}
	}

	return &Result{
		Success: true,
		Output:  buf.String(),
	}
}
