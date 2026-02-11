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
Self-service: any user can view own profile, update email, change own password.
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

## Rules Engine

- **Rule** = 1+ conditions combined with AND/OR
- **Condition** = check X's field comparator value, optionally N times in M seconds
- **Field** = `status`, `response_ms`, or `metrics.<key>` (e.g., `metrics.packet_loss`)
- Evaluated after each check result. State changes trigger alerts.
- Severity levels: critical, warning, info

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
| `/targets` | Target list + CRUD | all (CRUD: operator+) |
| `/targets/:id` | Target detail, checks, results | all |
| `/rules` | Rule list + CRUD | all (CRUD: operator+) |
| `/rules/new`, `/rules/:id/edit` | Rule builder | operator+ |
| `/alerts` | Alert history + acknowledge | all (ack: operator+) |
| `/users` | User management | admin |
| `/settings` | System settings | admin (edit), operator (view) |
| `/profile` | Own profile, password change | all |

### Dashboard
- Two bars per check: 90-day (1 bar/day, 90 bars) + 4-hour (1 bar/5 min, 48 bars)
- Colors: green(100%) / yellow(95-99%) / orange(80-94%) / red(<80%) / gray(no data)
- Problems sorted to top
- Hover tooltip: date/time + short status summary
- Drill-down to configure (operator+)
- 30s auto-refresh

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

### Projects
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/projects` | any | List |
| POST | `/api/projects` | operator+ | Create |
| PUT | `/api/projects/:id` | operator+ | Update |
| DELETE | `/api/projects/:id` | admin | Delete |

### Targets
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/targets` | any | List (filterable by project) |
| POST | `/api/targets` | operator+ | Create |
| GET | `/api/targets/:id` | any | Detail + checks |
| PUT | `/api/targets/:id` | operator+ | Update |
| DELETE | `/api/targets/:id` | operator+ | Delete (cascades checks) |

### Checks
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/targets/:id/checks` | any | List checks for target |
| POST | `/api/targets/:id/checks` | operator+ | Add check |
| PUT | `/api/checks/:id` | operator+ | Update check |
| DELETE | `/api/checks/:id` | operator+ | Delete check |
| POST | `/api/checks/:id/run` | operator+ | Trigger immediate check |
| GET | `/api/checks/:id/results` | any | Query results (time range) |

### Rules
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/rules` | any | List |
| POST | `/api/rules` | operator+ | Create + conditions |
| GET | `/api/rules/:id` | any | Detail + conditions + state |
| PUT | `/api/rules/:id` | operator+ | Update |
| DELETE | `/api/rules/:id` | operator+ | Delete |

### Alerts
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/alerts` | any | History (paginated) |
| POST | `/api/alerts/:id/ack` | operator+ | Acknowledge |
| GET | `/api/alert-channels` | admin | List channels |
| POST | `/api/alert-channels` | admin | Create channel |
| PUT | `/api/alert-channels/:id` | admin | Update channel |
| DELETE | `/api/alert-channels/:id` | admin | Delete channel |

### Dashboard
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/dashboard/status` | any | Full status |
| GET | `/api/dashboard/history/:check_id` | any | `?range=90d` or `?range=4h` |

### Settings
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/settings` | admin, operator | Read settings |
| PUT | `/api/settings` | admin | Update settings |

### System
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/health` | Public self-check |

## Configuration

### config.yaml (bootstrap only, needs restart to change)
- `server.port` — web port (default 65000)
- `server.db_path` — SQLite path
- `auth.jwt_secret` — JWT signing key (required, no default)
- `logging.level` — debug/info/warn/error
- `logging.path` — log file path
- `init_admin.username` / `init_admin.password` — first-boot admin seed

Env var overrides: `BEKCI_JWT_SECRET`, `BEKCI_ADMIN_PASSWORD`, `BEKCI_PORT`, `BEKCI_DB_PATH`.

### settings table (runtime, editable via UI)
- `session_timeout_hours` (default: 24)
- `history_days` (default: 90)
- `default_check_interval` (default: 300)

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

## v2 Scope (deferred)

- SSH command checks
- Service restart (local, SSH, Docker)
- Signal gateway alerting
- MS Teams alerting
- 2FA (TOTP)
- Scripted/custom checks
- Mobile push notifications
