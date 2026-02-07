package checker

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bekci/internal/config"
)

// clientKey uniquely identifies an HTTP client configuration
type clientKey struct {
	skipTLS        bool
	followRedirect bool
	timeout        time.Duration
}

var (
	httpClients   = make(map[clientKey]*http.Client)
	httpClientsMu sync.Mutex
)

func getHTTPClient(svc *config.Service) *http.Client {
	key := clientKey{
		skipTLS:        svc.Check.SkipTLSVerify,
		followRedirect: svc.Check.FollowRedirect,
		timeout:        svc.Check.Timeout,
	}

	httpClientsMu.Lock()
	defer httpClientsMu.Unlock()

	if client, ok := httpClients[key]; ok {
		return client
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: svc.Check.SkipTLSVerify,
		},
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     90 * time.Second,
	}

	client := &http.Client{
		Timeout:   svc.Check.Timeout,
		Transport: transport,
	}

	if !svc.Check.FollowRedirect {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	httpClients[key] = client
	return client
}

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

	client := getHTTPClient(svc)

	// Make request
	resp, err := client.Get(url)
	responseMs := measureTime(start)

	if err != nil {
		return resultDown(fmt.Sprintf("request failed: %v", err), responseMs)
	}
	defer resp.Body.Close()

	// Drain body to allow connection reuse
	io.Copy(io.Discard, resp.Body)

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
