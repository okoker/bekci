# Bekci API Reference

Base URL: `http://<host>:65000/api`

All request/response bodies are JSON (`Content-Type: application/json`) unless noted otherwise.

## Authentication

Auth uses HttpOnly cookies. On successful login, the server sets a `token` cookie (HttpOnly, Secure, SameSite=Strict). All authenticated endpoints read the JWT from this cookie automatically — no `Authorization` header needed.

**Roles** (hierarchical for RBAC checks):
- `admin` -- full access
- `operator` -- monitoring management (targets, checks, recipients, run checks, audit log, user list)
- `viewer` -- read-only dashboards and monitoring data

**Error format** (all endpoints):
```json
{ "error": "error message" }
```

**Rate limiting**: Login endpoint is rate-limited per IP -- 5 attempts per 5-minute window, 15-minute lockout after threshold.

---

## Auth

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/login` | public | Authenticate and get JWT |
| POST | `/api/logout` | any | Invalidate current session |
| GET | `/api/me` | any | Get current user profile |
| PUT | `/api/me` | any | Update own profile |
| PUT | `/api/me/password` | any | Change own password |

### POST /api/login

Rate limited: 5 attempts/5min per IP, 15min lockout.

**Request:**
```json
{
  "username": "string",
  "password": "string"
}
```

**Response (200):**

Sets `Set-Cookie: token=<jwt>; Path=/; MaxAge=<session_timeout>; HttpOnly; Secure; SameSite=Strict`

```json
{
  "user": {
    "id": "uuid",
    "username": "admin",
    "email": "admin@example.com",
    "role": "admin"
  }
}
```

| Error | Code |
|-------|------|
| Missing fields | 400 |
| Invalid credentials | 401 |
| Rate limited | 429 |

### POST /api/logout

**Response (200):**
```json
{ "message": "logged out" }
```

### GET /api/me

**Response (200):**
```json
{
  "id": "uuid",
  "username": "admin",
  "email": "admin@example.com",
  "phone": "+1234567890",
  "role": "admin",
  "status": "active"
}
```

### PUT /api/me

**Request:** (all fields optional, empty = keep current)
```json
{
  "email": "new@example.com",
  "phone": "+1234567890"
}
```

**Response (200):**
```json
{ "message": "profile updated" }
```

### PUT /api/me/password

Invalidates all other sessions on success.

**Request:**
```json
{
  "current_password": "string",
  "new_password": "string (min 15 chars)"
}
```

**Response (200):**
```json
{ "message": "password changed" }
```

| Error | Code |
|-------|------|
| Password too short (<15) | 400 |
| Wrong current password | 401 |

---

## Users

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/users` | operator+ | List all users |
| POST | `/api/users` | admin | Create user |
| GET | `/api/users/{id}` | admin | Get user by ID |
| PUT | `/api/users/{id}` | admin | Update user |
| PUT | `/api/users/{id}/suspend` | admin | Suspend/activate user |
| PUT | `/api/users/{id}/password` | admin | Reset user password |

### GET /api/users

**Response (200):** Array of User objects.
```json
[
  {
    "id": "uuid",
    "username": "admin",
    "email": "admin@example.com",
    "phone": "",
    "role": "admin",
    "status": "active",
    "created_at": "2026-01-15T10:00:00Z",
    "updated_at": "2026-01-15T10:00:00Z"
  }
]
```

### POST /api/users

**Request:**
```json
{
  "username": "string (required)",
  "email": "string",
  "password": "string (required, min 15 chars)",
  "role": "admin | operator | viewer (required)"
}
```

**Response (201):**
```json
{
  "id": "uuid",
  "username": "newuser",
  "email": "user@example.com",
  "role": "operator",
  "status": "active"
}
```

| Error | Code |
|-------|------|
| Missing required fields | 400 |
| Password too short (<15) | 400 |
| Invalid role | 400 |
| Username exists | 409 |

### GET /api/users/{id}

**Response (200):**
```json
{
  "id": "uuid",
  "username": "admin",
  "email": "admin@example.com",
  "role": "admin",
  "status": "active",
  "created_at": "2026-01-15T10:00:00Z",
  "updated_at": "2026-01-15T10:00:00Z"
}
```

### PUT /api/users/{id}

**Request:** (all fields optional, empty = keep current)
```json
{
  "email": "string",
  "phone": "string",
  "role": "admin | operator | viewer"
}
```

**Response (200):**
```json
{ "message": "user updated" }
```

