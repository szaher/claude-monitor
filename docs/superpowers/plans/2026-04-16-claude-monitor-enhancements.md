# Claude Monitor Enhancements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add 9 features (token efficiency, error analysis, file heatmap, session timeline, git integration, cost budgets, prompt patterns, session notes/tags, UI export) and expand data capture from Claude Code JSONL logs.

**Architecture:** Each feature follows the same vertical-slice pattern: schema migration (if needed) → Go API handler → JS API client method → frontend component update. All schema changes are additive (new tables or ALTER TABLE with defaults). API handlers follow the existing pattern in `internal/server/api.go`: method guard → parse params → SQL query → `writeJSON()`. Frontend components follow the existing pattern: object with `render(container)` and `loadData()` methods using Chart.js for visualization.

**Tech Stack:** Go 1.21+ (stdlib only, no frameworks), SQLite3 via `github.com/mattn/go-sqlite3`, vanilla JavaScript, Chart.js (embedded), CSS.

**Spec:** `docs/superpowers/specs/2026-04-16-claude-monitor-enhancements-design.md`

---

## File Map

### New Files
- `internal/server/api_errors.go` — Error analysis API handler
- `internal/server/api_efficiency.go` — Token efficiency API handler
- `internal/server/api_heatmap.go` — File change heatmap API handler
- `internal/server/api_timeline.go` — Session timeline API handler
- `internal/server/api_patterns.go` — Prompt pattern analysis API handler
- `internal/server/api_budgets.go` — Budget CRUD API handlers
- `internal/server/api_tags.go` — Session notes/tags and tags list API handlers
- `internal/server/api_export.go` — Export download API handler
- `internal/server/api_gitsync.go` — Git sync API handler
- `internal/cli/gitsync.go` — Git sync CLI command
- `internal/server/api_errors_test.go` — Error analysis handler tests
- `internal/server/api_budgets_test.go` — Budget handler tests
- `internal/server/api_tags_test.go` — Tags handler tests

### Modified Files
- `internal/db/schema.go` — Add new tables and ALTER TABLE migrations
- `internal/parser/jsonl.go` — Add `Speed`, `ServerToolUse`, `StopReason`, `ServiceTier`, `CacheCreation` fields
- `internal/models/models.go` — Add `Notes`, `Tags`, `Stderr`, `StdoutPreview` fields to models
- `internal/ingestion/pipeline.go` — Extract new fields, insert into new tables, populate `stop_reason`/`duration_ms`/`success` on tool_calls
- `internal/cli/importcmd.go` — Extract new fields during import (same as pipeline)
- `internal/server/server.go` — Register new API routes
- `internal/db/queries.go` — Add `InsertSessionMetric`, `InsertContextCompaction`, `InsertSessionAttachment`, `InsertSessionCommit`, `UpdateSessionNotesTags` functions
- `web/static/js/api.js` — Add API client methods for all new endpoints
- `web/static/js/app.js` — Add export button to nav
- `web/static/js/components/tools.js` — Add Error Analysis section
- `web/static/js/components/cost.js` — Add Token Efficiency section, Budget section
- `web/static/js/components/projects.js` — Add File Change Heatmap section
- `web/static/js/components/sessions.js` — Add Timeline, Git Commits, Notes/Tags sections; tag filtering
- `web/static/js/components/dashboard.js` — Add Prompt Patterns section, Budget status badges
- `web/static/js/components/settings.js` — Add Budget management section
- `cmd/claude-monitor/main.go` — Register `git-sync` CLI command

---

## Task 1: Schema Migrations

**Files:**
- Modify: `internal/db/schema.go`
- Test: `internal/db/db_test.go`

- [ ] **Step 1: Add migration SQL to schema.go**

Append the migration SQL after the existing `schema` constant. Use a separate `migrations` constant that runs after schema creation.

```go
// Add to internal/db/schema.go after the existing schema constant:

const migrations = `
-- Migration 1: Add stderr/stdout_preview to tool_calls
ALTER TABLE tool_calls ADD COLUMN stderr TEXT DEFAULT '';
ALTER TABLE tool_calls ADD COLUMN stdout_preview TEXT DEFAULT '';

-- Migration 2: Add notes/tags to sessions
ALTER TABLE sessions ADD COLUMN notes TEXT DEFAULT '';
ALTER TABLE sessions ADD COLUMN tags TEXT DEFAULT '';

-- Migration 3: session_metrics table
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

-- Migration 4: context_compactions table
CREATE TABLE IF NOT EXISTS context_compactions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    pre_tokens INTEGER NOT NULL,
    post_tokens INTEGER NOT NULL,
    trigger_reason TEXT,
    duration_ms INTEGER,
    timestamp DATETIME NOT NULL,
    FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_context_compactions_session ON context_compactions(session_id);

-- Migration 5: session_attachments table
CREATE TABLE IF NOT EXISTS session_attachments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    attachment_type TEXT NOT NULL,
    content TEXT,
    timestamp DATETIME NOT NULL,
    FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_session_attachments_session_type ON session_attachments(session_id, attachment_type);

-- Migration 6: session_commits table
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

-- Migration 7: budgets table
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
`
```

- [ ] **Step 2: Update InitDB to run migrations**

In `internal/db/db.go`, after the existing `db.Exec(schema)` call, add migration execution that silently ignores "duplicate column" errors (for idempotent re-runs):

```go
// Add to InitDB function after schema execution:

// Run migrations (idempotent — ALTER TABLE errors are ignored)
for _, stmt := range strings.Split(migrations, ";") {
    stmt = strings.TrimSpace(stmt)
    if stmt == "" || strings.HasPrefix(stmt, "--") {
        continue
    }
    _, err := database.Exec(stmt)
    if err != nil && !strings.Contains(err.Error(), "duplicate column") {
        // Only ignore duplicate-column errors (re-running ALTER TABLE)
        if !strings.Contains(stmt, "ALTER TABLE") {
            return nil, fmt.Errorf("migration: %w", err)
        }
    }
}
```

- [ ] **Step 3: Write test to verify new tables exist**

Add to `internal/db/db_test.go`:

```go
func TestMigrations(t *testing.T) {
    dir := t.TempDir()
    database, err := InitDB(filepath.Join(dir, "test.db"))
    if err != nil {
        t.Fatalf("InitDB: %v", err)
    }
    defer database.Close()

    tables := []string{
        "session_metrics", "context_compactions", "session_attachments",
        "session_commits", "budgets",
    }
    for _, table := range tables {
        var name string
        err := database.QueryRow(
            "SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
        ).Scan(&name)
        if err != nil {
            t.Errorf("table %s not found: %v", table, err)
        }
    }

    // Verify new columns on tool_calls
    var count int
    err = database.QueryRow(
        "SELECT COUNT(*) FROM pragma_table_info('tool_calls') WHERE name IN ('stderr','stdout_preview')",
    ).Scan(&count)
    if err != nil || count != 2 {
        t.Errorf("tool_calls missing stderr/stdout_preview columns: count=%d err=%v", count, err)
    }

    // Verify new columns on sessions
    err = database.QueryRow(
        "SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name IN ('notes','tags')",
    ).Scan(&count)
    if err != nil || count != 2 {
        t.Errorf("sessions missing notes/tags columns: count=%d err=%v", count, err)
    }
}
```

- [ ] **Step 4: Run test**

Run: `cd /Users/szaher/go/src/github.com/szaher/claude-monitor && go test ./internal/db/ -run TestMigrations -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/db/schema.go internal/db/db.go internal/db/db_test.go
git commit -m "feat: add schema migrations for new tables and columns"
```

---

## Task 2: Extend Parser and Models for New Fields

**Files:**
- Modify: `internal/parser/jsonl.go`
- Modify: `internal/models/models.go`
- Modify: `internal/parser/jsonl_test.go`

- [ ] **Step 1: Add new struct fields to parser**

In `internal/parser/jsonl.go`, add to `LogEntry`:

```go
type LogEntry struct {
    // ... existing fields (keep all) ...

    // New fields for expanded capture
    Speed         string         `json:"speed,omitempty"`
    ServerToolUse *ServerToolUse `json:"server_tool_use,omitempty"`
}

// ServerToolUse tracks web search and fetch counts per message.
type ServerToolUse struct {
    WebSearchCount int `json:"web_search_count"`
    WebFetchCount  int `json:"web_fetch_count"`
}
```

Add `StopReason` and `ServiceTier` to `MessageData`:

```go
type MessageData struct {
    Role       string      `json:"role"`
    Content    interface{} `json:"content"`
    Model      string      `json:"model"`
    Usage      Usage       `json:"usage"`
    StopReason string      `json:"stop_reason"`
}
```

Extend `Usage` with cache creation detail:

```go
type Usage struct {
    InputTokens              int                  `json:"input_tokens"`
    OutputTokens             int                  `json:"output_tokens"`
    CacheReadInputTokens     int                  `json:"cache_read_input_tokens"`
    CacheCreationInputTokens int                  `json:"cache_creation_input_tokens"`
    ServiceTier              string               `json:"service_tier,omitempty"`
    CacheCreation            *CacheCreationDetail `json:"cache_creation,omitempty"`
}

type CacheCreationDetail struct {
    Ephemeral5mTokens int `json:"ephemeral_5m_input_tokens"`
    Ephemeral1hTokens int `json:"ephemeral_1h_input_tokens"`
}
```

- [ ] **Step 2: Add new fields to models**

In `internal/models/models.go`, extend:

```go
type Session struct {
    // ... existing fields ...
    Notes string `json:"notes"`
    Tags  string `json:"tags"`
}

type ToolCall struct {
    // ... existing fields ...
    Stderr       string `json:"stderr"`
    StdoutPreview string `json:"stdout_preview"`
}

type SessionMetric struct {
    ID                     int       `json:"id"`
    SessionID              string    `json:"session_id"`
    MessageID              string    `json:"message_id"`
    Speed                  string    `json:"speed"`
    ServiceTier            string    `json:"service_tier"`
    InferenceGeo           string    `json:"inference_geo"`
    CacheEphemeral5mTokens int       `json:"cache_ephemeral_5m_tokens"`
    CacheEphemeral1hTokens int       `json:"cache_ephemeral_1h_tokens"`
    Timestamp              time.Time `json:"timestamp"`
}

type ContextCompaction struct {
    ID            int       `json:"id"`
    SessionID     string    `json:"session_id"`
    PreTokens     int       `json:"pre_tokens"`
    PostTokens    int       `json:"post_tokens"`
    TriggerReason string    `json:"trigger_reason"`
    DurationMS    int       `json:"duration_ms"`
    Timestamp     time.Time `json:"timestamp"`
}

type SessionAttachment struct {
    ID             int       `json:"id"`
    SessionID      string    `json:"session_id"`
    AttachmentType string    `json:"attachment_type"`
    Content        string    `json:"content"`
    Timestamp      time.Time `json:"timestamp"`
}

type SessionCommit struct {
    ID            int       `json:"id"`
    SessionID     string    `json:"session_id"`
    CommitHash    string    `json:"commit_hash"`
    CommitMessage string    `json:"commit_message"`
    Author        string    `json:"author"`
    FilesChanged  int       `json:"files_changed"`
    Insertions    int       `json:"insertions"`
    Deletions     int       `json:"deletions"`
    CommittedAt   time.Time `json:"committed_at"`
}

type Budget struct {
    ID          int       `json:"id"`
    Name        string    `json:"name"`
    ProjectPath string    `json:"project_path"`
    Period      string    `json:"period"`
    AmountUSD   float64   `json:"amount_usd"`
    Enabled     bool      `json:"enabled"`
    CreatedAt   time.Time `json:"created_at"`
}
```

- [ ] **Step 3: Add parser test for new fields**

In `internal/parser/jsonl_test.go`:

```go
func TestParseLogEntry_NewFields(t *testing.T) {
    raw := `{"type":"assistant","uuid":"abc","timestamp":"2026-04-16T10:00:00Z","sessionId":"s1","speed":"fast","server_tool_use":{"web_search_count":2,"web_fetch_count":1},"message":{"role":"assistant","content":"hello","model":"claude-opus-4-6","usage":{"input_tokens":100,"output_tokens":50,"service_tier":"standard","cache_creation":{"ephemeral_5m_input_tokens":500,"ephemeral_1h_input_tokens":0}},"stop_reason":"end_turn"}}`

    entry, err := ParseLogEntry([]byte(raw))
    if err != nil {
        t.Fatalf("ParseLogEntry: %v", err)
    }

    if entry.Speed != "fast" {
        t.Errorf("Speed = %q, want %q", entry.Speed, "fast")
    }
    if entry.ServerToolUse == nil || entry.ServerToolUse.WebSearchCount != 2 {
        t.Errorf("ServerToolUse.WebSearchCount = %v, want 2", entry.ServerToolUse)
    }
    if entry.Message.StopReason != "end_turn" {
        t.Errorf("StopReason = %q, want %q", entry.Message.StopReason, "end_turn")
    }
    if entry.Message.Usage.ServiceTier != "standard" {
        t.Errorf("ServiceTier = %q, want %q", entry.Message.Usage.ServiceTier, "standard")
    }
    if entry.Message.Usage.CacheCreation == nil || entry.Message.Usage.CacheCreation.Ephemeral5mTokens != 500 {
        t.Errorf("CacheCreation.Ephemeral5mTokens = %v, want 500", entry.Message.Usage.CacheCreation)
    }
}
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/szaher/go/src/github.com/szaher/claude-monitor && go test ./internal/parser/ -run TestParseLogEntry_NewFields -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/parser/jsonl.go internal/parser/jsonl_test.go internal/models/models.go
git commit -m "feat: extend parser and models for expanded data capture"
```

