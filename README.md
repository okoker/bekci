# Bekci

Lightweight service watchdog written in Go. Monitors health of your services, auto-restarts on failure, sends email alerts via [Resend](https://resend.com), and displays a status dashboard.

![Status Page](https://img.shields.io/badge/dashboard-port_65000-blue)

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

## License

MIT
