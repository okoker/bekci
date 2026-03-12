package alerter

import "errors"

// ErrNotConfigured is returned when email alerting is not configured.
var ErrNotConfigured = errors.New("email alerting not configured — check provider settings and from address")