---

## Task 3: DB Query Functions for New Tables

**Files:**
- Modify: `internal/db/queries.go`
- Modify: `internal/db/db_test.go`

- [ ] **Step 1: Add insert functions for new tables**

Add to `internal/db/queries.go`:

```go
// InsertSessionMetric inserts a session metric record.
func InsertSessionMetric(db *sql.DB, m *models.SessionMetric) error {
    _, err := db.Exec(`
        INSERT INTO session_metrics (
            session_id, message_id, speed, service_tier, inference_geo,
            cache_ephemeral_5m_tokens, cache_ephemeral_1h_tokens, timestamp
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `,
        m.SessionID, m.MessageID, m.Speed, m.ServiceTier, m.InferenceGeo,
        m.CacheEphemeral5mTokens, m.CacheEphemeral1hTokens,
        m.Timestamp.UTC().Format(time.RFC3339),
    )
    if err != nil {
        return fmt.Errorf("insert session metric: %w", err)
    }
    return nil
}

// InsertContextCompaction inserts a context compaction record.
func InsertContextCompaction(db *sql.DB, c *models.ContextCompaction) error {
    _, err := db.Exec(`
        INSERT INTO context_compactions (
            session_id, pre_tokens, post_tokens, trigger_reason, duration_ms, timestamp
        ) VALUES (?, ?, ?, ?, ?, ?)
    `,
        c.SessionID, c.PreTokens, c.PostTokens, c.TriggerReason, c.DurationMS,
        c.Timestamp.UTC().Format(time.RFC3339),
    )
    if err != nil {
        return fmt.Errorf("insert context compaction: %w", err)
    }
    return nil
}

// InsertSessionAttachment inserts a session attachment record.
func InsertSessionAttachment(db *sql.DB, a *models.SessionAttachment) error {
    _, err := db.Exec(`
        INSERT INTO session_attachments (
            session_id, attachment_type, content, timestamp
        ) VALUES (?, ?, ?, ?)
    `,
        a.SessionID, a.AttachmentType, a.Content,
        a.Timestamp.UTC().Format(time.RFC3339),
    )
    if err != nil {
        return fmt.Errorf("insert session attachment: %w", err)
    }
    return nil
}

// InsertSessionCommit inserts a git commit linked to a session. Duplicates (same hash) are ignored.
func InsertSessionCommit(db *sql.DB, c *models.SessionCommit) error {
    _, err := db.Exec(`
        INSERT INTO session_commits (
            session_id, commit_hash, commit_message, author,
            files_changed, insertions, deletions, committed_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(commit_hash) DO NOTHING
    `,
        c.SessionID, c.CommitHash, c.CommitMessage, c.Author,
        c.FilesChanged, c.Insertions, c.Deletions,
        c.CommittedAt.UTC().Format(time.RFC3339),
    )
    if err != nil {
        return fmt.Errorf("insert session commit: %w", err)
    }
    return nil
}

// UpdateSessionNotesTags updates the notes and tags for a session.
func UpdateSessionNotesTags(db *sql.DB, sessionID, notes, tags string) error {
    _, err := db.Exec(
        "UPDATE sessions SET notes = ?, tags = ? WHERE id = ?",
        notes, tags, sessionID,
    )
    if err != nil {
        return fmt.Errorf("update session notes/tags: %w", err)
    }
    return nil
}

// InsertBudget inserts a new budget and returns its ID.
func InsertBudget(db *sql.DB, b *models.Budget) (int64, error) {
    result, err := db.Exec(`
        INSERT INTO budgets (name, project_path, period, amount_usd, enabled)
        VALUES (?, ?, ?, ?, ?)
    `, b.Name, b.ProjectPath, b.Period, b.AmountUSD, b.Enabled)
    if err != nil {
        return 0, fmt.Errorf("insert budget: %w", err)
    }
    return result.LastInsertId()
}

// UpdateBudget updates an existing budget.
func UpdateBudget(db *sql.DB, b *models.Budget) error {
    _, err := db.Exec(`
        UPDATE budgets SET name = ?, project_path = ?, period = ?, amount_usd = ?, enabled = ?
        WHERE id = ?
    `, b.Name, b.ProjectPath, b.Period, b.AmountUSD, b.Enabled, b.ID)
    if err != nil {
        return fmt.Errorf("update budget: %w", err)
    }
    return nil
}

// DeleteBudget deletes a budget by ID.
func DeleteBudget(db *sql.DB, id int) error {
    _, err := db.Exec("DELETE FROM budgets WHERE id = ?", id)
    if err != nil {
        return fmt.Errorf("delete budget: %w", err)
    }
    return nil
}

// UpdateToolCallResult updates tool_calls with result data (duration, success, stderr, stdout_preview).
func UpdateToolCallResult(db *sql.DB, toolUseID string, durationMS int, success bool, stderr, stdoutPreview string) error {
    _, err := db.Exec(`
        UPDATE tool_calls SET duration_ms = ?, success = ?, stderr = ?, stdout_preview = ?
        WHERE id = ?
    `, durationMS, success, stderr, stdoutPreview, toolUseID)
    if err != nil {
        return fmt.Errorf("update tool call result: %w", err)
    }
    return nil
}
```

- [ ] **Step 2: Write tests for new query functions**

Add to `internal/db/db_test.go`:

```go
func TestInsertSessionMetric(t *testing.T) {
    dir := t.TempDir()
    database, err := InitDB(filepath.Join(dir, "test.db"))
    if err != nil {
        t.Fatalf("InitDB: %v", err)
    }
    defer database.Close()

    // Insert prerequisite session and message
    now := time.Now().UTC()
    InsertSession(database, &models.Session{
        ID: "s1", ProjectPath: "/test", ProjectName: "test", StartedAt: now,
    })
    InsertMessage(database, &models.Message{
        ID: "m1", SessionID: "s1", Type: "assistant", Timestamp: now,
    })

    m := &models.SessionMetric{
        SessionID: "s1", MessageID: "m1", Speed: "fast",
        ServiceTier: "standard", CacheEphemeral5mTokens: 500,
        Timestamp: now,
    }
    if err := InsertSessionMetric(database, m); err != nil {
        t.Fatalf("InsertSessionMetric: %v", err)
    }

    var speed string
    database.QueryRow("SELECT speed FROM session_metrics WHERE session_id = ?", "s1").Scan(&speed)
    if speed != "fast" {
        t.Errorf("speed = %q, want %q", speed, "fast")
    }
}

func TestUpdateSessionNotesTags(t *testing.T) {
    dir := t.TempDir()
    database, err := InitDB(filepath.Join(dir, "test.db"))
    if err != nil {
        t.Fatalf("InitDB: %v", err)
    }
    defer database.Close()

    now := time.Now().UTC()
    InsertSession(database, &models.Session{
        ID: "s1", ProjectPath: "/test", ProjectName: "test", StartedAt: now,
    })

    if err := UpdateSessionNotesTags(database, "s1", "my note", "tag1,tag2"); err != nil {
        t.Fatalf("UpdateSessionNotesTags: %v", err)
    }

    var notes, tags string
    database.QueryRow("SELECT notes, tags FROM sessions WHERE id = ?", "s1").Scan(&notes, &tags)
    if notes != "my note" || tags != "tag1,tag2" {
        t.Errorf("notes=%q tags=%q, want 'my note' 'tag1,tag2'", notes, tags)
    }
}

func TestBudgetCRUD(t *testing.T) {
    dir := t.TempDir()
    database, err := InitDB(filepath.Join(dir, "test.db"))
    if err != nil {
        t.Fatalf("InitDB: %v", err)
    }
    defer database.Close()

    b := &models.Budget{
        Name: "Daily", Period: "daily", AmountUSD: 50.0, Enabled: true,
    }
    id, err := InsertBudget(database, b)
    if err != nil {
        t.Fatalf("InsertBudget: %v", err)
    }
    if id == 0 {
        t.Fatal("InsertBudget returned 0 id")
    }

    b.ID = int(id)
    b.AmountUSD = 100.0
    if err := UpdateBudget(database, b); err != nil {
        t.Fatalf("UpdateBudget: %v", err)
    }

    var amount float64
    database.QueryRow("SELECT amount_usd FROM budgets WHERE id = ?", id).Scan(&amount)
    if amount != 100.0 {
        t.Errorf("amount = %f, want 100.0", amount)
    }

    if err := DeleteBudget(database, int(id)); err != nil {
        t.Fatalf("DeleteBudget: %v", err)
    }

    var count int
    database.QueryRow("SELECT COUNT(*) FROM budgets").Scan(&count)
    if count != 0 {
        t.Errorf("budget not deleted: count=%d", count)
    }
}
```

- [ ] **Step 3: Run tests**

Run: `cd /Users/szaher/go/src/github.com/szaher/claude-monitor && go test ./internal/db/ -v`
Expected: All tests PASS

- [ ] **Step 4: Commit**

```bash
git add internal/db/queries.go internal/db/db_test.go
git commit -m "feat: add query functions for new tables"
```

---

## Task 4: Expand Pipeline & Import Data Capture

**Files:**
- Modify: `internal/ingestion/pipeline.go`
- Modify: `internal/cli/importcmd.go`
- Modify: `internal/ingestion/pipeline_test.go`

- [ ] **Step 1: Update pipeline to extract stop_reason and populate it on messages**

In `internal/ingestion/pipeline.go`, in `ProcessLogEntry`, after building the `msg` struct (around line 159), add:

```go
msg := &models.Message{
    // ... existing fields ...
    StopReason:   entry.Message.StopReason, // ADD THIS LINE
    // ...
}
```

- [ ] **Step 2: Add session metrics insertion to pipeline**

After the tool call loop in `ProcessLogEntry` (after line 196), add session metric insertion for assistant messages:

```go
// For assistant messages, also insert session metrics if we have extra metadata
if entry.Type == "assistant" {
    // ... existing tool call extraction code ...

    // Insert session metric for expanded capture
    metric := &models.SessionMetric{
        SessionID:   entry.SessionID,
        MessageID:   entry.UUID,
        Speed:       entry.Speed,
        ServiceTier: entry.Message.Usage.ServiceTier,
        Timestamp:   entry.Timestamp,
    }
    if entry.Message.Usage.CacheCreation != nil {
        metric.CacheEphemeral5mTokens = entry.Message.Usage.CacheCreation.Ephemeral5mTokens
        metric.CacheEphemeral1hTokens = entry.Message.Usage.CacheCreation.Ephemeral1hTokens
    }
    // Best-effort insert (don't fail the pipeline)
    db.InsertSessionMetric(p.database, metric)
}
```

- [ ] **Step 3: Add toolUseResult extraction from user entries**

In `ProcessLogEntry`, for `entry.Type == "user"`, extract tool use results from the content. Add this block before the existing assistant check:

```go
// For user entries, extract toolUseResult blocks to update tool_calls
if entry.Type == "user" {
    p.extractToolResults(entry)
}
```

Then add the helper method:

```go
// extractToolResults parses toolUseResult blocks from user entries and updates tool_calls.
func (p *Pipeline) extractToolResults(entry *parser.LogEntry) {
    arr, ok := entry.Message.Content.([]interface{})
    if !ok {
        return
    }
    for _, item := range arr {
        block, ok := item.(map[string]interface{})
        if !ok {
            continue
        }
        blockType, _ := block["type"].(string)
        if blockType != "tool_result" {
            continue
        }
        toolUseID, _ := block["tool_use_id"].(string)
        if toolUseID == "" {
            continue
        }

        isError, _ := block["is_error"].(bool)
        success := !isError

        // Extract duration, stderr, stdout from nested content
        var durationMS int
        var stderr, stdoutPreview string

        if content, ok := block["content"].([]interface{}); ok {
            for _, c := range content {
                cb, ok := c.(map[string]interface{})
                if !ok {
                    continue
                }
                if dur, ok := cb["durationMs"].(float64); ok {
                    durationMS = int(dur)
                }
                if s, ok := cb["stderr"].(string); ok {
                    stderr = s
                }
                if s, ok := cb["stdout"].(string); ok {
                    if len(s) > 500 {
                        s = s[:500]
                    }
                    stdoutPreview = s
                }
            }
        }

        db.UpdateToolCallResult(p.database, toolUseID, durationMS, success, stderr, stdoutPreview)
    }
}
```

- [ ] **Step 4: Add compactMetadata extraction from system entries**

In `processSystemEntry`, add detection and insertion of context compactions:

```go
func (p *Pipeline) processSystemEntry(entry *parser.LogEntry) error {
    // ... existing code ...

    // Check for compact metadata
    if entry.Subtype == "compact" {
        // Parse raw JSON again to get compactMetadata
        raw := make(map[string]interface{})
        // entry already has DurationMs field
        compaction := &models.ContextCompaction{
            SessionID:     entry.SessionID,
            PreTokens:     0, // Will be populated from raw JSON
            PostTokens:    0,
            TriggerReason: "context_limit",
            DurationMS:    entry.DurationMs,
            Timestamp:     entry.Timestamp,
        }
        db.InsertContextCompaction(p.database, compaction)
    }

    // ... existing message insert code ...
}
```

- [ ] **Step 5: Update import to capture stop_reason**

In `internal/cli/importcmd.go`, in the message creation loop (around line 181), add:

```go
msg := &models.Message{
    // ... existing fields ...
    StopReason:   entry.Message.StopReason, // ADD THIS LINE
    // ...
}
```

- [ ] **Step 6: Update import to extract tool results from user entries**

In the import loop, after inserting a message for user entries, add tool result extraction:

```go
// After message insert, for user entries extract tool results
if entry.Type == "user" {
    if arr, ok := entry.Message.Content.([]interface{}); ok {
        for _, item := range arr {
            block, ok := item.(map[string]interface{})
            if !ok {
                continue
            }
            if blockType, _ := block["type"].(string); blockType != "tool_result" {
                continue
            }
            toolUseID, _ := block["tool_use_id"].(string)
            if toolUseID == "" {
                continue
            }
            isError, _ := block["is_error"].(bool)
            var durationMS int
            var stderr, stdoutPreview string
            if content, ok := block["content"].([]interface{}); ok {
                for _, c := range content {
                    cb, ok := c.(map[string]interface{})
                    if !ok {
                        continue
                    }
                    if dur, ok := cb["durationMs"].(float64); ok {
                        durationMS = int(dur)
                    }
                    if s, ok := cb["stderr"].(string); ok {
                        stderr = s
                    }
                    if s, ok := cb["stdout"].(string); ok {
                        if len(s) > 500 {
                            s = s[:500]
                        }
                        stdoutPreview = s
                    }
                }
            }
            db.UpdateToolCallResult(database, toolUseID, durationMS, !isError, stderr, stdoutPreview)
        }
    }
}
```

- [ ] **Step 7: Write pipeline test for new fields**

Add to `internal/ingestion/pipeline_test.go`:

```go
func TestProcessLogEntry_StopReason(t *testing.T) {
    dir := t.TempDir()
    database := setupTestDB(t, dir)
    defer database.Close()

    pipeline := NewPipeline(database, nil, nil)

    // Insert a session first
    sessionEntry := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-04-16T10:00:00Z","cwd":"/test","message":{"role":"user","content":"hello"}}`
    if err := pipeline.ProcessLogEntry([]byte(sessionEntry)); err != nil {
        t.Fatalf("ProcessLogEntry (user): %v", err)
    }

    // Assistant entry with stop_reason
    assistantEntry := `{"type":"assistant","uuid":"a1","sessionId":"s1","timestamp":"2026-04-16T10:00:01Z","speed":"fast","message":{"role":"assistant","content":"world","model":"claude-opus-4-6","stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5,"service_tier":"standard"}}}`
    if err := pipeline.ProcessLogEntry([]byte(assistantEntry)); err != nil {
        t.Fatalf("ProcessLogEntry (assistant): %v", err)
    }

    var stopReason string
    database.QueryRow("SELECT stop_reason FROM messages WHERE id = 'a1'").Scan(&stopReason)
    if stopReason != "end_turn" {
        t.Errorf("stop_reason = %q, want %q", stopReason, "end_turn")
    }

    var speed string
    err := database.QueryRow("SELECT speed FROM session_metrics WHERE message_id = 'a1'").Scan(&speed)
    if err != nil {
        t.Errorf("session_metrics not found: %v", err)
    }
    if speed != "fast" {
        t.Errorf("speed = %q, want %q", speed, "fast")
    }
}
```

- [ ] **Step 8: Run tests**

Run: `cd /Users/szaher/go/src/github.com/szaher/claude-monitor && go test ./internal/ingestion/ -v`
Expected: All tests PASS

- [ ] **Step 9: Commit**

```bash
git add internal/ingestion/pipeline.go internal/cli/importcmd.go internal/ingestion/pipeline_test.go
git commit -m "feat: capture stop_reason, speed, tool results in pipeline and import"
```

---

## Task 5: Session Notes & Tags (API + Frontend)

**Files:**
- Create: `internal/server/api_tags.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/api.go` (scanSession, handleSessions, handleSessionDetail)
- Modify: `web/static/js/api.js`
- Modify: `web/static/js/components/sessions.js`

- [ ] **Step 1: Create api_tags.go with PATCH handler and tags list**

Create `internal/server/api_tags.go`:

```go
package server

import (
    "encoding/json"
    "io"
    "net/http"
    "strings"

    "github.com/szaher/claude-monitor/internal/db"
)

// handleSessionPatch handles PATCH /api/sessions/{id} — update notes/tags.
func (s *Server) handleSessionPatch(w http.ResponseWriter, r *http.Request) {
    id := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
    if id == "" {
        writeError(w, http.StatusBadRequest, "missing session id")
        return
    }

    body, err := io.ReadAll(r.Body)
    if err != nil {
        writeError(w, http.StatusBadRequest, "read body: "+err.Error())
        return
    }

    var update struct {
        Notes *string `json:"notes"`
        Tags  *string `json:"tags"`
    }
    if err := json.Unmarshal(body, &update); err != nil {
        writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
        return
    }

    // Get current values
    var notes, tags string
    s.db.QueryRow("SELECT COALESCE(notes,''), COALESCE(tags,'') FROM sessions WHERE id = ?", id).Scan(&notes, &tags)

    if update.Notes != nil {
        notes = *update.Notes
    }
    if update.Tags != nil {
        tags = *update.Tags
    }

    if err := db.UpdateSessionNotesTags(s.db, id, notes, tags); err != nil {
        writeError(w, http.StatusInternalServerError, "update: "+err.Error())
        return
    }

    writeJSON(w, http.StatusOK, map[string]interface{}{
        "id":    id,
        "notes": notes,
        "tags":  tags,
    })
}

// handleTags handles GET /api/tags — list all unique tags with counts.
func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeError(w, http.StatusMethodNotAllowed, "method not allowed")
        return
    }

    rows, err := s.db.Query("SELECT tags FROM sessions WHERE tags != '' AND tags IS NOT NULL")
    if err != nil {
        writeError(w, http.StatusInternalServerError, "query tags: "+err.Error())
        return
    }
    defer rows.Close()

    counts := map[string]int{}
    for rows.Next() {
        var tags string
        if err := rows.Scan(&tags); err != nil {
            continue
        }
        for _, tag := range strings.Split(tags, ",") {
            tag = strings.TrimSpace(tag)
            if tag != "" {
                counts[tag]++
            }
        }
    }

    result := []map[string]interface{}{}
    for tag, count := range counts {
        result = append(result, map[string]interface{}{
            "tag":   tag,
            "count": count,
        })
    }

    writeJSON(w, http.StatusOK, map[string]interface{}{
        "tags": result,
    })
}
```

- [ ] **Step 2: Update handleSessionDetail to support PATCH and include notes/tags**

In `internal/server/api.go`, modify `handleSessionDetail` to dispatch on method:

```go
func (s *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        // existing GET logic (keep as-is)
    case http.MethodPatch:
        s.handleSessionPatch(w, r)
        return
    default:
        writeError(w, http.StatusMethodNotAllowed, "method not allowed")
        return
    }
    // ... rest of existing GET handler code ...
}
```

- [ ] **Step 3: Update scanSession to include notes/tags**

In `internal/server/api.go`, modify `scanSession` to scan the two new columns. This requires updating the SELECT queries in `handleSessions` and `handleSessionDetail` to include `notes, tags`.

Add to the query SELECT lists: `, COALESCE(notes,'') as notes, COALESCE(tags,'') as tags`

In `scanSession`, add:

```go
var notes, tags sql.NullString
// ... add to Scan call: &notes, &tags
sess["notes"] = notes.String
sess["tags"]  = tags.String
```

- [ ] **Step 4: Add tag filter to handleSessions**

In `handleSessions`, add tag filtering after the existing filters:

```go
tag := r.URL.Query().Get("tag")
if tag != "" {
    query += " AND (',' || tags || ',' LIKE '%,' || ? || ',%')"
    countQuery += " AND (',' || tags || ',' LIKE '%,' || ? || ',%')"
    args = append(args, tag)
}
```

- [ ] **Step 5: Register routes**

In `internal/server/server.go`, add:

```go
s.mux.HandleFunc("/api/tags", s.handleTags)
```

- [ ] **Step 6: Add API client methods**

In `web/static/js/api.js`, add:

```js
updateSession(id, data) {
    return fetch(`/api/sessions/${id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
    }).then(r => r.json());
},

getTags() {
    return API.get('/api/tags');
},
```

- [ ] **Step 7: Add notes/tags UI to session detail**

In `web/static/js/components/sessions.js`, add a notes/tags section below the session header in the detail view. Add after the metadata section:

```js
// Notes & Tags section
const notesTagsHtml = `
<div class="card" style="margin-bottom:1.5rem">
    <div class="card-header"><h3>Notes & Tags</h3></div>
    <div class="card-body">
        <div style="margin-bottom:1rem">
            <label style="font-weight:600;display:block;margin-bottom:0.25rem">Notes</label>
            <textarea id="session-notes" rows="3" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:4px;background:var(--bg-secondary);color:var(--text)"
                placeholder="Add notes about this session...">${this._esc(session.notes || '')}</textarea>
        </div>
        <div>
            <label style="font-weight:600;display:block;margin-bottom:0.25rem">Tags</label>
            <div id="session-tags-container" style="display:flex;flex-wrap:wrap;gap:0.5rem;align-items:center">
                ${(session.tags || '').split(',').filter(t => t.trim()).map(t =>
                    `<span class="badge badge-info" style="cursor:pointer" data-tag="${this._esc(t.trim())}">${this._esc(t.trim())} &times;</span>`
                ).join('')}
                <input id="session-tag-input" type="text" placeholder="Add tag..."
                    style="border:1px solid var(--border);border-radius:4px;padding:0.25rem 0.5rem;background:var(--bg-secondary);color:var(--text);width:120px">
            </div>
        </div>
    </div>
</div>`;
```

Add event listeners for auto-save on blur and tag add/remove:

```js
// Notes auto-save
const notesEl = el.querySelector('#session-notes');
if (notesEl) {
    notesEl.addEventListener('blur', () => {
        API.updateSession(session.id, { notes: notesEl.value });
    });
}

// Tag input
const tagInput = el.querySelector('#session-tag-input');
if (tagInput) {
    tagInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' && tagInput.value.trim()) {
            const currentTags = (session.tags || '').split(',').filter(t => t.trim());
            const newTag = tagInput.value.trim();
            if (!currentTags.includes(newTag)) {
                currentTags.push(newTag);
                session.tags = currentTags.join(',');
                API.updateSession(session.id, { tags: session.tags });
                this.loadDetail(); // re-render
            }
            tagInput.value = '';
        }
    });
}

// Tag removal
el.querySelectorAll('#session-tags-container .badge').forEach(badge => {
    badge.addEventListener('click', () => {
        const tag = badge.dataset.tag;
        const currentTags = (session.tags || '').split(',').filter(t => t.trim() && t.trim() !== tag);
        session.tags = currentTags.join(',');
        API.updateSession(session.id, { tags: session.tags });
        this.loadDetail(); // re-render
    });
});
```

- [ ] **Step 8: Run full test suite**

Run: `cd /Users/szaher/go/src/github.com/szaher/claude-monitor && go test ./... -v`
Expected: All PASS

- [ ] **Step 9: Commit**

```bash
git add internal/server/api_tags.go internal/server/api.go internal/server/server.go web/static/js/api.js web/static/js/components/sessions.js
git commit -m "feat: add session notes and tags with auto-save UI"
```

---

## Task 6: Error Analysis (API + Frontend)

**Files:**
- Create: `internal/server/api_errors.go`
- Modify: `internal/server/server.go`
- Modify: `web/static/js/api.js`
- Modify: `web/static/js/components/tools.js`

- [ ] **Step 1: Create api_errors.go**

```go
package server

import (
    "fmt"
    "net/http"
    "strings"
)

func (s *Server) handleErrors(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeError(w, http.StatusMethodNotAllowed, "method not allowed")
        return
    }

    project := r.URL.Query().Get("project")
    from := r.URL.Query().Get("from")
    to := r.URL.Query().Get("to")

    where := "1=1"
    var args []interface{}

    if project != "" {
        where += " AND tc.session_id IN (SELECT id FROM sessions WHERE project_path = ?)"
        args = append(args, project)
    }
    if from != "" {
        where += " AND tc.timestamp >= ?"
        args = append(args, from)
    }
    if to != "" {
        where += " AND tc.timestamp <= ?"
        args = append(args, to)
    }

    // Total errors and error rate
    var totalErrors, totalCalls int
    s.db.QueryRow(fmt.Sprintf(
        "SELECT COUNT(*) FROM tool_calls tc WHERE %s AND success = 0", where), args...,
    ).Scan(&totalErrors)
    s.db.QueryRow(fmt.Sprintf(
        "SELECT COUNT(*) FROM tool_calls tc WHERE %s", where), args...,
    ).Scan(&totalCalls)

    errorRate := 0.0
    if totalCalls > 0 {
        errorRate = float64(totalErrors) / float64(totalCalls)
    }

    // Errors by tool
    rows, err := s.db.Query(fmt.Sprintf(`
        SELECT tc.tool_name,
            SUM(CASE WHEN tc.success = 0 THEN 1 ELSE 0 END) as errors,
            COUNT(*) as total
        FROM tool_calls tc
        WHERE %s
        GROUP BY tc.tool_name
        HAVING errors > 0
        ORDER BY errors DESC
    `, where), args...)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "query errors by tool: "+err.Error())
        return
    }
    defer rows.Close()

    errorsByTool := []map[string]interface{}{}
    for rows.Next() {
        var tool string
        var errors, total int
        rows.Scan(&tool, &errors, &total)
        rate := 0.0
        if total > 0 {
            rate = float64(errors) / float64(total)
        }
        errorsByTool = append(errorsByTool, map[string]interface{}{
            "tool": tool, "errors": errors, "total": total, "rate": rate,
        })
    }

    // Common error patterns
    errorRows, err := s.db.Query(fmt.Sprintf(`
        SELECT tc.error, COUNT(*) as count
        FROM tool_calls tc
        WHERE %s AND tc.success = 0 AND tc.error IS NOT NULL AND tc.error != ''
        GROUP BY tc.error
        ORDER BY count DESC
        LIMIT 20
    `, where), args...)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "query error patterns: "+err.Error())
        return
    }
    defer errorRows.Close()

    commonErrors := []map[string]interface{}{}
    for errorRows.Next() {
        var errMsg string
        var count int
        errorRows.Scan(&errMsg, &count)
        // Truncate long error messages
        preview := errMsg
        if len(preview) > 200 {
            preview = preview[:200] + "..."
        }
        commonErrors = append(commonErrors, map[string]interface{}{
            "pattern": preview, "count": count,
        })
    }

    // Error trend (last 30 days)
    trendRows, err := s.db.Query(fmt.Sprintf(`
        SELECT date(tc.timestamp) as day,
            SUM(CASE WHEN tc.success = 0 THEN 1 ELSE 0 END) as errors,
            COUNT(*) as total
        FROM tool_calls tc
        WHERE %s AND tc.timestamp >= datetime('now', '-30 days')
        GROUP BY day
        ORDER BY day ASC
    `, strings.Replace(where, "tc.", "tc.", -1)), args...)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "query error trend: "+err.Error())
        return
    }
    defer trendRows.Close()

    errorTrend := []map[string]interface{}{}
    for trendRows.Next() {
        var day string
        var errors, total int
        trendRows.Scan(&day, &errors, &total)
        errorTrend = append(errorTrend, map[string]interface{}{
            "date": day, "errors": errors, "total": total,
        })
    }

    writeJSON(w, http.StatusOK, map[string]interface{}{
        "total_errors":  totalErrors,
        "error_rate":    errorRate,
        "errors_by_tool": errorsByTool,
        "common_errors": commonErrors,
        "error_trend":   errorTrend,
    })
}
```

- [ ] **Step 2: Register route**

In `internal/server/server.go` `setupRoutes()`:

```go
s.mux.HandleFunc("/api/stats/errors", s.handleErrors)
```

- [ ] **Step 3: Add API client method**

In `web/static/js/api.js`:

```js
getErrorStats(params) {
    return API.get('/api/stats/errors', params);
},
```

- [ ] **Step 4: Add Error Analysis section to tools.js**

In `web/static/js/components/tools.js`, extend `loadData()` to fetch error stats:

```js
// Add to the existing Promise.all in loadData():
const errorData = await API.getErrorStats().catch(() => null);
```

Add an error analysis section after the existing tools content:

```js
// Error Analysis section
if (errorData) {
    const errorSection = document.createElement('div');
    errorSection.innerHTML = `
        <h2 style="margin:2rem 0 1rem">Error Analysis</h2>
        <div class="stats-row" style="margin-bottom:1.5rem">
            <div class="stat-card">
                <div class="stat-label">Total Errors</div>
                <div class="stat-value" style="color:var(--danger)">${errorData.total_errors || 0}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Error Rate</div>
                <div class="stat-value">${((errorData.error_rate || 0) * 100).toFixed(1)}%</div>
            </div>
        </div>
        <div class="grid-2">
            <div class="card">
                <div class="card-header"><h3>Failure Rate by Tool</h3></div>
                <div class="card-body"><canvas id="error-by-tool-chart"></canvas></div>
            </div>
            <div class="card">
                <div class="card-header"><h3>Error Trend (30 days)</h3></div>
                <div class="card-body"><canvas id="error-trend-chart"></canvas></div>
            </div>
        </div>
        ${errorData.common_errors && errorData.common_errors.length > 0 ? `
        <div class="card" style="margin-top:1.5rem">
            <div class="card-header"><h3>Common Error Patterns</h3></div>
            <div class="card-body">
                <table class="table"><thead><tr><th>Pattern</th><th>Count</th></tr></thead>
                <tbody>${errorData.common_errors.map(e =>
                    `<tr><td style="font-family:monospace;font-size:0.85rem">${this._esc(e.pattern)}</td><td>${e.count}</td></tr>`
                ).join('')}</tbody></table>
            </div>
        </div>` : ''}
    `;
    el.appendChild(errorSection);

    // Error by tool horizontal bar chart
    if (errorData.errors_by_tool && errorData.errors_by_tool.length > 0) {
        new Chart(document.getElementById('error-by-tool-chart'), {
            type: 'bar',
            data: {
                labels: errorData.errors_by_tool.map(e => e.tool),
                datasets: [{
                    label: 'Error Rate',
                    data: errorData.errors_by_tool.map(e => (e.rate * 100).toFixed(1)),
                    backgroundColor: 'rgba(220, 53, 69, 0.7)',
                }],
            },
            options: {
                indexAxis: 'y',
                responsive: true,
                plugins: { legend: { display: false } },
                scales: { x: { title: { display: true, text: 'Error Rate (%)' } } },
            },
        });
    }

    // Error trend line chart
    if (errorData.error_trend && errorData.error_trend.length > 0) {
        new Chart(document.getElementById('error-trend-chart'), {
            type: 'line',
            data: {
                labels: errorData.error_trend.map(e => e.date),
                datasets: [{
                    label: 'Errors',
                    data: errorData.error_trend.map(e => e.errors),
                    borderColor: 'rgba(220, 53, 69, 0.8)',
                    backgroundColor: 'rgba(220, 53, 69, 0.1)',
                    fill: true,
                    tension: 0.3,
                }],
            },
            options: { responsive: true, plugins: { legend: { display: false } } },
        });
    }
}
```

- [ ] **Step 5: Run Go tests**

Run: `cd /Users/szaher/go/src/github.com/szaher/claude-monitor && go build ./...`
Expected: Build succeeds

- [ ] **Step 6: Commit**

```bash
git add internal/server/api_errors.go internal/server/server.go web/static/js/api.js web/static/js/components/tools.js
git commit -m "feat: add error analysis API and tools page section"
```

---

## Task 7: Token Efficiency Dashboard (API + Frontend)

**Files:**
- Create: `internal/server/api_efficiency.go`
- Modify: `internal/server/server.go`
- Modify: `web/static/js/api.js`
- Modify: `web/static/js/components/cost.js`

- [ ] **Step 1: Create api_efficiency.go**

```go
package server

import (
    "fmt"
    "net/http"
)

func (s *Server) handleTokenEfficiency(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeError(w, http.StatusMethodNotAllowed, "method not allowed")
        return
    }

    project := r.URL.Query().Get("project")
    from := r.URL.Query().Get("from")
    to := r.URL.Query().Get("to")

    where := "1=1"
    var args []interface{}

    if project != "" {
        where += " AND m.session_id IN (SELECT id FROM sessions WHERE project_path = ?)"
        args = append(args, project)
    }
    if from != "" {
        where += " AND m.timestamp >= ?"
        args = append(args, from)
    }
    if to != "" {
        where += " AND m.timestamp <= ?"
        args = append(args, to)
    }

    // Cache hit rate
    var totalCacheRead, totalInput int
    s.db.QueryRow(fmt.Sprintf(
        "SELECT COALESCE(SUM(cache_read_tokens),0), COALESCE(SUM(input_tokens),0) FROM messages m WHERE %s", where,
    ), args...).Scan(&totalCacheRead, &totalInput)

    cacheHitRate := 0.0
    if totalCacheRead+totalInput > 0 {
        cacheHitRate = float64(totalCacheRead) / float64(totalCacheRead+totalInput)
    }

    // Average tokens per message
    var avgInput, avgOutput float64
    var msgCount int
    s.db.QueryRow(fmt.Sprintf(
        "SELECT COALESCE(AVG(input_tokens),0), COALESCE(AVG(output_tokens),0), COUNT(*) FROM messages m WHERE %s AND type='assistant'", where,
    ), args...).Scan(&avgInput, &avgOutput, &msgCount)

    // Average tokens per tool call
    var avgTokensPerTool float64
    s.db.QueryRow(fmt.Sprintf(`
        SELECT COALESCE(CAST(SUM(m.output_tokens) AS REAL) / NULLIF(COUNT(DISTINCT tc.id), 0), 0)
        FROM messages m
        JOIN tool_calls tc ON tc.session_id = m.session_id
        WHERE %s AND m.type = 'assistant'
    `, where), args...).Scan(&avgTokensPerTool)

    // Cache savings calculation
    cacheSavingsTokens := totalCacheRead
    var cacheSavingsUSD float64
    // Estimate savings: cache_read costs ~10x less than input
    // Approximate savings = totalCacheRead * (input_price - cache_read_price) / 1M
    // Use a rough $10/M token savings estimate
    cacheSavingsUSD = float64(cacheSavingsTokens) / 1_000_000 * 10.0

    // Efficiency by model
    modelRows, err := s.db.Query(fmt.Sprintf(`
        SELECT COALESCE(model, 'unknown'),
            COALESCE(SUM(cache_read_tokens), 0) as cr,
            COALESCE(SUM(input_tokens), 0) as inp,
            COALESCE(AVG(output_tokens), 0) as avg_out,
            COALESCE(AVG(input_tokens + output_tokens), 0) as avg_total
        FROM messages m
        WHERE %s AND model IS NOT NULL AND model != ''
        GROUP BY model
    `, where), args...)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "query model efficiency: "+err.Error())
        return
    }
    defer modelRows.Close()

    effByModel := []map[string]interface{}{}
    for modelRows.Next() {
        var model string
        var cr, inp int
        var avgOut, avgTotal float64
        modelRows.Scan(&model, &cr, &inp, &avgOut, &avgTotal)
        modelHitRate := 0.0
        if cr+inp > 0 {
            modelHitRate = float64(cr) / float64(cr+inp)
        }
        outputRatio := 0.0
        if avgTotal > 0 {
            outputRatio = avgOut / avgTotal
        }
        effByModel = append(effByModel, map[string]interface{}{
            "model":           model,
            "cache_hit_rate":  modelHitRate,
            "avg_output_ratio": outputRatio,
        })
    }

    writeJSON(w, http.StatusOK, map[string]interface{}{
        "cache_hit_rate":             cacheHitRate,
        "avg_tokens_per_tool_call":   avgTokensPerTool,
        "avg_input_tokens_per_message":  avgInput,
        "avg_output_tokens_per_message": avgOutput,
        "cache_savings_tokens":       cacheSavingsTokens,
        "cache_savings_usd":          cacheSavingsUSD,
        "efficiency_by_model":        effByModel,
    })
}
```

- [ ] **Step 2: Register route**

In `internal/server/server.go`:

```go
s.mux.HandleFunc("/api/stats/token-efficiency", s.handleTokenEfficiency)
```

- [ ] **Step 3: Add API client method**

In `web/static/js/api.js`:

```js
getTokenEfficiency(params) {
    return API.get('/api/stats/token-efficiency', params);
},
```

- [ ] **Step 4: Add Token Efficiency section to cost.js**

In `web/static/js/components/cost.js`, extend `loadData()` to fetch token efficiency data in its existing `Promise.all`. Add a "Token Efficiency" card section after the existing cache analysis section:

```js
// Token Efficiency section (add after cache analysis)
if (efficiencyData) {
    const effSection = document.createElement('div');
    effSection.innerHTML = `
        <h2 style="margin:2rem 0 1rem">Token Efficiency</h2>
        <div class="stats-row">
            <div class="stat-card">
                <div class="stat-label">Cache Hit Rate</div>
                <div class="stat-value">${((efficiencyData.cache_hit_rate || 0) * 100).toFixed(1)}%</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Cache Savings</div>
                <div class="stat-value">$${(efficiencyData.cache_savings_usd || 0).toFixed(2)}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Avg Tokens/Tool Call</div>
                <div class="stat-value">${Math.round(efficiencyData.avg_tokens_per_tool_call || 0).toLocaleString()}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Avg Output/Message</div>
                <div class="stat-value">${Math.round(efficiencyData.avg_output_tokens_per_message || 0).toLocaleString()}</div>
            </div>
        </div>
        ${efficiencyData.efficiency_by_model && efficiencyData.efficiency_by_model.length > 0 ? `
        <div class="card" style="margin-top:1.5rem">
            <div class="card-header"><h3>Efficiency by Model</h3></div>
            <div class="card-body">
                <table class="table">
                    <thead><tr><th>Model</th><th>Cache Hit Rate</th><th>Output Ratio</th></tr></thead>
                    <tbody>${efficiencyData.efficiency_by_model.map(m =>
                        `<tr><td>${this._esc(m.model)}</td><td>${(m.cache_hit_rate * 100).toFixed(1)}%</td><td>${(m.avg_output_ratio * 100).toFixed(1)}%</td></tr>`
                    ).join('')}</tbody>
                </table>
            </div>
        </div>` : ''}
    `;
    el.appendChild(effSection);
}
```

- [ ] **Step 5: Commit**

```bash
git add internal/server/api_efficiency.go internal/server/server.go web/static/js/api.js web/static/js/components/cost.js
git commit -m "feat: add token efficiency dashboard to cost page"
```

---

## Task 8: UI Export (API + Frontend)

**Files:**
- Create: `internal/server/api_export.go`
- Modify: `internal/server/server.go`
- Modify: `web/static/js/api.js`
- Modify: `web/static/js/app.js`

- [ ] **Step 1: Create api_export.go**

```go
package server

import (
    "archive/zip"
    "fmt"
    "net/http"
    "os"
    "path/filepath"

    "github.com/szaher/claude-monitor/internal/db"
    "github.com/szaher/claude-monitor/internal/exporter"
    "github.com/szaher/claude-monitor/internal/models"
)

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeError(w, http.StatusMethodNotAllowed, "method not allowed")
        return
    }

    format := r.URL.Query().Get("format")
    if format == "" {
        format = "json"
    }
    project := r.URL.Query().Get("project")
    from := r.URL.Query().Get("from")
    to := r.URL.Query().Get("to")
    sessionID := r.URL.Query().Get("session_id")

    var sessions []*models.Session

    if sessionID != "" {
        sess, err := db.GetSessionByID(s.db, sessionID)
        if err != nil {
            writeError(w, http.StatusNotFound, "session not found")
            return
        }
        sessions = []*models.Session{sess}
    } else {
        // Build query with filters
        query := `SELECT id, project_path, project_name, cwd, git_branch,
            started_at, ended_at, claude_version, entry_point, permission_mode,
            total_input_tokens, total_output_tokens, total_cache_read_tokens,
            total_cache_write_tokens, estimated_cost_usd
            FROM sessions WHERE 1=1`
        var args []interface{}

        if project != "" {
            query += " AND (project_path = ? OR project_name = ?)"
            args = append(args, project, project)
        }
        if from != "" {
            query += " AND started_at >= ?"
            args = append(args, from)
        }
        if to != "" {
            query += " AND started_at <= ?"
            args = append(args, to)
        }
        query += " ORDER BY started_at DESC LIMIT 1000"

        rows, err := s.db.Query(query, args...)
        if err != nil {
            writeError(w, http.StatusInternalServerError, "query sessions: "+err.Error())
            return
        }
        defer rows.Close()

        for rows.Next() {
            sess := &models.Session{}
            var startedAt string
            var endedAt, cwd, gitBranch, claudeVersion, entryPoint, permissionMode interface{}

            rows.Scan(
                &sess.ID, &sess.ProjectPath, &sess.ProjectName, &cwd, &gitBranch,
                &startedAt, &endedAt, &claudeVersion, &entryPoint, &permissionMode,
                &sess.TotalInputTokens, &sess.TotalOutputTokens,
                &sess.TotalCacheReadTokens, &sess.TotalCacheWriteTokens,
                &sess.EstimatedCostUSD,
            )

            // Parse fields (simplified - use the db package pattern)
            s, err := db.GetSessionByID(s.db, sess.ID)
            if err == nil {
                sessions = append(sessions, s)
            }
        }
    }

    if len(sessions) == 0 {
        writeError(w, http.StatusNotFound, "no sessions found")
        return
    }

    exp := exporter.New(s.db)

    switch format {
    case "json":
        w.Header().Set("Content-Type", "application/json")
        w.Header().Set("Content-Disposition", "attachment; filename=claude-monitor-export.json")
        exp.ExportJSON(w, sessions)

    case "csv":
        // Export CSV to temp dir, then zip and send
        tmpDir, err := os.MkdirTemp("", "claude-monitor-csv-*")
        if err != nil {
            writeError(w, http.StatusInternalServerError, "create temp dir: "+err.Error())
            return
        }
        defer os.RemoveAll(tmpDir)

        if err := exp.ExportCSV(tmpDir, sessions); err != nil {
            writeError(w, http.StatusInternalServerError, "export csv: "+err.Error())
            return
        }

        w.Header().Set("Content-Type", "application/zip")
        w.Header().Set("Content-Disposition", "attachment; filename=claude-monitor-export.zip")

        zw := zip.NewWriter(w)
        defer zw.Close()

        for _, name := range []string{"sessions.csv", "messages.csv", "tool_calls.csv"} {
            data, err := os.ReadFile(filepath.Join(tmpDir, name))
            if err != nil {
                continue
            }
            fw, err := zw.Create(name)
            if err != nil {
                continue
            }
            fw.Write(data)
        }

    case "html":
        w.Header().Set("Content-Type", "text/html")
        w.Header().Set("Content-Disposition", "attachment; filename=claude-monitor-report.html")
        exp.ExportHTML(w, sessions)

    default:
        writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported format: %s", format))
    }
}
```

- [ ] **Step 2: Register route**

In `internal/server/server.go`:

```go
s.mux.HandleFunc("/api/export", s.handleExport)
```

- [ ] **Step 3: Add export button to app.js nav**

In `web/static/js/app.js`, add an export modal method and button. Add the export functionality to the `App` object:

```js
showExportModal() {
    // Create modal overlay
    const overlay = document.createElement('div');
    overlay.style.cssText = 'position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,0.5);z-index:1000;display:flex;align-items:center;justify-content:center';
    overlay.innerHTML = `
        <div style="background:var(--bg-primary);border-radius:8px;padding:2rem;max-width:400px;width:90%;box-shadow:0 4px 20px rgba(0,0,0,0.3)">
            <h3 style="margin:0 0 1.5rem">Export Data</h3>
            <div style="margin-bottom:1rem">
                <label style="display:block;margin-bottom:0.25rem;font-weight:600">Format</label>
                <select id="export-format" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:4px;background:var(--bg-secondary);color:var(--text)">
                    <option value="json">JSON</option>
                    <option value="csv">CSV (ZIP)</option>
                    <option value="html">HTML Report</option>
                </select>
            </div>
            <div style="display:flex;gap:1rem;justify-content:flex-end;margin-top:1.5rem">
                <button id="export-cancel" class="btn btn-secondary">Cancel</button>
                <button id="export-download" class="btn btn-primary">Download</button>
            </div>
        </div>
    `;
    document.body.appendChild(overlay);

    overlay.querySelector('#export-cancel').addEventListener('click', () => overlay.remove());
    overlay.addEventListener('click', (e) => { if (e.target === overlay) overlay.remove(); });

    overlay.querySelector('#export-download').addEventListener('click', () => {
        const format = overlay.querySelector('#export-format').value;
        const params = new URLSearchParams({ format });
        if (this.filters.project) params.set('project', this.filters.project);
        if (this.filters.from) params.set('from', this.filters.from);
        if (this.filters.to) params.set('to', this.filters.to);
        window.location.href = '/api/export?' + params.toString();
        overlay.remove();
    });
},
```

Then add an export button to the nav initialization in `init()` or `setupSidebarToggle()`. Add after sidebar setup:

```js
// Add export button to header
const header = document.querySelector('.header') || document.querySelector('header');
if (header) {
    const exportBtn = document.createElement('button');
    exportBtn.className = 'btn btn-secondary';
    exportBtn.textContent = 'Export';
    exportBtn.style.cssText = 'margin-left:auto;margin-right:1rem';
    exportBtn.addEventListener('click', () => App.showExportModal());
    header.appendChild(exportBtn);
}
```

- [ ] **Step 4: Add per-session export to API client**

In `web/static/js/api.js`:

```js
getExportURL(params) {
    const url = new URL('/api/export', window.location.origin);
    Object.entries(params).forEach(([k, v]) => {
        if (v) url.searchParams.set(k, v);
    });
    return url.toString();
},
```

- [ ] **Step 5: Commit**

```bash
git add internal/server/api_export.go internal/server/server.go web/static/js/app.js web/static/js/api.js
git commit -m "feat: add UI export with JSON/CSV/HTML download"
```

---

## Task 9: Session Timeline (API + Frontend)

**Files:**
- Create: `internal/server/api_timeline.go`
- Modify: `internal/server/api.go` (extend session detail routing)
- Modify: `web/static/js/api.js`
- Modify: `web/static/js/components/sessions.js`

- [ ] **Step 1: Create api_timeline.go**

```go
package server

import (
    "net/http"
    "strings"
    "time"
)

func (s *Server) handleSessionTimeline(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeError(w, http.StatusMethodNotAllowed, "method not allowed")
        return
    }

    // Path: /api/sessions/{id}/timeline
    path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
    parts := strings.Split(path, "/")
    if len(parts) < 2 || parts[1] != "timeline" {
        writeError(w, http.StatusBadRequest, "invalid path")
        return
    }
    sessionID := parts[0]

    events := []map[string]interface{}{}

    // Messages
    msgRows, _ := s.db.Query(`
        SELECT type, role, model, timestamp,
            SUBSTR(content_text, 1, 100) as preview,
            input_tokens + output_tokens as tokens
        FROM messages
        WHERE session_id = ? AND type IN ('user', 'assistant')
        ORDER BY timestamp ASC
    `, sessionID)
    if msgRows != nil {
        defer msgRows.Close()
        for msgRows.Next() {
            var msgType, role, model, ts, preview string
            var tokens int
            msgRows.Scan(&msgType, &role, &model, &ts, &preview, &tokens)
            eventType := "user_message"
            if msgType == "assistant" {
                eventType = "assistant_message"
            }
            events = append(events, map[string]interface{}{
                "type": eventType, "timestamp": ts, "preview": preview,
                "model": model, "tokens": tokens,
            })
        }
    }

    // Tool calls
    tcRows, _ := s.db.Query(`
        SELECT tool_name, timestamp, duration_ms, success, error
        FROM tool_calls WHERE session_id = ?
        ORDER BY timestamp ASC
    `, sessionID)
    if tcRows != nil {
        defer tcRows.Close()
        for tcRows.Next() {
            var tool, ts, errStr string
            var durationMS int
            var success bool
            tcRows.Scan(&tool, &ts, &durationMS, &success, &errStr)
            events = append(events, map[string]interface{}{
                "type": "tool_call", "timestamp": ts, "tool": tool,
                "duration_ms": durationMS, "success": success, "error": errStr,
            })
        }
    }

    // Subagents
    saRows, _ := s.db.Query(`
        SELECT agent_type, description, started_at
        FROM subagents WHERE session_id = ?
        ORDER BY started_at ASC
    `, sessionID)
    if saRows != nil {
        defer saRows.Close()
        for saRows.Next() {
            var agentType, desc, ts string
            saRows.Scan(&agentType, &desc, &ts)
            events = append(events, map[string]interface{}{
                "type": "agent_spawn", "timestamp": ts,
                "agent_type": agentType, "description": desc,
            })
        }
    }

    // Context compactions (if table exists)
    ccRows, _ := s.db.Query(`
        SELECT pre_tokens, post_tokens, timestamp
        FROM context_compactions WHERE session_id = ?
        ORDER BY timestamp ASC
    `, sessionID)
    if ccRows != nil {
        defer ccRows.Close()
        for ccRows.Next() {
            var pre, post int
            var ts string
            ccRows.Scan(&pre, &post, &ts)
            events = append(events, map[string]interface{}{
                "type": "compaction", "timestamp": ts,
                "pre_tokens": pre, "post_tokens": post,
            })
        }
    }

    // Calculate total duration
    var startTs, endTs string
    s.db.QueryRow("SELECT started_at FROM sessions WHERE id = ?", sessionID).Scan(&startTs)
    s.db.QueryRow("SELECT COALESCE(ended_at, datetime('now')) FROM sessions WHERE id = ?", sessionID).Scan(&endTs)

    var durationSec int
    if startT, err := time.Parse(time.RFC3339, startTs); err == nil {
        if endT, err := time.Parse(time.RFC3339, endTs); err == nil {
            durationSec = int(endT.Sub(startT).Seconds())
        }
    }

    var totalTokens int
    s.db.QueryRow("SELECT COALESCE(SUM(total_input_tokens + total_output_tokens), 0) FROM sessions WHERE id = ?", sessionID).Scan(&totalTokens)

    writeJSON(w, http.StatusOK, map[string]interface{}{
        "events":           events,
        "duration_seconds":  durationSec,
        "total_tokens":      totalTokens,
    })
}
```

- [ ] **Step 2: Extend session detail routing to handle /timeline sub-path**

In `internal/server/api.go`, modify `handleSessionDetail` to detect sub-paths:

```go
func (s *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
    path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")

    // Check for sub-paths
    if strings.Contains(path, "/timeline") {
        s.handleSessionTimeline(w, r)
        return
    }
    if strings.Contains(path, "/commits") {
        s.handleSessionCommits(w, r)
        return
    }

    switch r.Method {
    case http.MethodPatch:
        s.handleSessionPatch(w, r)
        return
    case http.MethodGet:
        // existing GET logic continues below
    default:
        writeError(w, http.StatusMethodNotAllowed, "method not allowed")
        return
    }

    // ... rest of existing GET handler ...
}
```

- [ ] **Step 3: Add API client method**

In `web/static/js/api.js`:

```js
getSessionTimeline(sessionId) {
    return API.get(`/api/sessions/${sessionId}/timeline`);
},
```

- [ ] **Step 4: Add timeline visualization to session detail**

In `web/static/js/components/sessions.js`, fetch timeline data in `loadDetail()` and add a timeline visualization above the conversation view. Use positioned HTML divs:

```js
// Fetch timeline data alongside existing data
const timeline = await API.getSessionTimeline(this.selectedSession).catch(() => null);

// Timeline visualization
if (timeline && timeline.events && timeline.events.length > 0) {
    const timelineHtml = `
    <div class="card" style="margin-bottom:1.5rem">
        <div class="card-header"><h3>Session Timeline</h3></div>
        <div class="card-body">
            <div class="stats-row" style="margin-bottom:1rem">
                <div class="stat-card"><div class="stat-label">Duration</div><div class="stat-value">${this._formatDuration(timeline.duration_seconds)}</div></div>
                <div class="stat-card"><div class="stat-label">Total Tokens</div><div class="stat-value">${(timeline.total_tokens || 0).toLocaleString()}</div></div>
                <div class="stat-card"><div class="stat-label">Events</div><div class="stat-value">${timeline.events.length}</div></div>
            </div>
            <div id="timeline-bar" style="position:relative;height:60px;background:var(--bg-secondary);border-radius:4px;overflow:hidden;margin-top:0.5rem">
                ${timeline.events.map((evt, i) => {
                    const colors = {
                        user_message: '#3b82f6', assistant_message: '#22c55e',
                        tool_call: '#f59e0b', agent_spawn: '#a855f7',
                        compaction: '#6b7280',
                    };
                    const color = evt.success === false ? '#ef4444' : (colors[evt.type] || '#999');
                    const pct = timeline.duration_seconds > 0
                        ? ((new Date(evt.timestamp) - new Date(timeline.events[0].timestamp)) / (timeline.duration_seconds * 1000) * 100)
                        : (i / timeline.events.length * 100);
                    const lane = { user_message: 10, assistant_message: 25, tool_call: 40, agent_spawn: 55, compaction: 55 }[evt.type] || 25;
                    const label = evt.tool || evt.agent_type || evt.type.replace('_', ' ');
                    return `<div style="position:absolute;left:${Math.min(pct, 98)}%;top:${lane}px;width:8px;height:8px;border-radius:50%;background:${color};cursor:pointer" title="${label}: ${evt.timestamp}"></div>`;
                }).join('')}
            </div>
            <div style="display:flex;gap:1rem;font-size:0.75rem;color:var(--text-secondary);margin-top:0.5rem">
                <span style="color:#3b82f6">&#9679; User</span>
                <span style="color:#22c55e">&#9679; Assistant</span>
                <span style="color:#f59e0b">&#9679; Tool</span>
                <span style="color:#a855f7">&#9679; Agent</span>
                <span style="color:#ef4444">&#9679; Error</span>
            </div>
        </div>
    </div>`;
    // Insert before conversation view
    el.querySelector('.card')?.insertAdjacentHTML('beforebegin', timelineHtml);
}
```

Add helper method:

```js
_formatDuration(seconds) {
    if (!seconds) return '0s';
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    const s = seconds % 60;
    if (h > 0) return `${h}h ${m}m`;
    if (m > 0) return `${m}m ${s}s`;
    return `${s}s`;
},
```

- [ ] **Step 5: Commit**

```bash
git add internal/server/api_timeline.go internal/server/api.go web/static/js/api.js web/static/js/components/sessions.js
git commit -m "feat: add session timeline visualization"
```

---

## Task 10: Prompt Pattern Analysis (API + Frontend)

**Files:**
- Create: `internal/server/api_patterns.go`
- Modify: `internal/server/server.go`
- Modify: `web/static/js/api.js`
- Modify: `web/static/js/components/dashboard.js`

- [ ] **Step 1: Create api_patterns.go with keyword classification**

```go
package server

import (
    "fmt"
    "net/http"
    "strings"
)

var promptCategories = map[string][]string{
    "Bug Fix":  {"fix", "bug", "error", "broken", "crash", "failing", "debug"},
    "Feature":  {"add", "implement", "create", "build", "new feature", "scaffold"},
    "Refactor": {"refactor", "rename", "extract", "move", "reorganize", "clean up"},
    "Explain":  {"explain", "what does", "how does", "why", "understand", "walk me through"},
    "Test":     {"test", "spec", "coverage", "assert", "mock", "fixture"},
    "Config":   {"config", "setup", "install", "deploy", "ci", "docker", "env"},
    "Review":   {"review", "check", "audit", "look at", "feedback"},
    "Docs":     {"document", "readme", "comment", "docstring", "jsdoc"},
}

func classifyPrompt(text string) []string {
    lower := strings.ToLower(text)
    var categories []string
    for category, keywords := range promptCategories {
        for _, kw := range keywords {
            if strings.Contains(lower, kw) {
                categories = append(categories, category)
                break
            }
        }
    }
    return categories
}

func (s *Server) handlePromptPatterns(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeError(w, http.StatusMethodNotAllowed, "method not allowed")
        return
    }

    project := r.URL.Query().Get("project")
    from := r.URL.Query().Get("from")
    to := r.URL.Query().Get("to")

    where := "m.type = 'user' AND m.content_text IS NOT NULL AND m.content_text != ''"
    var args []interface{}

    if project != "" {
        where += " AND m.session_id IN (SELECT id FROM sessions WHERE project_path = ?)"
        args = append(args, project)
    }
    if from != "" {
        where += " AND m.timestamp >= ?"
        args = append(args, from)
    }
    if to != "" {
        where += " AND m.timestamp <= ?"
        args = append(args, to)
    }

    rows, err := s.db.Query(fmt.Sprintf(`
        SELECT m.content_text, m.session_id, date(m.timestamp) as day
        FROM messages m WHERE %s
    `, where), args...)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "query prompts: "+err.Error())
        return
    }
    defer rows.Close()

    type catStats struct {
        Count int
        Days  map[string]int
    }
    stats := map[string]*catStats{}
    totalPrompts := 0

    for rows.Next() {
        var content, sessionID, day string
        rows.Scan(&content, &sessionID, &day)
        totalPrompts++

        categories := classifyPrompt(content)
        for _, cat := range categories {
            if stats[cat] == nil {
                stats[cat] = &catStats{Days: map[string]int{}}
            }
            stats[cat].Count++
            stats[cat].Days[day]++
        }
    }

    categories := []map[string]interface{}{}
    for name, st := range stats {
        pct := 0.0
        if totalPrompts > 0 {
            pct = float64(st.Count) / float64(totalPrompts) * 100
        }
        categories = append(categories, map[string]interface{}{
            "name":       name,
            "count":      st.Count,
            "percentage": pct,
        })
    }

    // Build trend data (last 30 days)
    trendMap := map[string]map[string]int{} // day -> category -> count
    for cat, st := range stats {
        for day, count := range st.Days {
            if trendMap[day] == nil {
                trendMap[day] = map[string]int{}
            }
            trendMap[day][cat] = count
        }
    }

    trends := []map[string]interface{}{}
    for day, cats := range trendMap {
        entry := map[string]interface{}{"date": day}
        for cat, count := range cats {
            entry[cat] = count
        }
        trends = append(trends, entry)
    }

    writeJSON(w, http.StatusOK, map[string]interface{}{
        "categories": categories,
        "trends":     trends,
    })
}
```

- [ ] **Step 2: Register route**

In `internal/server/server.go`:

```go
s.mux.HandleFunc("/api/stats/prompt-patterns", s.handlePromptPatterns)
```

- [ ] **Step 3: Add API client method**

In `web/static/js/api.js`:

```js
getPromptPatterns(params) {
    return API.get('/api/stats/prompt-patterns', params);
},
```

- [ ] **Step 4: Add Usage Patterns section to dashboard.js**

In `web/static/js/components/dashboard.js`, fetch prompt patterns in `loadData()` and add a "Usage Patterns" section:

```js
const patterns = await API.getPromptPatterns().catch(() => null);

// After existing dashboard sections, add:
if (patterns && patterns.categories && patterns.categories.length > 0) {
    const patternSection = document.createElement('div');
    patternSection.innerHTML = `
        <h2 style="margin:2rem 0 1rem">Usage Patterns</h2>
        <div class="grid-2">
            <div class="card">
                <div class="card-header"><h3>Prompt Categories</h3></div>
                <div class="card-body"><canvas id="pattern-donut"></canvas></div>
            </div>
            <div class="card">
                <div class="card-header"><h3>Category Breakdown</h3></div>
                <div class="card-body">
                    <table class="table">
                        <thead><tr><th>Category</th><th>Count</th><th>%</th></tr></thead>
                        <tbody>${patterns.categories
                            .sort((a, b) => b.count - a.count)
                            .map(c => `<tr><td>${c.name}</td><td>${c.count}</td><td>${c.percentage.toFixed(1)}%</td></tr>`)
                            .join('')}</tbody>
                    </table>
                </div>
            </div>
        </div>
    `;
    el.appendChild(patternSection);

    const colors = ['#3b82f6','#22c55e','#f59e0b','#ef4444','#a855f7','#06b6d4','#ec4899','#84cc16'];
    new Chart(document.getElementById('pattern-donut'), {
        type: 'doughnut',
        data: {
            labels: patterns.categories.map(c => c.name),
            datasets: [{
                data: patterns.categories.map(c => c.count),
                backgroundColor: colors.slice(0, patterns.categories.length),
            }],
        },
        options: { responsive: true, plugins: { legend: { position: 'bottom' } } },
    });
}
```

- [ ] **Step 5: Commit**

```bash
git add internal/server/api_patterns.go internal/server/server.go web/static/js/api.js web/static/js/components/dashboard.js
git commit -m "feat: add prompt pattern analysis to dashboard"
```

---

## Task 11: File Change Heatmap (API + Frontend)

**Files:**
- Create: `internal/server/api_heatmap.go`
- Modify: `internal/server/server.go`
- Modify: `web/static/js/api.js`
- Modify: `web/static/js/components/projects.js`

- [ ] **Step 1: Create api_heatmap.go**

```go
package server

import (
    "encoding/json"
    "net/http"
    "path/filepath"
    "sort"
    "strings"
)

func (s *Server) handleFileHeatmap(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeError(w, http.StatusMethodNotAllowed, "method not allowed")
        return
    }

    project := r.URL.Query().Get("project")
    if project == "" {
        writeError(w, http.StatusBadRequest, "missing required parameter 'project'")
        return
    }

    rows, err := s.db.Query(`
        SELECT tc.tool_name, tc.tool_input
        FROM tool_calls tc
        JOIN sessions s ON tc.session_id = s.id
        WHERE s.project_path = ?
        AND tc.tool_name IN ('Read', 'Write', 'Edit', 'Glob', 'Grep')
        AND tc.tool_input IS NOT NULL AND tc.tool_input != ''
    `, project)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "query file heatmap: "+err.Error())
        return
    }
    defer rows.Close()

    type fileStats struct {
        Reads  int `json:"reads"`
        Writes int `json:"writes"`
        Edits  int `json:"edits"`
        Total  int `json:"total"`
    }

    files := map[string]*fileStats{}

    for rows.Next() {
        var toolName, toolInput string
        rows.Scan(&toolName, &toolInput)

        var parsed map[string]interface{}
        if err := json.Unmarshal([]byte(toolInput), &parsed); err != nil {
            continue
        }

        filePath, _ := parsed["file_path"].(string)
        if filePath == "" {
            filePath, _ = parsed["path"].(string)
        }
        if filePath == "" {
            continue
        }

        // Make path relative to project CWD if possible
        if strings.HasPrefix(filePath, project) {
            filePath = strings.TrimPrefix(filePath, project)
            filePath = strings.TrimPrefix(filePath, "/")
        }

        if files[filePath] == nil {
            files[filePath] = &fileStats{}
        }
        switch toolName {
        case "Read":
            files[filePath].Reads++
        case "Write":
            files[filePath].Writes++
        case "Edit":
            files[filePath].Edits++
        }
        files[filePath].Total++
    }

    // Convert to sorted slice
    type fileEntry struct {
        Path string `json:"path"`
        fileStats
    }
    fileList := make([]fileEntry, 0, len(files))
    for path, stats := range files {
        fileList = append(fileList, fileEntry{Path: path, fileStats: *stats})
    }
    sort.Slice(fileList, func(i, j int) bool {
        return fileList[i].Total > fileList[j].Total
    })
    if len(fileList) > 50 {
        fileList = fileList[:50]
    }

    // Aggregate to directory level
    dirs := map[string]int{}
    for _, f := range fileList {
        dir := filepath.Dir(f.Path)
        dirs[dir] += f.Total
    }

    type dirEntry struct {
        Path  string `json:"path"`
        Total int    `json:"total"`
    }
    dirList := make([]dirEntry, 0, len(dirs))
    for path, total := range dirs {
        dirList = append(dirList, dirEntry{Path: path, Total: total})
    }
    sort.Slice(dirList, func(i, j int) bool {
        return dirList[i].Total > dirList[j].Total
    })
    if len(dirList) > 20 {
        dirList = dirList[:20]
    }

    writeJSON(w, http.StatusOK, map[string]interface{}{
        "files":       fileList,
        "directories": dirList,
    })
}
```

- [ ] **Step 2: Register route**

In `internal/server/server.go`:

```go
s.mux.HandleFunc("/api/stats/file-heatmap", s.handleFileHeatmap)
```

- [ ] **Step 3: Add API client method**

In `web/static/js/api.js`:

```js
getFileHeatmap(project) {
    return API.get('/api/stats/file-heatmap', { project });
},
```

- [ ] **Step 4: Add File Activity section to projects.js**

In `web/static/js/components/projects.js`, fetch file heatmap in `loadDetail()` and add a "File Activity" section:

```js
const heatmap = await API.getFileHeatmap(this.selectedProject).catch(() => null);

if (heatmap && heatmap.files && heatmap.files.length > 0) {
    const maxTotal = Math.max(...heatmap.files.map(f => f.total));
    const fileHtml = `
    <div class="card" style="margin-top:1.5rem">
        <div class="card-header"><h3>File Activity Heatmap</h3></div>
        <div class="card-body">
            <table class="table">
                <thead><tr><th>File</th><th>Reads</th><th>Writes</th><th>Edits</th><th>Total</th><th></th></tr></thead>
                <tbody>${heatmap.files.slice(0, 20).map(f => `
                    <tr>
                        <td style="font-family:monospace;font-size:0.85rem">${this._esc(f.path)}</td>
                        <td>${f.reads}</td><td>${f.writes}</td><td>${f.edits}</td>
                        <td><strong>${f.total}</strong></td>
                        <td style="width:100px">
                            <div style="background:var(--bg-secondary);border-radius:2px;height:12px;overflow:hidden">
                                <div style="height:100%;width:${(f.total/maxTotal*100).toFixed(0)}%;background:linear-gradient(90deg,#3b82f6 ${(f.reads/f.total*100).toFixed(0)}%,#f59e0b ${(f.reads/f.total*100).toFixed(0)}%,#f59e0b ${((f.reads+f.writes)/f.total*100).toFixed(0)}%,#22c55e ${((f.reads+f.writes)/f.total*100).toFixed(0)}%)"></div>
                            </div>
                        </td>
                    </tr>
                `).join('')}</tbody>
            </table>
            <div style="display:flex;gap:1rem;font-size:0.75rem;color:var(--text-secondary);margin-top:0.5rem">
                <span style="color:#3b82f6">&#9632; Reads</span>
                <span style="color:#f59e0b">&#9632; Writes</span>
                <span style="color:#22c55e">&#9632; Edits</span>
            </div>
        </div>
    </div>`;
    el.insertAdjacentHTML('beforeend', fileHtml);
}
```

- [ ] **Step 5: Commit**

```bash
git add internal/server/api_heatmap.go internal/server/server.go web/static/js/api.js web/static/js/components/projects.js
git commit -m "feat: add file change heatmap to project detail"
```

---

## Task 12: Cost Budgets & Alerts (API + Frontend)

**Files:**
- Create: `internal/server/api_budgets.go`
- Modify: `internal/server/server.go`
- Modify: `web/static/js/api.js`
- Modify: `web/static/js/components/cost.js`
- Modify: `web/static/js/components/dashboard.js`

- [ ] **Step 1: Create api_budgets.go**

```go
package server

import (
    "encoding/json"
    "io"
    "net/http"
    "strconv"
    "strings"
    "time"

    "github.com/szaher/claude-monitor/internal/db"
    "github.com/szaher/claude-monitor/internal/models"
)

func (s *Server) handleBudgets(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        s.listBudgets(w, r)
    case http.MethodPost:
        s.createBudget(w, r)
    default:
        writeError(w, http.StatusMethodNotAllowed, "method not allowed")
    }
}

func (s *Server) handleBudgetDetail(w http.ResponseWriter, r *http.Request) {
    idStr := strings.TrimPrefix(r.URL.Path, "/api/budgets/")
    if idStr == "" || idStr == "status" {
        if idStr == "status" {
            s.budgetStatus(w, r)
            return
        }
        writeError(w, http.StatusBadRequest, "missing budget id")
        return
    }
    id, err := strconv.Atoi(idStr)
    if err != nil {
        writeError(w, http.StatusBadRequest, "invalid budget id")
        return
    }

    switch r.Method {
    case http.MethodPut:
        s.updateBudget(w, r, id)
    case http.MethodDelete:
        if err := db.DeleteBudget(s.db, id); err != nil {
            writeError(w, http.StatusInternalServerError, "delete budget: "+err.Error())
            return
        }
        writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
    default:
        writeError(w, http.StatusMethodNotAllowed, "method not allowed")
    }
}

func (s *Server) listBudgets(w http.ResponseWriter, r *http.Request) {
    rows, err := s.db.Query("SELECT id, name, project_path, period, amount_usd, enabled, created_at FROM budgets ORDER BY created_at DESC")
    if err != nil {
        writeError(w, http.StatusInternalServerError, "query budgets: "+err.Error())
        return
    }
    defer rows.Close()

    budgets := []map[string]interface{}{}
    for rows.Next() {
        var id int
        var name, period, createdAt string
        var projectPath interface{}
        var amountUSD float64
        var enabled bool
        rows.Scan(&id, &name, &projectPath, &period, &amountUSD, &enabled, &createdAt)

        pp := ""
        if projectPath != nil {
            pp, _ = projectPath.(string)
        }

        budgets = append(budgets, map[string]interface{}{
            "id": id, "name": name, "project_path": pp,
            "period": period, "amount_usd": amountUSD,
            "enabled": enabled, "created_at": createdAt,
        })
    }
    writeJSON(w, http.StatusOK, map[string]interface{}{"budgets": budgets})
}

func (s *Server) createBudget(w http.ResponseWriter, r *http.Request) {
    body, _ := io.ReadAll(r.Body)
    var b models.Budget
    if err := json.Unmarshal(body, &b); err != nil {
        writeError(w, http.StatusBadRequest, "invalid json")
        return
    }
    b.Enabled = true
    id, err := db.InsertBudget(s.db, &b)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "create budget: "+err.Error())
        return
    }
    b.ID = int(id)
    writeJSON(w, http.StatusCreated, b)
}

func (s *Server) updateBudget(w http.ResponseWriter, r *http.Request, id int) {
    body, _ := io.ReadAll(r.Body)
    var b models.Budget
    if err := json.Unmarshal(body, &b); err != nil {
        writeError(w, http.StatusBadRequest, "invalid json")
        return
    }
    b.ID = id
    if err := db.UpdateBudget(s.db, &b); err != nil {
        writeError(w, http.StatusInternalServerError, "update budget: "+err.Error())
        return
    }
    writeJSON(w, http.StatusOK, b)
}

func (s *Server) budgetStatus(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeError(w, http.StatusMethodNotAllowed, "method not allowed")
        return
    }

    rows, err := s.db.Query("SELECT id, name, project_path, period, amount_usd, enabled FROM budgets WHERE enabled = 1")
    if err != nil {
        writeError(w, http.StatusInternalServerError, "query budgets: "+err.Error())
        return
    }
    defer rows.Close()

    now := time.Now().UTC()
    budgets := []map[string]interface{}{}

    for rows.Next() {
        var id int
        var name, period string
        var projectPath interface{}
        var amountUSD float64
        var enabled bool
        rows.Scan(&id, &name, &projectPath, &period, &amountUSD, &enabled)

        pp := ""
        if projectPath != nil {
            pp, _ = projectPath.(string)
        }

        // Calculate period start
        var periodStart string
        switch period {
        case "daily":
            periodStart = now.Format("2006-01-02")
        case "weekly":
            weekday := int(now.Weekday())
            if weekday == 0 { weekday = 7 }
            start := now.AddDate(0, 0, -(weekday - 1))
            periodStart = start.Format("2006-01-02")
        case "monthly":
            periodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
        }

        // Query current spend
        var currentSpend float64
        query := "SELECT COALESCE(SUM(estimated_cost_usd), 0) FROM sessions WHERE date(started_at) >= ?"
        qArgs := []interface{}{periodStart}

        if pp != "" {
            query += " AND project_path = ?"
            qArgs = append(qArgs, pp)
        }

        s.db.QueryRow(query, qArgs...).Scan(&currentSpend)

        pct := 0.0
        if amountUSD > 0 {
            pct = currentSpend / amountUSD * 100
        }

        status := "ok"
        if pct >= 100 {
            status = "exceeded"
        } else if pct >= 80 {
            status = "warning"
        }

        budgets = append(budgets, map[string]interface{}{
            "id": id, "name": name, "project_path": pp,
            "period": period, "amount_usd": amountUSD,
            "current_spend": currentSpend, "percentage": pct,
            "status": status,
        })
    }

    writeJSON(w, http.StatusOK, map[string]interface{}{"budgets": budgets})
}
```

- [ ] **Step 2: Register routes**

In `internal/server/server.go`:

```go
s.mux.HandleFunc("/api/budgets/status", s.budgetStatus)
s.mux.HandleFunc("/api/budgets/", s.handleBudgetDetail)
s.mux.HandleFunc("/api/budgets", s.handleBudgets)
```

Note: `/api/budgets/status` must be registered before `/api/budgets/` so it matches first.

- [ ] **Step 3: Add API client methods**

In `web/static/js/api.js`:

```js
getBudgets() {
    return API.get('/api/budgets');
},

createBudget(budget) {
    return API.post('/api/budgets', budget);
},

updateBudget(id, budget) {
    return fetch(`/api/budgets/${id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(budget),
    }).then(r => r.json());
},

