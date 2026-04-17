---
layout: default
title: Dashboard & UI
---

# Web Dashboard

The web UI is a single-page application served by the Go binary at `http://localhost:3000`. It provides eight pages accessible from the sidebar.

## Dashboard

The main overview page showing:

- **Stat cards** -- Sessions, tool calls, tokens, and cost for today (or filtered date range)
- **Active sessions banner** -- Shows currently running sessions with a link to the live view
- **Activity heatmap** -- GitHub-style contribution heatmap of daily session activity
- **Token usage chart** -- 30-day trend of input/output tokens
- **Top tools** -- Most frequently used tools
- **Top projects** -- Most active projects by session count
- **Recent sessions** -- Latest sessions with duration and token counts
- **Prompt patterns** -- Donut chart categorizing prompts (bug fix, feature, refactor, etc.)
- **Budget warnings** -- Alerts when spending approaches or exceeds budget limits

All stat cards respond to the global date and project filters in the top bar.

## Sessions

Browse and search all captured sessions.

- **List view** with sortable columns: project, duration, tokens, cost, model
- **Detail view** with:
  - Session metadata (model, tokens, cost, duration)
  - Notes and tags (editable, auto-saved)
  - Session timeline with color-coded events
  - Conversation thread (user messages, assistant responses, tool calls)
  - Git commits linked to the session (with a "Sync Git" button)

## Live

Real-time activity feed powered by WebSocket.

- Color-coded event badges: session start/end, tool calls, agent spawns, notifications, tasks
- Pause/resume and clear controls
- Auto-reconnect on disconnect

The sidebar "Live" link shows a pulsing green indicator and badge count when sessions are active.

## Tools

Tool usage analytics.

- **Stats cards** -- Total tools, unique tools, average calls per session, success rate
- **Usage chart** -- Bar chart of top tools by call count
- **Success rate chart** -- Horizontal bar chart of tool success rates
- **Tool calls table** -- Searchable, paginated list of all tool calls
- **Error analysis** -- Error rate by tool, error trend chart, common error patterns

## Agents

Subagent analytics.

- **Stats cards** -- Total agents, unique types, average duration
- **Type distribution** -- Donut chart of agent types
- **Spawn frequency** -- Daily agent spawn bar chart
- **Agent tasks table** -- Searchable list with type, description, duration

## Projects

Project-level analytics.

- **Project list** -- All projects with session counts and total cost
- **Project detail** with:
  - Session breakdown (tools, skills, MCP servers, agents)
  - File activity heatmap (most read, written, edited files)
  - Session history for the project

## Cost

Cost tracking and budgeting.

- **Cost overview** -- Total cost, average per session, daily spend chart
- **Model costs** -- Breakdown by model with per-token pricing
- **Token efficiency** -- Cache hit rate, average tokens per message/tool call, cache savings in USD, efficiency by model
- **Budgets** -- Create daily/weekly/monthly budgets with warning thresholds, progress bars showing current spend

## Settings

- Theme selection (auto, light, dark)
- Model pricing configuration
- Data retention settings

[Back to Home](.)
