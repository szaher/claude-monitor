# Claude Monitor - Design Specification

A single Go binary that captures, stores, and visualizes all Claude Code interactions.

## Overview

Claude Monitor provides complete observability into Claude Code usage through a hybrid architecture combining real-time hook capture with historical log parsing. It stores data in SQLite (for queries) and JSON (for archiving), and serves an embedded web UI for browsing, filtering, and analyzing interactions.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    claude-monitor                        │
│                   (single Go binary)                     │
│                                                          │
│  ┌──────────┐   ┌──────────────┐   ┌─────────────────┐  │
│  │  Hook     │──>│  Ingestion   │──>│  SQLite DB      │  │
│  │  Receiver │   │  Pipeline    │   │  + JSON Archive  │  │
│  │  (socket) │   │  (dedup +    │   │                  │  │
│  └──────────┘   │   normalize)  │   └────────┬────────┘  │
│                  └──────────────┘            │           │
│  ┌──────────┐          ▲                    │           │
│  │  Log      │──────────┘                    ▼           │
│  │  Watcher  │              ┌─────────────────────────┐  │
│  │  (fsnotify)│              │  Web Server              │  │
│  └──────────┘              │  ├─ REST API              │  │
│                             │  ├─ WebSocket (live)      │  │
│                             │  └─ Embedded SPA          │  │
│                             └─────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

### Components

1. **Hook Receiver** - Unix socket listener that receives fire-and-forget JSON from Claude Code hooks
2. **Log Watcher** - Watches `~/.claude/projects/` with fsnotify, tails JSONL files for new entries
3. **Ingestion Pipeline** - Normalizes data from both sources, deduplicates by UUID, writes to SQLite + JSON archive
4. **SQLite Database** - Primary query store with full-text search (FTS5)
5. **JSON Archive** - One JSON file per session for backup/export
6. **Web Server** - Serves embedded SPA, REST API for queries, WebSocket for live streaming

### Data Flow

**Real-time path (hooks):**
Hook fires → `claude-monitor hook` subcommand reads stdin → sends JSON to Unix socket → daemon buffers → batched write to SQLite → broadcasts to WebSocket clients

**Historical path (log parser):**
fsnotify detects change → tail new JSONL lines → parse entry → deduplicate by UUID → write to SQLite

**Backfill path:**
`claude-monitor import` → scan all `~/.claude/projects/**/*.jsonl` → parse all entries → deduplicate → bulk insert to SQLite

## Data Model

### SQLite Schema

#### sessions
| Column | Type | Description |
|--------|------|-------------|
| id | TEXT PK | Session UUID |
| project_path | TEXT | Original project path |
| project_name | TEXT | Derived short name (last path segment) |
| cwd | TEXT | Working directory |
| git_branch | TEXT | Branch at session start |
| started_at | DATETIME | Session start time |
| ended_at | DATETIME | Session end time (nullable) |
| claude_version | TEXT | Claude Code version |
| entry_point | TEXT | cli, web, ide |
| permission_mode | TEXT | default, plan, etc. |
| total_input_tokens | INTEGER | Aggregated |
| total_output_tokens | INTEGER | Aggregated |
| total_cache_read_tokens | INTEGER | Aggregated |
| total_cache_write_tokens | INTEGER | Aggregated |
| estimated_cost_usd | REAL | Computed from token counts + model pricing |

#### messages
| Column | Type | Description |
|--------|------|-------------|
| id | TEXT PK | Message UUID |
| session_id | TEXT FK | references sessions.id |
| parent_id | TEXT | Parent message UUID |
| type | TEXT | user, assistant, system |
| role | TEXT | user, assistant |
| model | TEXT | Model ID (assistant only) |
| content_text | TEXT | Extracted text content |
| content_json | TEXT | Full content array as JSON |
| stop_reason | TEXT | end_turn, tool_use, etc. |
| input_tokens | INTEGER | Per-message token usage |
| output_tokens | INTEGER | Per-message token usage |
| cache_read_tokens | INTEGER | |
| cache_write_tokens | INTEGER | |
| timestamp | DATETIME | |

#### tool_calls
| Column | Type | Description |
|--------|------|-------------|
| id | TEXT PK | Tool use ID |
| message_id | TEXT FK | references messages.id |
| session_id | TEXT FK | references sessions.id |
| tool_name | TEXT | Bash, Read, Write, etc. |
| tool_input | TEXT | JSON input params |
| tool_response | TEXT | JSON response |
| success | BOOLEAN | Did it succeed |
| error | TEXT | Error message if failed |
| duration_ms | INTEGER | Execution time |
| timestamp | DATETIME | |

