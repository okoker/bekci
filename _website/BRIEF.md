# Bekci Website — Shared Brief (source of truth for all demos)

You are building a **single-page marketing landing page** for **Bekci**, an open-source,
self-hosted monitoring platform. Every fact below is accurate to the real product — do NOT
invent features, metrics, integrations, or testimonials. No fake customer logos, no fake
star counts, no fake numbers. Use only what's here.

## What Bekci is
- **Bekci** (Turkish *bekçi* = "watchman / night guard"). Mascot: a vigilant owl.
- A **self-hosted, tactical host & network monitoring platform**. Open source, **MIT licensed**.
- **Single Go binary** (Vue 3 UI embedded), **SQLite** storage, **Docker-first**. One container,
  one port (`65000`). Scales to ~1000 hosts.
- Built by security people, for people who are tired of alert noise.

## THE SIGNATURE CONCEPT — "Tactical monitoring: 4 hours + 90 days"
Every monitored check is shown on **two timescales at once**:
- **4-hour bar** — 48 segments, one per **5 minutes**. The *tactical* view: what's happening right now.
- **90-day bar** — 90 segments, one per **day**. The *strategic* view: long-term reliability & trend.
Both side by side, so an operator sees the immediate incident AND the 90-day pattern in one glance.
Segment colors: green = 100%, yellow = 95–99%, orange = 80–94%, red = <80%, grey = no data.
This dual-timescale readout is the product's signature UI — feature it as the hero visual.

## THE PITCH — "Zero false positives"
Bekci's core promise: **alert only when it's real.** Three mechanisms, all real:
1. **Corroboration (AND/OR condition groups).** Combine checks with boolean logic so an alert
   fires only when independent signals agree. Real syntax example: **`(TCP AND HTTP) OR PING`**.
   Groups are combined by AND/OR internally; across groups the logic is OR. Nestable / composable.
2. **Sustained failure (fail count × fail window).** A condition must fail **N times within a
   time window** before it counts as down. A single blip is ignored. (Window minimum = fail_count × interval.)
3. **Flap suppression.** Per-rule alert **cooldown**, configurable **re-alert** interval for still-firing
   issues, and **recovery cooldown** — so a flapping target can't create an alert storm.
Positioning: most uptime monitors alert on a *single* failed check. Bekci demands corroboration
AND sustained failure — driving false positives toward zero.

## THE 8 CHECK TYPES (exact list)
1. **Ping (ICMP)** — reachability; reports RTT + packet loss (pass = not 100% loss)
2. **HTTP / HTTPS** — expected status code, endpoint/path, custom port, GET (follows redirects)
3. **TCP** — port connect
4. **DNS** — resolve host, optional expected record value, record type, nameserver
5. **Page Hash** — SHA-256 of response body → detects defacement / unexpected change
6. **TLS Certificate** — certificate expiry, warn N days ahead
7. **SNMP v2c** — sysUpTime, sysDescr, best-effort CPU / memory
8. **SNMP v3** — same, with authPriv / authNoPriv / noAuthNoPriv

Each check yields a pass/fail result (e.g. status matches, DNS value matches, cert has ≥ warn_days
left). Conditions add confirmation (fail_count × fail_window) and compose into the AND/OR groups above.

**Do NOT claim (not in the product):** multi-region / multi-vantage probing, response-time SLA
thresholds, body keyword/regex/JSON-path matching, custom SNMP OIDs, alert acknowledge/escalation,
SMS/PagerDuty/Teams/Slack-native channels. Keep copy to what's listed above.

## ALERTING CHANNELS (exact)
- **Email** (Resend or Microsoft 365)
- **Signal** (Signal REST gateway)
- **Webhook** — generic JSON POST to any endpoint (SOAR, Slack, etc.), Bearer or Basic auth
Recovery notifications include downtime duration (down since / recovered / total).