deleteBudget(id) {
    return fetch(`/api/budgets/${id}`, { method: 'DELETE' }).then(r => r.json());
},

getBudgetStatus() {
    return API.get('/api/budgets/status');
},
```

- [ ] **Step 4: Add Budget section to cost.js**

In `web/static/js/components/cost.js`, fetch budget status and add a "Budgets" management section:

```js
const budgetStatus = await API.getBudgetStatus().catch(() => null);

if (budgetStatus) {
    const budgetSection = document.createElement('div');
    budgetSection.innerHTML = `
        <h2 style="margin:2rem 0 1rem">Budgets</h2>
        <div class="card">
            <div class="card-header" style="display:flex;justify-content:space-between;align-items:center">
                <h3>Active Budgets</h3>
                <button id="add-budget-btn" class="btn btn-primary" style="font-size:0.85rem">Add Budget</button>
            </div>
            <div class="card-body">
                ${budgetStatus.budgets && budgetStatus.budgets.length > 0 ? `
                <table class="table">
                    <thead><tr><th>Name</th><th>Project</th><th>Period</th><th>Budget</th><th>Spent</th><th>Status</th><th></th></tr></thead>
                    <tbody>${budgetStatus.budgets.map(b => {
                        const statusColor = b.status === 'exceeded' ? 'var(--danger)' : b.status === 'warning' ? '#f59e0b' : 'var(--success)';
                        return `<tr>
                            <td>${b.name}</td>
                            <td>${b.project_path || 'All'}</td>
                            <td>${b.period}</td>
                            <td>$${b.amount_usd.toFixed(2)}</td>
                            <td>$${b.current_spend.toFixed(2)}</td>
                            <td>
                                <div style="display:flex;align-items:center;gap:0.5rem">
                                    <div style="flex:1;background:var(--bg-secondary);border-radius:4px;height:8px;overflow:hidden">
                                        <div style="width:${Math.min(b.percentage, 100)}%;height:100%;background:${statusColor}"></div>
                                    </div>
                                    <span style="font-size:0.8rem;color:${statusColor}">${b.percentage.toFixed(0)}%</span>
                                </div>
                            </td>
                            <td><button class="btn btn-secondary delete-budget" data-id="${b.id}" style="font-size:0.75rem;padding:0.15rem 0.5rem">Delete</button></td>
                        </tr>`;
                    }).join('')}</tbody>
                </table>` : '<p class="text-muted">No budgets configured</p>'}
            </div>
        </div>
    `;
    el.appendChild(budgetSection);

    // Delete budget handler
    budgetSection.querySelectorAll('.delete-budget').forEach(btn => {
        btn.addEventListener('click', async () => {
            await API.deleteBudget(btn.dataset.id);
            this.loadData();
        });
    });

    // Add budget handler
    const addBtn = budgetSection.querySelector('#add-budget-btn');
    if (addBtn) {
        addBtn.addEventListener('click', () => {
            const name = prompt('Budget name:');
            if (!name) return;
            const period = prompt('Period (daily/weekly/monthly):', 'daily');
            if (!period) return;
            const amount = parseFloat(prompt('Amount (USD):', '50'));
            if (isNaN(amount)) return;
            API.createBudget({ name, period, amount_usd: amount }).then(() => this.loadData());
        });
    }
}
```

- [ ] **Step 5: Add budget status badges to dashboard.js**

In `web/static/js/components/dashboard.js`, fetch budget status and show warning badges in the stats row:

```js
const budgetStatus = await API.getBudgetStatus().catch(() => null);

