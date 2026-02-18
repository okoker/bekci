# Bekci v2 — Design Document

## 1. Overview

Rewrite of Bekci from a YAML-configured watchdog into a web-managed monitoring platform.
Composite rules engine, RBAC, multiple check types, Docker-first deployment.

**Scope**: Monitoring only. Service restart = v2.
**Scale target**: <1000 hosts, typically <400.

## 2. Architecture

```
┌─────────────────────────────────────────────────┐
│                 Docker Container                 │
│                                                  │
│  ┌──────────┐   ┌───────────┐   ┌────────────┐  │
│  │ Vue 3    │   │  Go API   │   │  Scheduler  │  │
│  │ SPA      │──▶│  Server   │──▶│  + Engine   │  │
│  │ (static) │   │ (net/http)│   │             │  │
│  └──────────┘   └─────┬─────┘   └──────┬──────┘  │
│                       │                 │         │
│                  ┌────▼─────────────────▼───┐    │
│                  │       SQLite (WAL)        │    │
│                  │       /data/bekci.db      │    │
│                  └──────────────────────────-┘    │
│                                                  │
│  ┌──────────┐   ┌───────────┐                    │
│  │ Checkers │   │  Alerter  │──▶ Resend API      │
│  │ ping,http│   │  (email)  │   (future: Signal, │
│  │ snmp,tcp │   └───────────┘    MS Teams)        │
│  │ dns,cert │                                    │
│  │ page_hash│                                    │
│  └──────────┘                                    │
└─────────────────────────────────────────────────┘
```

Go serves both API (`/api/*`) and Vue SPA (all other routes → `index.html`).
Single binary, single container, single port.

## 3. Tech Stack

| Layer | Choice | Rationale |
|-------|--------|-----------|
| Backend | Go 1.22+ | Existing, simple, good concurrency |
| Frontend | Vue 3 + Vite | ScanTracker familiarity, SPA for RBAC |
| CSS | Hand-rolled | Continue current minimal aesthetic, no build dependency |
| DB | SQLite WAL | Single-file, no extra container, fine at <1K hosts |
| Auth | JWT (HS256) + server sessions | ScanTracker pattern, immediate logout via session kill |
| Alerting | Resend API | v1. Pluggable for Signal/Teams in v2 |
| Container | Alpine-based multi-stage | Small image, single binary |

### Go Libraries (new)
| Library | Purpose |
|---------|---------|
| `pro-bing` | ICMP ping + packet loss |
| `gosnmp` | SNMPv2c/v3 queries |
| `golang-jwt/jwt/v5` | JWT tokens |
| `golang.org/x/crypto/bcrypt` | Password hashing |

## 4. Database Schema

### Auth & Users

