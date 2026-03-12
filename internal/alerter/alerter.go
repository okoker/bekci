package alerter

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/bekci/internal/store"
)

// AlertService dispatches alerts on state transitions.
type AlertService struct {
	store *store.Store
}

// New creates a new AlertService.
func New(st *store.Store) *AlertService {
	return &AlertService{store: st}
}

// Dispatch is called by the engine when a rule state changes.
func (a *AlertService) Dispatch(ruleID, oldState, newState string) {
	// Look up the target for this rule
	targetID, err := a.store.GetTargetIDByRuleID(ruleID)
	if err != nil {
		slog.Error("Alerter: failed to get target for rule", "rule_id", ruleID, "error", err)
		return
	}

	// Read global settings
	method, _ := a.store.GetSetting("alert_method")
	apiKey, _ := a.store.GetSetting("resend_api_key")
	fromEmail, _ := a.store.GetSetting("alert_from_email")
	webhookEnabled, _ := a.store.GetSetting("webhook_enabled")

	if method == "" && webhookEnabled != "true" {
		return // no alerting configured
	}

	// Check cooldown (skip for recovery alerts)
	if newState == "unhealthy" {
		cooldownStr, _ := a.store.GetSetting("alert_cooldown_s")
		cooldown, _ := strconv.Atoi(cooldownStr)
		if cooldown <= 0 {
			cooldown = 1800
		}

		lastAlert, _ := a.store.GetLastAlertTime(ruleID)
		if !lastAlert.IsZero() && time.Since(lastAlert) < time.Duration(cooldown)*time.Second {
			slog.Debug("Alerter: skipping alert, within cooldown", "rule_id", ruleID)
			return
		}
	}

	// Get target details
	target, err := a.store.GetTarget(targetID)
	if err != nil || target == nil {
		slog.Error("Alerter: failed to get target", "target_id", targetID, "error", err)
		return
	}

	// Get recipients
	recipients, err := a.store.ListTargetRecipients(targetID)
	if err != nil {
		slog.Error("Alerter: failed to get recipients", "target_id", targetID, "error", err)
		return
	}
	if len(recipients) == 0 {
		slog.Debug("Alerter: no recipients for target", "target_id", targetID)
		return
	}

	// Determine alert type
	alertType := "firing"
	if newState == "healthy" {
		alertType = "recovery"
	}

	now := time.Now()

	// Send emails
	if (method == "email" || method == "email+signal") && apiKey != "" && fromEmail != "" {
		subject, htmlBody := RenderEmailAlert(target.Name, target.Host, newState, nil, now)

		for _, user := range recipients {
			if user.Email == "" {
				continue
			}
			err := SendEmail(apiKey, fromEmail, []string{user.Email}, subject, htmlBody)
			if err != nil {
				slog.Error("Alerter: failed to send email",
					"target", target.Name, "recipient", user.Username, "error", err)
			} else {
				slog.Info("Alerter: email sent",
					"target", target.Name, "recipient", user.Username, "type", alertType)
			}
			// Log regardless of send success (so we know we tried)
			if err := a.store.LogAlert(targetID, ruleID, user.ID, alertType, subject); err != nil {
				slog.Error("Alerter: failed to log alert", "target_id", targetID, "rule_id", ruleID, "error", err)
			}
		}
	}

	// Signal
	if method == "signal" || method == "email+signal" {
		sigURL, _ := a.store.GetSetting("signal_api_url")
		sigNumber, _ := a.store.GetSetting("signal_number")
		sigUser, _ := a.store.GetSetting("signal_username")
		sigPass, _ := a.store.GetSetting("signal_password")

		if sigURL == "" || sigNumber == "" || sigUser == "" || sigPass == "" {
			slog.Warn("Alerter: signal alerting configured but signal settings incomplete")
		} else {
			msg := RenderSignalAlert(target.Name, target.Host, newState, nil, now)

			for _, user := range recipients {
				if user.Phone == "" {
					continue
				}
				err := SendSignal(sigURL, sigUser, sigPass, sigNumber, []string{user.Phone}, msg)
				if err != nil {
					slog.Error("Alerter: failed to send signal",
						"target", target.Name, "recipient", user.Username, "error", err)
				} else {
					slog.Info("Alerter: signal sent",
						"target", target.Name, "recipient", user.Username, "type", alertType)
				}
				summary := "[Signal] " + msg[:min(len(msg), 80)]
				if err := a.store.LogAlert(targetID, ruleID, user.ID, alertType, summary); err != nil {
					slog.Error("Alerter: failed to log signal alert", "target_id", targetID, "rule_id", ruleID, "error", err)
				}
			}
		}
	}

	// Webhook (independent of alert_method)
	if webhookEnabled == "true" {
		webhookURL, _ := a.store.GetSetting("webhook_url")
		if webhookURL != "" {
			auth := a.getWebhookAuth()
			skipTLSStr, _ := a.store.GetSetting("webhook_skip_tls")
			skipTLS := skipTLSStr == "true"

			failingChecks := a.getFailingChecks(targetID, newState)
			payload := WebhookPayload{
				Event:         alertType,
				Target:        target.Name,
				TargetAddress: target.Host,
				Category:      target.Category,
				Message:       fmt.Sprintf("Target %s is %s", target.Name, newState),
				FailingChecks: failingChecks,
				Timestamp:     now.UTC().Format(time.RFC3339),
			}

			if err := SendWebhook(webhookURL, auth, skipTLS, payload); err != nil {
				slog.Error("Alerter: webhook failed", "target", target.Name, "error", err)
				a.store.SetSettings(map[string]string{
					"webhook_last_error": now.UTC().Format(time.RFC3339) + " — " + err.Error(),
				})
				a.auditWebhook("webhook_dispatch", target.Name, alertType+" — error: "+err.Error(), "failure")
			} else {
				slog.Info("Alerter: webhook sent", "target", target.Name, "type", alertType)
				a.store.SetSettings(map[string]string{
					"webhook_last_error":   "",
					"webhook_last_success": now.UTC().Format(time.RFC3339),
				})
				a.auditWebhook("webhook_dispatch", target.Name, alertType, "success")
			}
			summary := "[Webhook] " + target.Name + " " + alertType
			if err := a.store.LogAlert(targetID, ruleID, "", alertType, summary); err != nil {
				slog.Error("Alerter: failed to log webhook alert", "error", err)
			}
		}
	}
}

