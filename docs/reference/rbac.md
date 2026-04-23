# RBAC Reference

## Roles

| Role | Purpose |
|------|---------|
| `admin` | Full system control. User management, settings, backup/restore, all monitoring operations. |
| `operator` | Monitoring operations. Create/edit/delete targets, run checks, manage alert recipients, view audit log. Cannot manage users or system settings. |
| `viewer` | Read-only access. View targets, checks, results, dashboard, alerts. Cannot create or modify anything except own profile/password. |

Valid roles enforced at user creation and update: `admin`, `operator`, `viewer`. Any other value is rejected (400).

## Auth Flow

### Login (`POST /api/login`)

1. Rate limiter checks IP -- reject with 429 if locked out
2. Parse username/password from JSON body
3. `auth.Service.Login()`:
   - Look up user by username
   - Verify status is `active` (reject suspended accounts)
   - bcrypt verify password (cost 12)
   - Read `session_timeout_hours` setting (default: 24h)
   - Create session record in DB (UUID, user_id, expires_at, ip_address)
   - Sign JWT (HS256) with claims: `sub` (user_id), `sid` (session_id), `role`, `exp`, `iat`
4. On success: reset rate limiter for IP, set HttpOnly cookie (`token`; Secure; SameSite=Strict; MaxAge=session duration), return user info in JSON body
5. On failure: record failure in rate limiter, audit log entry. All failures return generic 401 to prevent enumeration

### Token Structure (JWT HS256)

```
{
  "sid":  "session-uuid",
  "role": "admin|operator|viewer",
  "sub":  "user-uuid",
  "exp":  <unix timestamp>,
  "iat":  <unix timestamp>
}
```

### Token Validation (`auth.Service.ValidateToken`)

1. Parse JWT, verify HS256 signature
2. Check session exists in DB (catches logged-out tokens)
3. Check session not expired (server-side expiry check independent of JWT `exp`)
4. If expired: delete session from DB, reject

### Request Auth (`requireAuth` middleware)

1. Extract JWT from `token` HttpOnly cookie
2. Call `ValidateToken` (JWT + session check)
3. Fetch user from DB by `claims.Subject` (user ID)
4. Verify user status is `active` (catches mid-session suspensions)
5. **Refresh role from DB** -- overrides JWT role claim with current DB role (handles role changes without re-login)
6. Store claims in request context

## Middleware Chain

```
Request
  -> gzipMiddleware (compress response if Accept-Encoding: gzip)
    -> recoveryMiddleware (panic recovery, returns 500)
      -> loggingMiddleware (DEBUG: all requests; WARN: 5xx with IP + user_id)
        -> corsMiddleware (CORS headers if origin configured)
          -> route match
            -> [requireAuth] (cookie JWT + session + active user check)
              -> [requireRole("admin", ...)] (role whitelist check)
                -> handler
```

### Auth Wrappers (defined in router.go)

| Wrapper | Middleware chain | Allowed roles |
|---------|-----------------|---------------|
| (none) | No auth | Public/anonymous |
| `anyAuth` | `requireAuth` | admin, operator, viewer |
| `opAuth` | `requireAuth` -> `requireRole("admin", "operator")` | admin, operator |
| `adminAuth` | `requireAuth` -> `requireRole("admin")` | admin |
| `socAuth` | Conditional: if `soc_public` setting is `"true"`, skip auth entirely; otherwise fall through to `requireAuth` | See SOC section |

## Permission Matrix

### Auth

| Endpoint | Method | Auth | Admin | Operator | Viewer | Notes |
|----------|--------|------|-------|----------|--------|-------|
| `/api/login` | POST | public | -- | -- | -- | Rate limited by IP + username |
| `/api/health` | GET | public | -- | -- | -- | Unauthenticated health probe |
| `/api/logout` | POST | anyAuth | Y | Y | Y | Deletes own session |
| `/api/me` | GET | anyAuth | Y | Y | Y | Own profile info |
| `/api/me` | PUT | anyAuth | Y | Y | Y | Update own email/phone |
| `/api/me/password` | PUT | anyAuth | Y | Y | Y | Change own password (requires current) |

### Users

| Endpoint | Method | Auth | Admin | Operator | Viewer | Notes |
|----------|--------|------|-------|----------|--------|-------|
| `/api/users` | GET | opAuth | Y | Y | N | Operators need this for recipient picker |
| `/api/users` | POST | adminAuth | Y | N | N | Create user |
| `/api/users/{id}` | GET | adminAuth | Y | N | N | Get user detail |
| `/api/users/{id}` | PUT | adminAuth | Y | N | N | Update user (email, phone, role) |
| `/api/users/{id}/suspend` | PUT | adminAuth | Y | N | N | Suspend/activate user |
| `/api/users/{id}/password` | PUT | adminAuth | Y | N | N | Admin reset of user password |