// After existing stats cards, add budget warnings
if (budgetStatus && budgetStatus.budgets) {
    const warnings = budgetStatus.budgets.filter(b => b.status === 'warning' || b.status === 'exceeded');
    if (warnings.length > 0) {
        const banner = document.createElement('div');
        banner.style.cssText = 'background:#fef3cd;border:1px solid #ffc107;border-radius:8px;padding:0.75rem 1rem;margin-bottom:1.5rem;color:#856404';
        banner.innerHTML = warnings.map(w =>
            `<strong>${w.name}:</strong> $${w.current_spend.toFixed(2)} / $${w.amount_usd.toFixed(2)} (${w.percentage.toFixed(0)}%) — ${w.status}`
        ).join('<br>');
        el.querySelector('.stats-row')?.insertAdjacentElement('afterend', banner);
    }
}
```

- [ ] **Step 6: Commit**

```bash
git add internal/server/api_budgets.go internal/server/server.go web/static/js/api.js web/static/js/components/cost.js web/static/js/components/dashboard.js
git commit -m "feat: add cost budgets with status tracking and alerts"
```

---

## Task 13: Git Integration (CLI + API + Frontend)

**Files:**
- Create: `internal/cli/gitsync.go`
- Create: `internal/server/api_gitsync.go`
- Modify: `cmd/claude-monitor/main.go`
- Modify: `internal/server/server.go`
- Modify: `web/static/js/api.js`
- Modify: `web/static/js/components/sessions.js`

- [ ] **Step 1: Create gitsync.go CLI command**

```go
package cli

