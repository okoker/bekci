# Bekci

<p align="center">
  <img src="frontend/public/bekci-icon.png" alt="Bekci" width="120" />
</p>

Web-managed monitoring platform written in Go + Vue 3. Multi-check monitoring with composite rules engine, RBAC, and Docker-first deployment.

## Features

- **6 Check Types** — Ping (ICMP), HTTP/HTTPS, TCP, DNS, Page Hash, TLS Certificate
- **Unified Targets** — Create a target with conditions (check + alert criteria) in one step
- **Rules Engine** — AND/OR conditions with fail count thresholds, auto-evaluated after each check
- **Dashboard** — 90-day + 4-hour uptime bars, per-target health state, problems sorted to top, 30s auto-refresh
- **SOC View** — Flat dashboard for security operations center displays
- **Auth & RBAC** — JWT + server-side sessions, bcrypt, three roles (admin / operator / viewer)
- **User Management** — Create, suspend, reset passwords, last-admin protection
- **Settings** — Runtime-configurable session timeout, history retention, check interval
- **Single Binary** — Vue 3 frontend embedded in Go binary via `go:embed`
- **Docker Ready** — Multi-stage Dockerfile, single container, single port

## Quick Start — Docker

```bash
git clone https://github.com/okoker/bekci.git
cd bekci
docker compose up -d
```

Open `http://localhost:65000` — login with `admin` and the password set via `BEKCI_ADMIN_PASSWORD` (default: see `config.go`).

### Docker Environment Variables (optional)

Add to the `environment:` section in `docker-compose.yml`:

| Variable | Default | Description |
|----------|---------|-------------|
| `BEKCI_JWT_SECRET` | auto-generated | JWT signing key |
| `BEKCI_ADMIN_PASSWORD` | (built-in) | Initial admin password (first boot only) |
| `BEKCI_PORT` | `65000` | HTTP port inside container |
| `BEKCI_DB_PATH` | `/data/bekci.db` | SQLite database path |

### Updating

```bash
git pull
docker compose up -d --build
```

## Quick Start — Native

### Prerequisites

- Go 1.22+
- Node.js 20+ (for frontend build)
- GCC (for SQLite CGO)

### Build & Run

```bash
make build
BEKCI_JWT_SECRET=your-secret BEKCI_ADMIN_PASSWORD=changeme123 ./bin/bekci
```

Or with config file:

```bash
cp config.example.yaml config.yaml
# Edit config.yaml — set auth.jwt_secret and init_admin.password
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

auth:
  jwt_secret: "your-secret-here"   # REQUIRED

logging:
  level: warn     # debug, info, warn, error
  path: bekci.log

init_admin:
  username: admin
  password: "changeme123"          # Only used on first boot
```

## API

### Public

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/login` | Authenticate, returns JWT |
| GET | `/api/health` | Health check |

### Auth (any role)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/logout` | End session |
| GET | `/api/me` | Current user profile |
| PUT | `/api/me` | Update own email |
| PUT | `/api/me/password` | Change own password |

### Targets (unified with conditions)

| Method | Endpoint | Role | Description |
|--------|----------|------|-------------|
| GET | `/api/targets` | any | List with condition_count + state |
| POST | `/api/targets` | operator+ | Create with conditions |
| GET | `/api/targets/:id` | any | Detail + conditions + state |
| PUT | `/api/targets/:id` | operator+ | Update + smart-diff conditions |
| DELETE | `/api/targets/:id` | operator+ | Delete (cascades checks + rule) |

### Checks

| Method | Endpoint | Role | Description |
|--------|----------|------|-------------|
| GET | `/api/targets/:id/checks` | any | List checks for target |
| POST | `/api/checks/:id/run` | operator+ | Trigger immediate check |
| GET | `/api/checks/:id/results` | any | Query results |

### Dashboard

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/dashboard/status` | Per-target health + checks |
| GET | `/api/dashboard/history/:check_id` | Uptime history (`?range=90d` or `?range=4h`) |
| GET | `/api/soc/status` | SOC flat view |
| GET | `/api/soc/history/:check_id` | SOC history |

### Admin

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/users` | List users |
| POST | `/api/users` | Create user |
| PUT | `/api/users/:id` | Update user |
| PUT | `/api/users/:id/suspend` | Suspend / activate |
| PUT | `/api/users/:id/password` | Reset password |
| GET/PUT | `/api/settings` | Read / update settings |

## Project Structure

```
cmd/bekci/main.go          Entry point, wiring, embed
internal/
  config/                   YAML + env config loader
  store/                    SQLite: users, sessions, settings, targets, checks, rules, results
  auth/                     JWT, bcrypt, login/logout
  api/                      HTTP router, middleware, handlers
  checker/                  6 check types (http, tcp, ping, dns, page_hash, tls_cert)
  scheduler/                DB-driven, per-check timers, event channel
  engine/                   Rules evaluator (AND/OR conditions, fail thresholds)
frontend/                   Vue 3 + Vite SPA
Makefile                    Build targets
Dockerfile                  3-stage production build
docker-compose.yml          Single-service deployment
```

## License

MIT - Copyright (c) 2026 Objects Consulting Ltd, UK
