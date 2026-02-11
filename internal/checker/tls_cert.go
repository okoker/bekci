package checker

import (
	"crypto/tls"
	"fmt"
	"math"
	"net"
	"time"
)

func runTLSCert(host string, config map[string]any) *Result {
	port := configInt(config, "port", 443)
	warnDays := configInt(config, "warn_days", 30)
	timeoutS := configInt(config, "timeout_s", 10)

	addr := fmt.Sprintf("%s:%d", host, port)
	start := time.Now()

	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: time.Duration(timeoutS) * time.Second},
		"tcp",
		addr,
		&tls.Config{InsecureSkipVerify: true}, // we want to read the cert even if untrusted
	)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		return &Result{
			Status:     "down",
			ResponseMs: elapsed,
			Message:    fmt.Sprintf("TLS connect failed: %v", err),
			Metrics:    map[string]any{"addr": addr},
		}
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return &Result{
			Status:     "down",
			ResponseMs: elapsed,
			Message:    "no certificates presented",
			Metrics:    map[string]any{"addr": addr},
		}
	}

	cert := certs[0]
	daysLeft := int(math.Floor(time.Until(cert.NotAfter).Hours() / 24))
	issuer := cert.Issuer.CommonName
	subject := cert.Subject.CommonName

	status := "up"
	msg := fmt.Sprintf("cert valid, %d days remaining", daysLeft)
	if daysLeft < warnDays {
		status = "down"
		msg = fmt.Sprintf("cert expires in %d days (warn threshold: %d)", daysLeft, warnDays)
	}
	if daysLeft < 0 {
		msg = fmt.Sprintf("cert expired %d days ago", -daysLeft)
	}

	return &Result{
		Status:     status,
		ResponseMs: elapsed,
		Message:    msg,
		Metrics: map[string]any{
			"days_left":  daysLeft,
			"issuer":     issuer,
			"subject":    subject,
			"not_after":  cert.NotAfter.Format(time.RFC3339),
			"not_before": cert.NotBefore.Format(time.RFC3339),
		},
	}
}
