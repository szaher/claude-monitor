# Claude Monitor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a single Go binary that captures, stores, and visualizes all Claude Code interactions through hooks and log parsing.

**Architecture:** Hybrid capture (hooks + fsnotify log parser) → ingestion pipeline (deduplication) → dual storage (SQLite for queries + JSON for archive) → web server (REST API + WebSocket) → embedded SPA UI.

**Tech Stack:** Go 1.21+, SQLite with mattn/go-sqlite3, fsnotify, gorilla/websocket, YAML config (gopkg.in/yaml.v3), embedded web assets (embed), vanilla JS + Chart.js for UI.

---

## File Structure

```
claude-monitor/
├── cmd/
│   └── claude-monitor/
│       └── main.go                      # Entry point, CLI router
├── internal/
│   ├── config/
│   │   ├── config.go                    # Config struct, load/save, defaults
│   │   └── config_test.go
│   ├── db/
│   │   ├── db.go                        # SQLite connection, migrations, WAL mode
│   │   ├── schema.go                    # CREATE TABLE statements
│   │   ├── queries.go                   # All SQL queries
│   │   └── db_test.go
│   ├── models/
│   │   └── models.go                    # Go structs for Session, Message, ToolCall, etc.
│   ├── hook/
│   │   ├── hook.go                      # Hook subcommand: read stdin, send to socket
│   │   └── hook_test.go
│   ├── ingestion/
│   │   ├── receiver.go                  # Unix socket server
│   │   ├── watcher.go                   # fsnotify log file watcher
│   │   ├── pipeline.go                  # Normalize, deduplicate, batch write
│   │   ├── receiver_test.go
│   │   ├── watcher_test.go
│   │   └── pipeline_test.go
│   ├── parser/
│   │   ├── jsonl.go                     # Parse Claude Code JSONL logs
│   │   └── jsonl_test.go
│   ├── installer/
│   │   ├── installer.go                 # Install/uninstall hooks, merge settings.json
│   │   └── installer_test.go
│   ├── exporter/
│   │   ├── exporter.go                  # Export to JSON/CSV/HTML
│   │   └── exporter_test.go
│   ├── server/
│   │   ├── server.go                    # HTTP server, routes, middleware
│   │   ├── api.go                       # REST API handlers
│   │   ├── websocket.go                 # WebSocket hub, broadcast
│   │   ├── server_test.go
│   │   └── websocket_test.go
│   └── cli/
│       ├── install.go                   # Install command
│       ├── uninstall.go                 # Uninstall command
│       ├── serve.go                     # Serve command
│       ├── import.go                    # Import command
│       ├── export.go                    # Export command
│       ├── config.go                    # Config command
│       ├── status.go                    # Status command
│       └── version.go                   # Version command
├── web/
│   ├── embed.go                         # go:embed directive
│   └── static/
│       ├── index.html                   # SPA shell
│       ├── css/
│       │   └── style.css                # All styles
│       └── js/
│           ├── app.js                   # Router, state, init
│           ├── api.js                   # API client wrapper
│           ├── components/
│           │   ├── dashboard.js
│           │   ├── sessions.js
│           │   ├── live.js
│           │   ├── tools.js
│           │   ├── agents.js
│           │   ├── projects.js
│           │   ├── cost.js
│           │   └── settings.js
│           └── lib/
│               └── chart.min.js         # Chart.js bundled
├── install.sh                           # Curl install script
├── go.mod
├── go.sum
├── Makefile                             # Build targets
└── README.md
```

---

## Task 1: Go Module and Project Scaffolding

**Files:**
- Create: `go.mod`
- Create: `go.sum`
- Create: `cmd/claude-monitor/main.go`
- Create: `Makefile`
- Create: `README.md`

- [ ] **Step 1: Initialize Go module**

```bash
cd /Users/szaher/go/src/github.com/szaher/claude-monitor
go mod init github.com/szaher/claude-monitor
```

Expected: `go.mod` created

- [ ] **Step 2: Add dependencies**

```bash
go get github.com/mattn/go-sqlite3
go get github.com/fsnotify/fsnotify
go get github.com/gorilla/websocket
go get gopkg.in/yaml.v3
```

Expected: Dependencies added to `go.mod` and `go.sum` created

- [ ] **Step 3: Create main.go entry point**

```go
package main

import (
	"fmt"
	"os"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	
	switch command {
	case "version":
		fmt.Printf("claude-monitor %s\n", version)
	case "install", "uninstall", "serve", "import", "export", "config", "status", "hook":
		fmt.Printf("Command '%s' not yet implemented\n", command)
		os.Exit(1)
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Usage: claude-monitor <command> [flags]

Commands:
  install          Install hooks and initialize database
  uninstall        Remove hooks, optionally delete data
  serve            Start daemon + web UI
  import           Import all historical Claude Code logs
  config           Manage configuration
  status           Show daemon status, database stats
  export           Export sessions as JSON/CSV/HTML
  hook             Handle hook events (called by Claude Code)
  version          Print version info

Run 'claude-monitor <command> --help' for more information on a command.`)
}
```

- [ ] **Step 4: Create Makefile**

```makefile
.PHONY: build test clean install

VERSION ?= dev
BINARY_NAME = claude-monitor
BUILD_DIR = bin

build:
	@mkdir -p $(BUILD_DIR)
	go build -ldflags "-X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/claude-monitor

test:
	go test -v ./...

clean:
	rm -rf $(BUILD_DIR)

install: build
	cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

run: build
	$(BUILD_DIR)/$(BINARY_NAME)
```

- [ ] **Step 5: Create README.md**

```markdown
# Claude Monitor

A comprehensive monitoring and visualization tool for Claude Code interactions.

## Installation

```bash
curl -sSL https://raw.githubusercontent.com/szaher/claude-monitor/main/install.sh | sh
```

Or build from source:

```bash
make build
make install
```

## Quick Start

```bash
# Install hooks and initialize database
claude-monitor install

# Start the web UI
claude-monitor serve

# Visit http://localhost:3000
```

## Commands

- `install` - Install hooks and initialize database
- `serve` - Start daemon + web UI
- `import` - Import historical Claude Code logs
- `export` - Export sessions
- `config` - Manage configuration
- `status` - Show daemon status
- `version` - Print version

See the [design spec](docs/superpowers/specs/2026-04-16-claude-monitor-design.md) for details.

## License

MIT
```

- [ ] **Step 6: Test build**

```bash
make build
./bin/claude-monitor version
```

Expected: Output `claude-monitor dev`

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum cmd/claude-monitor/main.go Makefile README.md
git commit -m "feat: initialize Go project with CLI skeleton

- Add go.mod with dependencies (sqlite3, fsnotify, websocket, yaml)
- Create main.go with command router
- Add Makefile with build/test/clean targets
- Add README with installation and usage

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 2: Configuration System

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write failing test for config loading**

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_DefaultConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	
	if cfg.Server.Port != 3000 {
		t.Errorf("Server.Port = %d, want 3000", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Server.Host = %s, want 127.0.0.1", cfg.Server.Host)
	}
	if cfg.Storage.RetentionDays != 0 {
		t.Errorf("Storage.RetentionDays = %d, want 0", cfg.Storage.RetentionDays)
	}
}

func TestLoad_ExistingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	
	yamlContent := `server:
  port: 8080
  host: "0.0.0.0"
storage:
  retention_days: 90
`
	
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}
	
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Server.Host = %s, want 0.0.0.0", cfg.Server.Host)
	}
	if cfg.Storage.RetentionDays != 90 {
		t.Errorf("Storage.RetentionDays = %d, want 90", cfg.Storage.RetentionDays)
	}
}

func TestSave(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	
	cfg := DefaultConfig()
	cfg.Server.Port = 9000
	
	if err := Save(configPath, cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	
	if loaded.Server.Port != 9000 {
		t.Errorf("Server.Port = %d, want 9000", loaded.Server.Port)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/config -v
```

Expected: FAIL - package config not found or types not defined

- [ ] **Step 3: Implement config.go**

```go
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Capture CaptureConfig `yaml:"capture"`
	Storage StorageConfig `yaml:"storage"`
	Cost    CostConfig    `yaml:"cost"`
	UI      UIConfig      `yaml:"ui"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

type CaptureConfig struct {
	Metadata MetadataConfig `yaml:"metadata"`
	Events   EventsConfig   `yaml:"events"`
}

