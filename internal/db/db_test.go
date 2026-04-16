package db

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/szaher/claude-monitor/internal/models"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestInitDB(t *testing.T) {
	db := setupTestDB(t)

	// Verify core tables exist
	tables := []string{"sessions", "messages", "tool_calls", "subagents"}
	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}

	// Verify FTS5 virtual table exists
	var ftsName string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='messages_fts'").Scan(&ftsName)
	if err != nil {
		t.Errorf("FTS5 virtual table messages_fts not found: %v", err)
	}
}

func TestInitDB_AlreadyExists(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db1, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("first InitDB failed: %v", err)
	}
	db1.Close()

	db2, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("second InitDB failed: %v", err)
	}
	db2.Close()
}

func TestInsertSession(t *testing.T) {
	db := setupTestDB(t)

	s := &models.Session{
		ID:               "sess-001",
		ProjectPath:      "/home/user/myproject",
		ProjectName:      "myproject",
		CWD:              "/home/user/myproject",
		GitBranch:        "main",
		StartedAt:        time.Now().UTC().Truncate(time.Second),
		ClaudeVersion:    "1.0.0",
		EntryPoint:       "cli",
		PermissionMode:   "default",
		TotalInputTokens: 100,
	}

	if err := InsertSession(db, s); err != nil {
		t.Fatalf("InsertSession failed: %v", err)
	}

	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM sessions WHERE id = ?", s.ID).Scan(&count)
	if err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 session, got %d", count)
	}
}

func TestInsertSession_UpsertUpdatesTokens(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now().UTC().Truncate(time.Second)
	s := &models.Session{
		ID:               "sess-upsert",
		ProjectPath:      "/home/user/proj",
		ProjectName:      "proj",
		StartedAt:        now,
		TotalInputTokens: 100,
	}
	if err := InsertSession(db, s); err != nil {
		t.Fatalf("first insert failed: %v", err)
	}

	ended := now.Add(5 * time.Minute)
	s.EndedAt = &ended
	s.TotalInputTokens = 500
	s.TotalOutputTokens = 200
	s.EstimatedCostUSD = 0.05
	if err := InsertSession(db, s); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	got, err := GetSessionByID(db, "sess-upsert")
	if err != nil {
		t.Fatalf("GetSessionByID failed: %v", err)
	}
	if got.TotalInputTokens != 500 {
		t.Errorf("expected input tokens 500, got %d", got.TotalInputTokens)
	}
	if got.TotalOutputTokens != 200 {
		t.Errorf("expected output tokens 200, got %d", got.TotalOutputTokens)
	}
	if got.EstimatedCostUSD != 0.05 {
		t.Errorf("expected cost 0.05, got %f", got.EstimatedCostUSD)
	}
	if got.EndedAt == nil {
		t.Error("expected ended_at to be set after upsert")
	}
}

func TestInsertSession_AutoPopulatesProjectName(t *testing.T) {
	db := setupTestDB(t)

	s := &models.Session{
		ID:          "sess-auto-name",
		ProjectPath: "/home/user/my-cool-project",
		StartedAt:   time.Now().UTC().Truncate(time.Second),
	}
	if err := InsertSession(db, s); err != nil {
		t.Fatalf("InsertSession failed: %v", err)
	}

	got, err := GetSessionByID(db, "sess-auto-name")
	if err != nil {
		t.Fatalf("GetSessionByID failed: %v", err)
	}
	if got.ProjectName != "my-cool-project" {
		t.Errorf("expected project_name 'my-cool-project', got %q", got.ProjectName)
	}
}

func TestInsertMessage(t *testing.T) {
	db := setupTestDB(t)

	// Insert a parent session first (FK constraint)
	s := &models.Session{
		ID:          "sess-msg",
		ProjectPath: "/tmp/proj",
		ProjectName: "proj",
		StartedAt:   time.Now().UTC().Truncate(time.Second),
	}
	if err := InsertSession(db, s); err != nil {
		t.Fatalf("InsertSession failed: %v", err)
	}

	m := &models.Message{
		ID:          "msg-001",
		SessionID:   "sess-msg",
		Type:        "assistant",
		Role:        "assistant",
		Model:       "claude-3",
		ContentText: "Hello from the database test",
		Timestamp:   time.Now().UTC().Truncate(time.Second),
	}
	if err := InsertMessage(db, m); err != nil {
		t.Fatalf("InsertMessage failed: %v", err)
	}

	// Verify the message was inserted
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM messages WHERE id = ?", m.ID).Scan(&count)
	if err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 message, got %d", count)
	}

	// Verify FTS index was populated
	var ftsCount int
	err = db.QueryRow("SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'database'").Scan(&ftsCount)
	if err != nil {
		t.Fatalf("FTS query failed: %v", err)
	}
	if ftsCount != 1 {
		t.Errorf("expected 1 FTS match for 'database', got %d", ftsCount)
	}
}

