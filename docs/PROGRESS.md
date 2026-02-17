# Bekci v2 — Progress

## Session Handover — 17/02/2026

1. **What was done** — Phase 4a+4b email alerting. SOC page compact redesign (single-line card headers, filters in header row, owl icon linking home).
2. **Decisions made** — Resend HTTP API (no SDK). API key masked in GET /settings. SOC icon links to `/` via plain `<a>` (works for unauthenticated users too).
3. **Server state** — Running locally on port 65000. Not deployed yet.
4. **What's next** — Deploy v2.4.0. Test with real Resend API key. Signal gateway (Phase 4c, deferred).

## Current Status
**Phase**: Phase 4 (Email Alerting) complete. Ready for deployment.

## Design Documents
- `docs/DESIGN.md` — Full architecture, schema, API, phases
- `docs/REQUIREMENTS.md` — Spec derived from design

## Implementation Phases

### Phase 1 — Foundation (DONE)
| Task | Status |
|------|--------|
| Scaffolding: go.mod, dirs, .gitignore | done |
| Bootstrap config (YAML + env overrides + defaults) | done |
| SQLite schema with schema_version migration | done |
| Store: users, sessions, settings CRUD | done |
| Auth: bcrypt, JWT HS256, login/logout, session validation | done |
| API: router, middleware (auth/RBAC/CORS/logging), handlers | done |
| User management endpoints (admin only, last-admin protection) | done |
| Settings endpoints (admin write, all read) | done |
| Self-service: /me, update email, change password | done |
| Health endpoint | done |
| Entry point: first-boot admin seed, SPA embed, session cleanup | done |
| Vue 3 shell: login, dashboard placeholder, users, settings, profile | done |
| Makefile: build, frontend, backend, run, dev, clean, test | done |
| Dockerfile (3-stage: node, golang, alpine) + docker-compose.yml | done |
| v1 cleanup: deleted internal/web/, com.bekci.agent.plist | done |

### Phase 2 — Monitoring Core (DONE)
| Task | Status |
|------|--------|
| DB migration002: projects, targets, checks, check_results tables | done |
| Store CRUD: projects.go, targets.go, checks.go, results.go | done |
| Checker rewrite: 6 check types (http, tcp, ping, dns, page_hash, tls_cert) | done |
| Scheduler rewrite: DB-driven, per-check timers, event channel, 60s safety-net | done |
| API handlers: project, target, check, dashboard endpoints | done |
| Router: new routes + RBAC (viewer read, operator CRUD, admin delete projects) | done |
| main.go wiring: scheduler start/stop, results purge in hourly cleanup | done |
| pro-bing dependency added for ICMP ping | done |
| v1 cleanup: deleted restarter/, alerter/, sshutil/, old checker + scheduler | done |
| Frontend: TargetsView.vue with projects panel, targets CRUD, check config forms | done |
| Frontend: DashboardView.vue with dual uptime bars (90d + 4h), problems first, 30s refresh | done |
| Frontend: router + nav updated (Targets link in sidebar) | done |
| Full build: `make clean && make build` passes | done |

### Phase 3 — Rules Engine (DONE)
| Task | Status |
|------|--------|
| migration005: 6 tables (rules, rule_conditions, rule_states, alert_channels, rule_alerts, alert_history) | done |
| Store: rules.go — Rule/RuleCondition/RuleState structs + CRUD + engine queries | done |
| Store: GetRecentResultsByWindow, ListAllChecksWithTarget | done |
| Engine: internal/engine/engine.go — Evaluate, extractField, compare | done |
| Scheduler integration: RuleEvaluator interface, call after SaveResult | done |
| main.go wiring: engine → scheduler | done |

