package alerter

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"time"
)

const smtpTimeout = 10 * time.Second

// SendEmailSMTP sends an email via SMTP with explicit STARTTLS, timeouts, and phase logging.
func SendEmailSMTP(host, port, username, password, from string, to []string, subject, htmlBody string) error {
	addr := host + ":" + port

	// Phase 1: TCP connect with timeout
	slog.Debug("SMTP: connecting", "addr", addr)
	conn, err := net.DialTimeout("tcp", addr, smtpTimeout)
	if err != nil {
		return fmt.Errorf("smtp connect to %s: %w", addr, err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(smtpTimeout))

	// Phase 2: Create SMTP client
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp client init: %w", err)
	}
	defer client.Close()

	// Phase 3: EHLO
	if err := client.Hello("localhost"); err != nil {
		return fmt.Errorf("smtp EHLO: %w", err)
	}

	// Phase 4: STARTTLS
	if ok, _ := client.Extension("STARTTLS"); ok {
		slog.Debug("SMTP: starting TLS", "host", host)
		tlsCfg := &tls.Config{ServerName: host}
		if err := client.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("smtp STARTTLS: %w", err)
		}
	} else {
		return fmt.Errorf("smtp: server %s does not support STARTTLS", host)
	}

	// Phase 5: Authenticate
	slog.Debug("SMTP: authenticating", "user", username)
	auth := smtp.PlainAuth("", username, password, host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}

	// Extend deadline for message delivery
	conn.SetDeadline(time.Now().Add(smtpTimeout))

	// Phase 6: Send message
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	for _, addr := range to {
		if err := client.Rcpt(addr); err != nil {
			return fmt.Errorf("smtp RCPT TO <%s>: %w", addr, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}

	headers := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=utf-8\r\n\r\n",
		from, strings.Join(to, ", "), subject)
	if _, err := w.Write([]byte(headers + htmlBody)); err != nil {
		return fmt.Errorf("smtp write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}

	client.Quit()
	slog.Debug("SMTP: email sent", "to", strings.Join(to, ", "))
	return nil
}
