package alerter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/bekci/internal/config"
)

type Alerter struct {
	config       config.ResendConfig
	cooldown     time.Duration
	lastAlerts   map[string]time.Time
	mu           sync.Mutex
}

func New(cfg config.ResendConfig, cooldown time.Duration) *Alerter {
	return &Alerter{
		config:     cfg,
		cooldown:   cooldown,
		lastAlerts: make(map[string]time.Time),
	}
}

// SendDownAlert sends an alert when a service goes down
func (a *Alerter) SendDownAlert(projectName, serviceName, errorMsg string) error {
	serviceKey := fmt.Sprintf("%s/%s", projectName, serviceName)

	// Check cooldown
	a.mu.Lock()
	if lastAlert, ok := a.lastAlerts[serviceKey]; ok {
		if time.Since(lastAlert) < a.cooldown {
			a.mu.Unlock()
			slog.Info("Alert cooldown active, skipping", "service", serviceKey)
			return nil
		}
	}
	a.lastAlerts[serviceKey] = time.Now()
	a.mu.Unlock()

	subject := fmt.Sprintf("[ALERT] %s/%s is DOWN", projectName, serviceName)
	body := fmt.Sprintf(`
<h2>Service Alert</h2>
<p><strong>Project:</strong> %s</p>
<p><strong>Service:</strong> %s</p>
<p><strong>Status:</strong> <span style="color: red;">DOWN</span></p>
<p><strong>Error:</strong> %s</p>
<p><strong>Time:</strong> %s</p>
<hr>
<p><em>Bekci Service Monitor</em></p>
`, projectName, serviceName, errorMsg, time.Now().Format("02/01/2006 15:04:05"))

	return a.send(subject, body)
}

// SendRecoveryAlert sends an alert when a service recovers
func (a *Alerter) SendRecoveryAlert(projectName, serviceName string, downtime time.Duration) error {
	subject := fmt.Sprintf("[RECOVERY] %s/%s is UP", projectName, serviceName)
	body := fmt.Sprintf(`
<h2>Service Recovery</h2>
<p><strong>Project:</strong> %s</p>
<p><strong>Service:</strong> %s</p>
<p><strong>Status:</strong> <span style="color: green;">UP</span></p>
<p><strong>Total Downtime:</strong> %s</p>
<p><strong>Time:</strong> %s</p>
<hr>
<p><em>Bekci Service Monitor</em></p>
`, projectName, serviceName, formatDuration(downtime), time.Now().Format("02/01/2006 15:04:05"))

	return a.send(subject, body)
}

func (a *Alerter) send(subject, htmlBody string) error {
	if a.config.APIKey == "" {
		slog.Info("No Resend API key configured, skipping email")
		return nil
	}

	if len(a.config.To) == 0 {
		slog.Info("No recipients configured, skipping email")
		return nil
	}

	payload := map[string]interface{}{
		"from":    a.config.From,
		"to":      a.config.To,
		"subject": subject,
		"html":    htmlBody,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling email payload: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+a.config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("sending email: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("resend API error (%d): %s", resp.StatusCode, string(body))
	}

	slog.Warn("Alert email sent", "subject", subject)
	return nil
}

// ClearCooldown removes cooldown for a service (e.g., after recovery)
func (a *Alerter) ClearCooldown(projectName, serviceName string) {
	serviceKey := fmt.Sprintf("%s/%s", projectName, serviceName)
	a.mu.Lock()
	delete(a.lastAlerts, serviceKey)
	a.mu.Unlock()
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if minutes == 0 {
		return fmt.Sprintf("%d hours", hours)
	}
	return fmt.Sprintf("%d hours %d minutes", hours, minutes)
}
