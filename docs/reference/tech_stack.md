# Bekci — Tech Stack & Environment Reference

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.25 (go.mod min 1.24), net/http stdlib router (Go 1.22+ method routing), SQLite WAL |
| Database | SQLite 3 via go-sqlite3 (CGO required), WAL mode, `SetMaxOpenConns(1)`, auto-migrate (20 migrations) |
| Frontend | Vue 3, Vite 7, Vue Router 4, Pinia 3, Axios, Chart.js + vue-chartjs |
| Auth | JWT HS256 (golang-jwt/v5) in HttpOnly cookie (`token`), bcrypt cost 12 |
| Reverse Proxy | Nginx 1.18 (prod only) — SSL termination, security headers, gzip |
| Email | Resend API |
| Signal | Signal Messenger via signal-cli REST API |
| Webhook | Generic JSON POST to any HTTP endpoint (SOAR, Slack, etc.), Bearer or Basic auth |
| Network | ICMP ping via pro-bing (requires NET_RAW capability), SNMP via gosnmp |
| Config | YAML base + env var overrides (`BEKCI_` prefix) + auto-generated defaults |
| Deploy | Docker multi-stage build (local) or bare-metal via Makefile (prod) |

---

## Environments

| | **Local Dev (Docker)** | **Production** |
|---|---|---|
| **Host** | Docker Desktop on macOS | `ssh cl@dias-bekci` (10.0.9.20) |
| **URL** | `http://localhost:65000` | `https://dias-bekci` (nginx on 443) |
| **OS** | Alpine 3.21 (container) | Ubuntu 22.04.5 LTS (kernel 5.15) |
| **Go** | 1.25-alpine (build stage) | 1.24.0 (`/usr/local/go`) — should upgrade to 1.25 |
| **Node** | 22-alpine (build stage) | None (frontend pre-built in repo) |
| **DB path** | `/data/bekci.db` (Docker volume `bekci-data`) | `/var/lib/bekci/bekci.db` |
| **Binary** | `/usr/local/bin/bekci` (in container) | `/opt/bekci/bekci` |
| **Config** | Env vars in `docker-compose.yml` | `/etc/bekci/config.yaml` + `/etc/bekci/env` |
| **Service** | Docker (`restart: unless-stopped`) | systemd (`bekci.service`, user `bekci:bekci`) |
| **NET_RAW** | `cap_add: [NET_RAW]` in compose | `AmbientCapabilities=CAP_NET_RAW` in systemd unit |
| **Reverse proxy** | None (direct access) | Nginx 1.18 on port 443 |
| **Default creds** | admin / sifresifresifre | admin / sifresifresifre |

> **Note**: Bare-metal Go development on macOS is possible (`make dev`, `make run`) but Docker is the primary local dev/test method. Frontend dev uses Vite HMR via `cd frontend && npm run dev` (proxies `/api` to localhost:65000).

---

## Production — Nginx

Nginx handles SSL termination, security headers, and compression in front of the Go binary.

**Config**: `/etc/nginx/sites-enabled/bekci`

| Setting | Value |
|---------|-------|
| HTTP (80) | Redirects to HTTPS |
| HTTPS (443) | SSL default server |
| SSL cert | `/etc/ssl/certs/bekci.crt` (self-signed) |
| SSL protocols | TLSv1.2, TLSv1.3 |
| Gzip | On — JSON, JS, CSS, XML; level 5; min 1000 bytes |
| Proxy target | `http://127.0.0.1:65000` |
| OpenSSL | 3.0.2 |

**Security headers** (set by nginx, not Go):
- `X-Frame-Options: SAMEORIGIN`
- `X-Content-Type-Options: nosniff`
- `Strict-Transport-Security: max-age=31536000; includeSubDomains`
- `Referrer-Policy: strict-origin-when-cross-origin`
- `Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'`

---

## Production — Systemd

**Unit file**: `/etc/systemd/system/bekci.service`

| Setting | Value |
|---------|-------|
| User/Group | `bekci:bekci` (uid/gid 999) |
| WorkingDirectory | `/var/lib/bekci` |
| ExecStart | `/opt/bekci/bekci -config /etc/bekci/config.yaml` |
| EnvironmentFile | `/etc/bekci/env` |
| Restart | on-failure, RestartSec=5 |
| AmbientCapabilities | CAP_NET_RAW |

---

## Build & Deploy

### Local (Docker) — primary dev method
```bash
docker compose down && docker compose up -d --build   # rebuild + restart
docker compose logs -f bekci                           # view logs
```

### Local (bare-metal) — optional
```bash
make build        # frontend + embed copy + Go binary
make run          # build + run
make dev          # Go run (use with `cd frontend && npm run dev` for HMR)
make test         # go test -v ./...
```

### Production (bare-metal)
```bash
ssh cl@dias-bekci
cd /home/cl/bekci-src && git pull
export PATH=/usr/local/go/bin:$PATH
CGO_ENABLED=1 go build -ldflags '-X main.version=X.Y.Z' -o bin/bekci ./cmd/bekci
sudo systemctl stop bekci && sudo cp bin/bekci /opt/bekci/bekci && sudo systemctl start bekci
```

