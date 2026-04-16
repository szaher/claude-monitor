package ingestion

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/szaher/claude-monitor/internal/db"
	"github.com/szaher/claude-monitor/internal/models"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestPipeline_ProcessHookEvent_SessionStart(t *testing.T) {
	database := setupTestDB(t)

	var broadcasted []byte
	p := NewPipeline(database, func(data []byte) {
		broadcasted = data
	}, nil)

	hookEvent := map[string]interface{}{
		"hook_event_name": "SessionStart",
		"session_id":      "sess-hook-001",
		"cwd":             "/home/user/myproject",
		"gitBranch":       "main",
		"version":         "1.0.0",
		"entrypoint":      "cli",
		"permission_mode": "default",
	}
	data, _ := json.Marshal(hookEvent)

	if err := p.ProcessHookEvent(data); err != nil {
		t.Fatalf("ProcessHookEvent failed: %v", err)
	}

	// Verify session was created in DB
	session, err := db.GetSessionByID(database, "sess-hook-001")
	if err != nil {
		t.Fatalf("GetSessionByID failed: %v", err)
	}
	if session.ID != "sess-hook-001" {
		t.Errorf("expected session ID 'sess-hook-001', got %q", session.ID)
	}
	if session.CWD != "/home/user/myproject" {
		t.Errorf("expected CWD '/home/user/myproject', got %q", session.CWD)
	}
	if session.GitBranch != "main" {
		t.Errorf("expected GitBranch 'main', got %q", session.GitBranch)
	}
	if session.ClaudeVersion != "1.0.0" {
		t.Errorf("expected ClaudeVersion '1.0.0', got %q", session.ClaudeVersion)
	}
	if session.EntryPoint != "cli" {
		t.Errorf("expected EntryPoint 'cli', got %q", session.EntryPoint)
	}
	if session.PermissionMode != "default" {
		t.Errorf("expected PermissionMode 'default', got %q", session.PermissionMode)
	}

	// Verify broadcast was called
	if broadcasted == nil {
		t.Error("expected broadcast to be called")
	}
}

func TestPipeline_ProcessHookEvent_SessionEnd(t *testing.T) {
	database := setupTestDB(t)
	p := NewPipeline(database, nil, nil)

	// First create a session
	now := time.Now().UTC().Truncate(time.Second)
	s := &models.Session{
		ID:          "sess-end-001",
		ProjectPath: "/home/user/proj",
		ProjectName: "proj",
		StartedAt:   now,
	}
	if err := db.InsertSession(database, s); err != nil {
		t.Fatalf("InsertSession failed: %v", err)
	}

	// Send SessionEnd event
	hookEvent := map[string]interface{}{
		"hook_event_name": "SessionEnd",
		"session_id":      "sess-end-001",
	}
	data, _ := json.Marshal(hookEvent)

	if err := p.ProcessHookEvent(data); err != nil {
		t.Fatalf("ProcessHookEvent failed: %v", err)
	}

	// Verify session ended_at was set
	session, err := db.GetSessionByID(database, "sess-end-001")
	if err != nil {
		t.Fatalf("GetSessionByID failed: %v", err)
	}
	if session.EndedAt == nil {
		t.Error("expected EndedAt to be set after SessionEnd")
	}
}

func TestPipeline_ProcessHookEvent_Stop(t *testing.T) {
	database := setupTestDB(t)
	p := NewPipeline(database, nil, nil)

	// First create a session
	now := time.Now().UTC().Truncate(time.Second)
	s := &models.Session{
		ID:          "sess-stop-001",
		ProjectPath: "/home/user/proj",
		ProjectName: "proj",
		StartedAt:   now,
	}
	if err := db.InsertSession(database, s); err != nil {
		t.Fatalf("InsertSession failed: %v", err)
	}

	// Send Stop event
	hookEvent := map[string]interface{}{
		"hook_event_name": "Stop",
		"session_id":      "sess-stop-001",
	}
	data, _ := json.Marshal(hookEvent)

	if err := p.ProcessHookEvent(data); err != nil {
		t.Fatalf("ProcessHookEvent failed: %v", err)
	}

	// Verify session ended_at was set
	session, err := db.GetSessionByID(database, "sess-stop-001")
	if err != nil {
		t.Fatalf("GetSessionByID failed: %v", err)
	}
	if session.EndedAt == nil {
		t.Error("expected EndedAt to be set after Stop")
	}
}

