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
| Auth | JWT (HS256) + server-side sessions, bcrypt |
| Alerting | Resend API (v1) |
| Deployment | Single Docker image (Alpine, multi-stage) |

## RBAC — 3 Roles

3 roles: admin (full), operator (monitoring ops), viewer (read-only). JWT + server sessions, bcrypt, configurable timeout. Self-service profile/password for all. Admin seeded on first boot.

Full permission matrix, auth flow, middleware chain, session management, rate limiting: **`reference/rbac.md`**

## Check Types (v1)

| Type | Description | Key Config |
|------|-------------|------------|
| **Ping (ICMP)** | Ping host, RTT + packet loss | count, interval_ms, size |
| **HTTP/HTTPS** | GET/HEAD, status code, custom port | scheme, port, endpoint, expect_status, headers, skip_tls_verify |
| **TCP** | Port connect | port |
| **DNS** | Resolve hostname, optional expected value | query, record_type, expect_value, nameserver |
| **Page Hash** | SHA256 of response body | endpoint, auto-capture baseline on first run |
| **TLS Certificate** | Cert expiry check | port, warn_days |

## Rules Engine (Unified Target Model)

- Rules are hidden — auto-managed per target. No standalone rules API.
- Each target has `operator` (AND/OR), `category` (Network/Security/Physical Security/Key Services/Other), and a linked `rule_id`.
- **Conditions** are defined inline when creating/editing a target.
- Each condition = check definition + alert criteria (field, comparator, value, fail_count, fail_window).
- Engine evaluates after each check result. State changes trigger alerts.
- DB tables (rules, rule_conditions, rule_states) unchanged — engine/scheduler zero changes.

## Alerting

- v1: Email via Resend API
- Pluggable channel system (v2: Signal gateway, MS Teams)
- Cooldown per rule (configurable via `alert_cooldown_s`)
- Re-alert for still-firing rules (configurable via `alert_realert_s`, 0 = disabled)
- Recovery alerts always sent (bypass cooldown)

## Web UI

| Route | Page | Access |
|-------|------|--------|
| `/login` | Login | public |
| `/` | Dashboard — status bars, problems first | all |
| `/targets` | Target list + CRUD (unified with conditions) | all (CRUD: operator+) |
| `/targets/:id` | Target detail, checks, results | all |
| `/alerts` | Alert history | all |
| `/settings` | System settings — 6 tabs: General, Audit Log (operator+), Users (admin), Backup & Restore (admin), Alerting (admin), Fail2Ban (admin) | all (General), operator+ (Audit), admin (Users/Backup/Alerting/F2B) |
| `/profile` | Own profile (email, phone), password change. Accessed via user dropdown menu in navbar. | all |

### Dashboard
- Two bars per check: 90-day (1 bar/day, 90 bars) + 4-hour (1 bar/5 min, 48 bars)
- Colors: green(100%) / yellow(95-99%) / orange(80-94%) / red(<80%) / gray(no data)
- Problems sorted to top
- Hover tooltip: date/time + short status summary
- Drill-down to configure (operator+)
- 30s auto-refresh

## API & DB

31 endpoints across 10 domains. Full specs extracted to reference docs:
- **`reference/api_reference.md`** — All endpoints, auth levels, request/response JSON, error codes
- **`reference/db_schema.md`** — All tables, columns, migrations, entity relationships
- **`reference/rbac.md`** — Permission matrix, auth flow, middleware, session management

## Configuration

### config.yaml (bootstrap only, needs restart to change)
- `server.port` — web port (default 65000)
- `server.host` — bind address (default `0.0.0.0`, use `127.0.0.1` behind reverse proxy)
- `server.db_path` — SQLite path
- `auth.jwt_secret` — JWT signing key (prefer env var over config file)
- `logging.level` — debug/info/warn/error
- `logging.path` — log file path
- `init_admin.username` / `init_admin.password` — first-boot admin seed

Env var overrides: `BEKCI_JWT_SECRET`, `BEKCI_ADMIN_PASSWORD`, `BEKCI_PORT`, `BEKCI_HOST`, `BEKCI_DB_PATH`, `BEKCI_CORS_ORIGIN`.

### settings table (runtime, editable via UI)
- `session_timeout_hours` (default: 24)
- `history_days` (default: 90)
- `default_check_interval` (default: 300)
- `audit_retention_days` (default: 91) — daily hard-delete of audit entries older than this

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

## Branding

- Owl icon (`frontend/public/bekci-icon.png`) — project mascot
- Navbar: 28px icon next to "Bekci" text
- Login page: 120px centered above title
- Favicons: 32px (browser tab), 180px (apple-touch-icon), 192px (Android)

## v2 Scope (deferred)

- Alert acknowledgment (DB columns exist but no API/UI wired up)
- SSH command checks
- Service restart (local, SSH, Docker)
- Signal gateway alerting
- MS Teams alerting
- 2FA (TOTP)
- Scripted/custom checks
- Mobile push notifications