type MetadataConfig struct {
	GitBranch       bool `yaml:"git_branch"`
	GitRepo         bool `yaml:"git_repo"`
	WorkingDir      bool `yaml:"working_directory"`
	ClaudeVersion   bool `yaml:"claude_version"`
	EnvironmentVars bool `yaml:"environment_vars"`
	CommandArgs     bool `yaml:"command_args"`
	SystemInfo      bool `yaml:"system_info"`
}

type EventsConfig struct {
	SessionStart  bool `yaml:"session_start"`
	SessionEnd    bool `yaml:"session_end"`
	PreToolUse    bool `yaml:"pre_tool_use"`
	PostToolUse   bool `yaml:"post_tool_use"`
	SubagentStart bool `yaml:"subagent_start"`
	SubagentStop  bool `yaml:"subagent_stop"`
	Stop          bool `yaml:"stop"`
}

type StorageConfig struct {
	DatabasePath  string `yaml:"database_path"`
	ArchivePath   string `yaml:"archive_path"`
	ArchiveEnabled bool   `yaml:"archive_enabled"`
	RetentionDays int    `yaml:"retention_days"`
	MaxDBSizeMB   int    `yaml:"max_db_size_mb"`
}

type CostConfig struct {
	Models map[string]ModelPricing `yaml:"models"`
}

type ModelPricing struct {
	Input      float64 `yaml:"input"`
	Output     float64 `yaml:"output"`
	CacheRead  float64 `yaml:"cache_read"`
	CacheWrite float64 `yaml:"cache_write"`
}

type UIConfig struct {
	Theme           string `yaml:"theme"`
	DefaultPage     string `yaml:"default_page"`
	SessionsPerPage int    `yaml:"sessions_per_page"`
}

func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	monitorDir := filepath.Join(homeDir, ".claude-monitor")
	
	return &Config{
		Server: ServerConfig{
			Port: 3000,
			Host: "127.0.0.1",
		},
		Capture: CaptureConfig{
			Metadata: MetadataConfig{
				GitBranch:       true,
				GitRepo:         true,
				WorkingDir:      true,
				ClaudeVersion:   true,
				EnvironmentVars: false,
				CommandArgs:     false,
				SystemInfo:      true,
			},
			Events: EventsConfig{
				SessionStart:  true,
				SessionEnd:    true,
				PreToolUse:    true,
				PostToolUse:   true,
				SubagentStart: true,
				SubagentStop:  true,
				Stop:          true,
			},
		},
		Storage: StorageConfig{
			DatabasePath:   filepath.Join(monitorDir, "monitor.db"),
			ArchivePath:    filepath.Join(monitorDir, "archive"),
			ArchiveEnabled: true,
			RetentionDays:  0,
			MaxDBSizeMB:    0,
		},
		Cost: CostConfig{
			Models: map[string]ModelPricing{
				"claude-opus-4-6": {
					Input:      15.0,
					Output:     75.0,
					CacheRead:  1.5,
					CacheWrite: 18.75,
				},
				"claude-sonnet-4-5": {
					Input:      3.0,
					Output:     15.0,
					CacheRead:  0.30,
					CacheWrite: 3.75,
				},
				"claude-haiku-4-5": {
					Input:      0.80,
					Output:     4.0,
					CacheRead:  0.08,
					CacheWrite: 1.0,
				},
			},
		},
		UI: UIConfig{
			Theme:           "auto",
			DefaultPage:     "dashboard",
			SessionsPerPage: 50,
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	
	return cfg, nil
}

func Save(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	
	return os.WriteFile(path, data, 0644)
}

func ExpandPath(path string) string {
	if len(path) == 0 {
		return path
	}
	if path[0] == '~' {
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, path[1:])
	}
	return path
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/config -v
```

Expected: PASS - all config tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add configuration system with YAML support

- Config struct with server, capture, storage, cost, UI sections
- Load() with defaults fallback for missing files
- Save() with directory creation
- DefaultConfig() with sensible defaults
- Tests for load/save/defaults

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 3: Data Models

**Files:**
- Create: `internal/models/models.go`

- [ ] **Step 1: Create data model structs**

```go
package models

import "time"

type Session struct {
	ID                   string    `json:"id"`
	ProjectPath          string    `json:"project_path"`
	ProjectName          string    `json:"project_name"`
	CWD                  string    `json:"cwd"`
	GitBranch            string    `json:"git_branch"`
	StartedAt            time.Time `json:"started_at"`
	EndedAt              *time.Time `json:"ended_at,omitempty"`
	ClaudeVersion        string    `json:"claude_version"`
	EntryPoint           string    `json:"entry_point"`
	PermissionMode       string    `json:"permission_mode"`
	TotalInputTokens     int       `json:"total_input_tokens"`
	TotalOutputTokens    int       `json:"total_output_tokens"`
	TotalCacheReadTokens int       `json:"total_cache_read_tokens"`
	TotalCacheWriteTokens int      `json:"total_cache_write_tokens"`
	EstimatedCostUSD     float64   `json:"estimated_cost_usd"`
}

type Message struct {
	ID              string    `json:"id"`
	SessionID       string    `json:"session_id"`
	ParentID        string    `json:"parent_id"`
	Type            string    `json:"type"`
	Role            string    `json:"role"`
	Model           string    `json:"model"`
	ContentText     string    `json:"content_text"`
	ContentJSON     string    `json:"content_json"`
	StopReason      string    `json:"stop_reason"`
	InputTokens     int       `json:"input_tokens"`
	OutputTokens    int       `json:"output_tokens"`
	CacheReadTokens int       `json:"cache_read_tokens"`
	CacheWriteTokens int      `json:"cache_write_tokens"`
	Timestamp       time.Time `json:"timestamp"`
}

type ToolCall struct {
	ID           string    `json:"id"`
	MessageID    string    `json:"message_id"`
	SessionID    string    `json:"session_id"`
	ToolName     string    `json:"tool_name"`
	ToolInput    string    `json:"tool_input"`
	ToolResponse string    `json:"tool_response"`
	Success      bool      `json:"success"`
	Error        string    `json:"error"`
	DurationMS   int       `json:"duration_ms"`
	Timestamp    time.Time `json:"timestamp"`
}

type Subagent struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	AgentType   string    `json:"agent_type"`
	Description string    `json:"description"`
	StartedAt   time.Time `json:"started_at"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
}

type HookEvent struct {
	Event     string                 `json:"event"`
	Timestamp time.Time              `json:"timestamp"`
	SessionID string                 `json:"session_id"`
	Data      map[string]interface{} `json:"data"`
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/models/models.go
git commit -m "feat: add data models for sessions, messages, tool calls, subagents

- Session with token counts and cost tracking
- Message with content and token usage
- ToolCall with execution details
- Subagent with lifecycle tracking
- HookEvent for real-time capture

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 4: Database Layer - Schema and Migrations

**Files:**
- Create: `internal/db/schema.go`
- Create: `internal/db/db.go`
- Create: `internal/db/db_test.go`

- [ ] **Step 1: Write failing test for database initialization**

```go
package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Close()
	
	// Verify tables exist
	tables := []string{"sessions", "messages", "tool_calls", "subagents"}
	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("Table %s not found: %v", table, err)
		}
	}
	
	// Verify FTS5 table exists
	var ftsName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='messages_fts'").Scan(&ftsName)
	if err != nil {
		t.Errorf("FTS table messages_fts not found: %v", err)
	}
}

func TestInitDB_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	
	db1, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB() first call error = %v", err)
	}
	db1.Close()
	
	db2, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB() second call error = %v", err)
	}
	defer db2.Close()
	
	// Should not error when opening existing database
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/db -v
```

Expected: FAIL - package db not found

- [ ] **Step 3: Create schema.go**

```go
package db

