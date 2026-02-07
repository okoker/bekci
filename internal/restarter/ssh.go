package restarter

import (
	"fmt"
	"os"
	"time"

	"github.com/bekci/internal/config"
	"golang.org/x/crypto/ssh"
)

func (r *Restarter) restartSSH(svc *config.Service) *Result {
	output, err := r.runSSHCommand(svc, svc.Restart.Command)
	if err != nil {
		return &Result{
			Success: false,
			Output:  output,
			Error:   err.Error(),
		}
	}

	return &Result{
		Success: true,
		Output:  output,
	}
}

type sshResult struct {
	output string
	err    error
}

func (r *Restarter) runSSHCommand(svc *config.Service, cmd string) (string, error) {
	// Determine SSH settings
	host := svc.Restart.Host
	user := svc.Restart.User
	if user == "" {
		user = r.sshDefaults.User
	}
	keyPath := svc.Restart.KeyPath
	if keyPath == "" {
		keyPath = r.sshDefaults.KeyPath
	}
	timeout := r.sshDefaults.Timeout

	// Read private key
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return "", fmt.Errorf("reading SSH key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return "", fmt.Errorf("parsing SSH key: %w", err)
	}

	// Connect
	sshConfig := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	// Add port if not present
	if !containsPort(host) {
		host = host + ":22"
	}

	client, err := ssh.Dial("tcp", host, sshConfig)
	if err != nil {
		return "", fmt.Errorf("SSH connection failed: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("SSH session failed: %w", err)
	}
	defer session.Close()

	// Run with timeout to prevent blocking on background processes
	ch := make(chan sshResult, 1)
	go func() {
		output, err := session.CombinedOutput(cmd)
		ch <- sshResult{output: string(output), err: err}
	}()

	select {
	case res := <-ch:
		return res.output, res.err
	case <-time.After(restartTimeout):
		// Timeout: if command ends with &, treat as success (background process started)
		if isBackgroundCommand(cmd) {
			return "", nil
		}
		return "", fmt.Errorf("SSH command timed out after %v", restartTimeout)
	}
}

func containsPort(host string) bool {
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			return true
		}
		if host[i] == '.' || host[i] == ']' {
			return false
		}
	}
	return false
}
