# Bekci v2 — Requirements

## Overview

Web-managed monitoring platform. Composite rules engine, RBAC, multiple check types, Docker-first.
Monitoring only in v1. Service restart, SSH checks = v2.
Scale: <1000 hosts, typically <400.

## Tech Stack

| Layer | Choice |
|-------|--------|
| Backend | Go 1.22+, net/http, no framework |
| Frontend | Vue 3 + Vite, hand-rolled CSS |
| DB | SQLite WAL |
| Auth | JWT (HS256) + server-side sessions, bcrypt |
| Alerting | Resend API (v1) |
| Deployment | Single Docker image (Alpine, multi-stage) |

## RBAC — 3 Roles

| Role | Dashboard | Targets/Checks | Rules | Alerts | Users | Settings |
|------|-----------|----------------|-------|--------|-------|----------|
| **Admin** | view | CRUD | CRUD | view, ack, config channels | CRUD | read/write |
| **Operator** | view | CRUD | CRUD | view, ack | view self | read |
| **Viewer** | view | view | view | view | view self | - |

Auth: JWT + server sessions. No 2FA in v1. bcrypt passwords. Configurable session timeout.
Self-service: any user can view own profile, update email and phone, change own password.
Initial admin seeded from config.yaml / env vars on first boot.

## Check Types (v1)

| Type | Description | Key Config |
|------|-------------|------------|
| **Ping (ICMP)** | Ping host, RTT + packet loss | count, interval_ms, size |
| **HTTP/HTTPS** | GET/HEAD, status code, custom port | scheme, port, endpoint, expect_status, headers, skip_tls_verify |
| **TCP** | Port connect | port |
| **DNS** | Resolve hostname, optional expected value | query, record_type, expect_value, nameserver |
| **SNMP v2c/v3** | Query OID, compare value | version, community/auth creds, oid, security_level |
| **Page Hash** | SHA256 of response body | endpoint, auto-capture baseline on first run |
| **TLS Certificate** | Cert expiry check | port, warn_days |

## Rules Engine (Unified Target Model)

- Rules are hidden — auto-managed per target. No standalone rules API.
- Each target has `operator` (AND/OR), `severity` (critical/warning/info), and a linked `rule_id`.
- **Conditions** are defined inline when creating/editing a target.
- Each condition = check definition + alert criteria (field, comparator, value, fail_count, fail_window).
- Engine evaluates after each check result. State changes trigger alerts.
- DB tables (rules, rule_conditions, rule_states) unchanged — engine/scheduler zero changes.

## Alerting

- v1: Email via Resend API
- Pluggable channel system (v2: Signal gateway, MS Teams)
- Cooldown per rule+channel
- Recovery alerts always sent (bypass cooldown)
- Alert acknowledgment by operator+

## Web UI

| Route | Page | Access |
|-------|------|--------|
| `/login` | Login | public |
| `/` | Dashboard — status bars, problems first | all |
| `/targets` | Target list + CRUD (unified with conditions) | all (CRUD: operator+) |
| `/targets/:id` | Target detail, checks, results | all |
| `/alerts` | Alert history + acknowledge | all (ack: operator+) |
| `/settings` | System settings — 6 tabs: General, Audit Log (operator+), Users (admin), Backup & Restore (admin), Alerting (admin), Fail2Ban (admin) | all (General), operator+ (Audit), admin (Users/Backup/Alerting/F2B) |
| `/profile` | Own profile (email, phone), password change. Accessed via user dropdown menu in navbar. | all |

### Dashboard
- Two bars per check: 90-day (1 bar/day, 90 bars) + 4-hour (1 bar/5 min, 48 bars)
- Colors: green(100%) / yellow(95-99%) / orange(80-94%) / red(<80%) / gray(no data)
- Problems sorted to top
- Hover tooltip: date/time + short status summary
- Drill-down to configure (operator+)
- 15s auto-refresh

## API Endpoints

### Auth
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| POST | `/api/auth/login` | public | Login → JWT |
| POST | `/api/auth/logout` | any | Kill session |
| GET | `/api/auth/me` | any | Own profile |
| PUT | `/api/auth/me` | any | Update own email |
| PUT | `/api/auth/me/password` | any | Change own password |

### Users (admin only)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/users` | List users |
| POST | `/api/users` | Create user |
| GET | `/api/users/:id` | Get user |
| PUT | `/api/users/:id` | Update user |
| DELETE | `/api/users/:id` | Suspend user |
| PUT | `/api/users/:id/password` | Reset password |

### Targets (Unified — includes conditions)
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/targets` | any | List with condition_count + state |
| POST | `/api/targets` | operator+ | Create with conditions (auto-manages rule) |
| GET | `/api/targets/:id` | any | Detail + conditions + state |
| PUT | `/api/targets/:id` | operator+ | Update + smart-diff conditions |
| DELETE | `/api/targets/:id` | operator+ | Delete (cascades checks + rule) |

**Target create/update body**:
```json
{
  "name": "Web Server", "host": "example.com", "description": "...",
  "enabled": true, "operator": "OR", "severity": "critical",
  "conditions": [
    {
      "check_id": "",
      "check_type": "ping", "check_name": "Ping", "config": "{\"count\":3}",
      "interval_s": 60, "field": "status", "comparator": "eq", "value": "down",
      "fail_count": 1, "fail_window": 0
    }
  ]
}
```

**Target detail response** includes `conditions[]` and `state: { rule_id, current_state, last_change, last_evaluated }`.

### Checks (read-only + run)
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/targets/:id/checks` | any | List checks for target |
| POST | `/api/checks/:id/run` | operator+ | Trigger immediate check |
| GET | `/api/checks/:id/results` | any | Query results (time range) |

