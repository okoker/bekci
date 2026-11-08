package checker

import (
	"fmt"
	"net/http"
	"time"
)

func runHTTP(host string, config map[string]any) *Result {
	scheme := configStr(config, "scheme", "https")
	if scheme != "http" && scheme != "https" {
		return &Result{Status: "error", Message: fmt.Sprintf("invalid scheme: %s (only http and https allowed)", scheme)}
	}
	port := configInt(config, "port", 0)
	endpoint := configStr(config, "endpoint", "/")
	expectStatus := configInt(config, "expect_status", 200)
	skipTLS := configBool(config, "skip_tls_verify", false)
	timeoutS := configInt(config, "timeout_s", 10)

	url := buildHTTPURL(scheme, host, port, endpoint)

	transport := sharedTransport
	if skipTLS {
		transport = skipTLSTransport
	}
	client := &http.Client{
		Timeout:   time.Duration(timeoutS) * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
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
