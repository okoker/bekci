package alerter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SendEmail sends an email via the Resend API. Standalone and reusable.
func SendEmail(apiKey, from string, to []string, subject, htmlBody string) error {
	payload := map[string]any{
		"from":    from,
		"to":      to,
		"subject": subject,
		"html":    htmlBody,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal email payload: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("resend API error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
