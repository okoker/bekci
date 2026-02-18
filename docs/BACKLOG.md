# Backlog

Unified from server review (17/02/2026), code review (18/02/2026), and detailed review (18/02/2026).
Promote to `TODO.md` when ready to tackle.

Tags: `[P0]` `[P1]` `[P2]` + `[security]` `[feature]` `[bug]` `[debt]`
Prefix: `S-` = server/infra, `A-` = application/code

---

## Completed

| ID | Date | Description |
|----|------|-------------|
| ~~S-H1~~ | 18/02 | UFW firewall enabled |
| ~~S-H4~~ | 17/02 | fail2ban installed |
| ~~S-M4~~ | 18/02 | Stale user accounts removed |
| ~~S-L3~~ | 17/02 | Go 1.18 removed, 1.24 only |
| ~~S-L4~~ | 17/02 | nginx server_tokens off |
| ~~A-H1~~ | 18/02 | Restore broken tables + multipart + schema version |
| ~~A-H3~~ | 18/02 | Cross-target check ownership validation |
| ~~A-M2~~ | 18/02 | Frontend can't clear recipients |
| ~~A-M3~~ | 18/02 | Recipients included in backup/restore |
| ~~A-M4~~ | 18/02 | Secure client IP extraction (loopback→X-Real-IP, else RemoteAddr) |
| ~~A-H4~~ | 18/02 | Login rate limiting (5/5min/15min lockout) + generic error message |
| ~~A-M1~~ | 18/02 | Scheduler detects interval changes and reschedules |
| ~~A-M8~~ | 18/02 | Operators can list users (GET /api/users) for recipient picker |
| ~~S-M1~~ | 18/02 | Nginx security headers (HSTS, X-Content-Type-Options, X-Frame-Options, CSP) |
| ~~A-M5~~ | 18/02 | IPv6 `net.JoinHostPort` in all checkers + main.go |
| ~~A-M6~~ | 18/02 | IPv6 `net.SplitHostPort` in DNS checker |
| ~~A-M9~~ | 18/02 | Error logging in handler-level paths (auth, user, scheduler, alerter). Store-layer errors moved to A-M10. |
| ~~A-L1~~ | 18/02 | Dead code removed: `loadRecipients`, `RenderSignalAlert` |
| ~~A-L2~~ | 18/02 | Stale "random delay" comment fixed |
| ~~A-L3~~ | 18/02 | gofmt formatting drift fixed |
| ~~A-M14~~ | 18/02 | Docs: settings masking + default alert_method fixed in api_reference.md |
| ~~A-M15~~ | 18/02 | Docs: `re-alert` type added to api_reference.md and db_schema.md |
| ~~A-L4~~ | 18/02 | Config docs: JWT secret comment updated in README + config.example.yaml |
| ~~A-L6~~ | 18/02 | README.md: Go version, JWT auto-gen, users RBAC roles fixed |
| ~~A-L7~~ | 18/02 | REQUIREMENTS.md: removed SNMP, severity→category, ack→deferred, 15s→30s refresh |

---

## Open Items

### High

| ID | Priority | Tags | Description |
|----|----------|------|-------------|
| A-H2 | `[P0]` | `[security]` | Default bootstrap admin password (`admin1234`). See KI-001 — accepting for now, mitigated by first-boot-only + log warning. |
| A-H5 | `[P0]` | `[bug]` | Restore endpoint rejects `application/json; charset=utf-8`. `backup_handlers.go:35` checks exact match. Use `mime.ParseMediaType`. |
| S-H3 | `[P0]` | `[security]` | SSH allows root login (`PermitRootLogin yes`) + password auth. Disable both. |
| S-C4 | `[P0]` | `[security]` | CGO_ENABLED=0 deploy caused ~26 restart cycles. Add systemd `StartLimitBurst` / deploy validation. |
| S-H5 | `[P1]` | `[security]` | systemd service has no sandboxing (`ProtectSystem`, `NoNewPrivileges`, `PrivateTmp`). |

### Medium

| ID | Priority | Tags | Description |
|----|----------|------|-------------|
| A-M10 | `[P1]` | `[bug]` | Silent errors in store layer: `alerts.go:31` (delete in recipient replace), `alerts.go:116-117` (GetLastAlertTime swallows DB error), `targets.go:272-276,325-330` (unchecked `tx.Exec` in target updates). Propagate/log errors. |
| A-M11 | `[P1]` | `[bug]` | `PUT /api/me` clears email/phone when fields omitted (`auth_handlers.go:103`). Preserve existing values for empty fields. |
| A-M12 | `[P1]` | `[bug]` | `PUT /api/users/{id}` clears phone when omitted (`user_handlers.go:161`). Same class as A-M11. |
| A-M13 | `[P1]` | `[bug]` | Creator auto-add as recipient skipped when target has no conditions — early return at `targets.go:153-155` bypasses auto-add at `targets.go:217`. |
| A-M7 | `[P1]` | `[debt]` | Signal alert mode selectable in UI but backend is stub-only. Hide or mark disabled until implemented. |
| A-M16 | `[P2]` | `[security]` | JWT stored in `localStorage` (`auth.js:6`). XSS can exfiltrate token. Mitigate with HttpOnly cookies or stricter CSP. |
| S-M2 | `[P2]` | `[security]` | Binary `/opt/bekci/bekci` owned by `cl`, not root. Compromised `cl` could replace binary. |
| S-M3 | `[P1]` | `[security]` | Two accounts (`omer`, `cl`) with `NOPASSWD: ALL` sudo. |
| S-M5 | `[P2]` | `[debt]` | Full source code on server at `/home/cl/bekci-src/`. Needed for builds but increases attack surface. |
| S-M6 | `[P2]` | `[security]` | Self-signed TLS cert, expires 16/02/2027, no auto-renewal. |

### Low

| ID | Priority | Tags | Description |
|----|----------|------|-------------|
| A-L5 | `[P2]` | `[debt]` | Dead store functions: `SetSetting` (settings.go:32), `AddTargetRecipient` (alerts.go:8), `RemoveTargetRecipient` (alerts.go:16). No callers. |
| A-L8 | `[P2]` | `[debt]` | Zero test files. Need baseline tests for auth/RBAC, backup/restore, target CRUD, alerting. |
| S-L1 | `[P2]` | `[debt]` | No log rotation for `/var/log/bekci/bekci.log`. |
| S-L2 | `[P2]` | `[feature]` | No automated DB backups (manual backup/restore exists in UI). |
