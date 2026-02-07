# Bekçi

Lightweight service watchdog written in Go. Monitors health of your services, auto-restarts on failure, sends email alerts via Resend, and displays a status dashboard.

I have a lot of endpoints and devices running in my local/remote environments and i jump from project to project as i found it keeps me more dynamic and creative. However remembering whats what was becoming an issue. Also what i do can break things a lot (so i can make them unbreakable). I had tiny bashscripts runnng in the corner for tracking but when I recently became curios about Go I figured might as well make something out of it. Hence Bekçi (AKA Sentry). 

![Status Page](https://img.shields.io/badge/dashboard-65000-blue)
![Status Page](https://img.shields.io/badge/api-65000-purple)

## Features

- **Health checks**: HTTP(S), TCP port, local process, SSH remote process, SSH remote command
- **Auto-restart**: Local shell, SSH remote, Docker container
- **Email alerts**: Failure and recovery notifications via Resend API (with cooldown)
- **Web dashboard**: Real-time status page with 90-day uptime history
- **Check Now**: Manual trigger via dashboard button or API
- **macOS service**: Launchd plist included for running as a background agent

## Quick Start

```bash
# Build
go mod tidy
make build

# Configure
cp config.example.yaml config.yaml
# Edit config.yaml with your services

# Run
./bin/bekci
```

Dashboard at `http://localhost:65000`

## Configuration

See `config.example.yaml` for the full reference. Key sections:

```yaml
global:
  check_interval: 5m      # How often to check services
  web_port: 65000          # Dashboard port

projects:
  - name: "My App"
    services:
      - name: "backend"
        url: "http://myapp.com:8000"
        check:
          type: https
          endpoint: "/health"
        restart:
          type: local
          command: "systemctl restart myapp"
```

### Check Types

| Type | Description | Required |
|------|-------------|----------|
| `https` | HTTP(S) GET, check status code | `url` |
| `tcp` | TCP port connect | `url` (host:port) |
| `process` | Local process by name | `name` |
| `ssh_process` | Remote process via SSH | `host`, `name` |
| `ssh_command` | Remote command via SSH | `host`, `command` |

### Restart Types

| Type | Description |
|------|-------------|
| `local` | Run local shell command |
| `ssh` | Run command on remote host via SSH |
| `docker` | `docker restart <container>` |
| `none` | Alert only, no restart |

## API

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | HTML status dashboard |
| `/health` | GET | Self-check (`{"status":"ok"}`) |
| `/api/status` | GET | JSON status for all services |
| `/api/check-now` | POST | Trigger immediate check of all services |

## macOS Launchd

```bash
cp com.bekci.agent.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.bekci.agent.plist
```


## Logging

Bekci logs to a file and stderr simultaneously. Log level and path are set in `config.yaml`:

```yaml
global:
  log_level: warn       # debug, info, warn, error
  log_path: bekci.log   # relative to working directory
```

**From terminal** — logs print to your terminal _and_ the log file:

```bash
./bin/bekci &                    # logs visible in terminal + bekci.log
./bin/bekci 2>/dev/null &        # terminal silent, logs go to bekci.log only
./bin/bekci 2>&1 | tee /tmp/b &  # terminal + bekci.log + custom file
```

**As a launchd service** — when running under launchd (parent PID 1), bekci automatically writes to `/var/log/bekci.log` instead of the config path. Stderr output is captured by launchd to the paths set in the plist (`StandardErrorPath`).

```bash
# Install as user agent
sudo mkdir -p /var/log
sudo touch /var/log/bekci.log && sudo chown $(whoami) /var/log/bekci.log
cp com.bekci.agent.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.bekci.agent.plist

# View logs
tail -f /var/log/bekci.log
```

## Road Map
* Add scripted checks. 
* Push results to a tiny mobile app. 
* Add a scheduler to turn services on/off on the cloud.



## License

MIT - Copyright (c) 2026 Objects Consulting Ltd, UK
