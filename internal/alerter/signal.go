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

// SendSignal sends a message via the Signal REST API gateway.
func SendSignal(apiURL, username, password, senderNumber string, recipients []string, message string) error {
	payload := map[string]any{
		"message":    message,
		"number":     senderNumber,
		"recipients": recipients,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal signal payload: %w", err)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth(username, password)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send signal message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("signal API error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
