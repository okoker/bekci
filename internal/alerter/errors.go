package alerter

import "errors"

// ErrNotConfigured is returned when email alerting is not configured.
var ErrNotConfigured = errors.New("email alerting not configured: set resend_api_key and alert_from_email")
