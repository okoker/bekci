package checker

import (
	"fmt"
	"os"
	"time"

	"github.com/bekci/internal/config"
	"golang.org/x/crypto/ssh"
)

func (c *Checker) checkSSHProcess(svc *config.Service) *Result {
	start := time.Now()

	processName := svc.Check.ProcessName
	argsContain := svc.Check.ArgsContain

	// Build pgrep command
	var cmd string
	if argsContain != "" {
		cmd = fmt.Sprintf("pgrep -f %q", argsContain)
	} else {
		cmd = fmt.Sprintf("pgrep %q", processName)
	}

	output, err := c.runSSHCommand(svc, cmd)
	responseMs := measureTime(start)

	if err != nil {
		return resultDown(fmt.Sprintf("process %q not found or SSH error: %v", processName, err), responseMs)
	}

	if output == "" {
		return resultDown(fmt.Sprintf("process %q not found", processName), responseMs)
	}

	return resultUp(0, responseMs)
}

func (c *Checker) checkSSHCommand(svc *config.Service) *Result {
	start := time.Now()

	output, err := c.runSSHCommand(svc, svc.Check.Command)
	responseMs := measureTime(start)

	if err != nil {
		return resultDown(fmt.Sprintf("command failed: %v, output: %s", err, output), responseMs)
	}

	return resultUp(0, responseMs)
}

func (c *Checker) runSSHCommand(svc *config.Service, cmd string) (string, error) {
	// Determine SSH settings
	host := svc.Check.Host
	user := svc.Check.User
	if user == "" {
		user = c.sshDefaults.User
	}
	keyPath := svc.Check.KeyPath
	if keyPath == "" {
		keyPath = c.sshDefaults.KeyPath
	}
	timeout := svc.Check.Timeout
	if timeout == 0 {
		timeout = c.sshDefaults.Timeout
	}

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
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	// Add port if not present
	if host[len(host)-1] != ':' && !containsPort(host) {
		host = host + ":22"
	}

	client, err := ssh.Dial("tcp", host, config)
	if err != nil {
		return "", fmt.Errorf("SSH connection failed: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("SSH session failed: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)
	return string(output), err
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