### Phase 3.5 — Unified Target Model (DONE)
| Task | Status |
|------|--------|
| migration006: operator, severity, rule_id columns on targets | done |
| Store: TargetCondition, TargetDetail, TargetListItem structs | done |
| Store: CreateTargetWithConditions, UpdateTargetWithConditions (transactional) | done |
| Store: GetTargetDetail, ListTargetSummaries | done |
| Store: DeleteTarget now cleans up linked rule | done |
| API: Unified target_handlers.go — create/update with conditions array | done |
| API: Dashboard returns flat `[]dashboardTarget` with per-target state+severity | done |
| API: Removed rule_handlers.go, standalone check CUD routes | done |
| Router: removed 5 rule routes, check CUD, GET /api/checks | done |
| Frontend: TargetsView.vue rewrite — unified form with conditions | done |
| Frontend: DashboardView.vue — flat response, per-target health badges | done |
| Frontend: Removed RulesView.vue, /rules route, Rules nav link | done |
| CSS: checkbox alignment fix, page max-width 1100px | done |
| Branding: owl icon in navbar (28px), login (120px), favicons (32/180/192px) | done |
| Visual test: all pages screenshot-verified, 25/25 E2E tests passed | done |
| Committed `6eafff5` and pushed to main | done |

### Backup & Restore (DONE)
| Task | Status |
|------|--------|
| Store: BackupData struct, ExportBackup, RestoreBackup (atomic tx) | done |
| API: GET /api/backup (JSON download), POST /api/backup/restore (10MB limit, validation) | done |
| Router: admin-only backup routes | done |
| Frontend: Backup & Restore card on Settings page (download, file picker, confirm restore) | done |
| Deployed to production | done |

### Security Hardening (DONE)
| Task | Status |
|------|--------|
| C1: Remove plaintext password from admin-seed log line | done |
| C2: Blank init_admin password in prod config | done |
| C3: Move JWT secret to env file (`/etc/bekci/env`), add `EnvironmentFile` to systemd | done |
| H2: Add `server.host` config + `BEKCI_HOST` env override, prod bound to 127.0.0.1 | done |
| H6: nginx `ssl_protocols TLSv1.2 TLSv1.3` (removed TLSv1/1.1) | done |
| Truncated prod log file containing leaked password | done |

### Fail2Ban Integration (DONE)
| Task | Status |
|------|--------|
| H4: slog.Warn on failed login for fail2ban log parsing | done |
| GET /api/fail2ban/status endpoint (admin only, shells out to fail2ban-client) | done |
| Fail2Ban tab on Settings page: jail status table, auto-refresh 30s, expandable banned IPs | done |
| Prod: sudoers, filter, bekci-login jail (10/10m/30m) | done |
| Deployed v2.1.0 to production, both jails verified | done |

### Settings Page Consolidation (DONE)
| Task | Status |
|------|--------|
| 5-tab Settings page: General, Audit Log, Users, Backup & Restore, Fail2Ban | done |
| Audit Log inlined from standalone page (loads on tab switch, pagination) | done |
| Users inlined from standalone page (admin only, loads on tab switch) | done |
| Backup & Restore extracted from General tab to own tab | done |
| Removed Audit Log + Users navbar links, routes, and view files | done |
| `/audit-log` and `/users` URLs redirect to `/settings` | done |
| Audit log rotation: daily cleanup, configurable retention (default 91 days) | done |
| SOC page auto-refresh: 30s → 15s | done |
| Deployed v2.2.0 to production | done |

### System Health Indicator (DONE)
| Task | Status |
|------|--------|
| `GET /api/system/health` endpoint (net/disk/cpu, any auth) | done |
| Net: TCP dial 1.1.1.1:53, Disk: syscall.Statfs, CPU: /proc/loadavg + sysctl fallback | done |
| `dbPath` field added to Server struct, wired from main.go | done |
| Navbar: 3 colored dots (green/yellow/red) with click popover | done |
| 30s poll interval, grey dots on failure | done |
| Deployed v2.3.0 to production | done |

