package checker

import (
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"time"
)

func runPageHash(host string, config map[string]any) *Result {
	scheme := configStr(config, "scheme", "https")
	port := configInt(config, "port", 0)
	endpoint := configStr(config, "endpoint", "/")
	baselineHash := configStr(config, "baseline_hash", "")
	timeoutS := configInt(config, "timeout_s", 10)

	url := fmt.Sprintf("%s://%s", scheme, host)
	if port > 0 {
		url = fmt.Sprintf("%s://%s:%d", scheme, host, port)
	}
	url += endpoint

	client := &http.Client{
		Timeout: time.Duration(timeoutS) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
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
