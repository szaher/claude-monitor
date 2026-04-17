---
layout: default
title: API Reference
nav_order: 5
---

# API Reference

All endpoints return JSON. The base URL is `http://localhost:3000` (configurable).

## Sessions

### List Sessions

```
GET /api/sessions
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `project` | string | Filter by project path |
| `from` | string | Start date (RFC3339 or YYYY-MM-DD) |
| `to` | string | End date |
| `model` | string | Filter by model name |
| `search` | string | Full-text search query |
| `limit` | int | Results per page (default: 50) |
| `offset` | int | Pagination offset |

### Get Session Detail

```
GET /api/sessions/{id}
```

Returns session metadata, messages, tool calls, and subagents.

### Update Session

```
PATCH /api/sessions/{id}
```

```json
{
  "notes": "Session notes text",
  "tags": ["tag1", "tag2"]
}
```

### Session Timeline

```
GET /api/sessions/{id}/timeline
```

Returns a unified timeline of messages, tool calls, subagents, and context compactions with duration and total tokens.

### Session Commits

```
GET /api/sessions/{id}/commits
```

Returns git commits linked to this session.

---

## Messages

```
GET /api/messages
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `session_id` | string | Filter by session |
| `type` | string | Filter by type (user, assistant) |
| `limit` | int | Results per page |
| `offset` | int | Pagination offset |

---

## Tool Calls

```
GET /api/tool-calls
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `session_id` | string | Filter by session |
| `tool_name` | string | Filter by tool name |
| `limit` | int | Results per page |
| `offset` | int | Pagination offset |

---

## Subagents

```
GET /api/subagents
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `session_id` | string | Filter by session |
| `limit` | int | Results per page |
| `offset` | int | Pagination offset |

---

## Statistics

### Aggregate Stats

```
GET /api/stats
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `project` | string | Filter by project |
| `from` | string | Start date |
| `to` | string | End date |

Returns today/filtered stats, total stats, and active session count.

### Daily Stats

```
GET /api/stats/daily
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `days` | int | 90 | Number of days |

### Tool Stats

```
GET /api/stats/tools
```

### Model Stats

```
GET /api/stats/models
```

### Project Stats

```
GET /api/stats/projects
```

### Skill Stats

```
GET /api/stats/skills
```

### MCP Server Stats

```
GET /api/stats/mcp
```

### Error Analysis

```
GET /api/stats/errors
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `project` | string | Filter by project |
| `from` | string | Start date |
| `to` | string | End date |

Returns total errors, error rate, errors by tool, common error patterns, and 30-day error trend.

### Token Efficiency

```
GET /api/stats/token-efficiency
```

Returns cache hit rate, average tokens per message/tool call, cache savings, and efficiency by model.

### Prompt Patterns

```
GET /api/stats/prompt-patterns
```

Returns prompt category distribution and daily trends.

### File Heatmap

```
GET /api/stats/file-heatmap
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `project` | string | **Required.** Project path |

Returns top 50 files and top 20 directories with read/write/edit counts.

### Session Breakdown

```
GET /api/stats/session-breakdown
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `session_id` | string | Session ID |

### Project Breakdown

```
GET /api/stats/project-breakdown
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `project` | string | Project path |

---

## Budgets

### List Budgets

```
GET /api/budgets
```

### Create Budget

```
POST /api/budgets
```

```json
{
  "name": "Daily Limit",
  "amount": 50.00,
  "period": "daily",
  "warning_threshold": 0.8,
  "project": ""
}
```

`period` is one of: `daily`, `weekly`, `monthly`.

### Update Budget

```
PUT /api/budgets/{id}
```

Same body as create.

### Delete Budget

```
DELETE /api/budgets/{id}
```

### Budget Status

```
GET /api/budgets/status
```

Returns all budgets with current spend, percentage used, and status (ok/warning/exceeded).

---

## Tags

```
GET /api/tags
```

Returns all tags with usage counts.

---

## Git Sync

```
POST /api/git-sync
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `session_id` | string | Sync a specific session's repo |

Triggers async git commit sync. Returns immediately with `{"status": "syncing"}`.

---

## Search

```
GET /api/search
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `q` | string | **Required.** Search query |
| `limit` | int | Max results (default: 50) |

Full-text search across message content using SQLite FTS5.

---

## Export

```
GET /api/export
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `format` | string | `json`, `csv`, or `html` |
| `project` | string | Filter by project |
| `from` | string | Start date |
| `to` | string | End date |
| `session_id` | string | Export a single session |

Returns file download (JSON, ZIP for CSV, or HTML report).

---

## Configuration

### Get Config

```
GET /api/config
```

### Save Config

```
POST /api/config
```

---

## WebSocket

```
ws://localhost:3000/ws
```

Real-time event stream. Events are JSON objects with a `hook_event_name` field:

- `SessionStart` -- New session started
- `SessionEnd` / `Stop` -- Session ended
- `PreToolUse` -- Tool about to execute
- `PostToolUse` -- Tool execution completed
- `SubagentStart` -- Agent spawned
- `SubagentStop` -- Agent completed
- `Notification` -- System notification
- `TaskStart` / `TaskComplete` / `TaskUpdate` -- Task lifecycle events
