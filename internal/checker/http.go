package checker

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func runHTTP(host string, config map[string]any) *Result {
	scheme := configStr(config, "scheme", "https")
	port := configInt(config, "port", 0)
	endpoint := configStr(config, "endpoint", "/")
	expectStatus := configInt(config, "expect_status", 200)
	skipTLS := configBool(config, "skip_tls_verify", false)
	timeoutS := configInt(config, "timeout_s", 10)

	// Build URL (bracket IPv6 addresses)
	hostPart := host
	if strings.Contains(host, ":") {
		hostPart = "[" + host + "]"
	}
	url := fmt.Sprintf("%s://%s", scheme, hostPart)
	if port > 0 {
		url = fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(host, strconv.Itoa(port)))
	}
	url += endpoint

	client := &http.Client{
		Timeout: time.Duration(timeoutS) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	if skipTLS {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	start := time.Now()
	resp, err := client.Get(url)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		return &Result{
			Status:     "down",
			ResponseMs: elapsed,
			Message:    err.Error(),
			Metrics:    map[string]any{"url": url},
		}
	}
	defer resp.Body.Close()

	status := "up"
	msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
	if resp.StatusCode != expectStatus {
		status = "down"
		msg = fmt.Sprintf("expected %d, got %d", expectStatus, resp.StatusCode)
	}

	return &Result{
		Status:     status,
		ResponseMs: elapsed,
		Message:    msg,
		Metrics: map[string]any{
			"status_code": resp.StatusCode,
			"url":         url,
		},
	}
}
