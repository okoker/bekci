# Backlog

Items discovered during security review (17/02/2026). Not blocking current work.
Promote to `TODO.md` when ready to tackle.

Tags: `[P0]` `[P1]` `[P2]` + `[security]` `[feature]` `[bug]` `[debt]`

---

## Security Review — Remaining Items

### Critical

| ID | Priority | Tags | Description |
|----|----------|------|-------------|
| C4 | `[P0]` | `[security]` | CGO_ENABLED=0 deploy caused ~26 restart cycles. Add systemd `StartLimitBurst`, health check endpoint in unit, or deploy script validation. |

### High

| ID | Priority | Tags | Description |
|----|----------|------|-------------|
| ~~H1~~ | ~~`[P0]`~~ | ~~`[security]`~~ | ~~No firewall~~ — **DONE 18/02/2026**: UFW active, default deny, allows 22/tcp, 80/tcp, 443/tcp only (IPv4+IPv6). |
| H3 | `[P0]` | `[security]` | SSH allows root login (`PermitRootLogin yes`) + password auth (`PasswordAuthentication yes`). |
| ~~H4~~ | ~~`[P1]`~~ | ~~`[security]`~~ | ~~No fail2ban~~ — **DONE 17/02/2026**: fail2ban installed, sshd jail (3 retries, 1h ban, systemd backend). |
| H5 | `[P1]` | `[security]` | systemd service has no sandboxing — missing `ProtectSystem`, `NoNewPrivileges`, `PrivateTmp`, etc. |

### Medium

| ID | Priority | Tags | Description |
|----|----------|------|-------------|
| M1 | `[P1]` | `[security]` | Missing nginx security headers: HSTS, X-Content-Type-Options, X-Frame-Options. |
| M2 | `[P2]` | `[security]` | Binary `/opt/bekci/bekci` owned by `cl`, not root. Compromised `cl` could replace binary. |
| M3 | `[P1]` | `[security]` | Two accounts (`omer`, `cl`) with `NOPASSWD: ALL` sudo. |
| ~~M4~~ | ~~`[P2]`~~ | ~~`[debt]`~~ | ~~Stale user accounts~~ — **DONE 18/02/2026**: Both `adm-tempubuntu` and `omer.koker` deleted (no files, no processes, unused since setup day). |
| M5 | `[P2]` | `[debt]` | Full source code on server at `/home/cl/bekci-src/`. Needed for builds but increases attack surface. |
| M6 | `[P2]` | `[security]` | Self-signed TLS cert, expires 16/02/2027, no auto-renewal. |

### Low

| ID | Priority | Tags | Description |
|----|----------|------|-------------|
| L1 | `[P2]` | `[debt]` | No log rotation for `/var/log/bekci/bekci.log`. |
| L2 | `[P2]` | `[feature]` | No automated DB backups (manual backup/restore exists in UI). |
| ~~L3~~ | ~~`[P2]`~~ | ~~`[debt]`~~ | ~~Outdated Go 1.18~~ — **DONE 17/02/2026**: `apt remove golang-1.18-*`, Go 1.24 only. |
| ~~L4~~ | ~~`[P2]`~~ | ~~`[security]`~~ | ~~Nginx server_tokens~~ — **DONE 17/02/2026**: `server_tokens off;` enabled. |