func TestInsertMessage_Dedup(t *testing.T) {
	db := setupTestDB(t)

	s := &models.Session{
		ID:          "sess-dedup",
		ProjectPath: "/tmp/proj",
		ProjectName: "proj",
		StartedAt:   time.Now().UTC().Truncate(time.Second),
	}
	if err := InsertSession(db, s); err != nil {
		t.Fatalf("InsertSession failed: %v", err)
	}

	m := &models.Message{
		ID:          "msg-dup",
		SessionID:   "sess-dedup",
		Type:        "user",
		ContentText: "original",
		Timestamp:   time.Now().UTC().Truncate(time.Second),
	}
	if err := InsertMessage(db, m); err != nil {
		t.Fatalf("first InsertMessage failed: %v", err)
	}

	// Insert again -- should not error (DO NOTHING)
	m.ContentText = "modified"
	if err := InsertMessage(db, m); err != nil {
		t.Fatalf("duplicate InsertMessage should not fail: %v", err)
	}

	// Original content should remain
	var text string
	err := db.QueryRow("SELECT content_text FROM messages WHERE id = ?", m.ID).Scan(&text)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if text != "original" {
		t.Errorf("expected original content to persist, got %q", text)
	}
}

func TestInsertToolCall(t *testing.T) {
	db := setupTestDB(t)

	s := &models.Session{
		ID:          "sess-tc",
		ProjectPath: "/tmp/proj",
		ProjectName: "proj",
		StartedAt:   time.Now().UTC().Truncate(time.Second),
	}
	if err := InsertSession(db, s); err != nil {
		t.Fatalf("InsertSession failed: %v", err)
	}

	m := &models.Message{
		ID:        "msg-tc",
		SessionID: "sess-tc",
		Type:      "assistant",
		Timestamp: time.Now().UTC().Truncate(time.Second),
	}
	if err := InsertMessage(db, m); err != nil {
		t.Fatalf("InsertMessage failed: %v", err)
	}

	tc := &models.ToolCall{
		ID:           "tc-001",
		MessageID:    "msg-tc",
		SessionID:    "sess-tc",
		ToolName:     "Read",
		ToolInput:    `{"path": "/tmp/file.txt"}`,
		ToolResponse: "file contents here",
		Success:      true,
		DurationMS:   42,
		Timestamp:    time.Now().UTC().Truncate(time.Second),
	}
	if err := InsertToolCall(db, tc); err != nil {
		t.Fatalf("InsertToolCall failed: %v", err)
	}

	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM tool_calls WHERE id = ?", tc.ID).Scan(&count)
	if err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 tool_call, got %d", count)
	}
}