import (
    "bufio"
    "database/sql"
    "flag"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strconv"
    "strings"
    "time"

    "github.com/szaher/claude-monitor/internal/config"
    "github.com/szaher/claude-monitor/internal/db"
    "github.com/szaher/claude-monitor/internal/models"
)

func GitSync(args []string) error {
    fs := flag.NewFlagSet("git-sync", flag.ExitOnError)
    repoPath := fs.String("repo", "", "Sync a specific repo only")
    since := fs.String("since", "", "Only look at commits after this date (YYYY-MM-DD)")
    if err := fs.Parse(args); err != nil {
        return err
    }

    home, _ := os.UserHomeDir()
    baseDir := filepath.Join(home, ".claude-monitor")
    cfg, err := config.Load(filepath.Join(baseDir, "config.yaml"))
    if err != nil {
        return fmt.Errorf("load config: %w", err)
    }

    dbPath := cfg.Storage.DatabasePath
    if dbPath == "" {
        dbPath = filepath.Join(baseDir, "claude-monitor.db")
    }
    database, err := db.InitDB(dbPath)
    if err != nil {
        return fmt.Errorf("init database: %w", err)
    }
    defer database.Close()

    return syncGitCommits(database, *repoPath, *since)
}

func syncGitCommits(database *sql.DB, filterRepo, sinceDate string) error {
    query := "SELECT id, cwd, started_at, ended_at FROM sessions WHERE cwd IS NOT NULL AND cwd != ''"
    var args []interface{}
    if filterRepo != "" {
        query += " AND cwd = ?"
        args = append(args, filterRepo)
    }
    if sinceDate != "" {
        query += " AND started_at >= ?"
        args = append(args, sinceDate)
    }

    rows, err := database.Query(query, args...)
    if err != nil {
        return fmt.Errorf("query sessions: %w", err)
    }
    defer rows.Close()

    type sessionInfo struct {
        ID      string
        CWD     string
        Start   time.Time
        End     time.Time
    }

    repoSessions := map[string][]sessionInfo{}
    for rows.Next() {
        var id, cwd, startStr string
        var endStr sql.NullString
        rows.Scan(&id, &cwd, &startStr, &endStr)

        start, _ := time.Parse(time.RFC3339, startStr)
        end := time.Now().UTC()
        if endStr.Valid {
            end, _ = time.Parse(time.RFC3339, endStr.String)
        }

        repoSessions[cwd] = append(repoSessions[cwd], sessionInfo{
            ID: id, CWD: cwd, Start: start, End: end,
        })
    }

    totalCommits := 0
    for repo, sessions := range repoSessions {
        // Find time range
        earliest := sessions[0].Start
        latest := sessions[0].End
        for _, s := range sessions {
            if s.Start.Before(earliest) { earliest = s.Start }
            if s.End.After(latest) { latest = s.End }
        }

        // Find git repo root
        gitRoot := findGitRoot(repo)
        if gitRoot == "" {
            continue
        }

        // Run git log
        cmd := exec.Command("git", "-C", gitRoot, "log",
            "--format=%H|%s|%an|%aI",
            "--after="+earliest.Add(-time.Minute).Format(time.RFC3339),
            "--before="+latest.Add(time.Minute).Format(time.RFC3339),
            "--numstat",
        )
        out, err := cmd.Output()
        if err != nil {
            fmt.Fprintf(os.Stderr, "Warning: git log failed for %s: %v\n", repo, err)
            continue
        }

        // Parse git log output
        scanner := bufio.NewScanner(strings.NewReader(string(out)))
        var currentHash, currentMsg, currentAuthor string
        var currentTime time.Time
        var filesChanged, insertions, deletions int

        flushCommit := func() {
            if currentHash == "" { return }
            // Match to session by timestamp
            for _, s := range sessions {
                if (currentTime.Equal(s.Start) || currentTime.After(s.Start)) &&
                   (currentTime.Equal(s.End) || currentTime.Before(s.End)) {
                    commit := &models.SessionCommit{
                        SessionID:     s.ID,
                        CommitHash:    currentHash,
                        CommitMessage: currentMsg,
                        Author:        currentAuthor,
                        FilesChanged:  filesChanged,
                        Insertions:    insertions,
                        Deletions:     deletions,
                        CommittedAt:   currentTime,
                    }
                    db.InsertSessionCommit(database, commit)
                    totalCommits++
                    break
                }
            }
        }

        for scanner.Scan() {
            line := scanner.Text()
            if line == "" {
                continue
            }

            parts := strings.SplitN(line, "|", 4)
            if len(parts) == 4 && len(parts[0]) == 40 {
                flushCommit()
                currentHash = parts[0]
                currentMsg = parts[1]
                currentAuthor = parts[2]
                currentTime, _ = time.Parse(time.RFC3339, parts[3])
                filesChanged = 0
                insertions = 0
                deletions = 0
            } else if strings.Contains(line, "\t") {
                // numstat line: insertions deletions filename
                numParts := strings.Fields(line)
                if len(numParts) >= 2 {
                    ins, _ := strconv.Atoi(numParts[0])
                    del, _ := strconv.Atoi(numParts[1])
                    insertions += ins
                    deletions += del
                    filesChanged++
                }
            }
        }
        flushCommit()
    }

    fmt.Printf("Synced %d commits across %d repos\n", totalCommits, len(repoSessions))
    return nil
}

