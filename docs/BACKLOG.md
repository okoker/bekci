# Backlog

Unified from server review (17/02/2026) and code review (18/02/2026).
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

---

## Open Items

### High

| ID | Priority | Tags | Description |
|----|----------|------|-------------|
| A-H2 | `[P0]` | `[security]` | Default bootstrap admin password (`admin1234`). See KI-001 — accepting for now, mitigated by first-boot-only + log warning. |
| S-H3 | `[P0]` | `[security]` | SSH allows root login (`PermitRootLogin yes`) + password auth. Disable both. |
| S-C4 | `[P0]` | `[security]` | CGO_ENABLED=0 deploy caused ~26 restart cycles. Add systemd `StartLimitBurst` / deploy validation. |
| S-H5 | `[P1]` | `[security]` | systemd service has no sandboxing (`ProtectSystem`, `NoNewPrivileges`, `PrivateTmp`). |

### Medium

| ID | Priority | Tags | Description |
|----|----------|------|-------------|
| A-M5 | `[P1]` | `[bug]` | IPv6 bug in TCP checker: `fmt.Sprintf("%s:%d")` breaks IPv6 literals. Use `net.JoinHostPort`. |
| A-M6 | `[P1]` | `[bug]` | IPv6 bug in DNS checker: `strings.Contains(":")` misclassifies raw IPv6 addresses. Use `net.JoinHostPort`. |
| A-M7 | `[P1]` | `[debt]` | Signal alert mode selectable in UI but backend is stub-only. Hide or mark disabled until implemented. |
| A-M9 | `[P2]` | `[debt]` | Error swallowing in state-changing paths (`alerts.go:31`, `alerts.go:116`, `auth_handlers.go:142`). Log errors explicitly. |
| S-M2 | `[P2]` | `[security]` | Binary `/opt/bekci/bekci` owned by `cl`, not root. Compromised `cl` could replace binary. |
| S-M3 | `[P1]` | `[security]` | Two accounts (`omer`, `cl`) with `NOPASSWD: ALL` sudo. |
| S-M5 | `[P2]` | `[debt]` | Full source code on server at `/home/cl/bekci-src/`. Needed for builds but increases attack surface. |
| S-M6 | `[P2]` | `[security]` | Self-signed TLS cert, expires 16/02/2027, no auto-renewal. |

### Low

| ID | Priority | Tags | Description |
|----|----------|------|-------------|
| A-L1 | `[P2]` | `[debt]` | Dead code: `loadRecipients` in TargetsView.vue, `RenderSignalAlert` in templates.go. Remove or wire. |
| A-L2 | `[P2]` | `[debt]` | Stale comment in scheduler.go:150 says "random delay" but code uses fixed 5s. Fix comment or add jitter. |
| A-L3 | `[P2]` | `[debt]` | gofmt formatting drift in 4 Go files. Run `gofmt -w` as CI step. |
| A-L4 | `[P2]` | `[debt]` | Config docs inconsistency: example says JWT secret required, code auto-generates one. Align. |
| S-L1 | `[P2]` | `[debt]` | No log rotation for `/var/log/bekci/bekci.log`. |
| S-L2 | `[P2]` | `[feature]` | No automated DB backups (manual backup/restore exists in UI). |
