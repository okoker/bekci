# Bekci v2 — Requirements

## Overview

Web-managed monitoring platform. Composite rules engine, RBAC, multiple check types, Docker-first.
Monitoring only in v1. Service restart, SSH checks = v2.
Scale: <1000 hosts, typically <400.

## Tech Stack

| Layer | Choice |
|-------|--------|
| Backend | Go 1.24+, net/http, no framework |
| Frontend | Vue 3 + Vite, hand-rolled CSS |
| DB | SQLite WAL |
| Auth | JWT (HS256) in HttpOnly cookie + server-side sessions, bcrypt |
| Alerting | Resend API (email), Signal REST API gateway (Signal) |
| Deployment | Single Docker image (Alpine, multi-stage) |

## RBAC — 3 Roles

3 roles: admin (full), operator (monitoring ops), viewer (read-only). JWT in HttpOnly cookie + server sessions, bcrypt, configurable timeout. Self-service profile/password for all. Admin seeded on first boot.

Full permission matrix, auth flow, middleware chain, session management, rate limiting: **`reference/rbac.md`**

## Check Types (v1)

| Type | Description | Key Config |
|------|-------------|------------|
| **Ping (ICMP)** | Ping host, RTT + packet loss | count, timeout_s |
| **HTTP/HTTPS** | GET/HEAD, status code, custom port | scheme, port, endpoint, expect_status, skip_tls_verify, timeout_s |
| **TCP** | Port connect | port |
| **DNS** | Resolve hostname, optional expected value | query, record_type, expect_value, nameserver |
| **Page Hash** | SHA256 of response body | endpoint, auto-capture baseline on first run |
| **TLS Certificate** | Cert expiry check | port, warn_days |

## Rules Engine (Unified Target Model)

- Rules are hidden — auto-managed per target. No standalone rules API.
- Each target has `category` (Network/Security/Physical Security/Key Services/Other) and a linked `rule_id`.
- **Conditions** are defined inline when creating/editing a target.
- Each condition = check definition + alert criteria (field, comparator, value, fail_count, fail_window).
- **Condition groups**: conditions carry `condition_group` (int) and `group_operator` (AND/OR). Within a group, conditions are combined by `group_operator`. Across groups, logic is always OR — any group triggering = unhealthy. Example: `(TCP AND HTTP) OR PING`.
- Engine evaluates after each check result. State changes trigger alerts.
- `targets.operator` column kept for backward compat but unused by engine — per-condition `group_operator` is the source of truth.

## Alerting

- **Email**: Resend API. Settings: `resend_api_key`, `alert_from_email`.
- **Signal**: Signal REST API gateway. Settings: `signal_api_url`, `signal_number`, `signal_username`, `signal_password`. TLS verification skipped (self-signed certs on local network).
- `alert_method`: `""` (disabled), `"email"`, `"signal"`, `"email+signal"`. Each channel sends independently — email failure doesn't block Signal and vice versa.
- Cooldown per rule (configurable via `alert_cooldown_s`)
- Re-alert for still-firing rules (configurable via `alert_realert_s`, 0 = disabled)
- Recovery alerts always sent (bypass cooldown)
- Test endpoints: `POST /api/settings/test-email`, `POST /api/settings/test-signal`
- Signal messages: plain text with emoji indicators (🔴 ALERT, 🟢 RECOVERED, 🟠 RE-ALERT)

## Web UI

| Route | Page | Access |
|-------|------|--------|
| `/login` | Login | public |
| `/` | Dashboard — status bars, problems first | all |
| `/targets` | Target list + CRUD (unified with conditions) | all (CRUD: operator+) |
| `/alerts` | Alert history | all |
| `/sla` | SLA Compliance — 90-day daily uptime charts per category | all |
| `/soc` | Public status page (if `soc_public` = true) | public/all |
| `/settings` | System settings — 7 tabs: General, SLA, Alerting (admin), Users (admin), Backup & Restore (admin: config backup, full DB backup with optional encryption, config restore), Audit Log (operator+), Fail2Ban (admin) | all (General/SLA), operator+ (Audit), admin (rest) |
| `/profile` | Own profile (email, phone), password change. Accessed via user dropdown menu in navbar. | all |