| Error | Code |
|-------|------|
| Invalid role | 400 |
| User not found | 404 |
| Demoting last active admin | 409 |

### PUT /api/users/{id}/suspend

Suspending a user kills all their active sessions.

**Request:**
```json
{
  "suspended": true
}
```

**Response (200):**
```json
{ "message": "user status updated" }
```

| Error | Code |
|-------|------|
| User not found | 404 |
| Suspending last active admin | 409 |

### PUT /api/users/{id}/password

Kills all sessions for the user, forcing re-login.

**Request:**
```json
{
  "password": "string (min 15 chars)"
}
```

**Response (200):**
```json
{ "message": "password reset" }
```

---

## Targets

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/targets` | any | List target summaries |
| POST | `/api/targets` | operator+ | Create target with conditions |
| GET | `/api/targets/{id}` | any | Get target detail |
| PUT | `/api/targets/{id}` | operator+ | Update target with conditions |
| DELETE | `/api/targets/{id}` | operator+ | Delete target |
| POST | `/api/targets/{id}/pause` | operator+ | Pause target (stops checks) |
| POST | `/api/targets/{id}/unpause` | operator+ | Unpause target (resumes checks) |

### GET /api/targets

Returns summary list (no full conditions, includes condition count and state).

**Response (200):**
```json
[
  {
    "id": "uuid",
    "name": "Web Server",
    "host": "192.168.1.1",
    "description": "Main web server",
    "enabled": true,
    "preferred_check_type": "http",
    "operator": "AND",
    "category": "Network",
    "rule_id": "uuid",
    "created_at": "2026-01-15T10:00:00Z",
    "updated_at": "2026-01-15T10:00:00Z",
    "condition_count": 2,  // number of checks for this target
    "state": {
      "rule_id": "uuid",
      "current_state": "healthy",
      "last_change": "2026-01-15T10:00:00Z",
      "last_evaluated": "2026-01-15T10:05:00Z"
    }
  }
]
```

**Notes:** `state` is `null` for targets with no rule (no conditions). `state.last_change` and `state.last_evaluated` are `null` until the first evaluation.

### POST /api/targets

Creates target, checks, rule, and rule conditions in one transaction. Creator is auto-added as alert recipient.

**Request:**
```json
{
  "name": "string (required)",
  "host": "string (required)",
  "description": "string",
  "enabled": true,
  "operator": "AND (kept for backward compat, ignored by engine)",
  "category": "Network | Security | Physical Security | Key Services | Other (default: Other)",
  "preferred_check_type": "string (optional, must match a condition's check_type; defaults to first condition's type)",
  "conditions": [
    {
      "check_type": "http | tcp | ping | dns | page_hash | tls_cert (required)",
      "check_name": "string (required)",
      "config": "JSON string (default: {})",
      "interval_s": 300,
      "field": "string (default: status)",
      "comparator": "string (default: eq)",
      "value": "string (default: down)",
      "fail_count": 1,
      "fail_window": 0,
      "condition_group": 0,
      "group_operator": "AND | OR (default: AND)"
    }
  ]
}
```

**Condition groups:** Conditions are grouped by `condition_group` (integer). Within a group, conditions are combined using `group_operator` (AND/OR). Across groups, the logic is always OR — any group triggering means the target is unhealthy. Example: `(TCP AND HTTP) OR PING` = group 0 with AND (TCP, HTTP), group 1 with AND (PING).

If `group_operator` is omitted, it defaults to the top-level `operator` value for backward compatibility.

**Failure evaluation modes:**
- `fail_window > 0`: Counts consecutive matching results from newest (streak mode). Must reach `fail_count` consecutive failures to trigger the condition.
- `fail_window = 0`: Single result check ("Once" mode). Triggers on a single matching result; `fail_count` is ignored.

**Response (201):** Full TargetDetail object (see GET /api/targets/{id}).

| Error | Code |
|-------|------|
| Missing name/host | 400 |
| No conditions provided | 400 |
| Invalid operator | 400 |
| Invalid category | 400 |
| Invalid check_type | 400 |
| Missing check_name | 400 |
| Invalid group_operator | 400 |
| Duplicate target name | 409 |

### GET /api/targets/{id}

Returns full target detail with conditions, state, and recipient IDs.

**Response (200):**
```json
{
  "id": "uuid",
  "name": "Web Server",
  "host": "192.168.1.1",
  "description": "Main web server",
  "enabled": true,
  "preferred_check_type": "http",
  "operator": "AND",
  "category": "Network",
  "rule_id": "uuid",
  "created_at": "2026-01-15T10:00:00Z",
  "updated_at": "2026-01-15T10:00:00Z",
  "conditions": [
    {
      "check_id": "uuid",
      "check_type": "http",
      "check_name": "HTTP Check",
      "config": "{\"url\":\"http://192.168.1.1\"}",
      "interval_s": 300,
      "field": "status",
      "comparator": "eq",
      "value": "down",
      "fail_count": 3,
      "fail_window": 600,
      "condition_group": 0,
      "group_operator": "AND"
    }
  ],
  "state": {
    "rule_id": "uuid",
    "current_state": "healthy",
    "last_change": "2026-01-15T10:00:00Z",
    "last_evaluated": "2026-01-15T10:05:00Z"
  },
  "recipient_ids": ["uuid1", "uuid2"]
}
```

### PUT /api/targets/{id}

Smart-diffs conditions: existing checks with `check_id` are updated, new conditions (no `check_id`) create new checks, missing checks are deleted.

**Note:** A check's `type` is immutable after creation. To change a check's type (e.g. http to ping), delete the condition and add a new one with the desired type. The `check_type` field in update payloads for existing checks is ignored.

**Request:** Same structure as POST /api/targets.

Conditions can include `check_id` to update existing checks:
```json
{
  "conditions": [
    {
      "check_id": "existing-uuid",
      "check_type": "http",
      "check_name": "Updated Name",
      "config": "{}",
      "interval_s": 60
    },
    {
      "check_type": "ping",
      "check_name": "New Check"
    }
  ]
}
```

**Response (200):** Full TargetDetail object.

| Error | Code |
|-------|------|
| Missing name/host | 400 |
| No conditions provided | 400 |
| Invalid operator/category/check_type | 400 |
| Target not found | 404 |
| Duplicate target name | 409 |

### DELETE /api/targets/{id}

Deletes target, all linked checks, rule, and conditions. Triggers scheduler reload.

**Response (200):**
```json
{ "status": "ok" }
```

| Error | Code |
|-------|------|
| Target not found | 404 |

### POST /api/targets/{id}/pause

Pauses the target and stops all its checks from running. Records pause event in history.

**Response (200):**
```json
{ "status": "paused" }
```

| Error | Code |
|-------|------|
| Target not found | 404 |
| Already paused | 400 |

### POST /api/targets/{id}/unpause

Unpauses the target and immediately triggers RunNow on all its checks.

**Response (200):**
```json
{ "status": "unpaused" }
```

| Error | Code |
|-------|------|
| Target not found | 404 |
| Not paused | 400 |

---

## Alert Recipients

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/targets/{id}/recipients` | any | List alert recipients for target |
| PUT | `/api/targets/{id}/recipients` | operator+ | Set alert recipients for target |

