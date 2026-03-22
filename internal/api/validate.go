package api

import (
	"net/mail"
	"regexp"
	"strconv"
	"strings"
)

// maxLen truncates s to n characters.
func maxLen(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// validEmail checks if s is a valid email address.
// Supports country TLDs (e.g. user@company.com.tr, user@company.uk).
func validEmail(s string) bool {
	if s == "" {
		return false
	}
	_, err := mail.ParseAddress(s)
	return err == nil
}

var phoneRe = regexp.MustCompile(`^\+?\d{7,15}$`)

// validPhone checks if s looks like a phone number (optional +, 7-15 digits).
func validPhone(s string) bool {
	return phoneRe.MatchString(s)
}

// validPort checks if s is a valid TCP port number (1-65535).
func validPort(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n >= 1 && n <= 65535
}

// validURL checks if s starts with http:// or https://.
func validURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}