### Dashboard
- Two bars per check: 90-day (1 bar/day, 90 bars) + 4-hour (1 bar/5 min, 48 bars)
- Colors: green(100%) / yellow(95-99%) / orange(80-94%) / red(<80%) / gray(no data)
- UP/DOWN badges from rule engine state
- SLA badges: HEALTHY/UNHEALTHY based on preferred check 90d uptime vs category SLA threshold
- Sort: DOWN first → UNHEALTHY SLA → worst uptime → alphabetical
- Hover tooltip: date/time + short status summary
- Drill-down to configure (operator+)
- 30s auto-refresh

### SLA Compliance
- Per-category SLA thresholds (settings: `sla_network`, `sla_security`, `sla_physical_security`, `sla_key_services`, `sla_other`)
- Default 99.9% for all categories. Set to 0 to disable for a category.
- Each target's **preferred check** 90d uptime is compared against its category threshold
- Preferred check selectable in target edit form (dropdown, only shown with 2+ conditions)
- Badge: HEALTHY (>= threshold, green) / UNHEALTHY (< threshold, orange) / hidden (no data or disabled)
- Displayed on both Dashboard and SOC views

### SLA Page — Daily Uptime Charts
- Dedicated `/sla` page with Chart.js line charts per category (5 categories)
- `GET /api/sla/history` — single API call returns all categories with per-target daily uptime arrays (90 days)
- Each category card: header + SLA threshold badge, Chart.js Line chart
- 2-column CSS grid (responsive 1-col below 900px)
- Dotted grey annotation line at SLA threshold
- 20-color palette, hover highlights hovered line (dims others), tooltip shows target name + date + uptime %
- Y-axis auto-scales to `min(95, threshold - 2, lowestDataPoint - 2)` → 100.5%
- Empty categories show "No targets in this category"
- No auto-refresh (90-day data changes at most daily)

## API & DB

40 endpoints across 12 domains. Full specs extracted to reference docs:
- **`reference/api_reference.md`** — All endpoints, auth levels, request/response JSON, error codes
- **`reference/db_schema.md`** — All tables, columns, migrations, entity relationships
- **`reference/rbac.md`** — Permission matrix, auth flow, middleware, session management

## Configuration

### config.yaml (bootstrap only, needs restart to change)
- `server.port` — web port (default 65000)
- `server.host` — bind address (default `0.0.0.0`, use `127.0.0.1` behind reverse proxy)
- `server.db_path` — SQLite path
- `server.backup_dir` — Server-side backup directory (default: `{db_dir}/backups/`)
- `auth.jwt_secret` — JWT signing key (prefer env var over config file)
- `logging.level` — debug/info/warn/error
- `logging.path` — log file path
- `init_admin.username` / `init_admin.password` — first-boot admin seed

Env var overrides: `BEKCI_JWT_SECRET`, `BEKCI_ADMIN_PASSWORD`, `BEKCI_PORT`, `BEKCI_HOST`, `BEKCI_DB_PATH`, `BEKCI_BACKUP_DIR`, `BEKCI_CORS_ORIGIN`, `BEKCI_LOG_LEVEL`.

### settings table (runtime, editable via UI)
- `session_timeout_hours` (default: 24)
- `history_days` (default: 90)
- `audit_retention_days` (default: 91) — hard-delete of audit entries and alert history older than this (runs at startup + daily)
- `soc_public` (default: false) — make SOC status page publicly accessible
- `alert_method` (default: empty) — `""`, `"email"`, `"signal"`, `"email+signal"`
- `resend_api_key` (default: empty) — Resend API key for email alerting (masked in GET)
- `alert_from_email` (default: empty) — sender address for alert emails
- `alert_cooldown_s` (default: 1800) — cooldown between repeated alerts per rule
- `alert_realert_s` (default: 3600) — re-alert interval for still-firing rules (0 = disabled)
- `signal_api_url` (default: empty) — full URL for Signal REST API gateway (e.g. `http://host:port/v2/send`)
- `signal_number` (default: empty) — sender phone number for Signal messages
- `signal_username` (default: empty) — basic auth username for Signal gateway
- `signal_password` (default: empty) — basic auth password for Signal gateway (masked in GET)
- `sla_network` / `sla_security` / `sla_physical_security` / `sla_key_services` / `sla_other` (default: 99.9) — SLA uptime thresholds per category (0 = disabled)

No overlap between config.yaml and settings table.

## Docker