func TestInsertSubagent(t *testing.T) {
	db := setupTestDB(t)

	s := &models.Session{
		ID:          "sess-sa",
		ProjectPath: "/tmp/proj",
		ProjectName: "proj",
		StartedAt:   time.Now().UTC().Truncate(time.Second),
	}
	if err := InsertSession(db, s); err != nil {
		t.Fatalf("InsertSession failed: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	sa := &models.Subagent{
		ID:          "sa-001",
		SessionID:   "sess-sa",
		AgentType:   "task",
		Description: "Doing some work",
		StartedAt:   now,
	}
	if err := InsertSubagent(db, sa); err != nil {
		t.Fatalf("InsertSubagent failed: %v", err)
	}

	// Upsert with ended_at
	ended := now.Add(2 * time.Minute)
	sa.EndedAt = &ended
	if err := InsertSubagent(db, sa); err != nil {
		t.Fatalf("InsertSubagent upsert failed: %v", err)
	}

	var endedStr sql.NullString
	err := db.QueryRow("SELECT ended_at FROM subagents WHERE id = ?", sa.ID).Scan(&endedStr)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if !endedStr.Valid {
		t.Error("expected ended_at to be set after upsert")
	}
}

func TestGetSessionByID(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now().UTC().Truncate(time.Second)
	s := &models.Session{
		ID:               "sess-get",
		ProjectPath:      "/home/user/proj",
		ProjectName:      "proj",
		CWD:              "/home/user/proj",
		GitBranch:        "feature-x",
		StartedAt:        now,
		ClaudeVersion:    "2.0.0",
		EntryPoint:       "hook",
		PermissionMode:   "auto",
		TotalInputTokens: 999,
		EstimatedCostUSD: 0.12,
	}
	if err := InsertSession(db, s); err != nil {
		t.Fatalf("InsertSession failed: %v", err)
	}

	got, err := GetSessionByID(db, "sess-get")
	if err != nil {
		t.Fatalf("GetSessionByID failed: %v", err)
	}

	if got.ID != s.ID {
		t.Errorf("ID: got %q, want %q", got.ID, s.ID)
	}
	if got.ProjectPath != s.ProjectPath {
		t.Errorf("ProjectPath: got %q, want %q", got.ProjectPath, s.ProjectPath)
	}
	if got.ProjectName != s.ProjectName {
		t.Errorf("ProjectName: got %q, want %q", got.ProjectName, s.ProjectName)
	}
	if got.GitBranch != s.GitBranch {
		t.Errorf("GitBranch: got %q, want %q", got.GitBranch, s.GitBranch)
	}
	if !got.StartedAt.Equal(s.StartedAt) {
		t.Errorf("StartedAt: got %v, want %v", got.StartedAt, s.StartedAt)
	}
	if got.TotalInputTokens != 999 {
		t.Errorf("TotalInputTokens: got %d, want 999", got.TotalInputTokens)
	}
	if got.EstimatedCostUSD != 0.12 {
		t.Errorf("EstimatedCostUSD: got %f, want 0.12", got.EstimatedCostUSD)
	}
}

func TestGetSessionByID_NotFound(t *testing.T) {
	db := setupTestDB(t)

	_, err := GetSessionByID(db, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session, got nil")
	}
}

func TestGetSessionsByProject(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 5; i++ {
		s := &models.Session{
			ID:          "sess-proj-" + string(rune('a'+i)),
			ProjectPath: "/home/user/project-a",
			ProjectName: "project-a",
			StartedAt:   now.Add(time.Duration(i) * time.Minute),
		}
		if err := InsertSession(db, s); err != nil {
			t.Fatalf("InsertSession %d failed: %v", i, err)
		}
	}

	// Insert a session for a different project
	other := &models.Session{
		ID:          "sess-other",
		ProjectPath: "/home/user/other",
		ProjectName: "other",
		StartedAt:   now,
	}
	if err := InsertSession(db, other); err != nil {
		t.Fatalf("InsertSession other failed: %v", err)
	}

	sessions, err := GetSessionsByProject(db, "/home/user/project-a", 3, 0)
	if err != nil {
		t.Fatalf("GetSessionsByProject failed: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}

	// Should be ordered by started_at DESC (most recent first)
	if sessions[0].StartedAt.Before(sessions[1].StartedAt) {
		t.Error("sessions not ordered by started_at DESC")
	}

	// With offset
	sessions2, err := GetSessionsByProject(db, "/home/user/project-a", 10, 3)
	if err != nil {
		t.Fatalf("GetSessionsByProject with offset failed: %v", err)
	}
	if len(sessions2) != 2 {
		t.Errorf("expected 2 sessions with offset 3, got %d", len(sessions2))
	}
}

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

func TestSearchMessages(t *testing.T) {
	db := setupTestDB(t)

	s := &models.Session{
		ID:          "sess-search",
		ProjectPath: "/tmp/proj",
		ProjectName: "proj",
		StartedAt:   time.Now().UTC().Truncate(time.Second),
	}
	if err := InsertSession(db, s); err != nil {
		t.Fatalf("InsertSession failed: %v", err)
	}

	msgs := []*models.Message{
		{
			ID:          "msg-s1",
			SessionID:   "sess-search",
			Type:        "assistant",
			ContentText: "The quantum flux capacitor needs recalibration",
			Timestamp:   time.Now().UTC().Truncate(time.Second),
		},
		{
			ID:          "msg-s2",
			SessionID:   "sess-search",
			Type:        "user",
			ContentText: "How do I fix the widget factory?",
			Timestamp:   time.Now().UTC().Truncate(time.Second),
		},
		{
			ID:          "msg-s3",
			SessionID:   "sess-search",
			Type:        "assistant",
			ContentText: "The widget factory requires a quantum entanglement reset",
			Timestamp:   time.Now().UTC().Truncate(time.Second),
		},
	}
	for _, m := range msgs {
		if err := InsertMessage(db, m); err != nil {
			t.Fatalf("InsertMessage %s failed: %v", m.ID, err)
		}
	}

	results, err := SearchMessages(db, "quantum", 10)
	if err != nil {
		t.Fatalf("SearchMessages failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 search results for 'quantum', got %d", len(results))
	}

	results2, err := SearchMessages(db, "widget", 1)
	if err != nil {
		t.Fatalf("SearchMessages failed: %v", err)
	}
	if len(results2) != 1 {
		t.Errorf("expected 1 result with limit 1, got %d", len(results2))
	}
}

func TestInsertSessionMetric(t *testing.T) {
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