// CheckRealerts checks for rules that are still firing and re-sends alerts if needed.
func (a *AlertService) CheckRealerts() {
	realertStr, _ := a.store.GetSetting("alert_realert_s")
	realertS, _ := strconv.Atoi(realertStr)
	if realertS <= 0 {
		return // re-alerting disabled
	}

	method, _ := a.store.GetSetting("alert_method")
	apiKey, _ := a.store.GetSetting("resend_api_key")
	fromEmail, _ := a.store.GetSetting("alert_from_email")
	webhookEnabled, _ := a.store.GetSetting("webhook_enabled")

	if method == "" && webhookEnabled != "true" {
		return
	}

	firingRules, err := a.store.GetFiringRules()
	if err != nil {
		slog.Error("Alerter: failed to get firing rules", "error", err)
		return
	}

	for _, fr := range firingRules {
		lastAlert, _ := a.store.GetLastAlertTime(fr.RuleID)
		if lastAlert.IsZero() || time.Since(lastAlert) < time.Duration(realertS)*time.Second {
			continue
		}

		target, err := a.store.GetTarget(fr.TargetID)
		if err != nil || target == nil {
			continue
		}

		recipients, err := a.store.ListTargetRecipients(fr.TargetID)
		if err != nil || len(recipients) == 0 {
			continue
		}

		now := time.Now()

		if (method == "email" || method == "email+signal") && apiKey != "" && fromEmail != "" {
			subject, htmlBody := RenderEmailAlert(target.Name, target.Host, "unhealthy", nil, now)
			subject = "[RE-ALERT] " + subject[8:] // replace [ALERT] with [RE-ALERT]

			for _, user := range recipients {
				if user.Email == "" {
					continue
				}
				err := SendEmail(apiKey, fromEmail, []string{user.Email}, subject, htmlBody)
				if err != nil {
					slog.Error("Alerter: re-alert email failed",
						"target", target.Name, "recipient", user.Username, "error", err)
				} else {
					slog.Info("Alerter: re-alert email sent",
						"target", target.Name, "recipient", user.Username)
				}
				if err := a.store.LogAlert(fr.TargetID, fr.RuleID, user.ID, "re-alert", subject); err != nil {
					slog.Error("Alerter: failed to log re-alert", "target_id", fr.TargetID, "rule_id", fr.RuleID, "error", err)
				}
			}
		}

		if method == "signal" || method == "email+signal" {
			sigURL, _ := a.store.GetSetting("signal_api_url")
			sigNumber, _ := a.store.GetSetting("signal_number")
			sigUser, _ := a.store.GetSetting("signal_username")
			sigPass, _ := a.store.GetSetting("signal_password")

			if sigURL != "" && sigNumber != "" && sigUser != "" && sigPass != "" {
				msg := RenderSignalAlert(target.Name, target.Host, "unhealthy", nil, now)
				msg = strings.Replace(msg, "[ALERT]", "[RE-ALERT]", 1)
				msg = strings.Replace(msg, "\U0001F534", "\U0001F7E0", 1) // red -> orange circle

				for _, user := range recipients {
					if user.Phone == "" {
						continue
					}
					err := SendSignal(sigURL, sigUser, sigPass, sigNumber, []string{user.Phone}, msg)
					if err != nil {
						slog.Error("Alerter: re-alert signal failed",
							"target", target.Name, "recipient", user.Username, "error", err)
					} else {
						slog.Info("Alerter: re-alert signal sent",
							"target", target.Name, "recipient", user.Username)
					}
					summary := "[Signal RE-ALERT] " + target.Name
					if err := a.store.LogAlert(fr.TargetID, fr.RuleID, user.ID, "re-alert", summary); err != nil {
						slog.Error("Alerter: failed to log signal re-alert", "target_id", fr.TargetID, "rule_id", fr.RuleID, "error", err)
					}
				}
			}
		}

		// Webhook re-alert (independent of alert_method)
		if webhookEnabled == "true" {
			webhookURL, _ := a.store.GetSetting("webhook_url")
			if webhookURL != "" {
				auth := a.getWebhookAuth()
				skipTLSStr, _ := a.store.GetSetting("webhook_skip_tls")
				skipTLS := skipTLSStr == "true"

				failingChecks := a.getFailingChecks(fr.TargetID, "unhealthy")
				payload := WebhookPayload{
					Event:         "re-alert",
					Target:        target.Name,
					TargetAddress: target.Host,
					Category:      target.Category,
					Message:       fmt.Sprintf("Target %s is still unhealthy", target.Name),
					FailingChecks: failingChecks,
					Timestamp:     now.UTC().Format(time.RFC3339),
				}

				if err := SendWebhook(webhookURL, auth, skipTLS, payload); err != nil {
					slog.Error("Alerter: webhook re-alert failed", "target", target.Name, "error", err)
					a.store.SetSettings(map[string]string{
						"webhook_last_error": now.UTC().Format(time.RFC3339) + " — " + err.Error(),
					})
					a.auditWebhook("webhook_dispatch", target.Name, "re-alert — error: "+err.Error(), "failure")
				} else {
					slog.Info("Alerter: webhook re-alert sent", "target", target.Name)
					a.store.SetSettings(map[string]string{
						"webhook_last_error":   "",
						"webhook_last_success": now.UTC().Format(time.RFC3339),
					})
					a.auditWebhook("webhook_dispatch", target.Name, "re-alert", "success")
				}
				summary := "[Webhook RE-ALERT] " + target.Name
				if err := a.store.LogAlert(fr.TargetID, fr.RuleID, "", "re-alert", summary); err != nil {
					slog.Error("Alerter: failed to log webhook re-alert", "error", err)
				}
			}
		}
	}
}