### GET /api/targets/{id}/recipients

**Response (200):** Array of User objects (same as GET /api/users list format).
```json
[
  {
    "id": "uuid",
    "username": "admin",
    "email": "admin@example.com",
    "phone": "",
    "role": "admin",
    "status": "active",
    "created_at": "2026-01-15T10:00:00Z",
    "updated_at": "2026-01-15T10:00:00Z"
  }
]
```

### PUT /api/targets/{id}/recipients

Replaces all recipients for the target.

**Request:**
```json
{
  "user_ids": ["uuid1", "uuid2"]
}
```

**Response (200):**
```json
{ "message": "recipients updated" }
```

| Error | Code |
|-------|------|
| Target not found | 404 |

---

## Checks

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/targets/{id}/checks` | any | List checks for a target |
| POST | `/api/checks/{id}/run` | operator+ | Trigger immediate check execution |
| GET | `/api/checks/{id}/results` | any | Get recent results (last 24h) |

### GET /api/targets/{id}/checks

**Response (200):**
```json
[
  {
    "id": "uuid",
    "target_id": "uuid",
    "type": "http",
    "name": "HTTP Check",
    "config": "{\"url\":\"http://example.com\"}",
    "interval_s": 300,
    "enabled": true,
    "created_at": "2026-01-15T10:00:00Z",
    "updated_at": "2026-01-15T10:00:00Z"
  }
]
```

### POST /api/checks/{id}/run

Queues the check for immediate execution via the scheduler.

**Response (200):**
```json
{ "status": "queued" }
```

| Error | Code |
|-------|------|
| Check not found | 404 |

### GET /api/checks/{id}/results

Returns raw check results from the last 24 hours.

**Response (200):**
```json
[
  {
    "id": 1,
    "check_id": "uuid",
    "status": "up",
    "response_ms": 45,
    "message": "HTTP 200 OK",
    "metrics": "{}",
    "checked_at": "2026-01-15T10:00:00Z"
  }
]
```

---

## Check Type Configuration

The `config` field in target conditions is a JSON string with check-type-specific fields. All fields are optional — sensible defaults are applied. Config is parsed at runtime using helpers that return defaults for missing keys.

### http

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `scheme` | string | `"https"` | `http` or `https` |
| `port` | int | `0` | Port (0 = scheme default: 80/443) |
| `endpoint` | string | `"/"` | URL path |
| `expect_status` | int | `200` | Expected HTTP status code |
| `skip_tls_verify` | bool | `false` | Skip TLS cert verification |
| `timeout_s` | int | `10` | Request timeout (seconds) |

```json
{ "scheme": "https", "port": 8443, "endpoint": "/health", "expect_status": 200 }
```

Metrics: `status_code`, `url`. Status "down" if response code != `expect_status`.

### tcp

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `port` | int | `80` | Destination port |
| `timeout_s` | int | `5` | Connection timeout (seconds) |

```json
{ "port": 443, "timeout_s": 10 }
```

Metrics: `addr`. Simple connect-then-close test.

### ping

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `count` | int | `3` | ICMP packets to send |
| `timeout_s` | int | `5` | Timeout for entire sequence (seconds) |

```json
{ "count": 5, "timeout_s": 10 }
```

Metrics: `packet_loss`, `avg_rtt_ms`, `packets_sent`, `packets_recv`. Status "down" only on 100% packet loss. Requires `NET_RAW` capability or runs in unprivileged UDP mode.

### dns

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `query` | string | target host | Domain to resolve |
| `record_type` | string | `"A"` | `A`, `AAAA`, `MX`, or `CNAME` |
| `expect_value` | string | `""` | Expected value (empty = any resolution is "up") |
| `nameserver` | string | `""` | Custom nameserver (empty = system resolver) |
| `timeout_s` | int | `5` | Query timeout (seconds) |

```json
{ "query": "www.example.com", "record_type": "A", "expect_value": "93.184.216.34", "nameserver": "8.8.8.8" }
```

Metrics: `query`, `record_type`, `resolved` (array). Trailing dots stripped for CNAME comparison.

### page_hash

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `scheme` | string | `"https"` | `http` or `https` |
| `port` | int | `0` | Port (0 = scheme default) |
| `endpoint` | string | `"/"` | URL path to fetch |
| `baseline_hash` | string | `""` | SHA256 of expected body (empty = capture mode) |
| `skip_tls_verify` | bool | `false` | Skip TLS cert verification |
| `timeout_s` | int | `10` | Request timeout (seconds) |

```json
{ "endpoint": "/index.html", "baseline_hash": "e3b0c44298fc1c14..." }
```

Metrics: `hash`, `baseline_hash`, `url`, `baseline_captured`. On first run (empty `baseline_hash`), returns "up" with `baseline_captured: true` and the computed hash. Subsequent runs compare against baseline — "down" on mismatch. Body limited to 10MB.

### tls_cert

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `port` | int | `443` | TLS port |
| `warn_days` | int | `30` | Days before expiry to flag "down" |
| `timeout_s` | int | `10` | Connection timeout (seconds) |

```json
{ "port": 443, "warn_days": 14 }
```

Metrics: `days_left`, `issuer`, `subject`, `not_after`, `not_before`. Uses SNI for hostname matching. Connects with `InsecureSkipVerify` to inspect even invalid certs. Status "down" if cert expires within `warn_days` or already expired.

---

## Dashboard

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/dashboard/status` | any | Get all targets with check status |
| GET | `/api/dashboard/history/{checkId}` | any | Get check history (4h or 90d) |

