package api

import (
	"context"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type jailStatus struct {
	Name            string   `json:"name"`
	CurrentlyFailed int      `json:"currently_failed"`
	TotalFailed     int      `json:"total_failed"`
	CurrentlyBanned int      `json:"currently_banned"`
	TotalBanned     int      `json:"total_banned"`
	BannedIPs       []string `json:"banned_ips"`
}

var jailNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func (s *Server) handleFail2BanStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Get list of jails
	out, err := exec.CommandContext(ctx, "sudo", "fail2ban-client", "status").Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			writeError(w, http.StatusGatewayTimeout, "fail2ban-client timed out")
			return
		}
		writeError(w, http.StatusServiceUnavailable, "fail2ban is not available")
		return
	}

	jailNames := parseJailList(string(out))
	jails := make([]jailStatus, 0, len(jailNames))

	for _, name := range jailNames {
		if !jailNameRe.MatchString(name) {
			continue
		}
		js, err := getJailStatus(ctx, name)
		if err != nil {
			continue // skip jails we can't read
		}
		jails = append(jails, js)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"jails":      jails,
		"fetched_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// parseJailList extracts jail names from `fail2ban-client status` output.
// Example line: `   |- Jail list:	sshd, bekci-login`
func parseJailList(output string) []string {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "Jail list:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) < 2 {
				return nil
			}
			raw := strings.TrimSpace(parts[1])
			if raw == "" {
				return nil
			}
			names := strings.Split(raw, ",")
			result := make([]string, 0, len(names))
			for _, n := range names {
				n = strings.TrimSpace(n)
				if n != "" {
					result = append(result, n)
				}
			}
			return result
		}
	}
	return nil
}

// getJailStatus runs `fail2ban-client status <jail>` and parses the output.
func getJailStatus(ctx context.Context, name string) (jailStatus, error) {
	out, err := exec.CommandContext(ctx, "sudo", "fail2ban-client", "status", name).Output()
	if err != nil {
		return jailStatus{}, err
	}

	js := jailStatus{Name: name, BannedIPs: []string{}}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if v, ok := extractInt(line, "Currently failed:"); ok {
			js.CurrentlyFailed = v
		} else if v, ok := extractInt(line, "Total failed:"); ok {
			js.TotalFailed = v
		} else if v, ok := extractInt(line, "Currently banned:"); ok {
			js.CurrentlyBanned = v
		} else if v, ok := extractInt(line, "Total banned:"); ok {
			js.TotalBanned = v
		} else if strings.Contains(line, "Banned IP list:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				raw := strings.TrimSpace(parts[1])
				if raw != "" {
					for _, ip := range strings.Fields(raw) {
						js.BannedIPs = append(js.BannedIPs, ip)
					}
				}
			}
		}
	}
	return js, nil
}

// extractInt looks for a line like `|- Currently failed:\t5` and returns 5.
func extractInt(line, key string) (int, bool) {
	if !strings.Contains(line, key) {
		return 0, false
	}
	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 2 {
		return 0, false
	}
	v, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, false
	}
	return v, true
}