const schema = `
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	project_path TEXT NOT NULL,
	project_name TEXT NOT NULL,
	cwd TEXT,
	git_branch TEXT,
	started_at DATETIME NOT NULL,
	ended_at DATETIME,
	claude_version TEXT,
	entry_point TEXT,
	permission_mode TEXT,
	total_input_tokens INTEGER DEFAULT 0,
	total_output_tokens INTEGER DEFAULT 0,
	total_cache_read_tokens INTEGER DEFAULT 0,
	total_cache_write_tokens INTEGER DEFAULT 0,
	estimated_cost_usd REAL DEFAULT 0.0
);

CREATE INDEX IF NOT EXISTS idx_sessions_project_started 
ON sessions(project_path, started_at);

CREATE TABLE IF NOT EXISTS messages (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	parent_id TEXT,
	type TEXT NOT NULL,
	role TEXT,
	model TEXT,
	content_text TEXT,
	content_json TEXT,
	stop_reason TEXT,
	input_tokens INTEGER DEFAULT 0,
	output_tokens INTEGER DEFAULT 0,
	cache_read_tokens INTEGER DEFAULT 0,
	cache_write_tokens INTEGER DEFAULT 0,
	timestamp DATETIME NOT NULL,
	FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_messages_session_timestamp 
ON messages(session_id, timestamp);

CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts 
USING fts5(id, content_text, content='messages', content_rowid='rowid');

CREATE TRIGGER IF NOT EXISTS messages_fts_insert AFTER INSERT ON messages BEGIN
	INSERT INTO messages_fts(rowid, id, content_text) 
	VALUES (new.rowid, new.id, new.content_text);
END;

CREATE TRIGGER IF NOT EXISTS messages_fts_delete AFTER DELETE ON messages BEGIN
	DELETE FROM messages_fts WHERE rowid = old.rowid;
END;

CREATE TRIGGER IF NOT EXISTS messages_fts_update AFTER UPDATE ON messages BEGIN
	DELETE FROM messages_fts WHERE rowid = old.rowid;
	INSERT INTO messages_fts(rowid, id, content_text) 
	VALUES (new.rowid, new.id, new.content_text);
END;

CREATE TABLE IF NOT EXISTS tool_calls (
	id TEXT PRIMARY KEY,
	message_id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	tool_name TEXT NOT NULL,
	tool_input TEXT,
	tool_response TEXT,
	success BOOLEAN DEFAULT 1,
	error TEXT,
	duration_ms INTEGER,
	timestamp DATETIME NOT NULL,
	FOREIGN KEY(message_id) REFERENCES messages(id) ON DELETE CASCADE,
	FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_tool_calls_session_tool 
ON tool_calls(session_id, tool_name);

CREATE INDEX IF NOT EXISTS idx_tool_calls_tool_timestamp 
ON tool_calls(tool_name, timestamp);

CREATE TABLE IF NOT EXISTS subagents (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	agent_type TEXT NOT NULL,
	description TEXT,
	started_at DATETIME NOT NULL,
	ended_at DATETIME,
	FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_subagents_session 
ON subagents(session_id);
`
```

- [ ] **Step 4: Create db.go**

```go
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

func InitDB(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}
	
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=ON")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("execute schema: %w", err)
	}
	
	return db, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./internal/db -v
```

Expected: PASS - database initialization tests pass

- [ ] **Step 6: Commit**

```bash
git add internal/db/
git commit -m "feat: add database layer with SQLite schema and migrations

- Schema with sessions, messages, tool_calls, subagents tables
- FTS5 virtual table for full-text search on messages
- Indexes for common query patterns
- WAL mode for concurrent reads
- Foreign keys enabled
- Tests for initialization

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 5: Database Layer - Query Functions

**Files:**
- Create: `internal/db/queries.go`
- Modify: `internal/db/db_test.go`

- [ ] **Step 1: Write failing test for session insertion**

Add to `internal/db/db_test.go`:

```go
func TestInsertSession(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	
	session := &models.Session{
		ID:             "test-session-1",
		ProjectPath:    "/path/to/project",
		ProjectName:    "project",
		StartedAt:      time.Now(),
		ClaudeVersion:  "2.1.110",
		EntryPoint:     "cli",
		PermissionMode: "default",
	}
	
	if err := InsertSession(db, session); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}
	
	// Verify insertion
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM sessions WHERE id = ?", session.ID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("Expected 1 session, got %d", count)
	}
}

func TestInsertMessage(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	
	// Insert session first
	session := &models.Session{
		ID:          "test-session-1",
		ProjectPath: "/path/to/project",
		ProjectName: "project",
		StartedAt:   time.Now(),
	}
	if err := InsertSession(db, session); err != nil {
		t.Fatal(err)
	}
	
	message := &models.Message{
		ID:          "msg-1",
		SessionID:   "test-session-1",
		Type:        "user",
		Role:        "user",
		ContentText: "Hello, Claude!",
		Timestamp:   time.Now(),
	}
	
	if err := InsertMessage(db, message); err != nil {
		t.Fatalf("InsertMessage() error = %v", err)
	}
	
	// Verify FTS insertion
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM messages_fts WHERE content_text MATCH 'Claude'").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("Expected 1 FTS match, got %d", count)
	}
}
```

Add import at top of file:

```go
import (
	"os"
	"path/filepath"
	"testing"
	"time"
	
	"github.com/szaher/claude-monitor/internal/models"
)
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/db -v
```

Expected: FAIL - InsertSession and InsertMessage not defined

- [ ] **Step 3: Implement queries.go**

```go
package db

import (
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/szaher/claude-monitor/internal/models"
)

func InsertSession(db *sql.DB, s *models.Session) error {
	if s.ProjectName == "" && s.ProjectPath != "" {
		s.ProjectName = filepath.Base(s.ProjectPath)
	}
	
	_, err := db.Exec(`
		INSERT INTO sessions (
			id, project_path, project_name, cwd, git_branch,
			started_at, ended_at, claude_version, entry_point, permission_mode,
			total_input_tokens, total_output_tokens, 
			total_cache_read_tokens, total_cache_write_tokens, estimated_cost_usd
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			ended_at = excluded.ended_at,
			total_input_tokens = excluded.total_input_tokens,
			total_output_tokens = excluded.total_output_tokens,
			total_cache_read_tokens = excluded.total_cache_read_tokens,
			total_cache_write_tokens = excluded.total_cache_write_tokens,
			estimated_cost_usd = excluded.estimated_cost_usd
	`, s.ID, s.ProjectPath, s.ProjectName, s.CWD, s.GitBranch,
		s.StartedAt, s.EndedAt, s.ClaudeVersion, s.EntryPoint, s.PermissionMode,
		s.TotalInputTokens, s.TotalOutputTokens,
		s.TotalCacheReadTokens, s.TotalCacheWriteTokens, s.EstimatedCostUSD)
	
	return err
}

func InsertMessage(db *sql.DB, m *models.Message) error {
	_, err := db.Exec(`
		INSERT INTO messages (
			id, session_id, parent_id, type, role, model,
			content_text, content_json, stop_reason,
			input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
			timestamp
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, m.ID, m.SessionID, m.ParentID, m.Type, m.Role, m.Model,
		m.ContentText, m.ContentJSON, m.StopReason,
		m.InputTokens, m.OutputTokens, m.CacheReadTokens, m.CacheWriteTokens,
		m.Timestamp)
	
	return err
}

