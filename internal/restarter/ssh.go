package restarter

import (
	"fmt"
	"os"
	"time"

	"github.com/bekci/internal/config"
	"github.com/bekci/internal/sshutil"
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
	if host == "" {
		return "", fmt.Errorf("SSH host is empty")
	}
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

	// Host key verification
	hostKeyCallback, err := sshutil.HostKeyCallback(r.sshDefaults)
	if err != nil {
		return "", fmt.Errorf("loading known_hosts: %w", err)
	}

	// Connect
	sshConfig := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	}

	host = sshutil.NormalizeHost(host)

	client, err := ssh.Dial("tcp", host, sshConfig)
	if err != nil {
		return "", fmt.Errorf("SSH connection failed: %w", err)
	}

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return "", fmt.Errorf("SSH session failed: %w", err)
	}

	// Run with timeout to prevent blocking on background processes
	ch := make(chan sshResult, 1)
	go func() {
		output, err := session.CombinedOutput(cmd)
		ch <- sshResult{output: string(output), err: err}
	}()

	select {
	case res := <-ch:
		session.Close()
		client.Close()
		return res.output, res.err
	case <-time.After(restartTimeout):
		// Force-close session and client to unblock the goroutine
		session.Close()
		client.Close()
		// If command ends with &, treat timeout as success (background process started)
		if isBackgroundCommand(cmd) {
			return "", nil
		}
		return "", fmt.Errorf("SSH command timed out after %v", restartTimeout)
	}
}