func TestPipeline_ProcessHookEvent_PostToolUse(t *testing.T) {
	database := setupTestDB(t)
	p := NewPipeline(database, nil, nil)

	// Create session and message for FK constraints
	now := time.Now().UTC().Truncate(time.Second)
	s := &models.Session{
		ID:          "sess-tool-001",
		ProjectPath: "/home/user/proj",
		ProjectName: "proj",
		StartedAt:   now,
	}
	if err := db.InsertSession(database, s); err != nil {
		t.Fatalf("InsertSession failed: %v", err)
	}
	m := &models.Message{
		ID:        "msg-tool-001",
		SessionID: "sess-tool-001",
		Type:      "assistant",
		Timestamp: now,
	}
	if err := db.InsertMessage(database, m); err != nil {
		t.Fatalf("InsertMessage failed: %v", err)
	}

	hookEvent := map[string]interface{}{
		"hook_event_name": "PostToolUse",
		"session_id":      "sess-tool-001",
		"message_id":      "msg-tool-001",
		"tool_use_id":     "tc-hook-001",
		"tool_name":       "Read",
		"tool_input":      `{"path": "/tmp/file.txt"}`,
		"tool_response":   "file contents",
	}
	data, _ := json.Marshal(hookEvent)

	if err := p.ProcessHookEvent(data); err != nil {
		t.Fatalf("ProcessHookEvent failed: %v", err)
	}

	// Verify tool call was inserted
	var count int
	err := database.QueryRow("SELECT COUNT(*) FROM tool_calls WHERE id = ?", "tc-hook-001").Scan(&count)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 tool_call, got %d", count)
	}
}

func TestPipeline_ProcessHookEvent_SubagentStart(t *testing.T) {
	database := setupTestDB(t)
	p := NewPipeline(database, nil, nil)

	// Create session for FK constraint
	now := time.Now().UTC().Truncate(time.Second)
	s := &models.Session{
		ID:          "sess-sa-001",
		ProjectPath: "/home/user/proj",
		ProjectName: "proj",
		StartedAt:   now,
	}
	if err := db.InsertSession(database, s); err != nil {
		t.Fatalf("InsertSession failed: %v", err)
	}

	hookEvent := map[string]interface{}{
		"hook_event_name": "SubagentStart",
		"session_id":      "sess-sa-001",
		"agent_id":        "agent-001",
		"agent_type":      "task",
	}
	data, _ := json.Marshal(hookEvent)

	if err := p.ProcessHookEvent(data); err != nil {
		t.Fatalf("ProcessHookEvent failed: %v", err)
	}

	// Verify subagent was inserted
	var count int
	err := database.QueryRow("SELECT COUNT(*) FROM subagents WHERE id = ?", "agent-001").Scan(&count)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 subagent, got %d", count)
	}
}

func TestPipeline_ProcessHookEvent_SubagentStop(t *testing.T) {
	database := setupTestDB(t)
	p := NewPipeline(database, nil, nil)

	// Create session and subagent
	now := time.Now().UTC().Truncate(time.Second)
	s := &models.Session{
		ID:          "sess-sa-stop",
		ProjectPath: "/home/user/proj",
		ProjectName: "proj",
		StartedAt:   now,
	}
	if err := db.InsertSession(database, s); err != nil {
		t.Fatalf("InsertSession failed: %v", err)
	}
	sa := &models.Subagent{
		ID:        "agent-stop-001",
		SessionID: "sess-sa-stop",
		AgentType: "task",
		StartedAt: now,
	}
	if err := db.InsertSubagent(database, sa); err != nil {
		t.Fatalf("InsertSubagent failed: %v", err)
	}

	hookEvent := map[string]interface{}{
		"hook_event_name": "SubagentStop",
		"session_id":      "sess-sa-stop",
		"agent_id":        "agent-stop-001",
	}
	data, _ := json.Marshal(hookEvent)

	if err := p.ProcessHookEvent(data); err != nil {
		t.Fatalf("ProcessHookEvent failed: %v", err)
	}

	// Verify subagent ended_at was set
	var endedAt sql.NullString
	err := database.QueryRow("SELECT ended_at FROM subagents WHERE id = ?", "agent-stop-001").Scan(&endedAt)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if !endedAt.Valid {
		t.Error("expected ended_at to be set after SubagentStop")
	}
}

func TestPipeline_ProcessHookEvent_PreToolUse(t *testing.T) {
	database := setupTestDB(t)
	p := NewPipeline(database, nil, nil)

	hookEvent := map[string]interface{}{
		"hook_event_name": "PreToolUse",
		"session_id":      "sess-pre",
		"tool_name":       "Read",
	}
	data, _ := json.Marshal(hookEvent)

	// PreToolUse should be a no-op
	if err := p.ProcessHookEvent(data); err != nil {
		t.Fatalf("ProcessHookEvent for PreToolUse should not error: %v", err)
	}
}