### Targets

| Endpoint | Method | Auth | Admin | Operator | Viewer | Notes |
|----------|--------|------|-------|----------|--------|-------|
| `/api/targets` | GET | anyAuth | Y | Y | Y | List all targets |
| `/api/targets` | POST | opAuth | Y | Y | N | Create target |
| `/api/targets/{id}` | GET | anyAuth | Y | Y | Y | Get target detail |
| `/api/targets/{id}` | PUT | opAuth | Y | Y | N | Update target |
| `/api/targets/{id}` | DELETE | opAuth | Y | Y | N | Delete target |
| `/api/targets/{id}/pause` | POST | opAuth | Y | Y | N | Pause target (skip checks) |
| `/api/targets/{id}/unpause` | POST | opAuth | Y | Y | N | Unpause target |
| `/api/targets/{id}/recipients` | GET | opAuth | Y | Y | N | List alert recipients |
| `/api/targets/{id}/recipients` | PUT | opAuth | Y | Y | N | Set alert recipients |

### Checks

| Endpoint | Method | Auth | Admin | Operator | Viewer | Notes |
|----------|--------|------|-------|----------|--------|-------|
| `/api/targets/{id}/checks` | GET | anyAuth | Y | Y | Y | List checks for target |
| `/api/checks/{id}/run` | POST | opAuth | Y | Y | N | Trigger immediate check run |
| `/api/checks/{id}/results` | GET | anyAuth | Y | Y | Y | View check results |

### Dashboard

| Endpoint | Method | Auth | Admin | Operator | Viewer | Notes |
|----------|--------|------|-------|----------|--------|-------|
| `/api/dashboard/status` | GET | anyAuth | Y | Y | Y | Overview status |
| `/api/dashboard/history/{checkId}` | GET | anyAuth | Y | Y | Y | Check history chart data |

### SLA

| Endpoint | Method | Auth | Admin | Operator | Viewer | Notes |
|----------|--------|------|-------|----------|--------|-------|
| `/api/sla/history` | GET | anyAuth | Y | Y | Y | 90-day daily uptime per category |

### Alerts

| Endpoint | Method | Auth | Admin | Operator | Viewer | Notes |
|----------|--------|------|-------|----------|--------|-------|
| `/api/alerts` | GET | anyAuth | Y | Y | Y | List alerts |

### SOC (Security Operations Center)

| Endpoint | Method | Auth | Admin | Operator | Viewer | Notes |
|----------|--------|------|-------|----------|--------|-------|
| `/api/soc/status` | GET | socAuth | Y | Y | Y | Conditional auth (see below) |
| `/api/soc/history/{checkId}` | GET | socAuth | Y | Y | Y | Conditional auth (see below) |

**SOC conditional auth**: When `soc_public` setting is `"true"`, these endpoints are fully public (no cookie required). Otherwise, standard `requireAuth` applies (any authenticated role).

### Tags

| Endpoint | Method | Auth | Admin | Operator | Viewer | Notes |
|----------|--------|------|-------|----------|--------|-------|
| `/api/tags` | GET | socAuth | Y | Y | Y | List tag options. Required param: `?group=project\|location\|category`; missing/invalid returns 400. Public when `soc_public=true` (needed by public SocView filter dropdowns) |
| `/api/tags` | POST | adminAuth | Y | N | N | Create tag option |
| `/api/tags/{id}` | PUT | adminAuth | Y | N | N | Rename tag option (category: cascades to targets + SLA key) |
| `/api/tags/{id}` | DELETE | adminAuth | Y | N | N | Delete tag option (category: blocked if in use) |

### Settings

| Endpoint | Method | Auth | Admin | Operator | Viewer | Notes |
|----------|--------|------|-------|----------|--------|-------|
| `/api/settings` | GET | anyAuth | Y | Y | Y | View all settings |
| `/api/settings` | PUT | adminAuth | Y | N | N | Update settings |
| `/api/settings/test-email` | POST | adminAuth | Y | N | N | Send test email |
| `/api/settings/test-signal` | POST | adminAuth | Y | N | N | Send test Signal message |
| `/api/settings/test-webhook` | POST | adminAuth | Y | N | N | Send test webhook |
| `/api/settings/webhook-status` | GET | adminAuth | Y | N | N | Get webhook delivery status |

### System

| Endpoint | Method | Auth | Admin | Operator | Viewer | Notes |
|----------|--------|------|-------|----------|--------|-------|
| `/api/system/health` | GET | socAuth | Y | Y | Y | Detailed system health (public when soc_public=true) |
| `/api/fail2ban/status` | GET | adminAuth | Y | N | N | Fail2Ban integration status |
| `/api/fail2ban/bans` | GET | adminAuth | Y | N | N | Historical ban records from fail2ban DB |

