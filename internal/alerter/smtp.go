package alerter

import (
	"fmt"
	"net/smtp"
	"strings"
)

// SendEmailSMTP sends an email via SMTP (MS365 or any SMTP server).
func SendEmailSMTP(host, port, username, password, from string, to []string, subject, htmlBody string) error {
	addr := host + ":" + port
	auth := smtp.PlainAuth("", username, password, host)

	headers := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=utf-8\r\n\r\n",
		from, strings.Join(to, ", "), subject)
	msg := []byte(headers + htmlBody)

	if err := smtp.SendMail(addr, auth, from, to, msg); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return nil
}
