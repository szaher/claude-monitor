---
layout: default
title: Architecture
nav_order: 7
---

# Architecture

Claude Monitor is a single Go binary that embeds a web UI and runs as a local daemon.

## System Overview

```
Claude Code ──hook events──> Unix Socket ──> Hook Receiver
                                                  │
                                                  v
~/.claude/projects/*/logs.jsonl ──> Log Watcher ──> Ingestion Pipeline
                                                        │
                                                   ┌────┴────┐
                                                   v         v
                                              SQLite DB   WebSocket Hub
                                                   │         │
                                                   v         v
                                              REST API   Live UI
                                                   │
                                                   v
                                            Embedded SPA
```

## Components

### Hook Receiver

Listens on a Unix socket at `~/.claude-monitor/monitor.sock`. Claude Code sends JSON events for every hook trigger (session start/end, tool use, agent spawn, etc.).

### Log Watcher

Uses [fsnotify](https://github.com/fsnotify/fsnotify) to monitor `~/.claude/projects/` for new and modified JSONL log files. Parses log entries and feeds them into the ingestion pipeline.

### Ingestion Pipeline

Processes events from both the hook receiver and log watcher:

1. Parses JSON into typed models
2. Creates or updates sessions via UPSERT
3. Inserts messages, tool calls, and subagents (deduplicating by ID)
4. Calculates cost estimates based on token counts and model pricing
5. Broadcasts raw events to the WebSocket hub for real-time display

### SQLite Database

All data is stored in a single SQLite database with FTS5 full-text search. Key tables:

| Table | Purpose |
|-------|---------|
| `sessions` | Session metadata, tokens, cost, project path |
| `messages` | User and assistant messages with full content |
| `tool_calls` | Tool invocations with input, output, success/error |
| `subagents` | Spawned agent metadata |
| `session_commits` | Git commits linked to sessions |
| `session_notes` | User-added notes per session |
| `session_tags` | Tags attached to sessions |
| `session_metrics` | Per-message speed, cache, and tier data |
| `budgets` | Cost budget definitions |
| `messages_fts` | FTS5 index for full-text search |

### REST API

27 endpoints serving JSON. See the [API Reference](api) for details.

### WebSocket Hub

Maintains a set of connected clients. When the ingestion pipeline processes a hook event, it broadcasts the raw JSON to all connected clients for real-time display on the Live page.

### Web UI

A vanilla JavaScript single-page application using Chart.js for charts. The static files are embedded into the Go binary using `go:embed`, so the entire app is a single binary with no external dependencies at runtime.

## Data Flow

### Real-time (hooks)

1. User interacts with Claude Code
2. Claude Code triggers a hook event
3. Hook script sends JSON to the Unix socket
4. Hook receiver passes it to the ingestion pipeline
5. Pipeline writes to SQLite and broadcasts via WebSocket
6. Live page receives and displays the event

### Historical (import)

1. User runs `claude-monitor import`
2. Import command scans `~/.claude/projects/*/logs.jsonl`
3. Each JSONL line is parsed and fed to the ingestion pipeline
4. Pipeline writes to SQLite (duplicates are skipped via UPSERT)

### Git sync

1. User runs `claude-monitor git-sync` or clicks "Sync Git" in the UI
2. For each session with a working directory, runs `git log` in the repo
3. Matches commits to sessions by timestamp overlap
4. Inserts matched commits into `session_commits`

## Build Requirements

- Go 1.25+
- CGO enabled (required for `go-sqlite3`)
- C compiler (gcc or clang)
- Build tag `fts5` for full-text search support

```sh
CGO_ENABLED=1 go build -tags fts5 -o claude-monitor ./cmd/claude-monitor
```