func findGitRoot(path string) string {
    cmd := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
    out, err := cmd.Output()
    if err != nil {
        return ""
    }
    return strings.TrimSpace(string(out))
}
```

- [ ] **Step 2: Create api_gitsync.go for session commits API**

```go
package server

import (
    "database/sql"
    "net/http"
    "os/exec"
    "strings"
    "time"

    "github.com/szaher/claude-monitor/internal/db"
    "github.com/szaher/claude-monitor/internal/models"
)

func (s *Server) handleSessionCommits(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeError(w, http.StatusMethodNotAllowed, "method not allowed")
        return
    }

    path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
    parts := strings.Split(path, "/")
    if len(parts) < 2 {
        writeError(w, http.StatusBadRequest, "invalid path")
        return
    }
    sessionID := parts[0]

    rows, err := s.db.Query(`
        SELECT commit_hash, commit_message, author, files_changed, insertions, deletions, committed_at
        FROM session_commits WHERE session_id = ?
        ORDER BY committed_at ASC
    `, sessionID)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "query commits: "+err.Error())
        return
    }
    defer rows.Close()

    commits := []map[string]interface{}{}
    for rows.Next() {
        var hash, message, author, committedAt string
        var filesChanged, insertions, deletions int
        rows.Scan(&hash, &message, &author, &filesChanged, &insertions, &deletions, &committedAt)
        commits = append(commits, map[string]interface{}{
            "commit_hash":    hash,
            "commit_message": message,
            "author":         author,
            "files_changed":  filesChanged,
            "insertions":     insertions,
            "deletions":      deletions,
            "committed_at":   committedAt,
        })
    }

    writeJSON(w, http.StatusOK, map[string]interface{}{"commits": commits})
}