#### subagents
| Column | Type | Description |
|--------|------|-------------|
| id | TEXT PK | Agent ID |
| session_id | TEXT FK | references sessions.id |
| agent_type | TEXT | Explore, Plan, etc. |
| description | TEXT | Task description |
| started_at | DATETIME | |
| ended_at | DATETIME | |

### Indexes
- `sessions(project_path, started_at)`
- `messages(session_id, timestamp)`
- `tool_calls(session_id, tool_name)`
- `tool_calls(tool_name, timestamp)`
- FTS5 virtual table on `messages(content_text)` for full-text search

### JSON Archive Structure
```
~/.claude-monitor/
├── config.yaml
├── monitor.db              (SQLite)
├── archive/
│   └── <project-name>/
│       └── <session-id>.json   (full session dump)
```

## Hook Integration

### Hooks Installed (7 total)

| Hook Event | Purpose | Async |
|------------|---------|-------|
| SessionStart | Record session start, capture metadata | sync (once: true) |
| SessionEnd | Mark session end, compute aggregates | async |
| PreToolUse | Capture tool call intent + params | async |
| PostToolUse | Capture tool result + duration | async |
| SubagentStart | Track agent spawns | async |
| SubagentStop | Track agent completion | async |
| Stop | Capture final assistant response per turn | async |

### Hook Configuration

All hooks use the same command:

```json
{
  "hooks": {
    "PreToolUse": [{
      "hooks": [{
        "type": "command",
        "command": "claude-monitor hook",
        "timeout": 1000,
        "async": true
      }]
    }],
    "PostToolUse": [{
      "hooks": [{
        "type": "command",
        "command": "claude-monitor hook",
        "timeout": 1000,
        "async": true
      }]
    }],
    "SessionStart": [{
      "hooks": [{
        "type": "command",
        "command": "claude-monitor hook",
        "timeout": 2000,
        "async": false
      }]
    }],
    "SessionEnd": [{
      "hooks": [{
        "type": "command",
        "command": "claude-monitor hook",
        "timeout": 1000,
        "async": true
      }]
    }],
    "SubagentStart": [{
      "hooks": [{
        "type": "command",
        "command": "claude-monitor hook",
        "timeout": 1000,
        "async": true
      }]
    }],
    "SubagentStop": [{
      "hooks": [{
        "type": "command",
        "command": "claude-monitor hook",
        "timeout": 1000,
        "async": true
      }]
    }],
    "Stop": [{
      "hooks": [{
        "type": "command",
        "command": "claude-monitor hook",
        "timeout": 1000,
        "async": true
      }]
    }]
  }
}
```

### Hook Behavior

- `claude-monitor hook` reads JSON from stdin, appends the event name, sends to Unix socket at `~/.claude-monitor/monitor.sock`
- If daemon isn't running, hook detects no socket and exits silently (< 1ms)
- No hook modifies Claude Code behavior - purely observational
- The daemon buffers incoming events and writes to SQLite in batches (every 500ms or 50 events)

### Log Watcher

- Uses fsnotify to watch `~/.claude/projects/` recursively
- Tails active `.jsonl` files for new lines
- Parses each JSONL entry using the same schemas as hooks
- Deduplicates against existing records using the UUID field
- On `claude-monitor import`, scans all historical `.jsonl` files

## Claude Code Log Format

### Source Files

- Session transcripts: `~/.claude/projects/<encoded-path>/<session-id>.jsonl`
- Subagent transcripts: `~/.claude/projects/<encoded-path>/<session-id>/subagents/agent-<id>.jsonl`
- Session metadata: `~/.claude/sessions/<pid>.json`
- Global history: `~/.claude/history.jsonl`

### Project Path Encoding

Project paths are encoded by replacing `/` with `-`:
`/Users/szaher/my-project` becomes `-Users-szaher-my-project`

### JSONL Entry Types

**User entries:** contain prompt text, permission mode, session ID, cwd, git branch, claude version
**Assistant entries:** contain model ID, content array (text, thinking, tool_use blocks), token usage (input, output, cache_read, cache_write), stop reason
**System entries:** contain turn duration, message count, slug
**Attachment entries:** contain hook results, permissions, deferred tools
**Other:** permission-mode, file-history-snapshot, last-prompt, queue-operation

### Token Usage Fields

```json
{
  "input_tokens": 10,
  "cache_creation_input_tokens": 19344,
  "cache_read_input_tokens": 0,
  "output_tokens": 506,
  "service_tier": "standard",
  "cache_creation": {
    "ephemeral_1h_input_tokens": 0,
    "ephemeral_5m_input_tokens": 19344
  }
}
```

## Web UI

### Technology

- Vanilla JavaScript + CSS (no framework, no build step)
- Embedded in Go binary via `go:embed`
- Charts via Chart.js (embedded)
- WebSocket for live view
- Responsive design