### GET /api/dashboard/status

Returns enabled targets with their checks, last status, response time, and 90-day uptime. Disabled targets (`enabled=false`) are filtered out. Paused targets are included with pause metadata.

**Response (200):**
```json
[
  {
    "id": "uuid",
    "name": "Web Server",
    "host": "192.168.1.1",
    "preferred_check_type": "http",
    "state": "healthy",
    "paused": false,
    "paused_at": null,
    "category": "Network",
    "sla_status": "healthy",
    "sla_target": 99.9,
    "checks": [
      {
        "id": "uuid",
        "name": "HTTP Check",
        "type": "http",
        "enabled": true,
        "interval_s": 300,
        "last_status": "up",
        "last_message": "HTTP 200 OK",
        "response_ms": 45,
        "uptime_90d_pct": 99.95
      }
    ]
  }
]
```

**Notes:**
- Disabled targets (`enabled=false`) are excluded from the response.
- Paused targets have `state: "paused"`, `paused: true`, and `paused_at` set to an ISO 8601 timestamp.
- `sla_status` is `""` (empty) and `sla_target` is `0` when the category's SLA threshold is 0 (disabled) or not configured. `sla_status` is also `""` when no check results exist yet for the preferred check.

### GET /api/dashboard/history/{checkId}

**Query params:**