Single image, single container, single port.
- Multi-stage: node (Vue build) → golang (Go build) → alpine (runtime)
- Vue dist embedded in Go binary
- SQLite in `/data` volume
- `NET_RAW` capability for ICMP ping
- `setcap cap_net_raw+ep` on binary

## Decisions Log

| # | Decision |
|---|----------|
| 1 | ~~SNMP credentials plaintext in DB~~ *(SNMP deferred, decision moot)* |
| 2 | Hybrid scheduler: event-driven + 60s poll safety net |
| 3 | Dashboard shows check results, problems first, drill-down to configure |
| 4 | SSH checks deferred to v2 |
| 5 | Hand-rolled CSS, continue current aesthetic |
| 6 | Page hash auto-capture on first run |
| 7 | No migration from v1 YAML config — clean start via web UI |
| 8 | Config.yaml = process bootstrap, settings table = runtime. Zero overlap. |
| 9 | SNMP deferred — removed from v1 check types |
| 10 | Check config stored as JSON blob in TEXT column — flexible per check type |
| 11 | Checker package: complete rewrite (v1 coupled to old config types) |
| 12 | Scheduler: complete rewrite (v1 reads YAML, v2 reads DB) |
| 13 | All 6 Phase 3+4 tables created in migration005 (alert tables empty until Phase 4) |
| 14 | Dashboard API: flat `[]dashboardTarget` array. SOC same shape. |
| 15 | Engine evaluates rules async (goroutine) after each check result save |
| 16 | Fail2ban status via sudo exec (not D-Bus/socket) — simplest, sudoers-restricted |
| 17 | SLA compliance: per-category thresholds vs preferred check 90d uptime. Preferred check user-selectable in target edit. |
| 18 | SLA page: Chart.js line charts with daily granularity, single API endpoint, Y-axis auto-scales to data. |
| 19 | Full backup: SQLite online backup API + tar.gz + optional AES-256-GCM. Restore is CLI-only (not web) for safety. Passphrase via query param (acceptable over HTTPS). |

## Branding

- Owl icon (`frontend/public/bekci-icon.png`) — project mascot
- Navbar: 28px icon next to "Bekci" text
- Login page: 120px centered above title
- Favicons: 32px (browser tab), 180px (apple-touch-icon), 192px (Android)

## Full Database Backup & Restore

Two backup types available in Settings > Backup & Restore:

### Config Backup (existing)
- JSON export of 9 config tables (users, targets, checks, rules, settings, etc.)
- Restore via UI upload — replaces config data, not historical data

### Full Database Backup (v3.1.0)
- Complete SQLite snapshot (all tables including check_results, audit_logs, alert_history) + config.yaml
- Uses SQLite online backup API for consistent snapshot
- Archive format: tar.gz containing `bekci.db` + `config.yaml`
- Optional AES-256-GCM encryption (Argon2id KDF, 4-word diceware passphrase)
- File extensions: `.tar.gz` (plain) / `.tar.gz.enc` (encrypted)
- **Download**: `GET /api/backup/full?encrypt=true&passphrase=...` — streams to browser
- **Save to server**: `POST /api/backup/full/save` — saves to `backup_dir` on disk
- **List/Download/Delete saved**: `GET/DELETE /api/backup/full/saved/{filename}`, `GET /api/backup/full/list`
- Server-side storage dir: `server.backup_dir` in config.yaml (default: `{db_dir}/backups/`). Env: `BEKCI_BACKUP_DIR`
- Metadata tracked in `{backup_dir}/index.json` with SHA256 hash per file
- No auto-purge — manual delete from web UI
- Passphrase generator: `GET /api/backup/generate-passphrase` (admin-only)

### CLI Restore
- `bekci restore-full <archive-path>` — interactive guided wizard
- Detects encrypted archives (`.enc` suffix) and prompts for passphrase
- Shows bundled config.yaml, offers to use it or customize via wizard
- Config wizard: port, db_path, log level/path, admin username/password
- Safety: default NO on confirmation, does NOT auto-start service
- Does not touch JWT secret (users re-login after restore)

## v2 Scope (deferred)

- Alert acknowledgment (DB columns exist but no API/UI wired up)
- SSH command checks
- Service restart (local, SSH, Docker)
- MS Teams alerting
- 2FA (TOTP)
- Scripted/custom checks
- Mobile push notifications