```sql
CREATE TABLE users (
    id          TEXT PRIMARY KEY,  -- UUID
    username    TEXT UNIQUE NOT NULL,
    email       TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role        TEXT NOT NULL CHECK(role IN ('admin','operator','viewer')),
    status      TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','suspended')),
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE sessions (
    id          TEXT PRIMARY KEY,  -- UUID
    user_id     TEXT NOT NULL REFERENCES users(id),
    expires_at  DATETIME NOT NULL,
    ip_address  TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Monitoring

```sql
CREATE TABLE projects (
    id          TEXT PRIMARY KEY,
    name        TEXT UNIQUE NOT NULL,
    description TEXT,
    created_by  TEXT REFERENCES users(id),
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE targets (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    name        TEXT NOT NULL,
    host        TEXT NOT NULL,           -- hostname or IP
    description TEXT,
    enabled     INTEGER DEFAULT 1,
    created_by  TEXT REFERENCES users(id),
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE checks (
    id          TEXT PRIMARY KEY,
    target_id   TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL,           -- ping, http, tcp, dns, snmp, page_hash, cert (ssh_command = v2)
    config      TEXT NOT NULL DEFAULT '{}', -- JSON: type-specific params
    interval_s  INTEGER NOT NULL DEFAULT 300,
    timeout_s   INTEGER NOT NULL DEFAULT 10,
    enabled     INTEGER DEFAULT 1,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- High-volume table. Composite index on (check_id, checked_at) for queries.
CREATE TABLE check_results (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    check_id    TEXT NOT NULL REFERENCES checks(id) ON DELETE CASCADE,
    status      TEXT NOT NULL,           -- up, down
    response_ms INTEGER,
    metrics     TEXT,                    -- JSON: {packet_loss, rtt_avg, rtt_max, value, hash, ...}
    error       TEXT,
    checked_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_cr_check_time ON check_results(check_id, checked_at);
```

### Rules Engine

```sql
CREATE TABLE rules (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT,
    operator    TEXT NOT NULL DEFAULT 'AND', -- AND, OR
    severity    TEXT NOT NULL DEFAULT 'critical', -- critical, warning, info
    enabled     INTEGER DEFAULT 1,
    created_by  TEXT REFERENCES users(id),
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE rule_conditions (
    id          TEXT PRIMARY KEY,
    rule_id     TEXT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
    check_id    TEXT NOT NULL REFERENCES checks(id) ON DELETE CASCADE,
    field       TEXT NOT NULL DEFAULT 'status',
        -- "status"              → compares check_results.status ("up"/"down")
        -- "response_ms"         → compares check_results.response_ms
        -- "metrics.<key>"       → compares JSON key in check_results.metrics
        --                         e.g. "metrics.packet_loss", "metrics.rtt_avg",
        --                              "metrics.days_remaining", "metrics.value"
    comparator  TEXT NOT NULL DEFAULT 'eq',     -- eq, neq, gt, lt, gte, lte
    value       TEXT NOT NULL,                  -- "down", "10", "500"
    fail_count  INTEGER DEFAULT 1,             -- must fail X times...
    fail_window INTEGER DEFAULT 0,             -- ...within Y seconds (0 = latest only)
    sort_order  INTEGER DEFAULT 0
);

CREATE TABLE rule_states (
    rule_id        TEXT PRIMARY KEY REFERENCES rules(id) ON DELETE CASCADE,
    current_state  TEXT NOT NULL DEFAULT 'healthy', -- healthy, unhealthy
    last_change    DATETIME,
    last_evaluated DATETIME
);
```

### Alerting

```sql
CREATE TABLE alert_channels (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL,           -- email (v1), signal, teams (v2)
    config      TEXT NOT NULL DEFAULT '{}', -- JSON: {api_key, from, to, ...}
    enabled     INTEGER DEFAULT 1,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE rule_alerts (
    id          TEXT PRIMARY KEY,
    rule_id     TEXT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
    channel_id  TEXT NOT NULL REFERENCES alert_channels(id),
    cooldown_s  INTEGER DEFAULT 1800,    -- 30 min default
    enabled     INTEGER DEFAULT 1
);

CREATE TABLE alert_history (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    rule_id         TEXT NOT NULL,
    channel_id      TEXT,
    alert_type      TEXT NOT NULL,       -- triggered, resolved
    message         TEXT,
    sent_at         DATETIME DEFAULT CURRENT_TIMESTAMP,
    acknowledged    INTEGER DEFAULT 0,
    acknowledged_by TEXT REFERENCES users(id),
    acknowledged_at DATETIME
);
CREATE INDEX idx_ah_rule ON alert_history(rule_id, sent_at);
```

### Settings & Purge

```sql
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
-- Defaults inserted on first run:
-- session_timeout_hours = 24
-- history_days = 90
-- default_check_interval = 300
```

## 5. RBAC

### Roles

| Role | Dashboard | Targets/Checks | Rules | Alerts | Users | Settings |
|------|-----------|----------------|-------|--------|-------|----------|
| **Admin** | view | CRUD | CRUD | view, ack, config channels | CRUD | CRUD |
| **Operator** | view | CRUD | CRUD | view, ack | view self | view |
| **Viewer** | view | view | view | view | view self | - |

### Auth Flow

```
POST /api/auth/login {username, password}
  → validate credentials (bcrypt)
  → create session in DB (configurable timeout)
  → return JWT {sub: user_id, session_id, role, exp}

Every request:
  → extract Bearer token
  → verify JWT signature
  → validate session exists + not expired in DB
  → check role against endpoint permission
  → proceed or 403
```

**No 2FA in v1.** Session-backed JWT enables immediate logout (delete session row).

### Middleware Permission Map (exhaustive)

```go
var routePermissions = map[string][]string{
    // Public (no auth)
    "POST /api/auth/login":          {},
    "GET  /api/health":              {},

    // Any authenticated user
    "POST /api/auth/logout":         {"admin", "operator", "viewer"},
    "GET  /api/auth/me":             {"admin", "operator", "viewer"},
    "PUT  /api/auth/me":             {"admin", "operator", "viewer"},
    "PUT  /api/auth/me/password":    {"admin", "operator", "viewer"},
    "GET  /api/dashboard/status":    {"admin", "operator", "viewer"},
    "GET  /api/dashboard/history/*": {"admin", "operator", "viewer"},
    "GET  /api/projects":            {"admin", "operator", "viewer"},
    "GET  /api/targets":             {"admin", "operator", "viewer"},
    "GET  /api/targets/*":           {"admin", "operator", "viewer"},
    "GET  /api/targets/*/checks":    {"admin", "operator", "viewer"},
    "GET  /api/checks/*/results":    {"admin", "operator", "viewer"},
    "GET  /api/rules":               {"admin", "operator", "viewer"},
    "GET  /api/rules/*":             {"admin", "operator", "viewer"},
    "GET  /api/alerts":              {"admin", "operator", "viewer"},

    // Operator+
    "POST /api/projects":            {"admin", "operator"},
    "PUT  /api/projects/*":          {"admin", "operator"},
    "POST /api/targets":             {"admin", "operator"},
    "PUT  /api/targets/*":           {"admin", "operator"},
    "DELETE /api/targets/*":         {"admin", "operator"},
    "POST /api/targets/*/checks":    {"admin", "operator"},
    "PUT  /api/checks/*":            {"admin", "operator"},
    "DELETE /api/checks/*":          {"admin", "operator"},
    "POST /api/checks/*/run":        {"admin", "operator"},
    "POST /api/rules":               {"admin", "operator"},
    "PUT  /api/rules/*":             {"admin", "operator"},
    "DELETE /api/rules/*":           {"admin", "operator"},
    "POST /api/alerts/*/ack":        {"admin", "operator"},

    // Admin only
    "DELETE /api/projects/*":        {"admin"},
    "GET  /api/users":               {"admin"},
    "POST /api/users":               {"admin"},
    "GET  /api/users/*":             {"admin"},
    "PUT  /api/users/*":             {"admin"},
    "DELETE /api/users/*":           {"admin"},
    "PUT  /api/users/*/password":    {"admin"},
    "GET  /api/settings":            {"admin", "operator"},
    "PUT  /api/settings":            {"admin"},
    "GET  /api/alert-channels":      {"admin"},
    "POST /api/alert-channels":      {"admin"},
    "PUT  /api/alert-channels/*":    {"admin"},
    "DELETE /api/alert-channels/*":  {"admin"},
}
```

## 6. Check Types

### 6.1 Ping (ICMP)

```json
// check.config
{
    "count": 5,          // packets to send
    "interval_ms": 200,  // between packets
    "size": 64           // packet size bytes
}
// Result metrics: {rtt_min, rtt_avg, rtt_max, packet_loss}
```
Requires `NET_RAW` Docker capability. Uses `pro-bing` library.

### 6.2 HTTP / HTTPS

```json
{
    "scheme": "https",       // http or https
    "port": 8443,            // custom port (0 = default 80/443)
    "method": "GET",
    "endpoint": "/health",
    "expect_status": 200,
    "follow_redirect": true,
    "skip_tls_verify": false,
    "headers": {"Authorization": "Bearer xxx"}  // optional
}
// Result metrics: {status_code, response_ms}
```
URL constructed as `{scheme}://{target.host}:{port}{endpoint}`. Port defaults to 80 (http) or 443 (https) if omitted.

### 6.3 TCP Port

```json
{
    "port": 5432
}
// Result metrics: {response_ms}
```

### 6.4 DNS

```json
{
    "query": "myapp.com",
    "record_type": "A",          // A, AAAA, CNAME, MX
    "expect_value": "1.2.3.4",   // optional: expected resolved value
    "nameserver": "8.8.8.8"      // optional: specific resolver
}
// Result metrics: {response_ms, resolved_value}
```

### 6.5 SNMP (v2c + v3)

Credentials stored plaintext in DB (check config JSON).

```json
// v2c
{
    "version": "2c",
    "community": "public",
    "oid": "1.3.6.1.2.1.1.3.0",
    "port": 161
}
// v3 (AuthPriv)
{
    "version": "3",
    "security_level": "authPriv",  // noAuthNoPriv, authNoPriv, authPriv
    "username": "monitor",
    "auth_protocol": "SHA",        // MD5, SHA, SHA256, SHA512
    "auth_passphrase": "...",
    "priv_protocol": "AES",        // DES, AES, AES192, AES256
    "priv_passphrase": "...",
    "oid": "1.3.6.1.2.1.1.3.0",
    "port": 161
}
// Result metrics: {value, response_ms}
```

### 6.6 Page Hash

Auto-capture: first check run stores the page hash as baseline.
Subsequent checks compare against it. User can reset baseline via UI at any time.

```json
{
    "scheme": "https",
    "port": 0,               // 0 = default 80/443
    "endpoint": "/",
    "baseline_hash": "",     // empty = auto-capture on first run
    "strip_dynamic": true    // optional: strip timestamps/tokens before hashing
}
// Result: up if hash matches baseline, down if changed
// metrics: {current_hash, baseline_hash, response_ms}
```

### 6.7 TLS Certificate

```json
{
    "port": 443,
    "warn_days": 30     // status=down if cert expires within N days
}
// Result metrics: {days_remaining, issuer, subject, expires_at}
```

### 6.8 SSH Command (v2)

Deferred to v2 along with service restart functionality.

```json
{
    "command": "pg_isready -U postgres",
    "user": "monitor",
    "key_path": "",
    "port": 22
}
```

## 7. Rules Engine

### Concepts

- **Check** = single probe, runs on interval, stores results
- **Rule** = 1+ conditions combined with AND/OR, evaluates to healthy/unhealthy
- **Condition** = "check X's [field] [comparator] [value], optionally [N times in M seconds]"

### Scheduling (Hybrid)

Scheduler maintains in-memory timers for all enabled checks.
- **Event-driven**: API sends Go channel signal when checks are created/updated/deleted → scheduler reloads immediately.
- **Safety net**: Scheduler polls DB every 60s to reconcile (catches missed signals, manual DB edits).
- On startup: load all enabled checks from DB, create timers.

### Evaluation

Trigger: after each check result is stored, evaluate all enabled rules referencing that check.

```
extract_field(result, field):
    if field == "status":     return result.status         // "up" or "down"
    if field == "response_ms": return result.response_ms   // int
    if field starts with "metrics.":
        key = field after "metrics."
        return json_extract(result.metrics, key)           // e.g. metrics.packet_loss → 12.5
    error: unknown field

matches(result, condition):
    actual = extract_field(result, condition.field)
    return compare(actual, condition.comparator, condition.value)
    // compare casts to numeric for gt/lt/gte/lte, string for eq/neq

evaluate_condition(condition):
    if condition.fail_window > 0:
        results = query check_results WHERE check_id = condition.check_id
                  AND checked_at >= now - condition.fail_window
        matching_count = count(r for r in results where matches(r, condition))
        return matching_count >= condition.fail_count
    else:
        result = latest check_result for condition.check_id
        return matches(result, condition)

for each rule referencing the completed check:
    condition_results = [evaluate_condition(c) for c in rule.conditions]

    if rule.operator == "AND": combined = all(condition_results)
    if rule.operator == "OR":  combined = any(condition_results)

    new_state = "unhealthy" if combined else "healthy"

    if new_state != rule_states.current_state:
        update rule_states
        if unhealthy: dispatch alert (triggered)
        if healthy:   dispatch alert (resolved)
```

### Rule Examples Mapped

**"ping dead AND URL unresponsive"**
```
Rule: operator=AND
  Condition 1: check=ping-server-a, field=status, comparator=eq, value=down
  Condition 2: check=http-server-a, field=status, comparator=eq, value=down
```

**"ping host-A dead AND ping host-B alive"**
```
Rule: operator=AND
  Condition 1: check=ping-host-a, field=status, comparator=eq, value=down
  Condition 2: check=ping-host-b, field=status, comparator=eq, value=up
```

**"ping unresponsive 3 times within 10 minutes"**
```
Rule: operator=AND
  Condition 1: check=ping-server, field=status, comparator=eq, value=down,
               fail_count=3, fail_window=600
```

**"packet loss above 10%"**
```
Rule: operator=AND
  Condition 1: check=ping-server, field=metrics.packet_loss, comparator=gt, value=10
```

**"page hash changed"**
```
Rule: operator=AND
  Condition 1: check=hash-myapp, field=status, comparator=eq, value=down
```

## 8. Alerting

### v1: Email via Resend

Same mechanism as current. Pluggable channel system via `alert_channels` table.

### Alert Lifecycle

```
Rule goes unhealthy
  → check cooldown (last alert for this rule+channel)
  → if not in cooldown: send alert, record in alert_history
  → dashboard shows unhealthy state immediately (no cooldown)

Rule goes healthy
  → send recovery alert (always, ignore cooldown)
  → clear cooldown
```

### Future Channels (v2)

Signal gateway and MS Teams added as new `alert_channels.type` values.
Each channel type implements a `Send(subject, body) error` interface.

## 9. Web UI

### Page Structure

| Route | Page | Roles |
|-------|------|-------|
| `/login` | Login form | public |
| `/` | Dashboard (status page) | all |
| `/targets` | Target list + CRUD | all (CRUD: operator+) |
| `/targets/:id` | Target detail, checks, results | all |
| `/rules` | Rule list + CRUD | all (CRUD: operator+) |
| `/rules/new`, `/rules/:id/edit` | Rule builder | operator+ |
| `/alerts` | Alert history + acknowledge | all (ack: operator+) |
| `/users` | User management | admin |
| `/settings` | System settings | admin (edit), operator (view) |
| `/profile` | Own profile, password change | all |

### Dashboard Design

Keep current aesthetic. Shows check results grouped by project.
**Sorting**: problems (unhealthy/down) float to top, then by project name.
**Drill-down**: operator+ can click through to configure targets/checks/rules.
**Hover**: bar tooltip shows date/time + short status summary (e.g., "02/02/2026 14:05 — 95.2% (2 failures)").

Two bars per check:

```
┌─────────────────────────────────────────────────────────┐
│ Project: ScanTracker                                     │
│                                                          │
│ ⚠ backend (https://scantracker.uk:8443/api/v1/health)   │
│   ▸ Down — "expected 200, got 502"    99.95%     45ms   │
│   ┌──────────────────────────────────────────┐           │
│   │ 90-day history (1 bar = 1 day, 90 bars)  │           │
│   └──────────────────────────────────────────┘           │
│   ┌──────────────────────────────────────────┐           │
│   │ 4-hour detail  (1 bar = 5 min, 48 bars)  │           │
│   └──────────────────────────────────────────┘           │
│                                                          │
│ frontend (https://scantracker.uk/)                       │
│   ▸ Operational     100%     32ms                        │
│   ┌──────────────────────────────────────────┐           │
│   │ 90-day history                            │           │
│   └──────────────────────────────────────────┘           │
│   ┌──────────────────────────────────────────┐           │
│   │ 4-hour detail                             │           │
│   └──────────────────────────────────────────┘           │
└──────────────────────────────────────────────────────────┘
```

Color scheme unchanged: green(100%) / yellow(95-99%) / orange(80-94%) / red(<80%) / gray(no data).
30s auto-refresh on dashboard.

### Rule Builder UI

Visual builder: select checks from dropdown, set conditions, combine with AND/OR toggle.

```
┌── Rule: "Web server fully down" ──────────────────────┐
│ Severity: [Critical ▼]         Combine with: [AND ▼]  │
│                                                        │
│ ┌─ Condition 1 ─────────────────────────────────────┐  │
│ │ Check: [ping-web-server ▼]                        │  │
│ │ Field: [status ▼]  is  [down ▼]                   │  │
│ │ Threshold: [3] times within [10] minutes           │  │
│ └───────────────────────────────────────────────────┘  │
│                                                        │
│ ┌─ Condition 2 ─────────────────────────────────────┐  │
│ │ Check: [http-web-server ▼]                        │  │
│ │ Field: [status ▼]  is  [down ▼]                   │  │
│ └───────────────────────────────────────────────────┘  │
│                                                        │
│ [+ Add Condition]                                      │
│                                                        │
│ Alert: [ops-email ▼]  Cooldown: [30m]                  │
│                                                        │
│ [Save Rule]                                            │
└────────────────────────────────────────────────────────┘
```

## 10. API Endpoints

### Auth
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| POST | `/api/auth/login` | public | Login, returns JWT |
| POST | `/api/auth/logout` | any | Kills session |
| GET | `/api/auth/me` | any | Current user info |

### Users (admin)
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/users` | admin | List users |
| POST | `/api/users` | admin | Create user |
| GET | `/api/users/:id` | admin | Get user |
| PUT | `/api/users/:id` | admin | Update user (role, status, email) |
| DELETE | `/api/users/:id` | admin | Suspend user |
| PUT | `/api/users/:id/password` | admin | Reset another user's password |

### Self-Service (any authenticated)
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/auth/me` | any | Get own profile (id, username, email, role) |
| PUT | `/api/auth/me` | any | Update own email |
| PUT | `/api/auth/me/password` | any | Change own password (requires current password) |

### Projects
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/projects` | any | List projects |
| POST | `/api/projects` | operator+ | Create project |
| PUT | `/api/projects/:id` | operator+ | Update project |
| DELETE | `/api/projects/:id` | admin | Delete project |

### Targets
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/targets` | any | List targets (filterable by project) |
| POST | `/api/targets` | operator+ | Create target |
| GET | `/api/targets/:id` | any | Target detail + checks |
| PUT | `/api/targets/:id` | operator+ | Update target |
| DELETE | `/api/targets/:id` | operator+ | Delete target + cascading checks |

### Checks
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/targets/:id/checks` | any | List checks for target |
| POST | `/api/targets/:id/checks` | operator+ | Add check to target |
| PUT | `/api/checks/:id` | operator+ | Update check |
| DELETE | `/api/checks/:id` | operator+ | Delete check |
| POST | `/api/checks/:id/run` | operator+ | Trigger immediate check |
| GET | `/api/checks/:id/results` | any | Query results (with time range) |

### Rules
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/rules` | any | List rules |
| POST | `/api/rules` | operator+ | Create rule + conditions |
| GET | `/api/rules/:id` | any | Rule detail + conditions + state |
| PUT | `/api/rules/:id` | operator+ | Update rule |
| DELETE | `/api/rules/:id` | operator+ | Delete rule |

### Alerts
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/alerts` | any | Alert history (paginated) |
| POST | `/api/alerts/:id/ack` | operator+ | Acknowledge alert |
| GET | `/api/alert-channels` | admin | List channels |
| POST | `/api/alert-channels` | admin | Create channel |
| PUT | `/api/alert-channels/:id` | admin | Update channel |
| DELETE | `/api/alert-channels/:id` | admin | Delete channel |

### Dashboard
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/dashboard/status` | any | Full status for dashboard rendering |
| GET | `/api/dashboard/history/:check_id` | any | `?range=90d` or `?range=4h` |

### Settings
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/settings` | admin | All settings |
| PUT | `/api/settings` | admin | Update settings |

### System
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/health` | public | Self-check |

## 11. Configuration

### config.yaml (bootstrap only)

```yaml
# Bootstrap config — read once at startup. NOT runtime-editable.
# These are process-level concerns that require a restart to change.
server:
  port: 65000              # or env: BEKCI_PORT
  db_path: /data/bekci.db  # or env: BEKCI_DB_PATH

auth:
  jwt_secret: ""           # or env: BEKCI_JWT_SECRET (required, no default)

logging:
  level: warn              # debug, info, warn, error
  path: /data/bekci.log

# Initial admin created on first run if no users exist
init_admin:
  username: admin
  password: ""             # or env: BEKCI_ADMIN_PASSWORD (required on first run)
```

Env vars override YAML: `BEKCI_JWT_SECRET`, `BEKCI_ADMIN_PASSWORD`, `BEKCI_PORT`, `BEKCI_DB_PATH`.

### Precedence: config.yaml vs settings table

| Concern | Where it lives | Why |
|---------|---------------|-----|
| Port, DB path, log path, JWT secret | `config.yaml` only | Process-level, needs restart |
| Session timeout, history days, default check interval | `settings` table only | Runtime-editable via UI |
| Initial admin credentials | `config.yaml` only | Used once on first boot to seed first user |

**No overlap.** Config.yaml handles process bootstrap. Settings table handles runtime behavior.
On first boot, the settings table is seeded with hardcoded defaults (not from config.yaml).

## 12. Docker

### Dockerfile (multi-stage)

```
Stage 1 — frontend build:
  FROM node:20-alpine
  WORKDIR /app/frontend
  npm ci && npm run build → /app/frontend/dist

Stage 2 — backend build:
  FROM golang:1.22-alpine
  CGO_ENABLED=1 (for SQLite)
  COPY frontend/dist → embed in binary
  go build → /app/bekci

Stage 3 — runtime:
  FROM alpine:3.19
  apk add ca-certificates libcap
  COPY --from=stage2 /app/bekci /usr/local/bin/bekci
  setcap cap_net_raw+ep /usr/local/bin/bekci   # for ICMP ping
  EXPOSE 65000
  VOLUME /data
  ENTRYPOINT ["bekci"]
```

### Run

```bash
docker run -d \
  --name bekci \
  --cap-add NET_RAW \
  -p 65000:65000 \
  -v bekci-data:/data \
  -e BEKCI_JWT_SECRET=your-secret \
  -e BEKCI_ADMIN_PASSWORD=your-password \
  bekci:latest
```

## 13. Go Package Structure

```
bekci/
├── cmd/bekci/main.go           # entry point, wiring
├── internal/
│   ├── config/                 # bootstrap YAML loader
│   ├── store/                  # SQLite: schema, migrations, queries
│   │   ├── store.go            # connection, migrate, close
│   │   ├── users.go            # user CRUD
│   │   ├── targets.go          # target + check CRUD
│   │   ├── rules.go            # rule CRUD
│   │   ├── results.go          # check results + daily stats
│   │   ├── alerts.go           # alert history + channels
│   │   └── settings.go         # key-value settings
│   ├── auth/                   # JWT, bcrypt, session validation
│   ├── api/                    # HTTP handlers
│   │   ├── router.go           # route registration
│   │   ├── middleware.go        # auth, CORS, logging
│   │   ├── auth_handlers.go
│   │   ├── user_handlers.go
│   │   ├── target_handlers.go
│   │   ├── check_handlers.go
│   │   ├── rule_handlers.go
│   │   ├── alert_handlers.go
│   │   ├── dashboard_handlers.go
│   │   └── settings_handlers.go
│   ├── checker/                # check execution
│   │   ├── checker.go          # registry + dispatch
│   │   ├── ping.go
│   │   ├── http.go
│   │   ├── tcp.go
│   │   ├── dns.go
│   │   ├── snmp.go
│   │   ├── page_hash.go
│   │   └── cert.go             # (ssh_command.go = v2)
│   ├── engine/                 # rules evaluation
│   │   └── engine.go           # evaluate rules after check results
│   ├── scheduler/              # check scheduling + dispatch
│   │   └── scheduler.go
│   ├── alerter/                # alert dispatching
│   │   ├── alerter.go          # interface + dispatch
│   │   └── email.go            # Resend implementation
│   └── sshutil/                # SSH helpers (v2, kept for reference)
├── frontend/                   # Vue 3 project
│   ├── src/
│   │   ├── views/              # LoginView, DashboardView, TargetsView, ...
│   │   ├── components/         # UptimeBar, StatusBadge, RuleBuilder, ...
│   │   ├── stores/             # auth.js, dashboard.js
│   │   ├── router/             # index.js with route guards
│   │   └── api/                # HTTP client wrapper
│   ├── package.json
│   └── vite.config.js
├── config.yaml
├── Dockerfile
├── Makefile
└── docs/
```

## 14. Implementation Phases

### Phase 1 — Foundation
- New SQLite schema with migrations
- Bootstrap config loader (YAML + env vars)
- Auth system: JWT + sessions + bcrypt
- User CRUD API
- Vue shell: login, router with guards, layout
- User management page (admin)
- Docker build pipeline

### Phase 2 — Monitoring Core
- Project/Target/Check CRUD (API + Vue pages)
- Check type implementations: ping, http, tcp, dns, snmp, page_hash, cert
- Hybrid scheduler: event-driven + 60s poll safety net
- Check results storage + purge routine
- Dashboard with dual uptime bars (90d + 4h), problem-first sorting, hover tooltips

### Phase 3 — Rules Engine
- Rule + condition CRUD (API + Vue)
- Rule builder UI
- Evaluation engine (triggered after each check result)
- Rule state tracking

### Phase 4 — Alerting
- Alert channel management (API + Vue)
- Email (Resend) sender
- Alert lifecycle: trigger, resolve, cooldown
- Alert history page + acknowledge UI

### Phase 5 — Polish
- Settings management page
- Profile / password change
- Error handling, loading states, edge cases
- Logging, health endpoint
- Dockerfile finalization + docker-compose.yml for dev
- Manual verification + testing

## 15. Decisions Log

| # | Question | Decision |
|---|----------|----------|
| 1 | SNMP credentials | Plaintext in DB. Acceptable for internal monitoring tool. |
| 2 | Scheduler strategy | Hybrid: event-driven + 60s poll safety net |
| 3 | Dashboard scope | Shows check results (bars) with problem-first sorting. Drill-down to configure for operator+. |
| 4 | SSH checks | Deferred to v2 (along with service restart) |
| 5 | Frontend CSS | Hand-rolled, continue current aesthetic |
| 6 | Page hash baseline | Auto-capture on first run. User can reset via UI. |
| 7 | Config migration | No migration from v1 YAML. Clean start via web UI. |

## 16. v2 Scope (deferred)

- SSH command checks
- Service restart (local, SSH, Docker)
- Signal gateway alerting
- MS Teams alerting
- 2FA (TOTP)
- Scripted/custom checks
- Mobile push notifications