// auditWebhook logs a webhook event to the audit log for visibility.
func (a *AlertService) auditWebhook(action, target, detail, status string) {
	_ = a.store.CreateAuditEntry(&store.AuditEntry{
		UserID:       "system",
		Username:     "system",
		Action:       action,
		ResourceType: "webhook",
		ResourceID:   target,
		Detail:       detail,
		Status:       status,
	})
}

// getWebhookAuth reads webhook authentication settings from the store.
func (a *AlertService) getWebhookAuth() WebhookAuth {
	authType, _ := a.store.GetSetting("webhook_auth_type")
	auth := WebhookAuth{Type: authType}
	switch authType {
	case "bearer":
		auth.BearerToken, _ = a.store.GetSetting("webhook_bearer_token")
	case "basic":
		auth.BasicUsername, _ = a.store.GetSetting("webhook_basic_username")
		auth.BasicPassword, _ = a.store.GetSetting("webhook_basic_password")
	}
	return auth
}

// getFailingChecks returns failing check details for a target.
// For recovery events (healthy state), returns empty slice.
func (a *AlertService) getFailingChecks(targetID, state string) []FailingCheck {
	if state == "healthy" {
		return []FailingCheck{}
	}
	checks, err := a.store.ListChecksByTarget(targetID)
	if err != nil {
		slog.Error("Alerter: failed to list checks for webhook", "target_id", targetID, "error", err)
		return []FailingCheck{}
	}
	var failing []FailingCheck
	for _, c := range checks {
		result, err := a.store.GetLastResult(c.ID)
		if err != nil || result == nil {
			continue
		}
		if result.Status != "up" {
			failing = append(failing, FailingCheck{
				Type:   c.Type,
				Detail: result.Message,
			})
		}
	}
	return failing
}