| Param | Values | Description |
|-------|--------|-------------|
| `range` | `4h` | Raw results for last 4 hours |
| `range` | `90d` (or empty) | Daily uptime percentages for last 90 days |

**Response (200) -- range=4h:** Slim format (3 fields only, for bar rendering):
```json
[
  {
    "status": "up",
    "response_ms": 45,
    "checked_at": "2026-01-15T10:00:00Z"
  }
]
```
> Note: Unlike GET /api/checks/{id}/results (which returns full CheckResult with 7 fields), this endpoint returns only the fields needed for history bar rendering.

**Response (200) -- range=90d (default):**
```json
[
  {
    "date": "2026-01-15",
    "uptime_pct": 99.5,
    "total_checks": 288
  }
]
```

---

## SLA

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/sla/history` | any | 90-day daily uptime per category |

### GET /api/sla/history

Returns all categories with per-target daily uptime arrays for the preferred check. Disabled targets (`enabled=false`) are filtered out. Categories are returned in fixed order: Network, Security, Physical Security, Key Services, Other, then any unknown categories appended.

**Response (200):**
```json
{
  "categories": [
    {
      "name": "Network",
      "sla_threshold": 99.9,
      "targets": [
        {
          "id": "uuid",
          "name": "Router-1",
          "daily_uptime": [
            { "date": "2026-01-20", "uptime_pct": 100.0 },
            { "date": "2026-01-21", "uptime_pct": 99.65 }
          ]
        }
      ]
    }
  ]
}
```

**Notes:**
- `sla_threshold` is `0` when the category has no SLA configured (disabled)
- `targets` is `[]` (empty array) for categories with no targets
- `daily_uptime` contains only days with check results (frontend pads to 90 days)
- Uses the target's preferred check type; falls back to first check if no match

---

## SOC (Status Page)

SOC endpoints return the same data as Dashboard but with conditional auth: if the `soc_public` setting is `"true"`, these endpoints are publicly accessible without authentication. Otherwise, standard cookie-based auth is required.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/soc/status` | conditional | Same as dashboard/status |
| GET | `/api/soc/history/{checkId}` | conditional | Same as dashboard/history |

Response formats are identical to their Dashboard counterparts.

---

## Alerts

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/alerts` | any | List alert history (paginated) |
| POST | `/api/settings/test-email` | admin | Send test email to current user |
| POST | `/api/settings/test-signal` | admin | Send test Signal message to a phone number |
| POST | `/api/settings/test-webhook` | admin | Send test webhook to configured endpoint |
| GET | `/api/settings/webhook-status` | admin | Get webhook delivery status |

### GET /api/alerts

**Query params:**

| Param | Default | Constraints |
|-------|---------|-------------|
| `page` | 1 | >= 1 |
| `limit` | 50 | 1-100 |

**Response (200):**
```json
{
  "entries": [
    {
      "id": 1,
      "rule_id": "uuid",
      "target_id": "uuid",
      "target_name": "Web Server",
      "recipient_id": "uuid",
      "recipient_name": "admin",
      "alert_type": "firing",
      "message": "Target Web Server is unhealthy",
      "sent_at": "2026-01-15T10:00:00Z"
    }
  ],
  "total": 42
}
```

`alert_type` values: `"firing"`, `"recovery"`, `"re-alert"`.

### POST /api/settings/test-email

Sends a test email to the authenticated user's email address. Requires the alerter service to be configured.

**Request:** No body required.

**Response (200):**
```json
{ "message": "test email sent to admin@example.com" }
```

| Error | Code |
|-------|------|
| No email on account | 400 |
| User not found | 404 |
| Email send failed | 500 |
| Alerter not initialized | 503 |

### POST /api/settings/test-signal

Sends a test Signal message to the specified phone number. Requires Signal gateway settings to be configured.

**Request:**
```json
{
  "phone": "+1234567890"
}
```

**Response (200):**
```json
{ "message": "test signal sent to +1234567890" }
```

| Error | Code |
|-------|------|
| No phone provided | 400 |
| Signal not configured | 500 |
| Signal send failed | 500 |
| Alerter not initialized | 503 |

### POST /api/settings/test-webhook

Sends a test webhook payload to the configured webhook URL. Uses the configured auth type (none, Bearer, or Basic). Requires webhook to be enabled with a URL configured.

**Request:** No body required.

**Response (200):**
```json
{ "message": "test webhook sent successfully" }
```

| Error | Code |
|-------|------|
| Webhook not configured | 500 |
| Webhook send failed | 500 |
| Alerter not initialized | 503 |

### GET /api/settings/webhook-status

Returns the last webhook delivery error and success timestamps.

**Response (200):**
```json
{
  "last_error": "2026-03-12T14:30:00Z — webhook error 500: server error",
  "last_success": "2026-03-12T14:00:00Z"
}
```

Both fields are empty strings when no webhook has been sent yet.

---

## Settings

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/settings` | any | Get all settings |
| PUT | `/api/settings` | admin | Update settings |

