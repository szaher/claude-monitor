# Claude Monitor Enhancements - Design Specification

## Overview

Nine feature additions to Claude Monitor plus expanded data capture from Claude Code JSONL logs. All features build on the existing architecture (Go backend, SQLite, embedded vanilla JS SPA) and follow established patterns.

## 1. Expanded Data Capture

Before building new features, capture additional fields already available in Claude Code JSONL logs but not yet stored.

### 1.1 Existing Fields to Populate

These columns already exist in the schema but are not being populated by the import or pipeline:

| Table | Column | Source | Description |
|-------|--------|--------|-------------|
| `messages` | `stop_reason` | `message.stop_reason` | `end_turn`, `tool_use`, `max_tokens` |
| `tool_calls` | `duration_ms` | `toolUseResult.durationMs` from user entries | Execution time in milliseconds |
| `tool_calls` | `success` | `toolUseResult.is_error` (inverted) | Whether tool call succeeded |

### 1.2 New Columns in `tool_calls` Table

These require `ALTER TABLE` additions:

```sql
ALTER TABLE tool_calls ADD COLUMN stderr TEXT DEFAULT '';
ALTER TABLE tool_calls ADD COLUMN stdout_preview TEXT DEFAULT '';
```

| Column | Type | Source | Description |
|--------|------|--------|-------------|
| `stderr` | TEXT | `toolUseResult.stderr` | Standard error output from tool execution |
| `stdout_preview` | TEXT | `toolUseResult.stdout` (first 500 chars) | Truncated stdout for quick inspection |

### 1.3 New `session_metrics` Table

Stores per-message metadata that doesn't fit in `messages`. One row per assistant message.

```sql
CREATE TABLE IF NOT EXISTS session_metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    speed TEXT,
    service_tier TEXT,
    inference_geo TEXT,
    cache_ephemeral_5m_tokens INTEGER DEFAULT 0,
    cache_ephemeral_1h_tokens INTEGER DEFAULT 0,
    timestamp DATETIME NOT NULL,
    FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE,
    FOREIGN KEY(message_id) REFERENCES messages(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_session_metrics_session ON session_metrics(session_id);
```

### 1.4 New `context_compactions` Table

Tracks when Claude Code compacts conversation context.

```sql
CREATE TABLE IF NOT EXISTS context_compactions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    pre_tokens INTEGER NOT NULL,
    post_tokens INTEGER NOT NULL,
    trigger TEXT,
    duration_ms INTEGER,
    timestamp DATETIME NOT NULL,
    FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_context_compactions_session ON context_compactions(session_id);
```

### 1.5 New `session_attachments` Table

Stores selected attachment metadata (ultrathink effort, skill listings).

```sql
CREATE TABLE IF NOT EXISTS session_attachments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    attachment_type TEXT NOT NULL,
    content TEXT,
    timestamp DATETIME NOT NULL,
    FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_session_attachments_session_type ON session_attachments(session_id, attachment_type);
```

### 1.6 Parser Changes

Extend `LogEntry` and `MessageData` structs in `internal/parser/jsonl.go`:

```go
type LogEntry struct {
    // ... existing fields ...
    Speed        string           `json:"speed,omitempty"`
    ServerToolUse *ServerToolUse  `json:"server_tool_use,omitempty"`
}

type ServerToolUse struct {
    WebSearchCount int `json:"web_search_count"`
    WebFetchCount  int `json:"web_fetch_count"`
}

type MessageData struct {
    // ... existing fields ...
    StopReason  string       `json:"stop_reason"`
    ServiceTier string       `json:"service_tier,omitempty"`
}

type Usage struct {
    // ... existing fields ...
    CacheCreation *CacheCreationDetail `json:"cache_creation,omitempty"`
}

type CacheCreationDetail struct {
    Ephemeral5mTokens int `json:"ephemeral_5m_input_tokens"`
    Ephemeral1hTokens int `json:"ephemeral_1h_input_tokens"`
}
```

### 1.7 Pipeline Changes

In `internal/ingestion/pipeline.go`, extend `ProcessLogEntry` to:

1. Extract `speed` from root-level `speed` field on assistant entries
2. Extract `service_tier` from `usage` on assistant entries
3. Insert into `session_metrics` for each assistant message
4. Parse `compactMetadata` entries from system messages and insert into `context_compactions`
5. Parse attachment entries with `type: "ultrathink_effort"` and insert into `session_attachments`
6. Extract `toolUseResult` blocks from user entries to populate `tool_calls.duration_ms`, `success`, `stderr`, `stdout_preview`