## EVERYTHING ELSE (real features — use for the secondary feature grid)
- **SOC status wall** — flat status page for a security-operations display, optionally public
- **SLA compliance** — per-category SLA thresholds, 90-day daily-uptime charts
- **RBAC** — three roles: admin / operator / viewer (JWT in HttpOnly cookie, bcrypt)
- **Audit log** — full trail of every admin/operator action, server-side search
- **Encrypted backups** — full DB snapshot, optional AES-256-GCM (Argon2id, diceware passphrase)
- **Self-health "dead man's switch"** — scheduler heartbeat; the monitor watches itself and alerts if it stalls
- **fail2ban integration** — login brute-force protection with ban detail
- **Automated backups**, config + full-DB restore

## DEPLOY (real quick-start — use verbatim in the code snippet)
```
git clone https://github.com/okoker/bekci.git
cd bekci
docker compose up -d
```
Then open `http://localhost:65000`. One container. One binary. One port.

## TECH STACK (for a small "built with" line)
Go 1.25 · Vue 3 + Vite · SQLite (WAL) · Docker · MIT license · single self-contained binary.

## STANDARD PAGE SECTIONS (implement all, in the demo's own visual language)
1. **Sticky/!top nav** — owl logo + "Bekci" wordmark; links: Tactical, Zero false positives,
   Checks, Deploy; a primary button ("Deploy" / "Get started") + a "GitHub" / "Star on GitHub" link.
2. **Hero** — headline + subhead + primary CTA + secondary (View on GitHub) + the **dual-bar readout**
   as the hero visual. Include a small "Open source · MIT · Self-hosted" eyebrow/badges.
3. **Tactical section** — explain 4-hour + 90-day dual timescale; ideally with live-looking bars.
4. **Zero-false-positives section** — the 3 mechanisms; feature the `(TCP AND HTTP) OR PING` example
   as a real code/logic element.
5. **Check types** — grid of the 8 checks (icon/label/one-liner each).
6. **Feature grid** — SOC wall, SLA, RBAC, alerting (email/Signal/webhook), encrypted backups,
   self-health, audit log.
7. **Deploy / quick-start** — the docker snippet + "one binary, one port" + MIT/open-source.
8. **Footer** — Bekci, MIT © 2026, GitHub link, "built with Go + Vue", small owl.

## BUILD REQUIREMENTS (all demos)
- **One self-contained `.html` file.** Inline CSS in a `<style>` tag; inline any JS in `<script>`.
- Web fonts: you MAY use Google Fonts via `<link>` (these are local files served over http, not CSP-restricted).
  Pick the exact families your direction specifies. Do NOT use Inter or Space Grotesk (overused).
- The owl mascot image is available at **`../assets/bekci-owl.png`** (relative to the demos/ folder).
  Use it where it fits the direction; otherwise draw a per-direction SVG owl/eye mark.
- **Fully responsive** — must look right at 1440px, 768px, and 390px (mobile). No horizontal body scroll;
  wide elements (bar strips, code) scroll inside their own container. Nav collapses cleanly on mobile.
- **The dual-bar readout must be generated by JS** so it looks alive: render ~48 four-hour segments
  and ~90 day segments per check, mostly green with a few realistic incidents (a red/orange cluster).
  Page JS may use `Math.random()` freely. Show 3–4 example checks (e.g. `api.acme.io · HTTPS`,
  `db-primary · TCP:5432`, `edge-router · SNMP`, `www.acme.io · TLS`). Include UP / DOWN / HEALTHY pills.
- Real copy only — write from the operator's point of view, active voice. No lorem, no fake stats.
- Accessible: visible focus states, `prefers-reduced-motion` respected for any animation,
  semantic HTML, alt text on the owl.
- Put a distinctive `<title>` and favicon (`<link rel="icon" href="../assets/favicon-32.png">`).
- Aim for a genuinely top-tier, non-templated result. Match the exact palette/type/layout your
  direction specifies — deviate only to improve, never toward a generic SaaS template.
