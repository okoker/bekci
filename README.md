# Bekci

Web-managed monitoring platform written in Go + Vue 3. Multi-check monitoring with composite rules engine, RBAC, email alerts, and Docker-first deployment.

Evolved from a lightweight YAML-configured watchdog into a full web UI for managing targets, checks, rules, and alerts — all from a single binary.

![Dashboard](https://img.shields.io/badge/dashboard-65000-blue)
![API](https://img.shields.io/badge/api-65000-purple)

## Features

- **Web UI** — Vue 3 SPA with login, dashboard, user management, settings, profile
- **Auth & RBAC** — JWT + server-side sessions, bcrypt passwords, three roles (admin / operator / viewer)
- **User management** — Create, suspend, reset passwords, last-admin protection
- **Settings** — Runtime-configurable session timeout, history retention, check interval
- **Single binary** — Frontend embedded in Go binary via `go:embed`
- **Docker ready** — Multi-stage Dockerfile, docker-compose included

### Planned (Phase 2+)

- Health checks: Ping (ICMP), HTTP/HTTPS, TCP, DNS, SNMP, Page Hash, TLS Certificate
- Composite rules engine (AND/OR conditions with thresholds)
- Email alerts via Resend API (with cooldown + recovery)
- Dashboard with 90-day + 4-hour uptime bars

## Quick Start

### Prerequisites

- Go 1.22+
- Node.js 20+ (for frontend build)
- GCC (for SQLite CGO)

### Build & Run

```bash
cp config.example.yaml config.yaml
# Edit config.yaml — set auth.jwt_secret and init_admin.password

make build
./bin/bekci
```

Or with environment variables only (no config file needed):

```bash
BEKCI_JWT_SECRET=your-secret-here BEKCI_ADMIN_PASSWORD=changeme123 make run
```

Open `http://localhost:65000` — login with the initial admin credentials.

### Development Mode

Run the Go backend and Vite dev server separately for hot-reload:

```bash
# Terminal 1 — backend (no embedded frontend)
BEKCI_JWT_SECRET=dev-secret BEKCI_ADMIN_PASSWORD=admin1234 make dev

# Terminal 2 — frontend with API proxy
cd frontend && npm run dev
```

Frontend dev server at `http://localhost:5173` proxies `/api/*` to the backend.

### Docker

```bash
BEKCI_JWT_SECRET=your-secret BEKCI_ADMIN_PASSWORD=changeme123 docker-compose up -d
```

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

### Environment Overrides

| Variable | Config Key | Description |
|----------|-----------|-------------|
| `BEKCI_JWT_SECRET` | `auth.jwt_secret` | JWT signing secret (required) |
| `BEKCI_ADMIN_PASSWORD` | `init_admin.password` | Initial admin password (first boot) |
| `BEKCI_PORT` | `server.port` | HTTP port (default: 65000) |
| `BEKCI_DB_PATH` | `server.db_path` | SQLite database path (default: bekci.db) |

## API

### Public

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/login` | Authenticate, returns JWT |
| GET | `/api/health` | Health check (`{status, version}`) |

### Authenticated

| Method | Endpoint | Role | Description |
|--------|----------|------|-------------|
| POST | `/api/logout` | any | End session |
| GET | `/api/me` | any | Current user profile |
| PUT | `/api/me` | any | Update own email |
| PUT | `/api/me/password` | any | Change own password |
| GET | `/api/settings` | any | View settings |
| PUT | `/api/settings` | admin | Update settings |

### Admin Only

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/users` | List all users |
| POST | `/api/users` | Create user |
| GET | `/api/users/:id` | Get user |
| PUT | `/api/users/:id` | Update user (email, role) |
| PUT | `/api/users/:id/suspend` | Suspend / activate user |
| PUT | `/api/users/:id/password` | Reset user password |

## RBAC

| Capability | Admin | Operator | Viewer |
|-----------|-------|----------|--------|
| Dashboard | view | view | view |
| Users | CRUD | - | - |
| Settings | read/write | read | read |
| Own profile | edit | edit | edit |

## Project Structure

```
cmd/bekci/main.go          Entry point, wiring, embed
internal/
  config/                   YAML + env config loader
  store/                    SQLite: users, sessions, settings
  auth/                     JWT, bcrypt, login/logout
  api/                      HTTP router, middleware, handlers
  checker/                  (v1, pending Phase 2 rework)
  scheduler/                (v1, pending Phase 2 rework)
  alerter/                  (v1, pending Phase 2 rework)
frontend/                   Vue 3 + Vite SPA
Makefile                    Build targets
Dockerfile                  3-stage production build
docker-compose.yml          Single-service deployment
```

## License

MIT - Copyright (c) 2026 Objects Consulting Ltd, UK
