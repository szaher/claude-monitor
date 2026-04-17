---
layout: default
title: Configuration
nav_order: 6
---

# Configuration

Configuration is stored in `~/.claude-monitor/config.yaml`. It is created with defaults on first run.

## Full Reference

```yaml
server:
  port: 3000              # HTTP port for the web UI
  host: "127.0.0.1"       # Bind address (use 0.0.0.0 for network access)

cost:
  models:
    opus:
      input: 15.0          # $ per million input tokens
      output: 75.0         # $ per million output tokens
      cache_read: 1.5      # $ per million cache read tokens
      cache_write: 18.75   # $ per million cache write tokens
    sonnet:
      input: 3.0
      output: 15.0
      cache_read: 0.3
      cache_write: 3.75
    haiku:
      input: 0.25
      output: 1.25
      cache_read: 0.03
      cache_write: 0.3

storage:
  database_path: "~/.claude-monitor/claude-monitor.db"
  retention_days: 0        # 0 = keep forever

ui:
  theme: "auto"            # auto, light, dark
  default_page: "dashboard"
  sessions_per_page: 50
```

## Server Settings

| Key | Default | Description |
|-----|---------|-------------|
| `server.port` | `3000` | HTTP port |
| `server.host` | `127.0.0.1` | Bind address |

## Cost Settings

Model pricing is used to estimate session costs. The model name from Claude Code is matched against these keys using substring matching (e.g., a model containing "opus" uses the `opus` pricing).

Prices are in **USD per million tokens**.

## Storage Settings

| Key | Default | Description |
|-----|---------|-------------|
| `storage.database_path` | `~/.claude-monitor/claude-monitor.db` | SQLite database path |
| `storage.retention_days` | `0` | Auto-delete sessions older than N days (0 = disabled) |

## UI Settings

| Key | Default | Description |
|-----|---------|-------------|
| `ui.theme` | `auto` | Color theme: `auto`, `light`, or `dark` |
| `ui.default_page` | `dashboard` | Page shown on load |
| `ui.sessions_per_page` | `50` | Sessions per page in the sessions list |

## Editing Configuration

You can edit the config file directly or use the Settings page in the web UI. Changes made via the UI are saved immediately.

```sh
# View current config
claude-monitor config --show

# Reset to defaults
claude-monitor config --reset

# Edit manually
$EDITOR ~/.claude-monitor/config.yaml
```
