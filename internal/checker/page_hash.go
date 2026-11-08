package checker

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"time"
)

func runPageHash(host string, config map[string]any) *Result {
	scheme := configStr(config, "scheme", "https")
	if scheme != "http" && scheme != "https" {
		return &Result{Status: "error", Message: fmt.Sprintf("invalid scheme: %s (only http and https allowed)", scheme)}
	}
	port := configInt(config, "port", 0)
	endpoint := configStr(config, "endpoint", "/")
	baselineHash := configStr(config, "baseline_hash", "")
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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &Result{
			Status:     "down",
			Message:    fmt.Sprintf("HTTP %d", resp.StatusCode),
			ResponseMs: elapsed,
			Metrics:    map[string]any{"url": url},
		}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024)) // 2MB limit
	if err != nil {
		return &Result{
			Status:     "down",
			ResponseMs: elapsed,
			Message:    fmt.Sprintf("failed to read body: %v", err),
			Metrics:    map[string]any{"url": url},
		}
	}

	hash := fmt.Sprintf("%x", sha256.Sum256(body))

	// If no baseline, report up with the hash (first-run capture)
	if baselineHash == "" {
		return &Result{
			Status:     "up",
			ResponseMs: elapsed,
			Message:    "baseline hash captured",
			Metrics:    map[string]any{"hash": hash, "url": url, "baseline_captured": true},
		}
	}

	status := "up"
	msg := "hash matches baseline"
	if hash != baselineHash {
		status = "down"
		msg = "hash mismatch"
	}

	return &Result{
		Status:     status,
		ResponseMs: elapsed,
		Message:    msg,
		Metrics:    map[string]any{"hash": hash, "baseline_hash": baselineHash, "url": url},
	}
}
