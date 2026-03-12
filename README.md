# Bekci

<p align="center">
  <img src="frontend/public/bekci-icon.png" alt="Bekci" width="120" />
</p>

Web-managed monitoring platform written in Go + Vue 3. Multi-check monitoring with composite rules engine, alerting, SLA tracking, RBAC, and Docker-first deployment.

## Features

- **6 Check Types** — Ping (ICMP), HTTP/HTTPS, TCP, DNS, Page Hash, TLS Certificate
- **Unified Targets** — Create a target with conditions (check + alert criteria) in one step
- **Rules Engine** — Condition groups with AND/OR logic, configurable fail count and fail window thresholds
- **Alerting** — Email (Resend API), Signal messaging, and generic webhook (JSON POST to any HTTP endpoint), with cooldown, re-alert, and recovery notifications
- **Dashboard** — 90-day + 4-hour uptime bars, per-target health state, problems sorted to top, 30s auto-refresh
- **SLA Compliance** — Per-category SLA thresholds, dedicated SLA page with Chart.js daily uptime charts
- **SOC View** — Flat status page for security operations center displays (optionally public)
- **Auth & RBAC** — JWT + server-side sessions, bcrypt, three roles (admin / operator / viewer)
- **User Management** — Create, suspend, reset passwords, last-admin protection
- **Backup & Restore** — Config backup (JSON, web UI restore) + full database backup (tar.gz, optional AES-256-GCM encryption, CLI restore). Server-side backup storage with download/delete management.
- **Audit Log** — Comprehensive audit trail for all admin/operator actions
- **Fail2Ban** — Integration for login brute-force protection
- **Settings** — Runtime-configurable session timeout, history retention, alerting, SLA thresholds
- **Single Binary** — Vue 3 frontend embedded in Go binary via `go:embed`
- **Docker Ready** — Multi-stage Dockerfile, single container, single port

## Quick Start — Docker

```bash
git clone https://github.com/okoker/bekci.git
cd bekci
docker compose up -d
```

Open `http://localhost:65000` — login with `admin` / `admin1234` (or the password set via `BEKCI_ADMIN_PASSWORD`).

### Docker Environment Variables (optional)

Add to the `environment:` section in `docker-compose.yml`:

| Variable | Default | Description |
|----------|---------|-------------|
| `BEKCI_JWT_SECRET` | auto-generated | JWT signing key |
| `BEKCI_ADMIN_PASSWORD` | `admin1234` | Initial admin password (first boot only) |
| `BEKCI_PORT` | `65000` | HTTP port inside container |
| `BEKCI_DB_PATH` | `/data/bekci.db` | SQLite database path |
| `BEKCI_BACKUP_DIR` | `{db_dir}/backups/` | Server-side backup storage directory |

### Updating

```bash
git pull
docker compose up -d --build
```

## Quick Start — Native

### Prerequisites

- Go 1.24+
- Node.js 22+ (for frontend build)
- GCC (for SQLite CGO)

### Build & Run

```bash
make build
BEKCI_ADMIN_PASSWORD=changeme123 ./bin/bekci
```

Or with config file:

```bash
cp config.example.yaml config.yaml
# Edit config.yaml — set init_admin.password (JWT secret auto-generates if not set)
make build && ./bin/bekci
```

### Development Mode

```bash
# Terminal 1 — backend
BEKCI_JWT_SECRET=dev-secret BEKCI_ADMIN_PASSWORD=changeme make dev

# Terminal 2 — frontend with hot-reload
cd frontend && npm run dev
```

Frontend dev server at `http://localhost:5173` proxies `/api/*` to the backend on port 65000.

## Configuration

Bootstrap config in `config.yaml` (or env vars). Runtime settings managed via web UI.

```yaml
server:
  port: 65000
  db_path: bekci.db
  backup_dir: ""                   # Default: {db_dir}/backups/

auth:
  jwt_secret: ""                   # Optional — auto-generated if not set

logging:
  level: warn     # debug, info, warn, error
  path: bekci.log

init_admin:
  username: admin
  password: "changeme123"          # Only used on first boot
```

## Backup & Restore

### Config Backup
JSON export of configuration tables (users, targets, checks, rules, settings). Download and restore via the web UI in Settings > Backup & Restore.

### Full Database Backup
Complete SQLite snapshot including all historical data (check results, audit logs, alert history) plus config.yaml, packaged as a tar.gz archive.

- **Download** — streams directly to browser
- **Save to server** — saves to the backup directory on disk for later download
- **Encryption** — optional AES-256-GCM with Argon2id KDF and 4-word diceware passphrase
- **Server-side management** — list, download, and delete saved backups from the web UI
- **Restore** — CLI only for safety: `bekci restore-full <archive-path>` (interactive guided wizard)

## Web UI

| Route | Page | Access |
|-------|------|--------|
| `/` | Dashboard — uptime bars, health state, problems first | all |
| `/targets` | Target list + CRUD with inline conditions | all (CRUD: operator+) |
| `/alerts` | Alert history | all |
| `/sla` | SLA Compliance — 90-day daily uptime charts per category | all |
| `/soc` | Status page (optionally public) | configurable |
| `/settings` | General, SLA, Alerting, Users, Backup & Restore, Audit Log, Fail2Ban | role-based |
| `/profile` | Own profile and password change | all |

## API Overview

40+ endpoints across 12 domains. Key groups:

| Domain | Endpoints | Auth |
|--------|-----------|------|
| Auth | login, logout, me, password | public / any |
| Targets | CRUD, pause/unpause | any / operator+ |
| Checks | list, run, results | any / operator+ |
| Dashboard | status, history | any |
| SLA | history | any |
| SOC | status, history | configurable |
| Alerts | list, test-email, test-signal, test-webhook, webhook-status | any / admin |
| Users | CRUD, suspend, reset password | operator+ / admin |
| Settings | read, update | any / admin |
| Backup | config backup/restore, full backup (download/save/list/delete), passphrase | admin |
| Audit | log | operator+ |
| System | health, fail2ban | any / admin |

Full API reference: [`docs/reference/api_reference.md`](docs/reference/api_reference.md)

## Project Structure

```
cmd/bekci/
  main.go                  Entry point, wiring, embed
  restore.go               CLI restore-full subcommand
internal/
  config/                  YAML + env config loader
  store/                   SQLite: all tables, migrations, backup
  auth/                    JWT, bcrypt, sessions
  api/                     HTTP router, middleware, handlers
  checker/                 6 check types (http, tcp, ping, dns, page_hash, tls_cert)
  scheduler/               DB-driven, per-check timers, event channel
  engine/                  Rules evaluator (condition groups, fail thresholds)
  alerter/                 Email (Resend) + Signal + Webhook dispatch, cooldown, re-alert
  crypto/                  AES-256-GCM encryption, diceware passphrase generator
frontend/                  Vue 3 + Vite SPA
docs/reference/            System documentation (API, DB schema, RBAC, backup)
Makefile                   Build targets
Dockerfile                 3-stage production build
docker-compose.yml         Single-service deployment
```

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.24, net/http (stdlib router), SQLite WAL |
| Frontend | Vue 3, Vite, Vue Router, Pinia, Chart.js |
| Auth | JWT HS256 in HttpOnly cookie, bcrypt |
| Alerting | Resend API (email), Signal REST API (messaging), Generic webhook (JSON POST) |
| Encryption | AES-256-GCM, Argon2id KDF |
| Deploy | Docker multi-stage (node + go + alpine) or bare-metal |

## License

MIT - Copyright (c) 2026 Objects Consulting Ltd, UK
