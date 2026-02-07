package sshutil

import (
	"log/slog"

	"github.com/bekci/internal/config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// ContainsPort checks if a host string already includes a port.
func ContainsPort(host string) bool {
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

// NormalizeHost ensures host has a port suffix (default :22).
func NormalizeHost(host string) string {
	if host == "" {
		return ""
	}
	if !ContainsPort(host) {
		return host + ":22"
	}
	return host
}

// HostKeyCallback returns an ssh.HostKeyCallback based on config.
// If SkipHostKeyVerify is true, returns InsecureIgnoreHostKey (with warning).
// Otherwise uses the known_hosts file.
func HostKeyCallback(cfg config.SSHConfig) (ssh.HostKeyCallback, error) {
	if cfg.SkipHostKeyVerify {
		slog.Warn("SSH host key verification disabled — vulnerable to MITM")
		return ssh.InsecureIgnoreHostKey(), nil
	}

	if cfg.KnownHostsPath == "" {
		slog.Warn("No known_hosts path configured, disabling host key verification")
		return ssh.InsecureIgnoreHostKey(), nil
	}

	cb, err := knownhosts.New(cfg.KnownHostsPath)
	if err != nil {
		return nil, err
	}
	return cb, nil
}