### GET /api/settings

Returns all settings as key-value map. Sensitive values (e.g. `resend_api_key`) are masked.

**Response (200):**
```json
{
  "session_timeout_hours": "24",
  "history_days": "90",
  "audit_retention_days": "91",
  "soc_public": "false",
  "alert_method": "email",
  "resend_api_key": "••••••••",
  "alert_from_email": "alerts@example.com",
  "alert_cooldown_s": "300",
  "alert_realert_s": "3600",
  "signal_api_url": "http://10.0.9.21:55555/v2/send",
  "signal_number": "+1234567890",
  "signal_username": "user",
  "signal_password": "••••••••",
  "sla_network": "99.9",
  "sla_security": "99.9",
  "sla_physical_security": "99.9",
  "sla_key_services": "99.9",
  "sla_other": "99.9"
}
```

### PUT /api/settings

Update one or more settings. Only known keys are accepted. Sending masked values (`"••••••••"`) for `resend_api_key` or `signal_password` is silently ignored (preserves existing value).

**Request:**
```json
{
  "session_timeout_hours": "24",
  "soc_public": "true"
}
```

**Known settings:**

| Key | Type | Validation |
|-----|------|------------|
| `session_timeout_hours` | positive integer | >= 1 |
| `history_days` | positive integer | >= 1 |
| `audit_retention_days` | positive integer | >= 1 |
| `soc_public` | boolean string | `"true"` or `"false"` |
| `alert_method` | string | `""`, `"email"`, `"signal"`, or `"email+signal"` |
| `resend_api_key` | string | any string (empty to clear) |
| `alert_from_email` | string | any string |
| `alert_cooldown_s` | non-negative integer | >= 0 |
| `alert_realert_s` | non-negative integer | >= 0 |
| `signal_api_url` | string | any string (full gateway URL) |
| `signal_number` | string | any string (sender phone number) |
| `signal_username` | string | any string |
| `signal_password` | string | any string (masked in GET as `"••••••••"`) |
| `webhook_enabled` | boolean string | `"true"` or `"false"` |
| `webhook_url` | string | must start with `http://` or `https://` (empty to clear) |
| `webhook_auth_type` | string | `""` (none), `"bearer"`, or `"basic"` |
| `webhook_bearer_token` | string | any string (masked in GET as `"••••••••"`), used when auth_type=bearer |
| `webhook_basic_username` | string | any string, used when auth_type=basic |
| `webhook_basic_password` | string | any string (masked in GET as `"••••••••"`), used when auth_type=basic |
| `webhook_skip_tls` | boolean string | `"true"` or `"false"` |
| `webhook_last_error` | string | auto-set by system (read-only in practice) |
| `webhook_last_success` | string | auto-set by system (read-only in practice) |
| `sla_network` | float string | 0–100 (0 = disabled) |
| `sla_security` | float string | 0–100 (0 = disabled) |
| `sla_physical_security` | float string | 0–100 (0 = disabled) |
| `sla_key_services` | float string | 0–100 (0 = disabled) |
| `sla_other` | float string | 0–100 (0 = disabled) |

**Response (200):**
```json
{ "message": "settings updated" }
```

