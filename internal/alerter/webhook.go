package alerter

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WebhookPayload is the fixed JSON structure sent to webhook endpoints.
type WebhookPayload struct {
	Event         string         `json:"event"`
	Target        string         `json:"target"`
	TargetAddress string         `json:"target_address"`
	Category      string         `json:"category"`
	Message       string         `json:"message"`
	FailingChecks []FailingCheck `json:"failing_checks"`
	Timestamp     string         `json:"timestamp"`
}

// FailingCheck describes a single failing check within a webhook payload.
type FailingCheck struct {
	Type   string `json:"type"`
	Detail string `json:"detail"`
}

// SendWebhook POSTs a JSON payload to the given URL with optional Bearer auth and TLS skip.
func SendWebhook(url, bearerToken string, skipTLS bool, payload WebhookPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	transport := &http.Transport{}
	if skipTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	client := &http.Client{Timeout: 10 * time.Second, Transport: transport}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("webhook error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
