package checker

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bekci/internal/config"
)

func (c *Checker) checkHTTPS(svc *config.Service) *Result {
	start := time.Now()

	// Build URL
	url := svc.URL
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}
	if svc.Check.Endpoint != "" {
		url = strings.TrimSuffix(url, "/") + svc.Check.Endpoint
	}

	// Configure client
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: svc.Check.SkipTLSVerify,
		},
	}

	client := &http.Client{
		Timeout:   svc.Check.Timeout,
		Transport: transport,
	}

	// Handle redirects
	if !svc.Check.FollowRedirect {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	// Make request
	resp, err := client.Get(url)
	responseMs := measureTime(start)

	if err != nil {
		return resultDown(fmt.Sprintf("request failed: %v", err), responseMs)
	}
	defer resp.Body.Close()

	// Check status code
	expectedStatus := svc.Check.ExpectStatus
	if expectedStatus == 0 {
		expectedStatus = 200
	}

	if resp.StatusCode != expectedStatus {
		return &Result{
			Status:     "down",
			StatusCode: resp.StatusCode,
			ResponseMs: responseMs,
			Error:      fmt.Sprintf("expected status %d, got %d", expectedStatus, resp.StatusCode),
		}
	}

	return resultUp(resp.StatusCode, responseMs)
}
