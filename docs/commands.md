---
layout: default
title: Commands
nav_order: 3
---

# CLI Commands

## install

Install Claude Code hooks and initialize the database.

```sh
claude-monitor install [--force]
```

| Flag | Description |
|------|-------------|
| `--force` | Overwrite existing hook configuration |

## uninstall

Remove hooks and optionally delete all data.

```sh
claude-monitor uninstall [--delete-data]
```

| Flag | Description |
|------|-------------|
| `--delete-data` | Also remove the database and config files |

## serve

Start the monitoring daemon and web UI.

```sh
claude-monitor serve [--port PORT] [--host HOST] [--no-browser]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | `3000` | HTTP port |
| `--host` | `127.0.0.1` | HTTP host |
| `--no-browser` | `false` | Don't open the browser on start |

The server starts three subsystems:
- **Hook receiver** -- listens on a Unix socket for real-time events
- **Log watcher** -- monitors `~/.claude/projects/` for JSONL changes
- **Web server** -- serves the REST API and embedded SPA

## import

Import historical Claude Code session logs from JSONL files.

```sh
claude-monitor import [--path PATH]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--path` | `~/.claude/projects` | Claude Code projects directory |

This scans all `logs.jsonl` files and imports sessions, messages, and tool calls. Duplicates are skipped automatically.

## export

Export sessions as JSON, CSV, or HTML.

```sh
claude-monitor export [--format FORMAT] [--output FILE] [--from DATE] [--to DATE] [--project NAME]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--format` | `json` | Output format: `json`, `csv`, or `html` |
| `--output` | stdout | Output file path |
| `--from` | | Start date filter (RFC3339) |
| `--to` | | End date filter (RFC3339) |
| `--project` | | Filter by project path |

## git-sync

Sync git commits to sessions by matching commit timestamps to session time windows.

```sh
claude-monitor git-sync [--repo PATH] [--since DATE]
```

| Flag | Description |
|------|-------------|
| `--repo` | Sync a specific repository only |
| `--since` | Only look at commits after this date (YYYY-MM-DD) |

For each session with a working directory, this runs `git log` and matches commits that fall within the session's time window.

## config

Manage configuration.

```sh
claude-monitor config [--show] [--reset]
```

| Flag | Description |
|------|-------------|
| `--show` | Display current configuration |
| `--reset` | Reset configuration to defaults |

## status

Show daemon status and database statistics.

```sh
claude-monitor status
```

## version

Print version information.

```sh
claude-monitor version
```