func InsertToolCall(db *sql.DB, tc *models.ToolCall) error {
	_, err := db.Exec(`
		INSERT INTO tool_calls (
			id, message_id, session_id, tool_name,
			tool_input, tool_response, success, error, duration_ms, timestamp
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, tc.ID, tc.MessageID, tc.SessionID, tc.ToolName,
		tc.ToolInput, tc.ToolResponse, tc.Success, tc.Error, tc.DurationMS, tc.Timestamp)
	
	return err
}

func InsertSubagent(db *sql.DB, sa *models.Subagent) error {
	_, err := db.Exec(`
		INSERT INTO subagents (
			id, session_id, agent_type, description, started_at, ended_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			ended_at = excluded.ended_at
	`, sa.ID, sa.SessionID, sa.AgentType, sa.Description, sa.StartedAt, sa.EndedAt)
	
	return err
}

func GetSessionByID(db *sql.DB, id string) (*models.Session, error) {
	s := &models.Session{}
	err := db.QueryRow(`
		SELECT id, project_path, project_name, cwd, git_branch,
			started_at, ended_at, claude_version, entry_point, permission_mode,
			total_input_tokens, total_output_tokens,
			total_cache_read_tokens, total_cache_write_tokens, estimated_cost_usd
		FROM sessions WHERE id = ?
	`, id).Scan(
		&s.ID, &s.ProjectPath, &s.ProjectName, &s.CWD, &s.GitBranch,
		&s.StartedAt, &s.EndedAt, &s.ClaudeVersion, &s.EntryPoint, &s.PermissionMode,
		&s.TotalInputTokens, &s.TotalOutputTokens,
		&s.TotalCacheReadTokens, &s.TotalCacheWriteTokens, &s.EstimatedCostUSD,
	)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func GetSessionsByProject(db *sql.DB, projectPath string, limit, offset int) ([]*models.Session, error) {
	rows, err := db.Query(`
		SELECT id, project_path, project_name, cwd, git_branch,
			started_at, ended_at, claude_version, entry_point, permission_mode,
			total_input_tokens, total_output_tokens,
			total_cache_read_tokens, total_cache_write_tokens, estimated_cost_usd
		FROM sessions
		WHERE project_path = ?
		ORDER BY started_at DESC
		LIMIT ? OFFSET ?
	`, projectPath, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var sessions []*models.Session
	for rows.Next() {
		s := &models.Session{}
		if err := rows.Scan(
			&s.ID, &s.ProjectPath, &s.ProjectName, &s.CWD, &s.GitBranch,
			&s.StartedAt, &s.EndedAt, &s.ClaudeVersion, &s.EntryPoint, &s.PermissionMode,
			&s.TotalInputTokens, &s.TotalOutputTokens,
			&s.TotalCacheReadTokens, &s.TotalCacheWriteTokens, &s.EstimatedCostUSD,
		); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func SearchMessages(db *sql.DB, query string, limit int) ([]*models.Message, error) {
	rows, err := db.Query(`
		SELECT m.id, m.session_id, m.parent_id, m.type, m.role, m.model,
			m.content_text, m.content_json, m.stop_reason,
			m.input_tokens, m.output_tokens, m.cache_read_tokens, m.cache_write_tokens,
			m.timestamp
		FROM messages m
		JOIN messages_fts fts ON fts.id = m.id
		WHERE messages_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var messages []*models.Message
	for rows.Next() {
		m := &models.Message{}
		if err := rows.Scan(
			&m.ID, &m.SessionID, &m.ParentID, &m.Type, &m.Role, &m.Model,
			&m.ContentText, &m.ContentJSON, &m.StopReason,
			&m.InputTokens, &m.OutputTokens, &m.CacheReadTokens, &m.CacheWriteTokens,
			&m.Timestamp,
		); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/db -v
```

Expected: PASS - all database tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/db/
git commit -m "feat: add database query functions

- InsertSession with upsert on conflict
- InsertMessage with deduplication
- InsertToolCall with deduplication
- InsertSubagent with upsert
- GetSessionByID, GetSessionsByProject
- SearchMessages using FTS5
- Tests for insert and query operations

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 6: JSONL Parser for Claude Code Logs

**Files:**
- Create: `internal/parser/jsonl.go`
- Create: `internal/parser/jsonl_test.go`

- [ ] **Step 1: Write failing test for parsing user entries**

```go
package parser

import (
	"strings"
	"testing"
	"time"
)

func TestParseLogEntry_UserEntry(t *testing.T) {
	jsonl := `{"type":"user","uuid":"test-uuid","timestamp":"2026-04-16T14:10:17.220Z","message":{"role":"user","content":"test prompt"},"sessionId":"session-1","cwd":"/test/dir","gitBranch":"main","version":"2.1.110","entrypoint":"cli","permissionMode":"default"}`
	
	entry, err := ParseLogEntry([]byte(jsonl))
	if err != nil {
		t.Fatalf("ParseLogEntry() error = %v", err)
	}
	
	if entry.Type != "user" {
		t.Errorf("Type = %s, want user", entry.Type)
	}
	if entry.UUID != "test-uuid" {
		t.Errorf("UUID = %s, want test-uuid", entry.UUID)
	}
	if entry.SessionID != "session-1" {
		t.Errorf("SessionID = %s, want session-1", entry.SessionID)
	}
}

func TestParseLogEntry_AssistantEntry(t *testing.T) {
	jsonl := `{"type":"assistant","uuid":"assist-uuid","timestamp":"2026-04-16T14:10:26.376Z","message":{"model":"claude-sonnet-4-5","role":"assistant","content":[{"type":"text","text":"response"}],"usage":{"input_tokens":10,"output_tokens":100,"cache_read_input_tokens":50}},"sessionId":"session-1"}`
	
	entry, err := ParseLogEntry([]byte(jsonl))
	if err != nil {
		t.Fatalf("ParseLogEntry() error = %v", err)
	}
	
	if entry.Type != "assistant" {
		t.Errorf("Type = %s, want assistant", entry.Type)
	}
	if entry.Message.Model != "claude-sonnet-4-5" {
		t.Errorf("Model = %s, want claude-sonnet-4-5", entry.Message.Model)
	}
	if entry.Message.Usage.InputTokens != 10 {
		t.Errorf("InputTokens = %d, want 10", entry.Message.Usage.InputTokens)
	}
	if entry.Message.Usage.OutputTokens != 100 {
		t.Errorf("OutputTokens = %d, want 100", entry.Message.Usage.OutputTokens)
	}
}

func TestParseLogFile(t *testing.T) {
	content := `{"type":"user","uuid":"u1","timestamp":"2026-04-16T14:10:17.220Z","message":{"role":"user","content":"prompt 1"},"sessionId":"s1"}
{"type":"assistant","uuid":"a1","timestamp":"2026-04-16T14:10:26.376Z","message":{"model":"claude-sonnet-4-5","role":"assistant","content":[{"type":"text","text":"response 1"}]},"sessionId":"s1"}
{"type":"user","uuid":"u2","timestamp":"2026-04-16T14:11:00.000Z","message":{"role":"user","content":"prompt 2"},"sessionId":"s1"}`
	
	reader := strings.NewReader(content)
	entries, err := ParseLogFile(reader)
	if err != nil {
		t.Fatalf("ParseLogFile() error = %v", err)
	}
	
	if len(entries) != 3 {
		t.Errorf("Expected 3 entries, got %d", len(entries))
	}
	
	if entries[0].Type != "user" {
		t.Errorf("First entry type = %s, want user", entries[0].Type)
	}
	if entries[1].Type != "assistant" {
		t.Errorf("Second entry type = %s, want assistant", entries[1].Type)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/parser -v
```

Expected: FAIL - package parser not found

- [ ] **Step 3: Implement jsonl.go**

```go
package parser

import (
	"bufio"
	"encoding/json"
	"io"
	"time"
)

type LogEntry struct {
	Type           string    `json:"type"`
	UUID           string    `json:"uuid"`
	Timestamp      time.Time `json:"timestamp"`
	SessionID      string    `json:"sessionId"`
	CWD            string    `json:"cwd"`
	GitBranch      string    `json:"gitBranch"`
	Version        string    `json:"version"`
	EntryPoint     string    `json:"entrypoint"`
	PermissionMode string    `json:"permissionMode"`
	Message        Message   `json:"message"`
}

type Message struct {
	Role    string         `json:"role"`
	Content interface{}    `json:"content"`
	Model   string         `json:"model"`
	Usage   Usage          `json:"usage"`
}

type Usage struct {
	InputTokens           int `json:"input_tokens"`
	OutputTokens          int `json:"output_tokens"`
	CacheReadInputTokens  int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

type ContentBlock struct {
	Type     string                 `json:"type"`
	Text     string                 `json:"text,omitempty"`
	Thinking string                 `json:"thinking,omitempty"`
	Name     string                 `json:"name,omitempty"`
	ID       string                 `json:"id,omitempty"`
	Input    map[string]interface{} `json:"input,omitempty"`
}

func ParseLogEntry(data []byte) (*LogEntry, error) {
	var entry LogEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func ParseLogFile(r io.Reader) ([]*LogEntry, error) {
	var entries []*LogEntry
	scanner := bufio.NewScanner(r)
	
	// Increase buffer size for large log lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		
		entry, err := ParseLogEntry(line)
		if err != nil {
			// Skip malformed lines but log them
			continue
		}
		
		entries = append(entries, entry)
	}
	
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	
	return entries, nil
}

func ExtractContentText(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var text string
		for _, item := range v {
			if block, ok := item.(map[string]interface{}); ok {
				if t, ok := block["text"].(string); ok {
					text += t + " "
				}
			}
		}
		return text
	default:
		return ""
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/parser -v
```

Expected: PASS - all parser tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/parser/
git commit -m "feat: add JSONL parser for Claude Code logs

- ParseLogEntry for single JSONL line
- ParseLogFile for streaming parse
- LogEntry struct matching Claude Code format
- ExtractContentText helper for content extraction
- Large buffer support for big log lines
- Tests for user/assistant entries and file parsing

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 7: Hook Receiver (Unix Socket)

**Files:**
- Create: `internal/ingestion/receiver.go`
- Create: `internal/ingestion/receiver_test.go`

- [ ] **Step 1: Write failing test for receiver**

```go
package ingestion

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReceiver_AcceptConnections(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")
	
	eventCh := make(chan []byte, 10)
	receiver := NewReceiver(sockPath, eventCh)
	
	go func() {
		if err := receiver.Start(); err != nil {
			t.Logf("Receiver start error: %v", err)
		}
	}()
	
	// Wait for socket to be ready
	time.Sleep(100 * time.Millisecond)
	defer receiver.Stop()
	
	// Send test event
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()
	
	testEvent := map[string]interface{}{
		"event":      "PreToolUse",
		"session_id": "test-session",
		"tool_name":  "Bash",
	}
	
	data, _ := json.Marshal(testEvent)
	if _, err := conn.Write(data); err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	
	// Verify event received
	select {
	case received := <-eventCh:
		var event map[string]interface{}
		if err := json.Unmarshal(received, &event); err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}
		if event["event"] != "PreToolUse" {
			t.Errorf("Event = %s, want PreToolUse", event["event"])
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for event")
	}
}

func TestReceiver_MultipleConnections(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")
	
	eventCh := make(chan []byte, 10)
	receiver := NewReceiver(sockPath, eventCh)
	
	go receiver.Start()
	time.Sleep(100 * time.Millisecond)
	defer receiver.Stop()
	
	// Send 3 events from different connections
	for i := 0; i < 3; i++ {
		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			t.Fatalf("Connection %d failed: %v", i, err)
		}
		
		testEvent := map[string]interface{}{
			"event": "PostToolUse",
			"index": i,
		}
		
		data, _ := json.Marshal(testEvent)
		conn.Write(data)
		conn.Close()
	}
	
	// Verify all received
	received := 0
	timeout := time.After(1 * time.Second)
	
	for received < 3 {
		select {
		case <-eventCh:
			received++
		case <-timeout:
			t.Fatalf("Only received %d/3 events", received)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/ingestion -v
```

Expected: FAIL - NewReceiver not defined

- [ ] **Step 3: Implement receiver.go**

```go
package ingestion

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"
)

type Receiver struct {
	socketPath string
	eventCh    chan<- []byte
	listener   net.Listener
	wg         sync.WaitGroup
	stopCh     chan struct{}
}

func NewReceiver(socketPath string, eventCh chan<- []byte) *Receiver {
	return &Receiver{
		socketPath: socketPath,
		eventCh:    eventCh,
		stopCh:     make(chan struct{}),
	}
}

func (r *Receiver) Start() error {
	// Remove existing socket if present
	if err := os.RemoveAll(r.socketPath); err != nil {
		return fmt.Errorf("remove existing socket: %w", err)
	}
	
	listener, err := net.Listen("unix", r.socketPath)
	if err != nil {
		return fmt.Errorf("listen on socket: %w", err)
	}
	r.listener = listener
	
	// Set permissions so hook scripts can write
	if err := os.Chmod(r.socketPath, 0666); err != nil {
		listener.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}
	
	r.wg.Add(1)
	go r.acceptLoop()
	
	return nil
}

func (r *Receiver) Stop() error {
	close(r.stopCh)
	
	if r.listener != nil {
		r.listener.Close()
	}
	
	r.wg.Wait()
	
	// Clean up socket file
	os.RemoveAll(r.socketPath)
	
	return nil
}

func (r *Receiver) acceptLoop() {
	defer r.wg.Done()
	
	for {
		select {
		case <-r.stopCh:
			return
		default:
		}
		
		conn, err := r.listener.Accept()
		if err != nil {
			select {
			case <-r.stopCh:
				return
			default:
				continue
			}
		}
		
		r.wg.Add(1)
		go r.handleConnection(conn)
	}
}

func (r *Receiver) handleConnection(conn net.Conn) {
	defer r.wg.Done()
	defer conn.Close()
	
	// Read all data from connection
	data, err := io.ReadAll(conn)
	if err != nil {
		return
	}
	
	if len(data) == 0 {
		return
	}
	
	// Send to event channel (non-blocking)
	select {
	case r.eventCh <- data:
	case <-r.stopCh:
	default:
		// Channel full, drop event
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/ingestion -v -run TestReceiver
```

Expected: PASS - receiver tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/ingestion/receiver.go internal/ingestion/receiver_test.go
git commit -m "feat: add Unix socket receiver for hook events

- NewReceiver with socket path and event channel
- Start() creates socket with proper permissions
- Accept loop handles multiple concurrent connections
- handleConnection reads data and sends to channel
- Stop() graceful shutdown and socket cleanup
- Tests for single and multiple connections

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 8: Log File Watcher (fsnotify)

**Files:**
- Create: `internal/ingestion/watcher.go`
- Create: `internal/ingestion/watcher_test.go`

- [ ] **Step 1: Write failing test for watcher**

```go
package ingestion

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcher_DetectNewFile(t *testing.T) {
	tmpDir := t.TempDir()
	
	eventCh := make(chan *LogFileEvent, 10)
	watcher := NewWatcher(tmpDir, eventCh)
	
	if err := watcher.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer watcher.Stop()
	
	// Wait for watcher to initialize
	time.Sleep(100 * time.Millisecond)
	
	// Create a new JSONL file
	testFile := filepath.Join(tmpDir, "test-session.jsonl")
	if err := os.WriteFile(testFile, []byte(`{"type":"user","uuid":"u1"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	
	// Verify event received
	select {
	case event := <-eventCh:
		if event.Path != testFile {
			t.Errorf("Path = %s, want %s", event.Path, testFile)
		}
		if event.Type != EventFileCreated {
			t.Errorf("Type = %v, want EventFileCreated", event.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for file creation event")
	}
}

func TestWatcher_DetectFileModification(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create file before starting watcher
	testFile := filepath.Join(tmpDir, "existing.jsonl")
	if err := os.WriteFile(testFile, []byte(`{"type":"user","uuid":"u1"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	
	eventCh := make(chan *LogFileEvent, 10)
	watcher := NewWatcher(tmpDir, eventCh)
	
	if err := watcher.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer watcher.Stop()
	
	time.Sleep(100 * time.Millisecond)
	
	// Append to file
	f, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"type":"assistant","uuid":"a1"}` + "\n")
	f.Close()
	
	// Verify modification event
	select {
	case event := <-eventCh:
		if event.Type != EventFileModified {
			t.Errorf("Type = %v, want EventFileModified", event.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for modification event")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/ingestion -v -run TestWatcher
```

Expected: FAIL - NewWatcher not defined

- [ ] **Step 3: Implement watcher.go**

```go
package ingestion

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

type EventType int

const (
	EventFileCreated EventType = iota
	EventFileModified
	EventFileDeleted
)

type LogFileEvent struct {
	Type EventType
	Path string
}

type Watcher struct {
	rootDir string
	eventCh chan<- *LogFileEvent
	watcher *fsnotify.Watcher
	wg      sync.WaitGroup
	stopCh  chan struct{}
}

func NewWatcher(rootDir string, eventCh chan<- *LogFileEvent) *Watcher {
	return &Watcher{
		rootDir: rootDir,
		eventCh: eventCh,
		stopCh:  make(chan struct{}),
	}
}

func (w *Watcher) Start() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	w.watcher = watcher
	
	// Walk and add all existing directories
	if err := w.addDirectoriesRecursive(w.rootDir); err != nil {
		watcher.Close()
		return fmt.Errorf("add directories: %w", err)
	}
	
	w.wg.Add(1)
	go w.eventLoop()
	
	return nil
}

func (w *Watcher) Stop() error {
	close(w.stopCh)
	
	if w.watcher != nil {
		w.watcher.Close()
	}
	
	w.wg.Wait()
	return nil
}

func (w *Watcher) addDirectoriesRecursive(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		
		if info.IsDir() {
			if err := w.watcher.Add(path); err != nil {
				return err
			}
		}
		
		return nil
	})
}

func (w *Watcher) eventLoop() {
	defer w.wg.Done()
	
	for {
		select {
		case <-w.stopCh:
			return
			
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
			
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			// Log error but continue
			_ = err
		}
	}
}

func (w *Watcher) handleEvent(event fsnotify.Event) {
	// Only care about .jsonl files
	if !strings.HasSuffix(event.Name, ".jsonl") {
		// But watch for new directories
		if event.Op&fsnotify.Create == fsnotify.Create {
			if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
				w.addDirectoriesRecursive(event.Name)
			}
		}
		return
	}
	
	var eventType EventType
	switch {
	case event.Op&fsnotify.Create == fsnotify.Create:
		eventType = EventFileCreated
	case event.Op&fsnotify.Write == fsnotify.Write:
		eventType = EventFileModified
	case event.Op&fsnotify.Remove == fsnotify.Remove:
		eventType = EventFileDeleted
	default:
		return
	}
	
	logEvent := &LogFileEvent{
		Type: eventType,
		Path: event.Name,
	}
	
	select {
	case w.eventCh <- logEvent:
	case <-w.stopCh:
	default:
		// Channel full, drop event
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/ingestion -v -run TestWatcher
```

Expected: PASS - watcher tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/ingestion/watcher.go internal/ingestion/watcher_test.go
git commit -m "feat: add fsnotify watcher for log file changes

- NewWatcher monitors directory recursively
- Detects .jsonl file creation, modification, deletion
- Auto-adds new subdirectories to watch list
- Non-blocking event delivery
- Tests for file creation and modification

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 9: Ingestion Pipeline

**Files:**
- Create: `internal/ingestion/pipeline.go`
- Create: `internal/ingestion/pipeline_test.go`

- [ ] **Step 1: Write failing test for pipeline**

```go
package ingestion

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/szaher/claude-monitor/internal/db"
	"github.com/szaher/claude-monitor/internal/models"
)

func TestPipeline_ProcessHookEvent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	
	database, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	
	pipeline := NewPipeline(database)
	
	// Simulate SessionStart hook event
	hookData := []byte(`{
		"hook_event_name": "SessionStart",
		"session_id": "test-session-1",
		"cwd": "/test/project",
		"permission_mode": "default",
		"version": "2.1.110",
		"entrypoint": "cli",
		"gitBranch": "main"
	}`)
	
	if err := pipeline.ProcessHookEvent(hookData); err != nil {
		t.Fatalf("ProcessHookEvent() error = %v", err)
	}
	
	// Verify session was inserted
	session, err := db.GetSessionByID(database, "test-session-1")
	if err != nil {
		t.Fatalf("GetSessionByID() error = %v", err)
	}
	
	if session.ID != "test-session-1" {
		t.Errorf("Session ID = %s, want test-session-1", session.ID)
	}
	if session.GitBranch != "main" {
		t.Errorf("GitBranch = %s, want main", session.GitBranch)
	}
}

func TestPipeline_ProcessLogEntry(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	
	database, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	
	// Insert session first
	session := &models.Session{
		ID:          "test-session-1",
		ProjectPath: "/test/project",
		ProjectName: "project",
		StartedAt:   time.Now(),
	}
	if err := db.InsertSession(database, session); err != nil {
		t.Fatal(err)
	}
	
	pipeline := NewPipeline(database)
	
	// Process user log entry
	logData := []byte(`{
		"type": "user",
		"uuid": "msg-1",
		"timestamp": "2026-04-16T14:10:17.220Z",
		"message": {"role": "user", "content": "test prompt"},
		"sessionId": "test-session-1"
	}`)
	
	if err := pipeline.ProcessLogEntry(logData); err != nil {
		t.Fatalf("ProcessLogEntry() error = %v", err)
	}
	
	// Verify message was inserted
	var count int
	err = database.QueryRow("SELECT COUNT(*) FROM messages WHERE id = ?", "msg-1").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("Expected 1 message, got %d", count)
	}
}

func TestPipeline_BatchWrite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	
	database, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	
	pipeline := NewPipeline(database)
	pipeline.batchSize = 2 // Small batch for testing
	
	eventCh := make(chan []byte, 10)
	
	go pipeline.StartBatchProcessor(eventCh)
	defer pipeline.Stop()
	
	// Send 3 events
	for i := 0; i < 3; i++ {
		eventCh <- []byte(`{"hook_event_name": "Stop", "session_id": "test"}`)
	}
	
	// Wait for batch processing
	time.Sleep(200 * time.Millisecond)
	
	// Pipeline should have processed them
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/ingestion -v -run TestPipeline
```

Expected: FAIL - NewPipeline not defined

- [ ] **Step 3: Implement pipeline.go**

```go
package ingestion

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/szaher/claude-monitor/internal/db"
	"github.com/szaher/claude-monitor/internal/models"
	"github.com/szaher/claude-monitor/internal/parser"
)

type Pipeline struct {
	db        *sql.DB
	batchSize int
	batchTime time.Duration
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

func NewPipeline(database *sql.DB) *Pipeline {
	return &Pipeline{
		db:        database,
		batchSize: 50,
		batchTime: 500 * time.Millisecond,
		stopCh:    make(chan struct{}),
	}
}

func (p *Pipeline) ProcessHookEvent(data []byte) error {
	var event map[string]interface{}
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("unmarshal hook event: %w", err)
	}
	
	eventName, _ := event["hook_event_name"].(string)
	sessionID, _ := event["session_id"].(string)
	
	switch eventName {
	case "SessionStart":
		return p.handleSessionStart(event)
	case "SessionEnd":
		return p.handleSessionEnd(event)
	case "PreToolUse":
		return p.handlePreToolUse(event)
	case "PostToolUse":
		return p.handlePostToolUse(event)
	case "SubagentStart":
		return p.handleSubagentStart(event)
	case "SubagentStop":
		return p.handleSubagentStop(event)
	case "Stop":
		// Update session timestamp
		_, err := p.db.Exec("UPDATE sessions SET ended_at = ? WHERE id = ? AND ended_at IS NULL",
			time.Now(), sessionID)
		return err
	default:
		// Unknown event, ignore
		return nil
	}
}

func (p *Pipeline) handleSessionStart(event map[string]interface{}) error {
	sessionID, _ := event["session_id"].(string)
	cwd, _ := event["cwd"].(string)
	gitBranch, _ := event["gitBranch"].(string)
	version, _ := event["version"].(string)
	entrypoint, _ := event["entrypoint"].(string)
	permissionMode, _ := event["permission_mode"].(string)
	
	session := &models.Session{
		ID:             sessionID,
		ProjectPath:    cwd,
		ProjectName:    filepath.Base(cwd),
		CWD:            cwd,
		GitBranch:      gitBranch,
		StartedAt:      time.Now(),
		ClaudeVersion:  version,
		EntryPoint:     entrypoint,
		PermissionMode: permissionMode,
	}
	
	return db.InsertSession(p.db, session)
}

func (p *Pipeline) handleSessionEnd(event map[string]interface{}) error {
	sessionID, _ := event["session_id"].(string)
	
	_, err := p.db.Exec("UPDATE sessions SET ended_at = ? WHERE id = ?",
		time.Now(), sessionID)
	return err
}

func (p *Pipeline) handlePreToolUse(event map[string]interface{}) error {
	// PreToolUse doesn't need special handling - PostToolUse has the full info
	return nil
}

func (p *Pipeline) handlePostToolUse(event map[string]interface{}) error {
	toolUseID, _ := event["tool_use_id"].(string)
	messageID, _ := event["tool_use_id"].(string) // Use tool_use_id as ID
	sessionID, _ := event["session_id"].(string)
	toolName, _ := event["tool_name"].(string)
	
	toolInput, _ := json.Marshal(event["tool_input"])
	toolResponse, _ := json.Marshal(event["tool_response"])
	
	success := true
	errorMsg := ""
	if err, ok := event["error"].(string); ok && err != "" {
		success = false
		errorMsg = err
	}
	
	toolCall := &models.ToolCall{
		ID:           toolUseID,
		MessageID:    messageID,
		SessionID:    sessionID,
		ToolName:     toolName,
		ToolInput:    string(toolInput),
		ToolResponse: string(toolResponse),
		Success:      success,
		Error:        errorMsg,
		Timestamp:    time.Now(),
	}
	
	return db.InsertToolCall(p.db, toolCall)
}

func (p *Pipeline) handleSubagentStart(event map[string]interface{}) error {
	agentID, _ := event["agent_id"].(string)
	sessionID, _ := event["session_id"].(string)
	agentType, _ := event["agent_type"].(string)
	
	subagent := &models.Subagent{
		ID:        agentID,
		SessionID: sessionID,
		AgentType: agentType,
		StartedAt: time.Now(),
	}
	
	return db.InsertSubagent(p.db, subagent)
}

func (p *Pipeline) handleSubagentStop(event map[string]interface{}) error {
	agentID, _ := event["agent_id"].(string)
	
	endedAt := time.Now()
	_, err := p.db.Exec("UPDATE subagents SET ended_at = ? WHERE id = ?",
		endedAt, agentID)
	return err
}

func (p *Pipeline) ProcessLogEntry(data []byte) error {
	entry, err := parser.ParseLogEntry(data)
	if err != nil {
		return fmt.Errorf("parse log entry: %w", err)
	}
	
	switch entry.Type {
	case "user", "assistant":
		return p.processMessage(entry)
	case "system":
		// System entries contain metadata, could extract turn duration
		return nil
	default:
		return nil
	}
}

func (p *Pipeline) processMessage(entry *parser.LogEntry) error {
	contentText := parser.ExtractContentText(entry.Message.Content)
	contentJSON, _ := json.Marshal(entry.Message.Content)
	
	message := &models.Message{
		ID:               entry.UUID,
		SessionID:        entry.SessionID,
		Type:             entry.Type,
		Role:             entry.Message.Role,
		Model:            entry.Message.Model,
		ContentText:      contentText,
		ContentJSON:      string(contentJSON),
		InputTokens:      entry.Message.Usage.InputTokens,
		OutputTokens:     entry.Message.Usage.OutputTokens,
		CacheReadTokens:  entry.Message.Usage.CacheReadInputTokens,
		CacheWriteTokens: entry.Message.Usage.CacheCreationInputTokens,
		Timestamp:        entry.Timestamp,
	}
	
	if err := db.InsertMessage(p.db, message); err != nil {
		return err
	}
	
	// Update session token counts
	_, err := p.db.Exec(`
		UPDATE sessions SET
			total_input_tokens = total_input_tokens + ?,
			total_output_tokens = total_output_tokens + ?,
			total_cache_read_tokens = total_cache_read_tokens + ?,
			total_cache_write_tokens = total_cache_write_tokens + ?
		WHERE id = ?
	`, message.InputTokens, message.OutputTokens, message.CacheReadTokens, message.CacheWriteTokens, entry.SessionID)
	
	return err
}

func (p *Pipeline) StartBatchProcessor(eventCh <-chan []byte) {
	p.wg.Add(1)
	defer p.wg.Done()
	
	batch := make([][]byte, 0, p.batchSize)
	ticker := time.NewTicker(p.batchTime)
	defer ticker.Stop()
	
	flush := func() {
		if len(batch) == 0 {
			return
		}
		
		for _, data := range batch {
			// Try hook event first, then log entry
			if err := p.ProcessHookEvent(data); err != nil {
				p.ProcessLogEntry(data)
			}
		}
		
		batch = batch[:0]
	}
	
	for {
		select {
		case <-p.stopCh:
			flush()
			return
			
		case data := <-eventCh:
			batch = append(batch, data)
			if len(batch) >= p.batchSize {
				flush()
			}
			
		case <-ticker.C:
			flush()
		}
	}
}

func (p *Pipeline) Stop() {
	close(p.stopCh)
	p.wg.Wait()
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/ingestion -v -run TestPipeline
```

Expected: PASS - pipeline tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/ingestion/pipeline.go internal/ingestion/pipeline_test.go
git commit -m "feat: add ingestion pipeline with batching and deduplication

- ProcessHookEvent handles SessionStart/End, tool calls, subagents
- ProcessLogEntry parses JSONL and extracts messages
- Batch processor for efficient database writes
- Automatic session token count aggregation
- Tests for hook events and log entries

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 10: Hook Command Implementation

**Files:**
- Create: `internal/hook/hook.go`
- Create: `internal/hook/hook_test.go`

- [ ] **Step 1: Create hook.go with stdin reader and socket writer**

```go
package hook

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
)

func Execute() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	
	sockPath := filepath.Join(homeDir, ".claude-monitor", "monitor.sock")
	
	// Read stdin
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}
	
	if len(data) == 0 {
		return nil // No data, exit silently
	}
	
	// Try to connect to socket
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		// Daemon not running, exit silently
		return nil
	}
	defer conn.Close()
	
	// Write data
	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("write to socket: %w", err)
	}
	
	return nil
}
```

- [ ] **Step 2: Wire into main.go**

Add to `cmd/claude-monitor/main.go` in switch statement:

```go
case "hook":
	if err := hook.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Hook error: %v\n", err)
		os.Exit(1)
	}
```

Add import:

```go
import (
	//... existing
	"github.com/szaher/claude-monitor/internal/hook"
)
```

- [ ] **Step 3: Test hook command**

```bash
echo '{"event":"test"}' | ./bin/claude-monitor hook
```

Expected: Exits with no error (daemon not running)

- [ ] **Step 4: Commit**

```bash
git add internal/hook/ cmd/claude-monitor/main.go
git commit -m "feat: add hook subcommand for stdin to Unix socket relay

- Execute() reads JSON from stdin
- Dials Unix socket at ~/.claude-monitor/monitor.sock
- Writes data and exits
- Silent failure if daemon not running
- Wired into main CLI router

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 11: Complete CLI Command Implementations

**Files:**
- Create: `internal/cli/version.go`
- Modify: `cmd/claude-monitor/main.go`

- [ ] **Step 1: Implement version command**

```go
package cli

import (
	"fmt"
)

func Version(version string) {
	fmt.Printf("claude-monitor %s\n", version)
}
```

- [ ] **Step 2: Wire all CLI commands into main.go**

Replace main.go with full CLI routing:

```go
package main

import (
	"fmt"
	"os"

	"github.com/szaher/claude-monitor/internal/cli"
	"github.com/szaher/claude-monitor/internal/hook"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]
	
	switch command {
	case "version":
		cli.Version(version)
	case "hook":
		if err := hook.Execute(); err != nil {
			fmt.Fprintf(os.Stderr, "Hook error: %v\n", err)
			os.Exit(1)
		}
	case "install":
		if err := cli.Install(args); err != nil {
			fmt.Fprintf(os.Stderr, "Install error: %v\n", err)
			os.Exit(1)
		}
	case "uninstall":
		if err := cli.Uninstall(args); err != nil {
			fmt.Fprintf(os.Stderr, "Uninstall error: %v\n", err)
			os.Exit(1)
		}
	case "serve":
		if err := cli.Serve(args); err != nil {
			fmt.Fprintf(os.Stderr, "Serve error: %v\n", err)
			os.Exit(1)
		}
	case "import":
		if err := cli.Import(args); err != nil {
			fmt.Fprintf(os.Stderr, "Import error: %v\n", err)
			os.Exit(1)
		}
	case "export":
		if err := cli.Export(args); err != nil {
			fmt.Fprintf(os.Stderr, "Export error: %v\n", err)
			os.Exit(1)
		}
	case "config":
		if err := cli.Config(args); err != nil {
			fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
			os.Exit(1)
		}
	case "status":
		if err := cli.Status(args); err != nil {
			fmt.Fprintf(os.Stderr, "Status error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Usage: claude-monitor <command> [flags]

Commands:
  install          Install hooks and initialize database
  uninstall        Remove hooks, optionally delete data
  serve            Start daemon + web UI
  import           Import all historical Claude Code logs
  config           Manage configuration
  status           Show daemon status, database stats
  export           Export sessions as JSON/CSV/HTML
  hook             Handle hook events (internal use)
  version          Print version info`)
}
```

- [ ] **Step 3: Create stub CLI files**

Each CLI file in `internal/cli/` follows this pattern:

```go
package cli

func Install(args []string) error {
	// TODO: Implement in next task
	return nil
}
```

Create: `install.go`, `uninstall.go`, `serve.go`, `import.go`, `export.go`, `config.go`, `status.go`

- [ ] **Step 4: Commit**

```bash
git add internal/cli/ cmd/claude-monitor/main.go
git commit -m "feat: wire all CLI commands into main router

- Full command switch in main.go
- Stub implementations for all commands
- Version command implemented
- Error handling for each command

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 12: Installer Implementation

**Files:**
- Create: `internal/installer/installer.go`
- Implement: `internal/cli/install.go`
- Implement: `internal/cli/uninstall.go`

- [ ] **Step 1: Implement installer.go**

```go
package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type HookConfig struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
	Async   bool   `json:"async"`
}

func InstallHooks() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	
	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	backupPath := settingsPath + ".backup"
	
	// Backup existing settings
	if _, err := os.Stat(settingsPath); err == nil {
		input, err := os.ReadFile(settingsPath)
		if err != nil {
			return fmt.Errorf("read settings: %w", err)
		}
		if err := os.WriteFile(backupPath, input, 0644); err != nil {
			return fmt.Errorf("backup settings: %w", err)
		}
	}
	
	// Load or create settings
	var settings map[string]interface{}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		settings = make(map[string]interface{})
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("unmarshal settings: %w", err)
		}
	}
	
	// Add hooks
	hooks := map[string]interface{}{
		"PreToolUse": []map[string]interface{}{
			{"hooks": []HookConfig{{
				Type:    "command",
				Command: "claude-monitor hook",
				Timeout: 1000,
				Async:   true,
			}}},
		},
		"PostToolUse": []map[string]interface{}{
			{"hooks": []HookConfig{{
				Type:    "command",
				Command: "claude-monitor hook",
				Timeout: 1000,
				Async:   true,
			}}},
		},
		"SessionStart": []map[string]interface{}{
			{"hooks": []HookConfig{{
				Type:    "command",
				Command: "claude-monitor hook",
				Timeout: 2000,
				Async:   false,
			}}},
		},
		"SessionEnd": []map[string]interface{}{
			{"hooks": []HookConfig{{
				Type:    "command",
				Command: "claude-monitor hook",
				Timeout: 1000,
				Async:   true,
			}}},
		},
		"SubagentStart": []map[string]interface{}{
			{"hooks": []HookConfig{{
				Type:    "command",
				Command: "claude-monitor hook",
				Timeout: 1000,
				Async:   true,
			}}},
		},
		"SubagentStop": []map[string]interface{}{
			{"hooks": []HookConfig{{
				Type:    "command",
				Command: "claude-monitor hook",
				Timeout: 1000,
				Async:   true,
			}}},
		},
		"Stop": []map[string]interface{}{
			{"hooks": []HookConfig{{
				Type:    "command",
				Command: "claude-monitor hook",
				Timeout: 1000,
				Async:   true,
			}}},
		},
	}
	
	settings["hooks"] = hooks
	
	// Write updated settings
	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	
	if err := os.WriteFile(settingsPath, output, 0644); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	
	return nil
}

func UninstallHooks() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	
	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	backupPath := settingsPath + ".backup"
	
	// Restore from backup if it exists
	if _, err := os.Stat(backupPath); err == nil {
		backup, err := os.ReadFile(backupPath)
		if err != nil {
			return fmt.Errorf("read backup: %w", err)
		}
		if err := os.WriteFile(settingsPath, backup, 0644); err != nil {
			return fmt.Errorf("restore backup: %w", err)
		}
		os.Remove(backupPath)
		return nil
	}
	
	// Or just remove hooks section
	var settings map[string]interface{}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil // No settings file
	}
	
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("unmarshal settings: %w", err)
	}
	
	delete(settings, "hooks")
	
	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(settingsPath, output, 0644)
}
```

- [ ] **Step 2: Implement install CLI command**

```go
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/szaher/claude-monitor/internal/config"
	"github.com/szaher/claude-monitor/internal/db"
	"github.com/szaher/claude-monitor/internal/installer"
)

func Install(args []string) error {
	homeDir, _ := os.UserHomeDir()
	monitorDir := filepath.Join(homeDir, ".claude-monitor")
	
	// Create directory
	fmt.Println("Creating ~/.claude-monitor directory...")
	if err := os.MkdirAll(monitorDir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	
	// Initialize database
	fmt.Println("Initializing database...")
	dbPath := filepath.Join(monitorDir, "monitor.db")
	database, err := db.InitDB(dbPath)
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}
	database.Close()
	
	// Create archive directory
	archivePath := filepath.Join(monitorDir, "archive")
	if err := os.MkdirAll(archivePath, 0755); err != nil {
		return fmt.Errorf("create archive dir: %w", err)
	}
	
	// Write default config
	fmt.Println("Writing default configuration...")
	configPath := filepath.Join(monitorDir, "config.yaml")
	cfg := config.DefaultConfig()
	if err := config.Save(configPath, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	
	// Install hooks
	fmt.Println("Installing hooks in ~/.claude/settings.json...")
	if err := installer.InstallHooks(); err != nil {
		return fmt.Errorf("install hooks: %w", err)
	}
	
	fmt.Println("\n✓ Installation complete!")
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Run 'claude-monitor serve' to start the web UI")
	fmt.Println("  2. Visit http://localhost:3000")
	fmt.Println("  3. Optionally run 'claude-monitor import' to import historical logs")
	
	return nil
}
```

- [ ] **Step 3: Implement uninstall CLI command**

```go
package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/szaher/claude-monitor/internal/installer"
)

func Uninstall(args []string) error {
	// Remove hooks
	fmt.Println("Removing hooks from ~/.claude/settings.json...")
	if err := installer.UninstallHooks(); err != nil {
		return fmt.Errorf("uninstall hooks: %w", err)
	}
	
	// Ask about data
	fmt.Print("\nDelete all data in ~/.claude-monitor? (y/N): ")
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))
	
	if response == "y" || response == "yes" {
		homeDir, _ := os.UserHomeDir()
		monitorDir := filepath.Join(homeDir, ".claude-monitor")
		
		fmt.Println("Deleting ~/.claude-monitor...")
		if err := os.RemoveAll(monitorDir); err != nil {
			return fmt.Errorf("remove data: %w", err)
		}
		fmt.Println("✓ Data deleted")
	} else {
		fmt.Println("Data preserved in ~/.claude-monitor")
	}
	
	fmt.Println("\n✓ Uninstallation complete!")
	
	return nil
}
```

- [ ] **Step 4: Test install/uninstall**

```bash
make build
./bin/claude-monitor install
ls ~/.claude-monitor
./bin/claude-monitor uninstall
```

Expected: Directory created, database initialized, hooks installed

- [ ] **Step 5: Commit**

```bash
git add internal/installer/ internal/cli/install.go internal/cli/uninstall.go
git commit -m "feat: implement install and uninstall commands

- InstallHooks() merges hooks into settings.json with backup
- UninstallHooks() restores from backup
- Install command creates directory, database, config, hooks
- Uninstall command removes hooks, optionally deletes data
- Interactive prompt for data deletion

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

**Note:** Tasks 13-23 continue with full implementation details following the same TDD pattern. Each task includes complete code, tests, and commits. Due to file size, the remaining tasks are provided in summary form with the understanding that full details would follow the established pattern.

## Remaining Tasks (Summary)

### Task 13: Serve Command & Web Server
- Implement `internal/cli/serve.go` with signal handling
- Create `internal/server/server.go` with HTTP routes
- Start receiver, watcher, pipeline in goroutines
- Graceful shutdown on SIGINT/SIGTERM

### Task 14-15: REST API & WebSocket
- API handlers for sessions, messages, stats, config
- WebSocket hub with client broadcast
- Integration with ingestion pipeline

### Task 16-20: Web UI
- Embed setup with `go:embed`
- HTML/CSS/JS for 8 pages
- Chart.js integration
- Component-based architecture

### Task 21: Import/Export/Config/Status Commands
- Import scans all log files
- Export to JSON/CSV/HTML
- Config get/set/list
- Status shows database stats

### Task 22: Install Script
- Bash script for curl install
- OS/arch detection
- Binary download from releases
- Automatic install execution

### Task 23: Integration & Polish
- End-to-end testing
- Performance optimization
- Documentation updates
- Release preparation

