package alerter

import (
	"fmt"
	"html"
	"strings"
	"time"
)

// RenderEmailAlert renders an HTML email for a firing or recovery alert.
func RenderEmailAlert(targetName, targetHost, state string, checks []string, ts time.Time) (subject, body string) {
	targetName = html.EscapeString(targetName)
	targetHost = html.EscapeString(targetHost)
	timestamp := ts.UTC().Format("02/01/2006 15:04 UTC")

	if state == "unhealthy" {
		subject = fmt.Sprintf("[ALERT] %s is DOWN", targetName)
		body = renderEmailHTML(targetName, targetHost, "DOWN", "#dc2626", checks, timestamp)
	} else {
		subject = fmt.Sprintf("[RECOVERED] %s is UP", targetName)
		body = renderEmailHTML(targetName, targetHost, "RECOVERED", "#16a34a", checks, timestamp)
	}
	return
}

// RenderSignalAlert renders a plain-text Signal message for a firing or recovery alert.
func RenderSignalAlert(targetName, targetHost, state string, checks []string, ts time.Time) string {
	timestamp := ts.UTC().Format("02/01/2006 15:04 UTC")

	var icon, label string
	if state == "unhealthy" {
		icon = "\U0001F534" // red circle
		label = "ALERT"
	} else {
		icon = "\U0001F7E2" // green circle
		label = "RECOVERED"
	}

	status := "DOWN"
	if state != "unhealthy" {
		status = "UP"
	}

	msg := fmt.Sprintf("%s [%s] %s is %s\nHost: %s\nTime: %s", icon, label, targetName, status, targetHost, timestamp)

	if len(checks) > 0 {
		msg += "\nChecks: " + strings.Join(checks, ", ")
	}

	return msg
}

func renderEmailHTML(targetName, targetHost, stateLabel, color string, checks []string, timestamp string) string {
	var checksHTML string
	if len(checks) > 0 {
		var items []string
		for _, c := range checks {
			items = append(items, fmt.Sprintf("<li>%s</li>", html.EscapeString(c)))
		}
		checksHTML = fmt.Sprintf(`<p style="margin:0 0 12px"><strong>Affected checks:</strong></p>
		<ul style="margin:0 0 16px;padding-left:20px">%s</ul>`, strings.Join(items, ""))
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="utf-8"></head>
<body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;color:#1e293b;max-width:600px;margin:0 auto;padding:20px">
  <div style="border-left:4px solid %s;padding:12px 16px;margin-bottom:20px;background:#f8fafc;border-radius:0 6px 6px 0">
    <h2 style="margin:0 0 4px;font-size:18px;color:%s">%s — %s</h2>
    <p style="margin:0;color:#64748b;font-size:14px">%s (%s)</p>
  </div>
  %s
  <p style="color:#94a3b8;font-size:12px;margin-top:24px">Sent by Bekci at %s</p>
</body>
</html>`, color, color, stateLabel, targetName, targetName, targetHost, checksHTML, timestamp)
}