### 1.8 Import Changes

In `internal/cli/importcmd.go`, extend the import loop to process the same new fields as the pipeline.

## 2. Token Efficiency Dashboard

### Purpose

Show how efficiently token budget is used: cache hit rates, tokens per tool call, context growth over sessions.

### Backend

**New API endpoint:** `GET /api/stats/token-efficiency`

Query params: `project` (optional), `from`/`to` (optional date range)

Response:

```json
{
    "cache_hit_rate": 0.73,
    "avg_tokens_per_tool_call": 1250,
    "avg_input_tokens_per_message": 8400,
    "avg_output_tokens_per_message": 620,
    "cache_savings_tokens": 1450000,
    "cache_savings_usd": 12.50,
    "context_growth": [
        {"session_id": "abc", "start_tokens": 1000, "end_tokens": 85000, "compactions": 2}
    ],
    "efficiency_by_model": [
        {"model": "claude-opus-4-6", "cache_hit_rate": 0.68, "avg_output_ratio": 0.07}
    ]
}
```

**SQL queries:**
- Cache hit rate: `SUM(cache_read_tokens) / (SUM(cache_read_tokens) + SUM(input_tokens))` from messages
- Tokens per tool call: `SUM(m.output_tokens) / COUNT(tc.id)` joined messages and tool_calls
- Context growth: first and last message token counts per session plus compaction count

### Frontend

Add a "Token Efficiency" card section to the existing **Cost** page (`cost.js`):

- **Cache Hit Rate gauge** - circular progress indicator showing percentage
- **Token Efficiency table** - rows: model, cache hit rate, avg input, avg output, output ratio
- **Context Growth chart** - line chart showing token accumulation across messages in a session, with compaction events marked as vertical lines
- **Cache Savings** - dollar amount saved by cache hits vs. full re-processing

## 3. Error Analysis

### Purpose

Surface tool call failures: which tools fail most, common error patterns, failure rate trends.

### Backend

**New API endpoint:** `GET /api/stats/errors`

Query params: `project` (optional), `from`/`to` (optional), `tool` (optional)

Response:

```json
{
    "total_errors": 245,
    "error_rate": 0.034,
    "errors_by_tool": [
        {"tool": "Bash", "errors": 120, "total": 2500, "rate": 0.048}
    ],
    "common_errors": [
        {"pattern": "permission denied", "count": 45, "tools": ["Bash", "Write"]},
        {"pattern": "file not found", "count": 38, "tools": ["Read", "Edit"]}
    ],
    "error_trend": [
        {"date": "2026-04-10", "errors": 12, "total": 350}
    ]
}
```

**SQL queries:**
- Errors by tool: `SELECT tool_name, COUNT(*) FILTER (WHERE success=0), COUNT(*) FROM tool_calls GROUP BY tool_name`
- Common errors: `SELECT error, COUNT(*) FROM tool_calls WHERE success=0 AND error IS NOT NULL GROUP BY error ORDER BY COUNT(*) DESC LIMIT 20`
- For pattern grouping: do keyword extraction in Go (split on common substrings like "permission denied", "no such file", "command not found")

### Frontend

Add an "Error Analysis" section to the **Tools** page (`tools.js`):

- **Error Rate badge** in the page header stats row
- **Failure Rate by Tool** - horizontal bar chart (red bars) showing error rate per tool
- **Common Error Patterns** - table with error message pattern, count, affected tools
- **Error Trend** - small line chart showing daily error count over last 30 days

## 4. File Change Heatmap

### Purpose

Show which files/directories get the most attention from Claude Code within a project.

### Backend

**New API endpoint:** `GET /api/stats/file-heatmap`

Query params: `project` (required), `from`/`to` (optional)

Response:

```json
{
    "files": [
        {"path": "internal/server/api.go", "reads": 15, "writes": 8, "edits": 12, "total": 35},
        {"path": "web/static/js/components/sessions.js", "reads": 10, "writes": 3, "edits": 7, "total": 20}
    ],
    "directories": [
        {"path": "internal/server", "total": 85},
        {"path": "web/static/js/components", "total": 60}
    ]
}
```

**SQL queries:**
- Extract file paths from `tool_calls.tool_input` JSON for tools Read, Write, Edit, Glob, Grep
- Parse `tool_input` as JSON, extract `file_path` or `path` field
- Group by file path, count by tool type (Read vs Write vs Edit)
- Aggregate to directory level by extracting parent path

### Frontend

Add a "File Activity" section to the **Projects** detail view (`projects.js`):