| Error | Code |
|-------|------|
| Unknown setting key | 400 |
| Invalid value for type | 400 |

---

## Audit Log

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/audit-log` | operator+ | List audit log entries (paginated) |

### GET /api/audit-log

**Query params:**

| Param | Default | Constraints |
|-------|---------|-------------|
| `page` | 1 | >= 1 |
| `limit` | 50 | 1-100 |

**Response (200):**
```json
{
  "entries": [
    {
      "id": 1,
      "user_id": "uuid",
      "username": "admin",
      "action": "login",
      "resource_type": "session",
      "resource_id": "",
      "detail": "",
      "ip_address": "192.168.1.100",
      "status": "success",
      "created_at": "2026-01-15T10:00:00Z"
    }
  ],
  "total": 150,
  "page": 1,
  "limit": 50
}
```

**Audit actions:** `login`, `login_failed`, `logout`, `create_user`, `update_user`, `suspend_user`, `activate_user`, `reset_password`, `change_password`, `change_password_failed`, `update_profile`, `create_target`, `update_target`, `delete_target`, `pause_target`, `unpause_target`, `set_alert_recipients`, `update_settings`, `restore_backup`, `export_backup`, `export_full_backup`, `save_full_backup`, `download_saved_backup`, `delete_saved_backup`, `run_check`, `test_email`, `test_signal`, `test_webhook`, `webhook_dispatch`.

**`webhook_dispatch`** is logged by the system (user=system) for all webhook events. Detail contains the event type: `test`, `firing`, `recovery`, `re-alert`, or `{type} — error: {message}` on failure.

**Status values:** `success`, `failure`. All mutating actions log both success and failure (with detail). Login and change_password also log dedicated failure actions (`login_failed`, `change_password_failed`).

**IP address:** All audit entries include the client's source IP. Behind a reverse proxy, `X-Real-IP` header is used when RemoteAddr is loopback.

---

## Backup & Restore

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/backup` | admin | Download config-only backup (JSON) |
| POST | `/api/backup/restore` | admin | Restore from config backup file |
| POST | `/api/backup/full` | admin | Download full database backup (tar.gz) |
| POST | `/api/backup/full/save` | admin | Save full backup to server |
| GET | `/api/backup/full/list` | admin | List saved backups on server |
| GET | `/api/backup/full/saved/{filename}` | admin | Download a saved backup |
| DELETE | `/api/backup/full/saved/{filename}` | admin | Delete a saved backup |
| GET | `/api/backup/generate-passphrase` | admin | Generate a random 4-word passphrase |

### GET /api/backup

Returns a JSON file download containing all config data (users with hashed passwords, targets, checks, rules, settings, recipients). Does NOT include historical data (check_results, audit_logs, alert_history).

**Response headers:**
```
Content-Type: application/json
Content-Disposition: attachment; filename="bekci-backup-20260115-100000.json"
```

**Response body:** BackupData JSON structure:
```json
{
  "version": 1,
  "schema_version": 5,
  "created_at": "2026-01-15T10:00:00Z",
  "app_version": "1.2.0",
  "users": [],
  "settings": {},
  "rules": [],
  "targets": [],
  "checks": [],
  "rule_conditions": [],
  "rule_states": [],
  "recipients": []
}
```

### POST /api/backup/restore

Accepts either multipart form upload (field name: `file`) or raw JSON body. **Destructive** -- wipes all config tables and replaces with backup data. Max body: 10MB.

**Request (multipart):**
```
Content-Type: multipart/form-data
Field: file = <backup.json>
```

**Request (raw JSON):** Same BackupData structure as returned by GET /api/backup.

**Validation:**
- `version` must be `1`
- `schema_version` must not exceed current server schema
- Must contain at least one active admin user

**Response (200):**
```json
{ "message": "restore successful" }
```

| Error | Code |
|-------|------|
| Invalid JSON / form data | 400 |
| Unsupported backup version | 400 |
| Schema version too new | 400 |
| No active admin in backup | 400 |

### POST /api/backup/full

Downloads a complete database backup as a tar.gz archive containing the SQLite database file and config.yaml. Optionally encrypts the archive with AES-256-GCM (Argon2id KDF).

**Request:**
```json
{
  "encrypt": true,
  "passphrase": "string (required if encrypt=true, min 8 chars)"
}
```

Both fields are optional. Omit or send `{}` for an unencrypted backup.

**Response headers:**
```
Content-Type: application/octet-stream
Content-Disposition: attachment; filename="bekci-full-20260115-100000.tar.gz"
Content-Length: <size>
```