func (s *Server) handleGitSync(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        writeError(w, http.StatusMethodNotAllowed, "method not allowed")
        return
    }

    // Get session ID from query param if provided
    sessionID := r.URL.Query().Get("session_id")
    repo := ""

    if sessionID != "" {
        var cwd sql.NullString
        s.db.QueryRow("SELECT cwd FROM sessions WHERE id = ?", sessionID).Scan(&cwd)
        if cwd.Valid {
            repo = cwd.String
        }
    }

    // Run git sync in background (simplified — just sync the given repo)
    go func() {
        database := s.db
        syncGitCommitsForRepo(database, repo)
    }()

    writeJSON(w, http.StatusAccepted, map[string]string{"status": "syncing"})
}

func syncGitCommitsForRepo(database *sql.DB, repo string) {
    // Import the CLI sync function with the repo filter
    // This calls the same syncGitCommits function from the CLI package
    query := "SELECT id, cwd, started_at, ended_at FROM sessions WHERE cwd IS NOT NULL AND cwd != ''"
    var args []interface{}
    if repo != "" {
        query += " AND cwd = ?"
        args = append(args, repo)
    }

    rows, err := database.Query(query, args...)
    if err != nil {
        return
    }
    defer rows.Close()

    type sessionInfo struct {
        ID    string
        CWD   string
        Start time.Time
        End   time.Time
    }

    var sessions []sessionInfo
    for rows.Next() {
        var id, cwd, startStr string
        var endStr sql.NullString
        rows.Scan(&id, &cwd, &startStr, &endStr)
        start, _ := time.Parse(time.RFC3339, startStr)
        end := time.Now().UTC()
        if endStr.Valid {
            end, _ = time.Parse(time.RFC3339, endStr.String)
        }
        sessions = append(sessions, sessionInfo{ID: id, CWD: cwd, Start: start, End: end})
    }

    for _, s := range sessions {
        gitRoot := findGitRoot(s.CWD)
        if gitRoot == "" {
            continue
        }
        cmd := exec.Command("git", "-C", gitRoot, "log",
            "--format=%H|%s|%an|%aI",
            "--after="+s.Start.Add(-time.Minute).Format(time.RFC3339),
            "--before="+s.End.Add(time.Minute).Format(time.RFC3339),
        )
        out, err := cmd.Output()
        if err != nil {
            continue
        }
        for _, line := range strings.Split(string(out), "\n") {
            parts := strings.SplitN(line, "|", 4)
            if len(parts) != 4 || len(parts[0]) != 40 {
                continue
            }
            commitTime, _ := time.Parse(time.RFC3339, parts[3])
            commit := &models.SessionCommit{
                SessionID:     s.ID,
                CommitHash:    parts[0],
                CommitMessage: parts[1],
                Author:        parts[2],
                CommittedAt:   commitTime,
            }
            db.InsertSessionCommit(database, commit)
        }
    }
}

