# Bekci v2 — Progress

## Current Status
**Phase**: Phase 2 — Monitoring Core. **Complete.**

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

### Phase 3 — Rules Engine
| Task | Status |
|------|--------|
| Rule + condition CRUD (API + Vue) | pending |
| Rule builder UI | pending |
| Evaluation engine | pending |
| Rule state tracking | pending |

### Phase 4 — Alerting
| Task | Status |
|------|--------|
| Alert channel management (API + Vue) | pending |
| Email (Resend) sender | pending |
| Alert lifecycle: trigger, resolve, cooldown | pending |
| Alert history page + acknowledge UI | pending |

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

## Phase 2 Architecture Notes
- **Check types**: http, tcp, ping, dns, page_hash, tls_cert (SNMP deferred)
- **Config storage**: JSON blob in `config` TEXT column — flexible per-type
- **Scheduler**: per-check goroutine timers, eventCh for RunNow, 60s safety-net poll
- **Dashboard API**: grouped by project→target→check, 90d daily uptime + 4h raw results
- **RBAC**: viewer=read all, operator=CRUD targets/checks, admin=delete projects
- **Results purge**: hourly, reads `history_days` setting (default 90)

## Test Results — Phase 1
All verified manually via curl:
- Health: GET /api/health -> 200, version correct
- Login: POST /api/login -> JWT + user returned
- Auth guard: Unauthenticated requests -> 401
- RBAC: Operator can't access /api/users -> 403
- User CRUD: Create, list, get, update, suspend, reset password
- Last admin protection: Can't suspend/demote only admin
- Suspended user can't login
- Settings: Get all, update (admin only)
- Self-service: /me, update email, change password
- SPA: Frontend serves on /, fallback to index.html for Vue routes

## Test Results — Phase 2
Pending runtime verification (requires Docker or server deployment).
Build verification: `make clean && make build` passes cleanly.
