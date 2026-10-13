# Bekci — Tech Stack & Environment Reference

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.25 language level (go.mod 1.25.0), built with the Go 1.27.1 toolchain; net/http stdlib router (Go 1.22+ method routing), SQLite WAL |
| Database | SQLite 3 via go-sqlite3 (CGO required), WAL mode, `SetMaxOpenConns(1)`, schema v27 (fresh installs create v27 from the baseline; existing v22+ databases, and v21 via a bridge, migrate incrementally) |
| Frontend | Vue 3, Vite 7, Vue Router 4, Pinia 3, Axios, Chart.js + vue-chartjs |
| Auth | JWT HS256 (golang-jwt/v5) in HttpOnly cookie (`token`), bcrypt cost 12 |
| Reverse Proxy | Nginx 1.18 (prod only) — SSL termination, security headers, gzip |
| Email | Resend API (default) or Microsoft 365 SMTP (`email_provider` = `ms365`; STARTTLS required, LOGIN auth, port 587 default) |
| Signal | Signal Messenger via signal-cli REST API |
| Webhook | Generic JSON POST to any HTTP endpoint (SOAR, Slack, etc.), Bearer or Basic auth |
| Network | ICMP ping via pro-bing (uses privileged raw ICMP if root, otherwise an unprivileged ICMP datagram socket — pro-bing's "udp" mode), SNMP via gosnmp |
| Config | YAML base + env var overrides (`BEKCI_` prefix) + auto-generated defaults |
| Deploy | Docker multi-stage build (local) or bare-metal via Makefile (prod) |
| Website | Static landing page in `_website/`, deployed to GitHub Pages via Actions → https://okoker.github.io/bekci/ |

---

## Environments

| | **Local Dev (Docker)** | **Production** |
|---|---|---|
| **Host** | Docker Desktop on macOS | `ssh cl@dias-bekci` (10.0.9.20) |
| **URL** | `http://localhost:65000` | `https://dias-bekci` (nginx on 443) |
| **OS** | Alpine 3.23 (container) | Ubuntu 22.04.5 LTS (kernel 5.15) |
| **Go** | 1.27-alpine (build stage) | 1.27.1 (`/usr/local/go`) |
| **Node** | 22-alpine (build stage) | None (frontend pre-built in repo) |
| **DB path** | `/data/bekci.db` (Docker volume `bekci-data`) | `/var/lib/bekci/bekci.db` |
| **Binary** | `/usr/local/bin/bekci` (in container) | `/opt/bekci/bekci` |
| **Config** | Env vars in `docker-compose.yml` | `/etc/bekci/config.yaml` + `/etc/bekci/env` |
| **Service** | Docker (`restart: unless-stopped`) | systemd (`bekci.service`, user `bekci:bekci`) |
| **NET_RAW** | `cap_add: [NET_RAW]` in compose | `AmbientCapabilities=CAP_NET_RAW` in systemd unit |
| **Reverse proxy** | None (direct access) | Nginx 1.18 on port 443 |
| **Default creds** | admin / admin1234 (code default on first boot; user changes post-boot) | admin / admin1234 (code default on first boot; user changes post-boot) |

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
| WatchdogSec | 120 (Go process sends WATCHDOG=1 every 30s; if process hangs, systemd kills+restarts) |

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

### Marketing website (GitHub Pages)
The public landing page (the "Sentinel" design) lives in `_website/` (`index.html` + `assets/`) — a self-contained static site, separate from the product app.
- **Live**: https://okoker.github.io/bekci/
- **Source**: `_website/index.html` — single self-contained HTML (inline CSS/JS, Google Fonts via `<link>`, assets referenced as `assets/...`).
- **Deploy**: `.github/workflows/pages.yml` runs on push to `main` touching `_website/**` and publishes `_website/` as-is via GitHub Actions (Pages build source = Actions). No build step (~18s). Edit `_website/index.html` → commit → push → auto-deploys.

---

## Go HTTP Server

| Setting | Value |
|---------|-------|
| ReadTimeout | 15s |
| WriteTimeout | 30s |
| IdleTimeout | 60s |
| Default port | 65000 |
| Gzip | stdlib `compress/gzip` middleware, wraps outermost in router chain. Compressed when client sends Accept-Encoding: gzip. |
| Health cache | `/api/system/health` — net/disk/cpu cached 120s TTL (`sync.Mutex`), scheduler status always fresh (atomic read). |

---

## Key Dependencies

### Backend (go.mod)
| Package | Version | Purpose |
|---------|---------|---------|
| golang-jwt/jwt/v5 | 5.3.1 | JWT signing & verification |
| google/uuid | 1.6.0 | UUID generation (target IDs, check IDs, etc.) |
| mattn/go-sqlite3 | 1.14.37 | SQLite driver (requires CGO) |
| prometheus-community/pro-bing | 0.8.0 | ICMP ping |
| gosnmp/gosnmp | 1.43.2 | SNMP v2c/v3 queries |
| golang.org/x/crypto | 0.55.0 | Bcrypt password hashing, Argon2id KDF (backup encryption) |
| golang.org/x/net | 0.57.0 | HTTP/2 + IDNA support used by stdlib http |
| gopkg.in/yaml.v3 | 3.0.1 | Config file parsing |

### Frontend (package.json)
| Package | Version | Purpose |
|---------|---------|---------|
| vue | ^3.5.25 | UI framework (Composition API, `<script setup>`) |
| vite | ^7.3.3 | Build tool + dev server |
| @vitejs/plugin-vue | ^6.0.2 | Vue 3 SFC support for Vite |
| playwright-core | 1.59.1 | Development-only browser regressions with Node's test runner and installed Chrome (`npm run test:ui`) |
| vue-router | ^4.6.4 | Client-side routing (11 routes, SearchView + SlaView lazy-loaded) |
| pinia | ^3.0.4 | State management |
| axios | ^1.16.0 | HTTP client (withCredentials for cookie auth) |
| chart.js | ^4.5.1 | Charts (SLA page) |
| vue-chartjs | ^5.3.3 | Vue Chart.js wrapper |
| chartjs-plugin-annotation | ^3.1.0 | Chart annotation overlays |
| chartjs-plugin-zoom | ^2.2.0 | Chart pan/zoom interaction |
| hammerjs | ^2.0.8 | Touch gestures (used by chartjs-plugin-zoom) |

Versions above are the `package.json` ranges. `package-lock.json` resolves vue 3.5.28 and @vitejs/plugin-vue 6.0.4; every other package resolves to the version shown.

### Frontend Build Output
Measured from the build in `cmd/bekci/frontend_dist/assets/` at v3.4.5 on 13/10/2026 (raw bytes; gzip -9; kB = 1000 bytes). Figures change with every frontend build.

| Asset | Size (raw) | Size (gzipped) |
|-------|-----------|----------------|
| index-*.js | 314.08 kB | 100.28 kB |
| index-*.css | 49.71 kB | 9.06 kB |
| SearchView-*.js | 15.16 kB | 5.21 kB |
| SearchView-*.css | 5.43 kB | 1.53 kB |
| SlaView-*.js | 239.78 kB | 82.92 kB |
| SlaView-*.css | 3.72 kB | 1.11 kB |

SearchView and SlaView are lazy-loaded (code-split). Lazy-loading SlaView keeps its ~240 kB chunk (SLA view + Chart.js) out of the main bundle; it loads on demand.

### SOC Page Performance
- **Poll interval:** 30 seconds (status refresh only; history loaded on page load/navigation, not polls)
- **90d history:** Off by default. User toggle loads on demand. Not re-fetched on polls — only loaded once when enabled.
- **Pagination:** 48 hosts per page (12 rows × 4 columns). History fetched only for visible page.
- **Page change:** `loadHistoryForVisiblePage()` fires immediately on page navigation — no waiting for next poll.
- **Net effect (historical measurement):** ~970 requests/min → ~100 requests/min and initial load 8s → 2s, recorded 17/03/2026 at v3.3.0 when the 30s poll, 90d toggle and pagination shipped. Host count and environment were not recorded; these are not current guarantees. At v3.4.5, an open SOC page with an unchanged host list sends 2 requests per 30s (`/soc/status` + `/system/health`) once the visible page's history is loaded.
- **Poll timeout:** `/api/soc/status` and `/api/system/health` polls give up after 20s (`POLL_TIMEOUT_MS`), so a hung server can't leave requests pending behind Chrome's 6-connections-per-host limit. Other API calls keep axios' default (no timeout).
- **Stale alarm:** if no `/api/soc/status` poll has succeeded for 92s (three missed polls + latency slack; `STALE_MS`, measured from page load if none ever succeeded), the SOC header row pulses brand orange (1 Hz; solid under `prefers-reduced-motion`) and the clock reads `STALE · last update HH:MM:SS (Xs/Xm ago)`. Checked every 5s; clears on the next successful poll. Cannot fire if the tab itself is dead (crash page, frozen renderer).
- **Visibility resume:** when the tab becomes visible again, status and health refresh immediately (hidden tabs have throttled timers), avoiding a false stale alarm after the wall showed other content.

---

## Runtime Architecture

### Scheduler
- Per-check `time.AfterFunc` timer — no worker pool, each check gets its own goroutine
- Startup jitter: initial delay randomized across the check's interval to prevent thundering herd
- Concurrency semaphore: buffered channel caps in-flight checks at 200
- Per-check mutex (`TryLock`) prevents overlapping runs of the same check
- Safety-net poll: reloads all enabled checks from DB every 60s
- Heartbeat: `lastTick` atomic timestamp updated every 60s tick. Exposed via `LastTick()` and `ActiveCheckCount()` methods. Health endpoint reports scheduler status (ok/stale/starting). Stale threshold: 120s (2x tick interval).
- SNMP settings cache: `getCachedSetting()` method with 15m TTL. All 7 SNMP keys cached together — avoids 7 DB reads per SNMP check execution. Long TTL justified because SNMP credentials change rarely (admin action only).

### Self-Health Monitoring
- **Scheduler dead man's switch**: 4th navbar dot (green ok / yellow starting / red stale), popover with age + check count, red banner when stale (auto-dismisses on recovery). The banner belongs to `DefaultLayout`, so it shows on every authenticated page but not on Login or the public-route SOC page (`App.vue` renders public routes without the layout). The SOC page has its own scheduler dot, plus a separate stale alarm driven by successful `/soc/status` polls (see SOC Page Performance), not by scheduler health. Audit log entries every 60s while stale; no system alert is sent.
- **Crash detection**: `.shutdown_clean` marker file written on graceful SIGTERM, consumed on startup. Missing marker = unclean restart → 4 red audit log entries + system alert to configured recipients.
- **System alerts**: `system_alert_admins` (bool) + `system_alert_users` (CSV user IDs). Sent directly, bypassing the scheduler: from main.go startup (unclean restart) and from `RunScheduledBackup` (failed scheduled backup). Uses configured alert channels (email/signal/webhook); the webhook is sent even when no recipients resolve.

### Automated Backups
- Hourly goroutine (first tick one hour after startup) checks `auto_backup_schedule` (off/weekly/10days/monthly; empty = off) + `auto_backup_time` (`HH:MM`, empty = `03:00`). Only the hour is used, in the server's local time zone; the minutes are ignored. The backup runs on the tick that falls inside that hour, so the exact minute depends on when the process started
- Due when the newest entry in index.json (any saved backup: scheduled, Save to server or the download auto-save) is at least 7 / 10 / 30 whole days old. `monthly` means 30 days, not a calendar month
- Calls `buildFullBackupArchive` (unencrypted) + `saveBackupCopy` (respects `backup_max_copies`; shared with Save to server and the download auto-save)
- Audit log entries: "Starting scheduled backup" + "Backup completed: {filename}" or "Backup failed: {error}". "Backup completed" is written only after the archive and `index.json` are saved; any build or save error audits a failure and sends a system alert ("Bekci scheduled backup failed")
- Backup files stored at 0600 (owner-only permissions)

### Data Architecture (A-011)

The single `check_results` table was split into a 3-table architecture to eliminate full-table scans on the dashboard poll. At 90-day steady state with 1,500 checks, `check_results` would hold ~194M rows (~30-45 GB). Row counts and sizes here and below are estimates for 1,500 checks at a 60s interval, not measurements. Now:

| Table | Rows at scale | Purpose |
|-------|--------------|---------|
| `check_state` | ~1,500 | Current status per check (1:1). Replaces `MAX(checked_at)` subqueries. |
| `check_daily_rollups` | ~135K (90d) | Pre-aggregated daily uptime. Replaces `GROUP BY date(checked_at)` scans. |
| `check_results` | ~6.5M (3d) | Tactical window for 4h bar chart + fail_window evaluation. Was ~194M. |

**Write path:** `SaveResult` writes all 3 tables in a single BEGIN/COMMIT transaction:
1. INSERT into `check_results` (raw row)
2. UPSERT into `check_state` (current status cache)
3. UPSERT into `check_daily_rollups` (increment counts, running average)

**Read path mapping:**

| Method | Reads from | Used by |
|--------|-----------|---------|
| `GetLastResult` | `check_state` | Engine (rule eval), alerter (webhook payload) |
| `GetBatchLastResultAndUptime` | `check_state` + `check_daily_rollups` | Dashboard status, SOC status |
| `GetDailyUptime` | `check_daily_rollups` | SLA page (90d chart), dashboard (90d history) |
| `GetUptimePercent` | `check_daily_rollups` | SLA badge (via batch query) |
| `GetRecentResultsSlim` | `check_results` | Dashboard 4h bar chart |
| `GetRecentResultsByWindow` | `check_results` | Engine fail_window condition evaluation |

**Retention:**
- Raw results (`check_results`): `history_days` setting, seeded as 3 days. Any positive value is applied, lower or higher than 3 (the settings API accepts 1–3650); `main.go` falls back to 3 days if the setting is missing or not a positive integer
- Daily rollups (`check_daily_rollups`): 90 days (hardcoded in `PurgeOldResults`)
- Current state (`check_state`): unbounded (removed only on check delete via CASCADE)

**Purge:** Hourly ticker in `main.go` runs `PurgeOldResults` — two DELETEs: raw results older than retention + rollups older than 90 days. At 3-day raw retention, purge is trivially fast (~150K rows max per cycle).

### Rule Engine
- Triggered per check result (async goroutine after `SaveResult`)
- Per-rule mutex serializes evaluation — prevents duplicate alerts when multiple checks tied to same rule complete simultaneously
- Atomic CAS state transition: `UPDATE ... WHERE current_state = ?` ensures only one evaluator dispatches alerts per transition
- Alert dispatch semaphore: buffered channel (cap 10) bounds concurrent alert goroutines — prevents unbounded HTTP/SMTP connections on mass failure

### HTTP Checker
- Shared `http.Transport` with connection pooling (100 max idle, 5 per host, 90s idle timeout)
- TLS session cache (128-entry LRU) for session resumption
- Separate transport for `InsecureSkipVerify` targets

### Docker
- `ulimits: nofile: 65535` — required for 1500+ concurrent check sockets
- `cap_add: NET_RAW` — required for ICMP ping (the container runs as root, so raw ICMP is used)

---

## Key Differences

- **Local Docker** is self-contained: multi-stage build produces binary + Alpine container. SQLite in a named volume. Single port (65000). No nginx.
- **Production** is bare-metal Ubuntu. Binary runs as `bekci` user via systemd. Nginx on port 443 handles SSL + gzip + security headers, proxies to Go on 65000. No Docker, no npm.
- **CGO is required everywhere** — go-sqlite3 needs it. Cannot use `CGO_ENABLED=0`.
- **ICMP mode depends on the process UID** (`internal/checker/ping.go`: `SetPrivileged(os.Getuid() == 0)`). As root (the Docker container), Bekci sends raw ICMP, which needs NET_RAW; Docker grants it via `cap_add`. As a non-root user (production `bekci`), it uses unprivileged ICMP datagram sockets, which Linux governs with the `net.ipv4.ping_group_range` sysctl, not CAP_NET_RAW. Production's systemd unit grants `AmbientCapabilities=CAP_NET_RAW` (not `setcap` on the binary), but the ping checker does not use raw sockets as non-root.
- **Fail2Ban integration** — Live jail status via `sudo fail2ban-client` (requires sudoers config). Historical ban records read from fail2ban's own SQLite DB (`/var/lib/fail2ban/fail2ban.sqlite3`, read-only). DB retention controlled by fail2ban's `dbpurgeage` setting.