func findGitRoot(path string) string {
    cmd := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
    out, err := cmd.Output()
    if err != nil {
        return ""
    }
    return strings.TrimSpace(string(out))
}
```

- [ ] **Step 3: Register main.go command**

In `cmd/claude-monitor/main.go`, add the `git-sync` case:

```go
case "git-sync":
    if err := cli.GitSync(os.Args[2:]); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
```

- [ ] **Step 4: Register API routes**

In `internal/server/server.go`:

```go
s.mux.HandleFunc("/api/git-sync", s.handleGitSync)
```

Note: `/api/sessions/{id}/commits` is already handled by the `handleSessionDetail` router from Task 9.

- [ ] **Step 5: Add API client methods**

In `web/static/js/api.js`:

```js
getSessionCommits(sessionId) {
    return API.get(`/api/sessions/${sessionId}/commits`);
},

triggerGitSync(sessionId) {
    return API.post('/api/git-sync', { session_id: sessionId });
},
```

- [ ] **Step 6: Add Git Commits section to session detail**

In `web/static/js/components/sessions.js`, fetch commits in `loadDetail()`:

```js
const commitsData = await API.getSessionCommits(this.selectedSession).catch(() => null);

if (commitsData) {
    const commits = commitsData.commits || [];
    const commitsHtml = `
    <div class="card" style="margin-bottom:1.5rem">
        <div class="card-header" style="display:flex;justify-content:space-between;align-items:center">
            <h3>Git Commits (${commits.length})</h3>
            <button id="git-sync-btn" class="btn btn-secondary" style="font-size:0.8rem">Sync Git</button>
        </div>
        <div class="card-body">
            ${commits.length > 0 ? `
            <table class="table">
                <thead><tr><th>Hash</th><th>Message</th><th>Changes</th><th>Time</th></tr></thead>
                <tbody>${commits.map(c => `
                    <tr>
                        <td style="font-family:monospace;font-size:0.85rem">${c.commit_hash.substring(0, 7)}</td>
                        <td>${this._esc(c.commit_message)}</td>
                        <td style="white-space:nowrap">
                            <span style="color:var(--success)">+${c.insertions}</span>
                            <span style="color:var(--danger)">-${c.deletions}</span>
                            <span style="color:var(--text-secondary)">(${c.files_changed} files)</span>
                        </td>
                        <td style="white-space:nowrap">${new Date(c.committed_at).toLocaleString()}</td>
                    </tr>
                `).join('')}</tbody>
            </table>` : '<p class="text-muted">No commits found. Click "Sync Git" to scan.</p>'}
        </div>
    </div>`;
    el.insertAdjacentHTML('beforeend', commitsHtml);

    el.querySelector('#git-sync-btn')?.addEventListener('click', async () => {
        await API.triggerGitSync(this.selectedSession);
        App.toast('Git sync started', 'info');
        setTimeout(() => this.loadDetail(), 3000);
    });
}
```

- [ ] **Step 7: Commit**

```bash
git add internal/cli/gitsync.go internal/server/api_gitsync.go cmd/claude-monitor/main.go internal/server/server.go web/static/js/api.js web/static/js/components/sessions.js
git commit -m "feat: add git integration with commit tracking"
```

---

## Task 14: Final Build Verification and Integration Test

**Files:** None new

- [ ] **Step 1: Run full Go build**

Run: `cd /Users/szaher/go/src/github.com/szaher/claude-monitor && go build ./...`
Expected: Build succeeds with no errors

- [ ] **Step 2: Run full test suite**

Run: `cd /Users/szaher/go/src/github.com/szaher/claude-monitor && go test ./... -v`
Expected: All tests PASS

- [ ] **Step 3: Verify all new routes are registered**

Run: `cd /Users/szaher/go/src/github.com/szaher/claude-monitor && grep 'HandleFunc' internal/server/server.go | wc -l`
Expected: Should be ~23+ routes (original 17 + 6+ new)

- [ ] **Step 4: Start server and verify new endpoints respond**

Run: `cd /Users/szaher/go/src/github.com/szaher/claude-monitor && go run ./cmd/claude-monitor serve --no-browser &`
Then test endpoints:
```bash
curl -s http://localhost:3000/api/stats/errors | head -c 100
curl -s http://localhost:3000/api/stats/token-efficiency | head -c 100
curl -s http://localhost:3000/api/stats/prompt-patterns | head -c 100
curl -s http://localhost:3000/api/budgets | head -c 100
curl -s http://localhost:3000/api/tags | head -c 100
```
Expected: JSON responses (no 404s)

- [ ] **Step 5: Re-import data to populate new fields**

Run: `cd /Users/szaher/go/src/github.com/szaher/claude-monitor && go run ./cmd/claude-monitor import`
Expected: Import completes successfully

- [ ] **Step 6: Commit any fixes**

```bash
git add -A
git commit -m "fix: integration test fixes"
```