### Pages

**Dashboard (home)**
- Activity heatmap (GitHub-style, sessions per day)
- Token usage over time (line chart, input vs output vs cache)
- Top 5 projects by activity
- Today's stats: sessions, tool calls, tokens, estimated cost
- Most used tools (horizontal bar chart)
- Recent sessions list (last 10)

**Sessions**
- Searchable, filterable table of all sessions
- Columns: date, project, duration, model, tokens, tool calls, cost
- Click to expand: full conversation thread view
  - User prompts and assistant responses (rendered markdown)
  - Tool calls as collapsible blocks with input/output
  - Thinking blocks as collapsible, dimmed sections
  - Subagent spawns as nested, indented threads

**Live**
- Real-time stream of events via WebSocket
- Events appear as they happen: tool calls, responses, agent spawns
- Pause/resume button
- Filter by active session
- Color-coded by event type

**Tools**
- Tool usage breakdown (pie/donut chart)
- Success/failure rates per tool (stacked bar)
- Average execution time per tool
- Most common Bash commands
- Most accessed files (Read/Write/Edit)
- Tool call timeline (scatter plot: time vs duration)

**Agents**
- Subagent spawn frequency
- Agent types distribution
- Agent task descriptions (searchable)
- Agent duration and nesting depth

**Projects**
- Project list with activity stats
- Per-project: sessions over time, most used tools, token usage, cost
- Directory tree heatmap (which dirs get most attention)

**Cost**
- Daily/weekly/monthly cost breakdown
- Cost by model (Opus vs Sonnet vs Haiku)
- Cost by project
- Cache hit rate and savings
- Cost trend with projection

**Settings**
- Capture configuration toggles (what metadata to collect)
- Data retention settings
- Export options (JSON, CSV, HTML report)
- Theme (light/dark/auto)
- Port configuration
- Database path
- Archive settings
- Raw YAML tab for advanced editing

### Filtering System

- Global filter bar at top, persists across pages
- Filters: project, date range, model, session, tool type
- Full-text search across prompts and responses (FTS5)
- Filters combine with AND logic
- URL-encoded filter state (shareable/bookmarkable)

## Configuration

### Config File

Location: `~/.claude-monitor/config.yaml`

```yaml
server:
  port: 3000
  host: "127.0.0.1"

capture:
  metadata:
    git_branch: true
    git_repo: true
    working_directory: true
    claude_version: true
    environment_vars: false
    command_args: false
    system_info: true
  events:
    session_start: true
    session_end: true
    pre_tool_use: true
    post_tool_use: true
    subagent_start: true
    subagent_stop: true
    stop: true

storage:
  database_path: "~/.claude-monitor/monitor.db"
  archive_path: "~/.claude-monitor/archive"
  archive_enabled: true
  retention_days: 0
  max_db_size_mb: 0

cost:
  models:
    claude-opus-4-6:
      input: 15.0
      output: 75.0
      cache_read: 1.5
      cache_write: 18.75
    claude-sonnet-4-5:
      input: 3.0
      output: 15.0
      cache_read: 0.30
      cache_write: 3.75
    claude-haiku-4-5:
      input: 0.80
      output: 4.0
      cache_read: 0.08
      cache_write: 1.0

ui:
  theme: "auto"
  default_page: "dashboard"
  sessions_per_page: 50
```

### Config Interfaces

**CLI:**
```bash
claude-monitor config get <key>
claude-monitor config set <key> <value>
claude-monitor config list
claude-monitor config reset
claude-monitor config edit
```

**UI:** Settings page with toggles, inputs, dropdowns, and raw YAML tab. Changes write to the same config.yaml and take effect immediately.

### Config Priority
1. CLI flags (highest)
2. Environment variables (`CLAUDE_MONITOR_*`)
3. Config file
4. Built-in defaults (lowest)

## CLI Reference

```
claude-monitor <command> [flags]

Commands:
  install          Install hooks and initialize database
  uninstall        Remove hooks, optionally delete data
  serve            Start daemon + web UI (default :3000)
  import           Import all historical Claude Code logs
  config           Manage configuration
  status           Show daemon status, database stats
  export           Export sessions as JSON/CSV/HTML
  hook             Handle hook events (called by Claude Code)
  version          Print version info

Flags (serve):
  --port           Override server port (default 3000)
  --host           Override bind address (default 127.0.0.1)
  --no-browser     Don't auto-open browser on start
  --daemon         Run in background

Flags (install):
  --skip-import    Don't import historical logs
  --force          Overwrite existing hooks

Flags (uninstall):
  --keep-data      Keep database and archive (default: ask)
  --delete-data    Delete all data without asking

Flags (export):
  --format         json, csv, html (default json)
  --project        Filter by project
  --from           Start date
  --to             End date
  --output         Output file path

Flags (import):
  --path           Custom path to Claude logs (default ~/.claude/projects)
```