File extension: `.tar.gz` (plain) or `.tar.gz.enc` (encrypted).

**Archive contents (tar.gz):**
- `bekci.db` — full SQLite database snapshot (via online backup API)
- `config.yaml` — server config file (if available; omitted in env-only setups)

**Encryption format:** salt (16B) || nonce (12B) || AES-256-GCM ciphertext+tag. Key derived via Argon2id (time=3, mem=64MB, threads=4).

| Error | Code |
|-------|------|
| Passphrase too short (<8 chars) | 400 |
| Backup/encryption failure | 500 |

**Restore:** Full backups cannot be restored via the web UI. Use the CLI: `bekci restore-full <archive-path>`. See `reference/full_backup.md`.

### POST /api/backup/full/save

Same as `POST /api/backup/full` but saves the archive to the server-side backup directory instead of streaming to browser. Same JSON body (`encrypt`, `passphrase`).

**Response (200):**
```json
{ "message": "backup saved", "filename": "bekci-full-20260307-015116.tar.gz", "sha256": "82d676c68f5e..." }
```

| Error | Code |
|-------|------|
| Passphrase too short (<8 chars) | 400 |
| Backup/encryption/write failure | 500 |

### GET /api/backup/full/list

Lists saved backups on the server with metadata (SHA256 hash, size, date, encrypted flag).

**Response (200):**
```json
[
  { "filename": "bekci-full-20260307-015116.tar.gz", "sha256": "82d676c68f5e...", "size": 535424, "created_at": "2026-03-07T01:51:16Z", "encrypted": false }
]
```

### GET /api/backup/full/saved/{filename}

Download a previously saved backup file. Filename must match `^bekci-full-\d{8}-\d{6}\.tar\.gz(\.enc)?$`.

**Response:** Binary stream with `Content-Disposition: attachment`.

| Error | Code |
|-------|------|
| Invalid filename | 400 |
| Backup not found | 404 |

### DELETE /api/backup/full/saved/{filename}

Delete a saved backup file. Same filename validation.

**Response (200):**
```json
{ "message": "deleted" }
```

| Error | Code |
|-------|------|
| Invalid filename | 400 |
| Backup not found | 404 |

### GET /api/backup/generate-passphrase

Generates a random 4-word passphrase from a curated word list (~960 words, ~10 bits per word, ~40 bits total entropy).

**Response (200):**
```json
{ "passphrase": "wolf-hard-pore-jobs" }
```

---

## System

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/health` | public | Basic health check |
| GET | `/api/system/health` | any | Detailed system health |
| GET | `/api/fail2ban/status` | admin | Fail2Ban jail status |

### GET /api/health

Public liveness check.

**Response (200):**
```json
{
  "status": "ok",
  "version": "1.2.0"
}
```

### GET /api/system/health

Returns network connectivity (ICMP ping to 1.1.1.1), disk usage, and CPU load.

**Response (200):**
```json
{
  "version": "2.8.0",
  "net": {
    "status": "ok",
    "latency_ms": 12
  },
  "disk": {
    "total_gb": 50.0,
    "free_gb": 32.5
  },
  "cpu": {
    "load1": 0.45,
    "num_cpu": 4
  }
}
```

`net.status`: `"ok"` or `"unreachable"`. When unreachable, `latency_ms` is `-1`.

### GET /api/fail2ban/status

Requires `fail2ban-client` available via `sudo` on the server. Returns status of all Fail2Ban jails.

**Response (200):**
```json
{
  "jails": [
    {
      "name": "sshd",
      "currently_failed": 2,
      "total_failed": 15,
      "currently_banned": 1,
      "total_banned": 5,
      "banned_ips": ["10.0.0.50"]
    }
  ],
  "fetched_at": "2026-01-15T10:00:00Z"
}
```

| Error | Code |
|-------|------|
| fail2ban not available | 503 |
| Command timed out (5s) | 504 |

---

## Global Middleware

### CORS
Enabled only when `cors_origin` is configured (development). Allows methods: `GET, POST, PUT, DELETE, OPTIONS`. Allows headers: `Content-Type`. Sets `Access-Control-Allow-Credentials: true` for cookie-based auth. `OPTIONS` requests return `204 No Content`.

### Request Body Limits
- General endpoints: 1MB max (`readJSON` helper)
- Backup restore: 10MB max

### Logging
All HTTP requests are logged at DEBUG level with method, path, status code, and duration.
