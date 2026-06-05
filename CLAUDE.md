# Claude Monitor

A Go CLI tool that monitors Claude Code sessions via hooks, stores data in SQLite, and serves a web dashboard. It tracks token usage, costs, tool calls, subagents, git commits, and more.

## Quick Reference

```bash
make build          # CGO_ENABLED=1, -tags fts5 (required for SQLite FTS)
make test           # CGO_ENABLED=1 go test -tags fts5 -v ./...
make install        # build + copy to /usr/local/bin
```

Binary: `bin/claude-monitor`. Entry point: `cmd/claude-monitor/main.go`.

## Architecture

```
cmd/claude-monitor/   CLI entry point — manual arg dispatch (no framework)
internal/
  cli/                Subcommand implementations (serve, install, import, export, etc.)
  config/             YAML config (~/.claude-monitor/config.yaml)
  db/                 SQLite with FTS5 — schema.go has base schema + migrations
  exporter/           JSON/CSV/HTML export
  hook/               Reads stdin, forwards JSON to Unix socket (~/.claude-monitor/monitor.sock)
  ingestion/          Pipeline: watcher + receiver parse JSONL session logs
  installer/          Reads/writes ~/.claude/settings.json to register hooks
  models/             Shared structs (Session, Message, ToolCall, etc.)
  parser/             JSONL parser for Claude Code session logs (10MB scanner buffer)
  server/             HTTP server + WebSocket hub + REST API handlers
web/
  embed.go            go:embed for static assets
  static/             Dashboard UI (vanilla JS, no build step)
```

## Key Details

- **Build tags**: Always use `-tags fts5` — the SQLite FTS5 virtual table is required
- **CGO**: Required (`CGO_ENABLED=1`) — sqlite3 driver is C-based
- **Config**: YAML at `~/.claude-monitor/config.yaml`, defaults in `internal/config/config.go:DefaultConfig()`
- **Database**: SQLite at `~/.claude-monitor/claude-monitor.db`, schema + migrations in `internal/db/schema.go`
- **Default server**: `127.0.0.1:3000`
- **Hook mechanism**: Installs into `~/.claude/settings.json` — the hook command reads JSON from stdin and sends it to a Unix domain socket
- **JSONL scanner buffer**: 10MB max line size (handles large assistant responses)
- **Web UI**: Embedded static files, no npm/node build step

## Development Guidelines

- Do NOT assume `gh` CLI is available — use `git` commands directly for any git operations
- Commit messages: lowercase, imperative style (e.g., `fix token tracking`, `feat: add budget alerts`)
- No external CLI tools beyond standard Go toolchain and `git`
- Tests use the standard `testing` package — no test frameworks
- The web UI is vanilla JS with no framework — changes go directly in `web/static/`

## Agent Instructions

See [AGENTS.md](AGENTS.md) for guidelines when working as a subagent or in multi-agent workflows.