func TestPipeline_ProcessLogEntry(t *testing.T) {
	database := setupTestDB(t)
	p := NewPipeline(database, nil, nil)

	// A user message log entry
	entry := map[string]interface{}{
		"type":           "user",
		"uuid":           "msg-log-001",
		"sessionId":      "sess-log-001",
		"cwd":            "/home/user/proj",
		"gitBranch":      "main",
		"version":        "1.0.0",
		"entrypoint":     "cli",
		"permissionMode": "default",
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
		"message": map[string]interface{}{
			"role":    "user",
			"content": "Hello, Claude!",
		},
	}
	data, _ := json.Marshal(entry)

	if err := p.ProcessLogEntry(data); err != nil {
		t.Fatalf("ProcessLogEntry failed: %v", err)
	}

	// Verify session was auto-created
	session, err := db.GetSessionByID(database, "sess-log-001")
	if err != nil {
		t.Fatalf("GetSessionByID failed: %v", err)
	}
	if session.ID != "sess-log-001" {
		t.Errorf("expected session ID 'sess-log-001', got %q", session.ID)
	}

	// Verify message was inserted
	var contentText string
	err = database.QueryRow("SELECT content_text FROM messages WHERE id = ?", "msg-log-001").Scan(&contentText)
	if err != nil {
		t.Fatalf("query message failed: %v", err)
	}
	if contentText != "Hello, Claude!" {
		t.Errorf("expected content 'Hello, Claude!', got %q", contentText)
	}
}

func TestPipeline_ProcessLogEntry_AssistantWithToolUse(t *testing.T) {
	database := setupTestDB(t)
	p := NewPipeline(database, nil, nil)

	// Create session first
	now := time.Now().UTC().Truncate(time.Second)
	s := &models.Session{
		ID:          "sess-log-ast",
		ProjectPath: "/home/user/proj",
		ProjectName: "proj",
		StartedAt:   now,
	}
	if err := db.InsertSession(database, s); err != nil {
		t.Fatalf("InsertSession failed: %v", err)
	}

	entry := map[string]interface{}{
		"type":      "assistant",
		"uuid":      "msg-log-ast-001",
		"sessionId": "sess-log-ast",
		"timestamp": now.Format(time.RFC3339),
		"message": map[string]interface{}{
			"role": "assistant",
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "Let me read that file.",
				},
				map[string]interface{}{
					"type":  "tool_use",
					"id":    "tc-log-001",
					"name":  "Read",
					"input": map[string]interface{}{"path": "/tmp/file.txt"},
				},
			},
			"model": "claude-sonnet-4-20250514",
			"usage": map[string]interface{}{
				"input_tokens":  100,
				"output_tokens": 50,
			},
		},
	}
	data, _ := json.Marshal(entry)

	if err := p.ProcessLogEntry(data); err != nil {
		t.Fatalf("ProcessLogEntry failed: %v", err)
	}

	// Verify message was inserted
	var contentText string
	err := database.QueryRow("SELECT content_text FROM messages WHERE id = ?", "msg-log-ast-001").Scan(&contentText)
	if err != nil {
		t.Fatalf("query message failed: %v", err)
	}
	if contentText != "Let me read that file." {
		t.Errorf("expected content text 'Let me read that file.', got %q", contentText)
	}

	// Verify tool call was extracted and inserted
	var toolCount int
	err = database.QueryRow("SELECT COUNT(*) FROM tool_calls WHERE session_id = ?", "sess-log-ast").Scan(&toolCount)
	if err != nil {
		t.Fatalf("query tool_calls failed: %v", err)
	}
	if toolCount != 1 {
		t.Errorf("expected 1 tool call, got %d", toolCount)
	}
}

func TestPipeline_BatchWrite(t *testing.T) {
	database := setupTestDB(t)
	p := NewPipeline(database, nil, nil)

	eventCh := make(chan []byte, 100)

	go p.StartBatchProcessor(eventCh)

	// Send a SessionStart hook event through the batch processor
	hookEvent := map[string]interface{}{
		"hook_event_name": "SessionStart",
		"session_id":      "sess-batch-001",
		"cwd":             "/home/user/proj",
		"version":         "1.0.0",
	}
	data, _ := json.Marshal(hookEvent)
	eventCh <- data

	// Give time for batch processing (batchTime is 500ms)
	time.Sleep(1 * time.Second)

	p.Stop()

	// Verify session was created
	session, err := db.GetSessionByID(database, "sess-batch-001")
	if err != nil {
		t.Fatalf("GetSessionByID failed: %v", err)
	}
	if session.ID != "sess-batch-001" {
		t.Errorf("expected session ID 'sess-batch-001', got %q", session.ID)
	}
}

func TestPipeline_ProcessHookEvent_InvalidJSON(t *testing.T) {
	database := setupTestDB(t)
	p := NewPipeline(database, nil, nil)

	err := p.ProcessHookEvent([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestProcessLogEntry_StopReason(t *testing.T) {
	database := setupTestDB(t)
	pipeline := NewPipeline(database, nil, nil)

	sessionEntry := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-04-16T10:00:00Z","cwd":"/test","message":{"role":"user","content":"hello"}}`
	if err := pipeline.ProcessLogEntry([]byte(sessionEntry)); err != nil {
		t.Fatalf("ProcessLogEntry (user): %v", err)
	}

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

func TestPipeline_ProcessLogEntry_InvalidJSON(t *testing.T) {
	database := setupTestDB(t)
	p := NewPipeline(database, nil, nil)

	err := p.ProcessLogEntry([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