// SendTestEmail sends a test email to verify the configuration.
func (a *AlertService) SendTestEmail(toEmail string) error {
	apiKey, _ := a.store.GetSetting("resend_api_key")
	fromEmail, _ := a.store.GetSetting("alert_from_email")

	if apiKey == "" || fromEmail == "" {
		return ErrNotConfigured
	}

	subject := "[Bekci] Test Email"
	html := `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"></head>
<body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;color:#1e293b;max-width:600px;margin:0 auto;padding:20px">
  <div style="border-left:4px solid #3b82f6;padding:12px 16px;background:#f8fafc;border-radius:0 6px 6px 0">
    <h2 style="margin:0 0 4px;font-size:18px;color:#3b82f6">Test Email</h2>
    <p style="margin:0;color:#64748b;font-size:14px">Your Bekci email alerting is configured correctly.</p>
  </div>
</body>
</html>`

	return SendEmail(apiKey, fromEmail, []string{toEmail}, subject, html)
}

// ErrSignalNotConfigured is returned when Signal alerting settings are incomplete.
var ErrSignalNotConfigured = errors.New("signal alerting not configured: set signal_api_url, signal_username, and signal_password")

// SendTestSignal sends a test Signal message to verify the configuration.
func (a *AlertService) SendTestSignal(toPhone string) error {
	sigURL, _ := a.store.GetSetting("signal_api_url")
	sigNumber, _ := a.store.GetSetting("signal_number")
	sigUser, _ := a.store.GetSetting("signal_username")
	sigPass, _ := a.store.GetSetting("signal_password")

	if sigURL == "" || sigNumber == "" || sigUser == "" || sigPass == "" {
		return ErrSignalNotConfigured
	}

	msg := "\u2705 [Bekci] Test Signal message.\nYour Signal alerting is configured correctly."
	return SendSignal(sigURL, sigUser, sigPass, sigNumber, []string{toPhone}, msg)
}

// ErrWebhookNotConfigured is returned when webhook settings are incomplete.
var ErrWebhookNotConfigured = errors.New("webhook not configured: set webhook_enabled and webhook_url")

// SendTestWebhook sends a test webhook to verify the configuration.
func (a *AlertService) SendTestWebhook() error {
	enabled, _ := a.store.GetSetting("webhook_enabled")
	url, _ := a.store.GetSetting("webhook_url")
	if enabled != "true" || url == "" {
		return ErrWebhookNotConfigured
	}

	auth := a.getWebhookAuth()
	skipTLSStr, _ := a.store.GetSetting("webhook_skip_tls")
	skipTLS := skipTLSStr == "true"

	payload := WebhookPayload{
		Event:         "test",
		Target:        "Bekci Test",
		TargetAddress: "127.0.0.1",
		Category:      "Test",
		Message:       "This is a test webhook from Bekci.",
		FailingChecks: []FailingCheck{},
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
	}

	if err := SendWebhook(url, auth, skipTLS, payload); err != nil {
		a.auditWebhook("webhook_dispatch", "Bekci Test", "test — error: "+err.Error(), "failure")
		return err
	}
	a.auditWebhook("webhook_dispatch", "Bekci Test", "test", "success")
	return nil
}