### Alerts
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/alerts` | any | Alert history (paginated: `?page=1&limit=50`) |
| GET | `/api/targets/:id/recipients` | any | List alert recipients for target |
| PUT | `/api/targets/:id/recipients` | operator+ | Set recipients: `{ user_ids: [...] }` |
| POST | `/api/settings/test-email` | admin | Send test email to current user |

**Alert response**: `{ entries: [...], total: N }` where each entry has `id, rule_id, target_id, target_name, recipient_id, recipient_name, alert_type, message, sent_at`.

**Target detail response** now includes `recipient_ids: [...]`.

**Alerting settings** (in settings table):
- `alert_method` — `email` | `signal` | `email+signal`
- `resend_api_key` — Resend API key (masked in GET response)
- `alert_from_email` — Sender email address
- `alert_cooldown_s` — Min seconds between alerts per rule (default 1800)
- `alert_realert_s` — Re-alert interval for ongoing issues (0 = disabled, default 3600)

### Dashboard
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/dashboard/status` | any | Flat `[]dashboardTarget` with per-target state+severity |
| GET | `/api/dashboard/history/:check_id` | any | `?range=90d` or `?range=4h` |
| GET | `/api/soc/status` | configurable | Flat `[]dashboardTarget` (no rules_summary) |
| GET | `/api/soc/history/:check_id` | configurable | Same as dashboard history |

### Settings
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/settings` | admin, operator | Read settings |
| PUT | `/api/settings` | admin | Update settings |

### Backup & Restore (admin only)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/backup` | Download JSON backup (config only — no check results) |
| POST | `/api/backup/restore` | Wipe DB + restore from JSON. Invalidates all sessions. |

**Backup JSON format** (version 1):
```json
{
  "version": 1, "schema_version": 7, "created_at": "...", "app_version": "2.0.0",
  "users": [], "settings": {}, "rules": [], "targets": [],
  "checks": [], "rule_conditions": [], "rule_states": []
}
```

**Restore behaviour**: Single atomic transaction — deletes all config tables (leaf→root), inserts from backup (root→leaf). On any error, entire transaction rolls back. After success, scheduler reloads. Passwords (bcrypt hashes) are preserved. 10MB body limit.

**Validation**: version must be 1, schema_version ≤ 7, at least one active admin user required.

**Frontend**: Backup & Restore tab on Settings page (admin only). Download button triggers blob download. File picker + warning banner + confirm dialog for restore. On success: clears auth, redirects to login.

### Fail2Ban Status (Settings → Fail2Ban tab, admin only)

Settings page uses 5 tabs: **General** (settings form, all users) | **Audit Log** (operator+, loads on tab switch) | **Users** (admin only, loads on tab switch) | **Backup & Restore** (admin only) | **Fail2Ban** (admin only). Old `/audit-log` and `/users` URLs redirect to `/settings`.

Fail2Ban tab displays jail status table with columns: Jail, Active Bans, Bans (total), Failed (window), Failed (total), Show IPs. Status badges: red for active bans > 0, green for 0; amber for active failures. Expandable row shows banned IPs (monospace, red-tinted). Auto-refresh every 30s (stops on tab switch/unmount). Refresh button + "last updated" timestamp. Graceful error when fail2ban unavailable.

### Fail2Ban (admin only)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/fail2ban/status` | Read-only jail status (shells out to `fail2ban-client`) |

**Response**:
```json
{
  "jails": [
    { "name": "sshd", "currently_failed": 0, "total_failed": 15,
      "currently_banned": 1, "total_banned": 8, "banned_ips": ["1.2.3.4"] }
  ],
  "fetched_at": "2026-02-17T01:18:00Z"
}
```

**Error codes**: 503 (fail2ban not installed), 504 (timeout). 5s exec timeout. Jail names validated against `^[a-zA-Z0-9_-]+$`.

**Prod config**: bekci user has sudoers access to `fail2ban-client status` only. Filter at `/etc/fail2ban/filter.d/bekci.conf` parses `slog.Warn("Login failed")` lines. `bekci-login` jail: 10 retries / 10min window / 30min ban.

### System
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/health` | public | Public self-check |
| GET | `/api/system/health` | any | Net/Disk/CPU vitals for navbar indicator |

**System health response**:
```json
{
  "net":  { "status": "ok", "latency_ms": 12 },
  "disk": { "total_gb": 60, "free_gb": 56 },
  "cpu":  { "load1": 0.3, "num_cpu": 4 }
}
```

**Navbar health indicator**: 3 dots (Net/Disk/CPU) with green/yellow/red thresholds. Click opens popover with details. Polls every 30s. Grey dots when endpoint unreachable.

| Metric | Green | Yellow | Red |
|--------|-------|--------|-----|
| Net | reachable | — | unreachable |
| Disk | >20% free | 10-20% free | <10% free |
| Load | <N cores | <2×N cores | ≥2×N cores |

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
| 1 | SNMP credentials plaintext in DB |
| 2 | Hybrid scheduler: event-driven + 60s poll safety net |
| 3 | Dashboard shows check results, problems first, drill-down to configure |
| 4 | SSH checks deferred to v2 |
| 5 | Hand-rolled CSS, continue current aesthetic |
| 6 | Page hash auto-capture on first run |
| 7 | No migration from v1 YAML config — clean start via web UI |
| 8 | Config.yaml = process bootstrap, settings table = runtime. Zero overlap. |
| 9 | SNMP deferred — not in Phase 2 |
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

- SSH command checks
- Service restart (local, SSH, Docker)
- Signal gateway alerting
- MS Teams alerting
- 2FA (TOTP)
- Scripted/custom checks
- Mobile push notifications