- **Top Files table** - ranked list with file path, read/write/edit counts, total, and a small inline horizontal bar
- **Directory Treemap** - nested rectangles sized by activity count, colored by read vs write ratio
  - Use Chart.js treemap plugin (or a simple CSS grid-based visualization if plugin is too heavy)
- Clicking a file filters the sessions list to sessions that touched that file

## 5. Session Timeline

### Purpose

Visualize the flow of a single session: when user prompts, assistant responses, tool calls, and agent spawns occurred relative to each other.

### Backend

**New API endpoint:** `GET /api/sessions/{id}/timeline`

Response:

```json
{
    "events": [
        {"type": "user_message", "timestamp": "...", "preview": "Fix the bug in..."},
        {"type": "assistant_message", "timestamp": "...", "model": "claude-opus-4-6", "tokens": 1500},
        {"type": "tool_call", "timestamp": "...", "tool": "Bash", "duration_ms": 450, "success": true},
        {"type": "agent_spawn", "timestamp": "...", "agent_type": "Explore", "description": "Find..."},
        {"type": "compaction", "timestamp": "...", "pre_tokens": 85000, "post_tokens": 12000}
    ],
    "duration_seconds": 1845,
    "total_tokens": 125000
}
```

**SQL queries:**
- UNION ALL of messages (with type tag), tool_calls (with type tag), subagents (with type tag), context_compactions
- ORDER BY timestamp
- Truncate content to preview length

### Frontend

Add a timeline visualization to the **Sessions** detail view (`sessions.js`):

- **Horizontal timeline bar** at the top of session detail, showing the full session duration
- Events plotted as icons/markers on the timeline:
  - Blue dots: user messages
  - Green dots: assistant responses (size proportional to token count)
  - Orange squares: tool calls (height proportional to duration)
  - Purple diamonds: agent spawns
  - Red triangles: errors
  - Gray lines: context compactions
- Hovering shows a tooltip with event details
- Clicking scrolls to that event in the conversation view below
- Below the timeline: summary stats (duration, messages, tool calls, tokens, compactions)

Implementation: Use a `<canvas>` element with Chart.js scatter plot (x=time offset, y=event type lane) or a custom SVG/HTML approach using positioned divs. The custom HTML approach is simpler and more accessible.

## 6. Git Integration

### Purpose

Link sessions to git commits made during the session, showing what code changes resulted from each Claude interaction.

### 6.1 New `session_commits` Table

```sql
CREATE TABLE IF NOT EXISTS session_commits (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    commit_hash TEXT NOT NULL,
    commit_message TEXT,
    author TEXT,
    files_changed INTEGER,
    insertions INTEGER,
    deletions INTEGER,
    committed_at DATETIME NOT NULL,
    FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_session_commits_session ON session_commits(session_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_session_commits_hash ON session_commits(commit_hash);
```

### 6.2 CLI Command

**`claude-monitor git-sync`** - Scan git repos for commits made during tracked sessions.

Algorithm:
1. Query all sessions with a non-empty `cwd`
2. Group sessions by `cwd` (repo root)
3. For each repo, run `git log --format=...` for the time range covering all sessions
4. Match commits to sessions by timestamp overlap (commit time falls between session start and end)
5. Insert matched commits into `session_commits`

Flags:
- `--repo <path>` - Sync a specific repo only
- `--since <date>` - Only look at commits after this date

### 6.3 Backend API

**New API endpoint:** `GET /api/sessions/{id}/commits`

Response: Array of commits with hash, message, files_changed, insertions, deletions, committed_at.

### 6.4 Frontend

Add a "Git Commits" section to **Sessions** detail view:

- Collapsible card showing commits made during the session
- Each commit row: short hash (monospace), message, `+N/-M` diff stat, timestamp
- "No commits" message if none found
- Small "Sync Git" button that triggers a POST to `/api/git-sync` (runs git-sync for the session's repo)

## 7. Cost Budgets & Alerts

### Purpose

Set spending limits per project/time period and get visual warnings when approaching or exceeding them.

### 7.1 New `budgets` Table

```sql
CREATE TABLE IF NOT EXISTS budgets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    project_path TEXT,
    period TEXT NOT NULL,
    amount_usd REAL NOT NULL,
    enabled BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_budgets_project ON budgets(project_path);
```

`period` values: `daily`, `weekly`, `monthly`

`project_path` NULL means global budget.

### 7.2 Backend API

- `GET /api/budgets` - List all budgets with current spend
- `POST /api/budgets` - Create a budget
- `PUT /api/budgets/{id}` - Update a budget
- `DELETE /api/budgets/{id}` - Delete a budget
- `GET /api/budgets/status` - Current spend vs. all active budgets

Budget status response:

```json
{
    "budgets": [
        {
            "id": 1,
            "name": "Daily Global",
            "project_path": null,
            "period": "daily",
            "amount_usd": 50.00,
            "current_spend": 32.50,
            "percentage": 65,
            "status": "ok"
        }
    ]
}
```

`status` values: `ok` (< 80%), `warning` (80-100%), `exceeded` (> 100%)

### 7.3 Frontend

**Dashboard** (`dashboard.js`):
- Budget status badges in the stats row: green/yellow/red based on status
- If any budget is in `warning` or `exceeded`, show a banner at the top

**Cost** page (`cost.js`):
- "Budgets" section with a table of all budgets, current spend, progress bars
- "Add Budget" button opens a form: name, project (dropdown or "All Projects"), period, amount

**Settings** page (`settings.js`):
- Budget management section (same as Cost page, alternative access point)

## 8. Prompt Pattern Analysis

### Purpose

Classify user prompts to reveal usage patterns: what kinds of tasks are most common, which task types cost the most.

### 8.1 Classification Approach

Keyword-based classification in Go. No ML needed. Categories:

| Category | Keywords |
|----------|----------|
| Bug Fix | fix, bug, error, broken, crash, failing, debug |
| Feature | add, implement, create, build, new feature, scaffold |
| Refactor | refactor, rename, extract, move, reorganize, clean up |
| Explain | explain, what does, how does, why, understand, walk me through |
| Test | test, spec, coverage, assert, mock, fixture |
| Config | config, setup, install, deploy, CI, docker, env |
| Review | review, check, audit, look at, feedback |
| Docs | document, readme, comment, docstring, jsdoc |

A prompt can match multiple categories. Classification is done at query time (no new table needed), scanning `messages.content_text` WHERE `type = 'user'`.

### 8.2 Backend API

**New API endpoint:** `GET /api/stats/prompt-patterns`

Query params: `project` (optional), `from`/`to` (optional)

Response:

```json
{
    "categories": [
        {"name": "Bug Fix", "count": 120, "percentage": 28.5, "avg_cost": 0.85, "avg_tokens": 45000},
        {"name": "Feature", "count": 95, "percentage": 22.6, "avg_cost": 1.20, "avg_tokens": 62000}
    ],
    "trends": [
        {"date": "2026-04-10", "Bug Fix": 5, "Feature": 3, "Refactor": 2}
    ]
}
```

### 8.3 Frontend

Add a "Usage Patterns" section to the **Dashboard** (`dashboard.js`):

- **Category donut chart** - showing distribution of prompt types
- **Category table** - name, count, percentage, avg cost, avg tokens
- **Trend stacked area chart** - daily category counts over last 30 days

## 9. Session Notes & Tags

### Purpose

Allow users to annotate sessions with free-text notes and tags for organization and filtering.

### 9.1 Schema Changes

Add columns to existing `sessions` table:

```sql
ALTER TABLE sessions ADD COLUMN notes TEXT DEFAULT '';
ALTER TABLE sessions ADD COLUMN tags TEXT DEFAULT '';
```

`tags` is stored as a comma-separated string (e.g., `"productive,refactor,frontend"`). This avoids a junction table for a simple feature.

### 9.2 Backend API

**New API endpoint:** `PATCH /api/sessions/{id}`

Request body:

```json
{
    "notes": "This session was particularly productive. Refactored the entire auth module.",
    "tags": "productive,refactor,auth"
}
```

**Existing endpoint changes:**
- `GET /api/sessions` gains `tag` query param for filtering: `?tag=productive`
- `GET /api/sessions/{id}` response includes `notes` and `tags` fields
- `GET /api/tags` - new endpoint returning all unique tags with counts

### 9.3 Frontend

**Sessions detail** (`sessions.js`):
- Editable notes textarea below the session header (auto-saves on blur via PATCH)
- Tag input with pill display: click to add, X to remove
- Existing tags shown as autocomplete suggestions

**Sessions list** (`sessions.js`):
- Tag pills displayed in each session row
- Tag filter dropdown in the filter bar
- Tags are clickable to filter

## 10. UI Export

### Purpose

Add a download button to the web UI that exports filtered session data using the existing `internal/exporter` package.

### 10.1 Backend API

**New API endpoint:** `GET /api/export`

Query params:
- `format`: `json`, `csv`, `html` (default `json`)
- `project`: filter by project
- `from`/`to`: date range
- `session_id`: export a single session

Response: File download with appropriate Content-Type and Content-Disposition headers.

- `json` → `application/json`, attachment filename `claude-monitor-export.json`
- `csv` → `application/zip` (zip of 3 CSV files), attachment filename `claude-monitor-export.zip`
- `html` → `text/html`, attachment filename `claude-monitor-report.html`

### 10.2 Frontend

- **Export button** in the top navigation bar (download icon)
- Clicking opens a dropdown/modal with:
  - Format selector: JSON / CSV / HTML
  - Optional filters: project, date range
  - "Download" button
- The download triggers a direct `window.location` navigation to `/api/export?format=...&...`
- Also add an export button per-session in the session detail view (exports just that session)

## Implementation Notes

### Database Migrations

All schema changes are additive (new tables, new columns with defaults). Apply via `ALTER TABLE` and `CREATE TABLE IF NOT EXISTS` in `internal/db/schema.go`. No data loss, no downtime.

Migration order:
1. Add `stderr`, `stdout_preview` columns to tool_calls
2. Add `notes`, `tags` columns to sessions
3. Create `session_metrics` table
4. Create `context_compactions` table
5. Create `session_attachments` table
6. Create `session_commits` table
7. Create `budgets` table

### API Route Registration

All new routes added to `server.go` `setupRoutes()`:

```go
s.mux.HandleFunc("/api/stats/token-efficiency", s.handleTokenEfficiency)
s.mux.HandleFunc("/api/stats/errors", s.handleErrors)
s.mux.HandleFunc("/api/stats/file-heatmap", s.handleFileHeatmap)
s.mux.HandleFunc("/api/stats/prompt-patterns", s.handlePromptPatterns)
s.mux.HandleFunc("/api/sessions/", s.handleSessionDetail) // extend for PATCH, /timeline, /commits
s.mux.HandleFunc("/api/budgets", s.handleBudgets)
s.mux.HandleFunc("/api/budgets/", s.handleBudgetDetail)
s.mux.HandleFunc("/api/budgets/status", s.handleBudgetStatus)
s.mux.HandleFunc("/api/tags", s.handleTags)
s.mux.HandleFunc("/api/export", s.handleExport)
s.mux.HandleFunc("/api/git-sync", s.handleGitSync)
```

### Frontend File Changes

| File | Changes |
|------|---------|
| `api.js` | Add methods for all new endpoints |
| `cost.js` | Add Token Efficiency section, Budget section |
| `tools.js` | Add Error Analysis section |
| `projects.js` | Add File Change Heatmap section |
| `sessions.js` | Add Timeline, Git Commits, Notes/Tags sections; extend list for tag filtering |
| `dashboard.js` | Add Prompt Patterns section, Budget status badges |
| `settings.js` | Add Budget management section |
| `app.js` | Add export button to nav bar |

### Testing Strategy

- **Unit tests** for each new API handler (mock DB, verify JSON responses)
- **Integration tests** for git-sync command (test repo with known commits)
- **Parser tests** for new JSONL fields (sample entries with speed, server_tool_use, compactMetadata)
- **Import tests** for new data capture (verify new tables populated after import)
- **Manual UI testing** via dev server for all new visualizations

### Dependencies

No new Go dependencies. All features use:
- `database/sql` + SQLite driver (existing)
- `os/exec` for git commands (git-sync only)
- `encoding/json` for API (existing)
- `archive/zip` for CSV export (stdlib)
- Chart.js (existing, already embedded) for frontend charts

### Performance Considerations

- File heatmap query parses JSON from `tool_input` — add a computed `file_path` column to `tool_calls` if this becomes slow on large datasets
- Prompt pattern classification scans content_text — for large datasets, consider caching results in a `prompt_categories` table
- Git-sync shells out to `git log` — safe since it only reads, and repos are local
- Budget status is computed on each request — cheap (single aggregate query per budget)

### Feature Independence

Features are independent and can be implemented in any order. The only dependency is:

1. **Expanded Data Capture** (Section 1) must come first — several features depend on the new tables and fields
2. All other features (Sections 2-10) are independent of each other

Recommended implementation order:
1. Expanded Data Capture (foundation)
2. Session Notes & Tags (small schema change, high value)
3. Error Analysis (uses existing data)
4. Token Efficiency Dashboard (uses existing + new data)
5. UI Export (wires existing code)
6. Session Timeline (uses existing + new data)
7. Prompt Pattern Analysis (query-time, no schema)
8. File Change Heatmap (JSON parsing)
9. Cost Budgets & Alerts (new CRUD)
10. Git Integration (external dependency on git)
