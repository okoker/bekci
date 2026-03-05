# Bekci — Tech Stack & Environment Reference

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.24, net/http (stdlib router), SQLite WAL |
| Database | SQLite 3 via go-sqlite3 (CGO), WAL mode, auto-migrate |
| Frontend | Vue 3, Vite 7, Vue Router, Pinia, Axios, Chart.js |
| Auth | JWT HS256 (golang-jwt/v5) in HttpOnly cookie, bcrypt cost 12 |
| Email | Resend API |
| Network | ICMP ping via pro-bing (NET_RAW capability) |
| Config | YAML base + env var overrides + auto-generated defaults |
| Deploy | Docker multi-stage build (node → go → alpine) or bare-metal via Makefile |

---

## Environments

| | **Dev (macOS)** | **Production (remote)** | **Docker (local)** |
|---|---|---|---|
| **Host** | Local machine | `ssh cl@dias-bekci` (10.0.9.20) | Docker Desktop |
| **URL** | localhost:65000 | :65000 | localhost:65000 |
| **OS** | macOS (Darwin 24.6) | Ubuntu | Alpine 3.21 (container) |
| **Go** | 1.24 (local) | 1.24 (/usr/local/go) | 1.25-alpine (build stage) |
| **Node** | 22+ (pinned in `frontend/.nvmrc`) | None (frontend pre-built) | 22-alpine (build stage) |
| **DB** | `bekci.db` (local) | `/var/lib/bekci/bekci.db` | `/data/bekci.db` (Docker volume) |
| **Binary** | `bin/bekci` | `/opt/bekci/bekci` | `/usr/local/bin/bekci` |
| **Config** | `config.yaml` (optional) | `/etc/bekci/config.yaml` | Env vars in docker-compose |
| **Service** | Manual | systemd (`bekci.service`) | Docker (`restart: unless-stopped`) |
| **Default creds** | admin / admin1234 | admin / admin1234 | admin / admin1234 |

---

## Build & Deploy

### Local dev
```
make dev          # Go run + use Vite dev server separately
make build        # Full build: frontend + embed copy + Go binary
make run          # Build + run
make test         # go test -v ./...
```

### Docker
```
make docker       # docker compose build (with version arg) + up -d
```

### Production (bare-metal)
```bash
ssh cl@dias-bekci
cd /home/cl/bekci-src && git pull
export PATH=/usr/local/go/bin:$PATH
CGO_ENABLED=1 go build -ldflags '-X main.version=X.Y.Z' -o bin/bekci ./cmd/bekci
sudo systemctl stop bekci && sudo cp bin/bekci /opt/bekci/bekci && sudo systemctl start bekci
```

No npm on server — `cmd/bekci/frontend_dist/` is committed to git. Go binary embeds frontend via `//go:embed`.

---

## Key Dependencies

### Backend (go.mod)
| Package | Version | Purpose |
|---------|---------|---------|
| golang-jwt/jwt/v5 | 5.3.1 | JWT signing & verification |
| mattn/go-sqlite3 | 1.14.19 | SQLite driver (requires CGO) |
| prometheus-community/pro-bing | 0.8.0 | ICMP ping |
| golang.org/x/crypto | 0.48.0 | Bcrypt password hashing |
| gopkg.in/yaml.v3 | 3.0.1 | Config file parsing |

### Frontend (package.json)
| Package | Version | Purpose |
|---------|---------|---------|
| vue | ^3.5.25 | UI framework |
| vite | ^7.3.1 | Build tool |
| vue-router | ^4.6.4 | Client-side routing |
| pinia | ^3.0.4 | State management |
| axios | ^1.13.5 | HTTP client (withCredentials for cookie auth) |
| chart.js | ^4.5.1 | Charts (SLA page) |
| vue-chartjs | ^5.3.3 | Vue Chart.js wrapper |

---

## Key Differences

- **Dev** uses `make dev` (Go run + Vite HMR). No Docker needed.
- **Docker** is self-contained: binary + SQLite in a named volume. Single port (65000). Good for local "production-like" testing.
- **Production** is bare-metal on Ubuntu. Binary runs as `bekci` user via systemd. No Docker, no npm — frontend is pre-embedded in the committed `frontend_dist/`.
- **CGO is required everywhere** — go-sqlite3 needs it. Cannot use `CGO_ENABLED=0`.
- **ICMP ping requires NET_RAW** — Docker gets it via `cap_add`. Bare-metal uses `setcap` on the binary.
