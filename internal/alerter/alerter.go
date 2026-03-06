package alerter

import (
	"errors"
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
	if method == "" {
		return // alerting disabled
	}
	apiKey, _ := a.store.GetSetting("resend_api_key")
	fromEmail, _ := a.store.GetSetting("alert_from_email")

	if method == "email" || method == "email+signal" {
		if apiKey == "" || fromEmail == "" {
			slog.Warn("Alerter: email alerting configured but resend_api_key or alert_from_email is empty")
			return
		}
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
	if method == "email" || method == "email+signal" {
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

		if sigURL == "" || sigUser == "" || sigPass == "" {
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
}

// CheckRealerts checks for rules that are still firing and re-sends alerts if needed.
func (a *AlertService) CheckRealerts() {
	realertStr, _ := a.store.GetSetting("alert_realert_s")
	realertS, _ := strconv.Atoi(realertStr)
	if realertS <= 0 {
		return // re-alerting disabled
	}

	method, _ := a.store.GetSetting("alert_method")
	if method == "" {
		return
	}
	apiKey, _ := a.store.GetSetting("resend_api_key")
	fromEmail, _ := a.store.GetSetting("alert_from_email")
	if (method == "email" || method == "email+signal") && (apiKey == "" || fromEmail == "") {
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

		if method == "email" || method == "email+signal" {
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

			if sigURL != "" && sigUser != "" && sigPass != "" {
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
	}
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

	if sigURL == "" || sigUser == "" || sigPass == "" {
		return ErrSignalNotConfigured
	}

	msg := "\u2705 [Bekci] Test Signal message.\nYour Signal alerting is configured correctly."
	return SendSignal(sigURL, sigUser, sigPass, sigNumber, []string{toPhone}, msg)
}