No npm on server — `cmd/bekci/frontend_dist/` is committed to git. Go binary embeds frontend via `//go:embed all:frontend_dist`.

**Critical**: After any frontend source change, run `make frontend` and commit `cmd/bekci/frontend_dist/` before pushing. Prod does NOT run `npm run build`.

---

## Go HTTP Server

| Setting | Value |
|---------|-------|
| ReadTimeout | 15s |
| WriteTimeout | 30s |
| IdleTimeout | 60s |
| Default port | 65000 |

---

## Key Dependencies

### Backend (go.mod)
| Package | Version | Purpose |
|---------|---------|---------|
| golang-jwt/jwt/v5 | 5.3.1 | JWT signing & verification |
| google/uuid | 1.6.0 | UUID generation (target IDs, check IDs, etc.) |
| mattn/go-sqlite3 | 1.14.19 | SQLite driver (requires CGO) |
| prometheus-community/pro-bing | 0.8.0 | ICMP ping |
| gosnmp/gosnmp | latest | SNMP v2c/v3 queries |
| golang.org/x/crypto | 0.48.0 | Bcrypt password hashing, Argon2id KDF (backup encryption) |
| gopkg.in/yaml.v3 | 3.0.1 | Config file parsing |

### Frontend (package.json)
| Package | Version | Purpose |
|---------|---------|---------|
| vue | ^3.5.25 | UI framework (Composition API, `<script setup>`) |
| vite | ^7.3.1 | Build tool + dev server |
| @vitejs/plugin-vue | ^6.0.2 | Vue 3 SFC support for Vite |
| vue-router | ^4.6.4 | Client-side routing (11 routes, SearchView lazy-loaded) |
| pinia | ^3.0.4 | State management |
| axios | ^1.13.5 | HTTP client (withCredentials for cookie auth) |
| chart.js | ^4.5.1 | Charts (SLA page) |
| vue-chartjs | ^5.3.3 | Vue Chart.js wrapper |
| chartjs-plugin-annotation | ^3.1.0 | Chart annotation overlays |

### Frontend Build Output
| Asset | Size (raw) | Size (gzipped) |
|-------|-----------|----------------|
| index-*.js | 471 KB | ~158 KB |
| index-*.css | 45 KB | ~8 KB |
| SearchView-*.js | 10 KB | ~3.5 KB |
| SearchView-*.css | 3.7 KB | ~1.1 KB |

SearchView is lazy-loaded (code-split). All other routes in single bundle.

---

## Runtime Architecture

### Scheduler
- Per-check `time.AfterFunc` timer — no worker pool, each check gets its own goroutine
- Startup jitter: initial delay randomized across the check's interval to prevent thundering herd
- Concurrency semaphore: buffered channel caps in-flight checks at 200
- Per-check mutex (`TryLock`) prevents overlapping runs of the same check
- Safety-net poll: reloads all enabled checks from DB every 60s

### Data Architecture (A-011)
- **3-table split**: `check_state` (1 row/check, current status), `check_daily_rollups` (1 row/check/day, pre-aggregated), `check_results` (3-day tactical window)
- `SaveResult` writes all 3 tables in a single transaction (raw insert + state upsert + rollup upsert)
- Raw retention: 3 days (default `history_days`). Rollup retention: 90 days (hardcoded in purge)
- Dashboard/SLA reads hit `check_state` + `check_daily_rollups` — no more full-scan of `check_results`

### Rule Engine
- Triggered per check result (async goroutine after `SaveResult`)
- Per-rule mutex serializes evaluation — prevents duplicate alerts when multiple checks tied to same rule complete simultaneously
- Atomic CAS state transition: `UPDATE ... WHERE current_state = ?` ensures only one evaluator dispatches alerts per transition

### HTTP Checker
- Shared `http.Transport` with connection pooling (100 max idle, 5 per host, 90s idle timeout)
- TLS session cache (128-entry LRU) for session resumption
- Separate transport for `InsecureSkipVerify` targets

### Docker
- `ulimits: nofile: 65535` — required for 1500+ concurrent check sockets
- `cap_add: NET_RAW` — required for ICMP ping

---

## Key Differences

- **Local Docker** is self-contained: multi-stage build produces binary + Alpine container. SQLite in a named volume. Single port (65000). No nginx.
- **Production** is bare-metal Ubuntu. Binary runs as `bekci` user via systemd. Nginx on port 443 handles SSL + gzip + security headers, proxies to Go on 65000. No Docker, no npm.
- **CGO is required everywhere** — go-sqlite3 needs it. Cannot use `CGO_ENABLED=0`.
- **ICMP ping requires NET_RAW** — Docker gets it via `cap_add`. Production uses systemd `AmbientCapabilities` (not `setcap` on the binary).
- **Fail2Ban integration** — Live jail status via `sudo fail2ban-client` (requires sudoers config). Historical ban records read from fail2ban's own SQLite DB (`/var/lib/fail2ban/fail2ban.sqlite3`, read-only). DB retention controlled by fail2ban's `dbpurgeage` setting.