### Phase 4 — Email Alerting (DONE)
| Task | Status |
|------|--------|
| migration011: target_alert_recipients, users.phone, alert_history columns, seed settings | done |
| Store: alerts.go (recipients CRUD, alert history, firing rules query) | done |
| User struct: added Phone field, updated all queries + callers | done |
| Targets: auto-add creator as alert recipient, include recipient_ids in detail | done |
| Alerter module: internal/alerter/ (dispatch, cooldown, re-alert, email sender, templates) | done |
| Engine: AlertDispatcher interface, async dispatch on state change | done |
| API: recipients endpoints, alert history, test email, alerting settings in knownSettings | done |
| Settings: API key masking, string/zero-allowed validation for alerting keys | done |
| main.go: wire alerter to engine + API, 60s re-alert ticker | done |
| Frontend: Alerting tab in Settings (method, API key, from email, cooldown, re-alert, test) | done |
| Frontend: Alert Recipients checkbox list in target edit form | done |
| Frontend: /alerts page with paginated history table + navbar link | done |
| Build + visual test: all pages verified working | done |

### Phase 5 — Polish
| Task | Status |
|------|--------|
| Error handling, loading states | pending |
| Logging improvements | pending |
| Dockerfile finalization | pending |
| Full test suite | pending |

## v1 Legacy
- All v1 code deleted: checker (process.go, ssh.go, https.go), restarter/, alerter/, sshutil/, old scheduler
- Only v2 code remains in internal/

## Deployment

### Production Server
- **Host**: `cl@dias-bekci` (10.0.9.20), Ubuntu 22.04, x86_64
- **Access**: `https://10.0.9.20` (self-signed cert), `https://bekci.home` when DNS ready
- **Binary**: `/opt/bekci/bekci` v2.2.0, runs as `bekci` system user
- **Config**: `/etc/bekci/config.yaml` (640 root:bekci), `/etc/bekci/env` (600 root:root — JWT secret)
- **DB**: `/var/lib/bekci/bekci.db`
- **Logs**: `/var/log/bekci/bekci.log` + `journalctl -u bekci`
- **Services**: `bekci.service` (systemd), nginx reverse proxy (443→65000, 80→443 redirect)
- **TLS cert**: `/etc/ssl/certs/bekci.crt` + `/etc/ssl/private/bekci.key` (365 days, IP SAN)
- **Fail2Ban**: 2 jails (sshd: 3/10m/1h, bekci-login: 10/10m/30m). Sudoers: `/etc/sudoers.d/fail2ban-bekci`. Filter: `/etc/fail2ban/filter.d/bekci.conf`.

### Build for Deployment
Mac is ARM (Apple Silicon), server is x86_64. Must cross-compile:
```bash
# 1. Build frontend locally
cd frontend && npm run build && cd ..
rm -rf cmd/bekci/frontend_dist && cp -r frontend/dist cmd/bekci/frontend_dist

# 2. Build Go binary for linux/amd64 via Docker
docker run --rm --platform linux/amd64 -v "$(pwd)":/src -w /src golang:1.24-bookworm \
  bash -c 'CGO_ENABLED=1 go build -ldflags "-X main.version=X.Y.Z" -o bekci-linux ./cmd/bekci'

# 3. Deploy
scp bekci-linux cl@dias-bekci:/tmp/bekci
ssh cl@dias-bekci 'sudo mv /tmp/bekci /opt/bekci/bekci && sudo chmod 755 /opt/bekci/bekci && sudo setcap cap_net_raw+ep /opt/bekci/bekci && sudo systemctl restart bekci'
rm bekci-linux
```
**Gotcha**: `--platform linux/amd64` is required — without it, Docker on Apple Silicon builds ARM64 binaries that won't run on the x86_64 server. Also: don't use `apt-get install nodejs` in Bookworm (ships Node 18, Vite 7 needs 20+) — build frontend separately.

## Architecture Notes
- **Check types**: http, tcp, ping, dns, page_hash, tls_cert (SNMP deferred)
- **Config storage**: JSON blob in `config` TEXT column — flexible per-type
- **Scheduler**: per-check goroutine timers, eventCh for RunNow, 60s safety-net poll
- **Dashboard API**: flat `[]dashboardTarget` with per-target state/severity
- **RBAC**: viewer=read all, operator=CRUD targets/checks, admin=delete projects
- **Results purge**: hourly, reads `history_days` setting (default 90)
- **Audit log purge**: daily, reads `audit_retention_days` setting (default 91)
- **Unified target model**: each target auto-manages a hidden rule. Engine/scheduler untouched.