### Backup

| Endpoint | Method | Auth | Admin | Operator | Viewer | Notes |
|----------|--------|------|-------|----------|--------|-------|
| `/api/backup` | GET | adminAuth | Y | N | N | Download DB backup (JSON) |
| `/api/backup/restore` | POST | adminAuth | Y | N | N | Restore DB from backup |
| `/api/backup/full` | POST | adminAuth | Y | N | N | Create encrypted full backup |
| `/api/backup/full/save` | POST | adminAuth | Y | N | N | Create and save backup to server |
| `/api/backup/full/list` | GET | adminAuth | Y | N | N | List saved backups |
| `/api/backup/full/saved/{filename}` | GET | adminAuth | Y | N | N | Download saved backup |
| `/api/backup/full/saved/{filename}` | DELETE | adminAuth | Y | N | N | Delete saved backup |
| `/api/backup/generate-passphrase` | GET | adminAuth | Y | N | N | Generate random passphrase |

### Audit

| Endpoint | Method | Auth | Admin | Operator | Viewer | Notes |
|----------|--------|------|-------|----------|--------|-------|
| `/api/audit-log` | GET | opAuth | Y | Y | N | View audit log entries |

## Rate Limiting

Login endpoint only. Dual tracking: per-IP **and** per-username (two independent limiters with identical parameters). Request is blocked if **either** limiter triggers.

| Parameter | Value |
|-----------|-------|
| Max attempts before lockout | 5 |
| Attempt window | 5 minutes |
| Lockout duration | 15 minutes |
| Cleanup interval | 10 minutes |

**Behavior**:
- Failures within the window increment counter for both IP and username
- At 5 failures on either: locked for 15 minutes (HTTP 429)
- Successful login: counters reset immediately for both IP and username
- Window expiry: counter reset
- Background goroutine prunes stale records every 10 minutes (entries where both window and lockout have expired)

## Special Cases

### Self-Service Endpoints

Any authenticated user (including viewer) can:
- `GET /api/me` -- view own profile
- `PUT /api/me` -- update own email and phone (not role, not username)
- `PUT /api/me/password` -- change own password (must provide current password, min 15 chars)

Password change invalidates all other sessions for the same user (`DeleteUserSessionsExcept`).

### Last Admin Protection

Two guards prevent locking out all admins:

1. **Demote protection** (`handleUpdateUser`): Cannot change role of last active admin away from `admin`. Returns 409.
2. **Suspend protection** (`handleSuspendUser`): Cannot suspend last active admin. Returns 409.

Both check `CountActiveAdmins()` -- users where `role = 'admin' AND status = 'active'`.

### Admin Password Reset

`PUT /api/users/{id}/password` (admin only):
- Does not require the target user's current password
- Invalidates **all** sessions for the target user (`DeleteUserSessions`)
- Forces re-login with new password

### Suspended User Handling

- Suspended at login: `auth.Login` rejects with "account suspended"
- Suspended mid-session: `requireAuth` middleware re-checks user status on every request, rejects with "account not active"
- On suspend: all sessions for the user are immediately deleted

### Role Refresh

`requireAuth` middleware fetches current role from DB on every request and overwrites the JWT claim. This means role changes (e.g., admin demotes operator to viewer) take effect immediately without requiring re-login.

## Session Management

| Parameter | Value |
|-----------|-------|
| Default timeout | 24 hours |
| Configurable via | `session_timeout_hours` setting |
| Storage | SQLite `sessions` table |
| Session ID | UUID v4 |
| Cleanup | Hourly background goroutine (`PurgeExpiredSessions`) |

### Session Lifecycle

1. **Created** on successful login (stored in DB with expiry)
2. **Validated** on every authenticated request (JWT parse + DB session lookup + expiry check)
3. **Deleted** on:
   - Explicit logout (`DELETE sessions WHERE id = ?`)
   - Password change by user (`DELETE sessions WHERE user_id = ? AND id != <current>`)
   - Admin password reset (`DELETE sessions WHERE user_id = ?`)
   - User suspension (`DELETE sessions WHERE user_id = ?`)
   - Session expiry detected during validation (single session cleanup)
   - Hourly background purge (bulk cleanup of all expired sessions)

### IP Address Tracking

Sessions store the client IP at creation time. IP extraction logic:
- Parse `RemoteAddr` (strips port)
- If IP is loopback (behind reverse proxy) and `X-Real-IP` header exists and passes `net.ParseIP` validation: use it; otherwise falls back to loopback address
- `X-Forwarded-For` is intentionally ignored (spoofable)
