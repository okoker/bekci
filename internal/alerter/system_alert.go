package alerter

import (
	"fmt"
	"html"
	"log/slog"
	"strings"

	"github.com/bekci/internal/store"
)

// SendSystemAlert sends an alert to system alert recipients (admins and/or specific users).
// Used for unclean restart notifications. Sends via all configured channels (email, signal, webhook).
// Does not go through the normal Dispatch flow — no cooldowns, no rules.
func SendSystemAlert(st *store.Store, subject, message string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Panic in SendSystemAlert", "panic", r)
		}
	}()

	// Resolve recipients
	var recipients []store.User

	alertAdmins, _ := st.GetSetting("system_alert_admins")
	if alertAdmins != "false" { // default true
		users, err := st.ListUsers()
		if err != nil {
			slog.Error("System alert: failed to list users", "error", err)
			return
		}
		for _, u := range users {
			if u.Role == "admin" && u.Status == "active" {
				recipients = append(recipients, u)
			}
		}
	}

	alertUserIDs, _ := st.GetSetting("system_alert_users")
	if alertUserIDs != "" {
		for _, id := range strings.Split(alertUserIDs, ",") {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			u, err := st.GetUserByID(id)
			if err != nil || u == nil || u.Status != "active" {
				continue
			}
			// Deduplicate
			found := false
			for _, r := range recipients {
				if r.ID == u.ID {
					found = true
					break
				}
			}
			if !found {
				recipients = append(recipients, *u)
			}
		}
	}

	if len(recipients) == 0 {
		slog.Warn("System alert: no recipients configured", "subject", subject)
		return
	}

	// Send via configured channels
	method, _ := st.GetSetting("alert_method")
	webhookEnabled, _ := st.GetSetting("webhook_enabled")

	// Email
	if strings.Contains(method, "email") {
		provider, _ := st.GetSetting("email_provider")
		fromEmail, _ := st.GetSetting("alert_from_email")

		var emails []string
		for _, u := range recipients {
			if u.Email != "" {
				emails = append(emails, u.Email)
			}
		}
		if len(emails) > 0 && fromEmail != "" {
			htmlBody := fmt.Sprintf("<h2>%s</h2><p>%s</p>", html.EscapeString(subject), html.EscapeString(message))
			var err error
			if provider == "smtp" {
				host, _ := st.GetSetting("smtp_host")
				port, _ := st.GetSetting("smtp_port")
				username, _ := st.GetSetting("smtp_username")
				password, _ := st.GetSetting("smtp_password")
				err = SendEmailSMTP(host, port, username, password, fromEmail, emails, subject, htmlBody)
			} else {
				apiKey, _ := st.GetSetting("resend_api_key")
				err = SendEmail(apiKey, fromEmail, emails, subject, htmlBody)
			}
			if err != nil {
				slog.Error("System alert: email send failed", "error", err)
			} else {
				slog.Info("System alert: email sent", "recipients", emails)
			}
		}
	}

	// Signal
	if strings.Contains(method, "signal") {
		apiURL, _ := st.GetSetting("signal_api_url")
		senderNumber, _ := st.GetSetting("signal_number")
		username, _ := st.GetSetting("signal_username")
		password, _ := st.GetSetting("signal_password")
		skipTLS, _ := st.GetSetting("signal_skip_tls")

		var phones []string
		for _, u := range recipients {
			if u.Phone != "" {
				phones = append(phones, u.Phone)
			}
		}
		if len(phones) > 0 && apiURL != "" && senderNumber != "" {
			err := SendSignal(apiURL, username, password, senderNumber, phones, fmt.Sprintf("[SYSTEM] %s\n%s", subject, message), skipTLS == "true")
			if err != nil {
				slog.Error("System alert: signal send failed", "error", err)
			} else {
				slog.Info("System alert: signal sent", "recipients", phones)
			}
		}
	}

	// Webhook
	if webhookEnabled == "true" {
		webhookURL, _ := st.GetSetting("webhook_url")
		authType, _ := st.GetSetting("webhook_auth_type")
		skipTLS, _ := st.GetSetting("webhook_skip_tls")

		if webhookURL != "" {
			auth := WebhookAuth{Type: authType}
			if authType == "bearer" {
				auth.BearerToken, _ = st.GetSetting("webhook_bearer_token")
			} else if authType == "basic" {
				auth.BasicUsername, _ = st.GetSetting("webhook_basic_username")
				auth.BasicPassword, _ = st.GetSetting("webhook_basic_password")
			}
			payload := WebhookPayload{
				Event:   "system_alert",
				Target:  "Bekci",
				Message: fmt.Sprintf("%s — %s", subject, message),
			}
			err := SendWebhook(webhookURL, auth, skipTLS == "true", payload)
			if err != nil {
				slog.Error("System alert: webhook send failed", "error", err)
			} else {
				slog.Info("System alert: webhook sent", "url", webhookURL)
			}
		}
	}
}
