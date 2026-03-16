package alerter

import (
	"fmt"
	"html"
	"strings"
	"time"
)

// formatDuration returns a human-readable duration string (e.g., "2h 15m", "3d 1h").
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

// RenderEmailAlert renders an HTML email for a firing or recovery alert.
// downSince is optional; when non-nil and state is "healthy", the email includes downtime info.
func RenderEmailAlert(targetName, targetHost, state string, checks []string, ts time.Time, downSince *time.Time) (subject, body string) {
	targetName = html.EscapeString(targetName)
	targetHost = html.EscapeString(targetHost)
	timestamp := ts.UTC().Format("02/01/2006 15:04 UTC")

	var downtimeHTML string
	if state == "healthy" && downSince != nil {
		dur := ts.Sub(*downSince)
		downtimeHTML = fmt.Sprintf(`<div style="background:#f0f9ff;border:1px solid #bae6fd;border-radius:6px;padding:10px 14px;margin-bottom:16px;font-size:13px;color:#1e40af">
    <p style="margin:0 0 4px"><strong>Down since:</strong> %s</p>
    <p style="margin:0 0 4px"><strong>Recovered at:</strong> %s</p>
    <p style="margin:0"><strong>Duration:</strong> %s</p>
  </div>`, downSince.UTC().Format("02/01/2006 15:04 UTC"), timestamp, formatDuration(dur))
	}

	if state == "unhealthy" {
		subject = fmt.Sprintf("[ALERT] %s is DOWN", targetName)
		body = renderEmailHTML(targetName, targetHost, "DOWN", "#dc2626", checks, timestamp, downtimeHTML)
	} else {
		subject = fmt.Sprintf("[RECOVERED] %s is UP", targetName)
		body = renderEmailHTML(targetName, targetHost, "RECOVERED", "#16a34a", checks, timestamp, downtimeHTML)
	}
	return
}

// RenderSignalAlert renders a plain-text Signal message for a firing or recovery alert.
// downSince is optional; when non-nil and state is "healthy", the message includes downtime info.
func RenderSignalAlert(targetName, targetHost, state string, checks []string, ts time.Time, downSince *time.Time) string {
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

	if state == "healthy" && downSince != nil {
		dur := ts.Sub(*downSince)
		msg += fmt.Sprintf("\n\U0001F53B Down: %s\n\U0001F53A Up: %s\n\u23F1 Duration: %s",
			downSince.UTC().Format("02/01/2006 15:04 UTC"), timestamp, formatDuration(dur))
	}

	return msg
}

func renderEmailHTML(targetName, targetHost, stateLabel, color string, checks []string, timestamp, downtimeHTML string) string {
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
  %s
  <p style="color:#94a3b8;font-size:12px;margin-top:24px">Sent by Bekci at %s</p>
</body>
</html>`, color, color, stateLabel, targetName, targetName, targetHost, downtimeHTML, checksHTML, timestamp)
}