### Install Process
1. Creates `~/.claude-monitor/` directory
2. Initializes empty SQLite database with schema
3. Creates `~/.claude-monitor/archive/` directory
4. Writes default `config.yaml`
5. Backs up existing `~/.claude/settings.json`
6. Merges hook definitions into `~/.claude/settings.json` (preserves existing hooks)
7. Optionally imports historical logs
8. Prints summary and next steps

### Uninstall Process
1. Removes claude-monitor hooks from `~/.claude/settings.json`
2. Asks whether to keep or delete `~/.claude-monitor/` data
3. Restores backup settings if available

### Install Script (curl)
```bash
curl -sSL https://raw.githubusercontent.com/szaher/claude-monitor/main/install.sh | sh
```
Detects OS/arch, downloads binary from GitHub releases, places in PATH, runs `claude-monitor install`.

## Export & Data Retention

### Export Formats

- **JSON** - Full session data, suitable for programmatic analysis or re-import
- **CSV** - Flat tables (separate files for sessions, messages, tool_calls), for spreadsheet analysis
- **HTML** - Self-contained report with embedded CSS/JS and charts, shareable

### Data Retention

- Configured via `storage.retention_days` (0 = keep forever)
- Retention runs daily when daemon is active
- Expired sessions are archived as JSON (if archiving enabled) then deleted from SQLite
- Archive files remain indefinitely; users can manually delete

## Performance

### Hook Overhead
- All hooks `async: true` except SessionStart (`once: true`)
- Hook script: single Unix socket write, exits in < 5ms
- Daemon down: hook exits in < 1ms
- Zero impact on Claude Code responsiveness

### Database Performance
- WAL mode for concurrent reads during writes
- Batched writes (every 500ms or 50 events)
- Paginated queries in the UI
- Indexes on all filter columns
- FTS5 for full-text search

### Estimated Storage
- ~1KB per message, ~500 bytes per tool call
- Heavy day (50 sessions, 1000 tool calls) ~ 2MB
- A year of heavy use ~ 700MB

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Daemon not running when hook fires | Hook exits silently, log watcher catches up later |
| Corrupt JSONL line | Skip line, log warning, continue |
| SQLite database locked | Retry with exponential backoff (3 attempts) |
| Disk full | Log error, pause archiving, continue serving |
| Port in use | Error message suggesting --port flag |
| Malformed hook input | Log raw input, skip event |
| WebSocket client disconnect | Clean up connection, no crash |
| Log file deleted while watching | Re-scan project directory |

### Graceful Shutdown
- SIGINT/SIGTERM flushes pending writes
- Closes WebSocket connections
- Removes Unix socket file
- Exits cleanly

## Go Project Structure

```
claude-monitor/
├── cmd/
│   └── claude-monitor/
│       └── main.go              # Entry point, CLI parsing
├── internal/
│   ├── config/
│   │   └── config.go            # Config loading, defaults, validation
│   ├── db/
│   │   ├── db.go                # SQLite connection, migrations
│   │   └── queries.go           # Query functions
│   ├── hook/
│   │   └── hook.go              # Hook subcommand (read stdin, send to socket)
│   ├── ingestion/
│   │   ├── pipeline.go          # Normalize, deduplicate, write
│   │   ├── receiver.go          # Unix socket listener
│   │   └── watcher.go           # fsnotify log file watcher
│   ├── parser/
│   │   └── jsonl.go             # JSONL log file parser
│   ├── installer/
│   │   └── installer.go         # Install/uninstall logic
│   ├── exporter/
│   │   └── exporter.go          # JSON/CSV/HTML export
│   └── server/
│       ├── server.go            # HTTP server setup
│       ├── api.go               # REST API handlers
│       └── websocket.go         # WebSocket hub
├── web/
│   ├── static/
│   │   ├── index.html           # SPA entry point
│   │   ├── css/
│   │   │   └── style.css
│   │   └── js/
│   │       ├── app.js           # Router, state management
│   │       ├── api.js           # API client
│   │       ├── components/      # UI components
│   │       │   ├── dashboard.js
│   │       │   ├── sessions.js
│   │       │   ├── live.js
│   │       │   ├── tools.js
│   │       │   ├── agents.js
│   │       │   ├── projects.js
│   │       │   ├── cost.js
│   │       │   └── settings.js
│   │       └── lib/
│   │           └── chart.min.js # Embedded Chart.js
│   └── embed.go                 # go:embed directive
├── install.sh                   # Curl install script
├── go.mod
├── go.sum
├── Makefile
└── README.md
```
