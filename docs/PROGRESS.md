# Bekci v2 — Progress

## Session Handover — 12/02/2026

1. **What was done** — Unified target model: merged rules into targets. Single form to create target + conditions. No changes to engine/scheduler/main.go.
2. **Decisions made** — Hidden auto-managed rules behind the scenes. `TargetListItem` struct for list view (avoids loading full conditions). Dashboard now returns flat `[]dashboardTarget` with per-target `state` and `severity`.
3. **Gotchas discovered** — `make([]T, 0, count)` has `len()=0` not `count`. SPA catch-all means deleted API routes still return 200 (index.html) — that's expected.
4. **Server state** — Running on port 65000, binary at `./bin/bekci`.
5. **What's next** — Commit these changes, then Phase 4 (Alerting).

## Current Status
**Phase**: Phase 3.5 — Unified Target Model. **Complete.**

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
| Visual test: all pages screenshot-verified | done |

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

## Architecture Notes
- **Check types**: http, tcp, ping, dns, page_hash, tls_cert (SNMP deferred)
- **Config storage**: JSON blob in `config` TEXT column — flexible per-type
- **Scheduler**: per-check goroutine timers, eventCh for RunNow, 60s safety-net poll
- **Dashboard API**: flat `[]dashboardTarget` with per-target state/severity
- **RBAC**: viewer=read all, operator=CRUD targets/checks, admin=delete projects
- **Results purge**: hourly, reads `history_days` setting (default 90)
- **Unified target model**: each target auto-manages a hidden rule. Engine/scheduler untouched.
