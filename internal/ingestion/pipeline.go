package ingestion

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/szaher/claude-monitor/internal/config"
	"github.com/szaher/claude-monitor/internal/db"
	"github.com/szaher/claude-monitor/internal/models"
	"github.com/szaher/claude-monitor/internal/parser"
)

// Pipeline is the central processor that takes raw data from both the receiver
// (hook events) and watcher (log entries), normalizes it, deduplicates, and
// writes to the database.
type Pipeline struct {
	database   *sql.DB
	batchSize  int
	batchTime  time.Duration
	stopCh     chan struct{}
	wg         sync.WaitGroup
	broadcast  func([]byte) // callback for WebSocket broadcasting
	costConfig map[string]config.ModelPricing
}

// NewPipeline creates a new Pipeline with default batch settings
// (50 events or 500ms flush interval).
func NewPipeline(database *sql.DB, broadcast func([]byte), costConfig map[string]config.ModelPricing) *Pipeline {
	if costConfig == nil {
		costConfig = make(map[string]config.ModelPricing)
	}
	return &Pipeline{
		database:   database,
		batchSize:  50,
		batchTime:  500 * time.Millisecond,
		stopCh:     make(chan struct{}),
		broadcast:  broadcast,
		costConfig: costConfig,
	}
}

// CalculateCost estimates the cost of a message based on model pricing.
func (p *Pipeline) CalculateCost(model string, inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int) float64 {
	pricing, ok := p.costConfig[model]
	if !ok {
		// Try partial match (model names may have version suffixes)
		for name, pr := range p.costConfig {
			if strings.Contains(model, name) || strings.Contains(name, model) {
				pricing = pr
				ok = true
				break
			}
		}
	}
	if !ok {
		return 0
	}

	cost := float64(inputTokens) / 1_000_000 * pricing.Input
	cost += float64(outputTokens) / 1_000_000 * pricing.Output
	cost += float64(cacheReadTokens) / 1_000_000 * pricing.CacheRead
	cost += float64(cacheWriteTokens) / 1_000_000 * pricing.CacheWrite

	return cost
}

// ProcessHookEvent processes a single hook event from the receiver.
// It unmarshals the JSON, dispatches based on hook_event_name, and
// optionally broadcasts the event via WebSocket.
func (p *Pipeline) ProcessHookEvent(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("unmarshal hook event: %w", err)
	}

	eventName, _ := raw["hook_event_name"].(string)
	sessionID, _ := raw["session_id"].(string)

	switch eventName {
	case "SessionStart":
		if err := p.handleSessionStart(raw, sessionID); err != nil {
			return err
		}

	case "SessionEnd", "Stop":
		if err := p.handleSessionEnd(sessionID); err != nil {
			return err
		}

	case "PostToolUse":
		if err := p.handlePostToolUse(raw, sessionID); err != nil {
			return err
		}

	case "SubagentStart":
		if err := p.handleSubagentStart(raw, sessionID); err != nil {
			return err
		}

	case "SubagentStop":
		if err := p.handleSubagentStop(raw, sessionID); err != nil {
			return err
		}

	case "PreToolUse":
		// No-op: PostToolUse has the complete data

	default:
		// Unknown event type, ignore
	}

	// Broadcast for real-time updates
	if p.broadcast != nil {
		p.broadcast(data)
	}

	return nil
}

// ProcessLogEntry processes a single JSONL log entry from the watcher.
// It parses the entry, creates/updates the session, and inserts messages
// and tool calls.
func (p *Pipeline) ProcessLogEntry(data []byte) error {
	entry, err := parser.ParseLogEntry(data)
	if err != nil {
		return fmt.Errorf("parse log entry: %w", err)
	}

	// Auto-create session if it doesn't exist
	if entry.SessionID != "" {
		if err := p.ensureSession(entry); err != nil {
			return err
		}
	}

	// Process system entries
	if entry.Type == "system" {
		return p.processSystemEntry(entry)
	}

	// Process attachment entries
	if entry.Type == "attachment" {
		return p.processAttachmentEntry(entry)
	}

	// Skip non-message types (permission-mode, file-history-snapshot, last-prompt, queue-operation)
	if entry.Type != "user" && entry.Type != "assistant" {
		return nil
	}

	contentText := parser.ExtractContentText(entry.Message.Content)
	contentJSON, _ := json.Marshal(entry.Message.Content)

	msg := &models.Message{
		ID:               entry.UUID,
		SessionID:        entry.SessionID,
		ParentID:         entry.ParentUUID,
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

	if err := db.InsertMessage(p.database, msg); err != nil {
		return fmt.Errorf("insert message: %w", err)
	}

	// For assistant messages, extract and insert tool_use blocks
	if entry.Type == "assistant" {
		toolCalls := parser.ExtractToolCalls(entry.Message.Content)
		for _, tc := range toolCalls {
			inputJSON, _ := json.Marshal(tc.Input)
			toolCall := &models.ToolCall{
				ID:        tc.ID,
				MessageID: entry.UUID,
				SessionID: entry.SessionID,
				ToolName:  tc.Name,
				ToolInput: string(inputJSON),
				Success:   true,
				Timestamp: entry.Timestamp,
			}
			if err := db.InsertToolCall(p.database, toolCall); err != nil {
				return fmt.Errorf("insert tool call: %w", err)
			}
		}

		// Update session token counts and cost by incrementing
		if entry.Message.Usage.InputTokens > 0 || entry.Message.Usage.OutputTokens > 0 {
			p.incrementSessionTokens(entry.SessionID, entry.Message.Usage, entry.Message.Model)
		}
	}

	return nil
}

func (p *Pipeline) processSystemEntry(entry *parser.LogEntry) error {
	metadata := map[string]interface{}{
		"subtype":       entry.Subtype,
		"duration_ms":   entry.DurationMs,
		"message_count": entry.MessageCount,
		"slug":          entry.Slug,
		"is_meta":       entry.IsMeta,
		"is_sidechain":  entry.IsSidechain,
	}
	contentJSON, _ := json.Marshal(metadata)

	contentText := ""
	if entry.Slug != "" {
		contentText = entry.Slug
	}
	if entry.Subtype != "" {
		contentText = entry.Subtype + ": " + contentText
	}

	msg := &models.Message{
		ID:          entry.UUID,
		SessionID:   entry.SessionID,
		ParentID:    entry.ParentUUID,
		Type:        "system",
		ContentText: contentText,
		ContentJSON: string(contentJSON),
		Timestamp:   entry.Timestamp,
	}

	return db.InsertMessage(p.database, msg)
}

func (p *Pipeline) processAttachmentEntry(entry *parser.LogEntry) error {
	if entry.Attachment == nil {
		return nil
	}

	metadata := map[string]interface{}{
		"attachment_type": entry.Attachment.Type,
		"hook_name":       entry.Attachment.HookName,
		"hook_event":      entry.Attachment.HookEvent,
		"content":         entry.Attachment.Content,
		"stdout":          entry.Attachment.Stdout,
		"stderr":          entry.Attachment.Stderr,
		"exit_code":       entry.Attachment.ExitCode,
		"command":         entry.Attachment.Command,
		"duration_ms":     entry.Attachment.DurationMs,
		"tool_use_id":     entry.Attachment.ToolUseID,
	}
	contentJSON, _ := json.Marshal(metadata)

	contentText := entry.Attachment.Type
	if entry.Attachment.HookName != "" {
		contentText += ": " + entry.Attachment.HookName
	}
	if entry.Attachment.Content != "" {
		contentText += " — " + entry.Attachment.Content
	}

	msg := &models.Message{
		ID:          entry.UUID,
		SessionID:   entry.SessionID,
		ParentID:    entry.ParentUUID,
		Type:        "attachment",
		ContentText: contentText,
		ContentJSON: string(contentJSON),
		Timestamp:   entry.Timestamp,
	}

	return db.InsertMessage(p.database, msg)
}

// StartBatchProcessor reads from eventCh, buffers events, and flushes
// them in batches (up to batchSize or after batchTime). It runs in a
// goroutine and stops when Stop() is called.
func (p *Pipeline) StartBatchProcessor(eventCh <-chan []byte) {
	p.wg.Add(1)
	defer p.wg.Done()

	batch := make([][]byte, 0, p.batchSize)
	timer := time.NewTimer(p.batchTime)
	defer timer.Stop()

	flush := func() {
		for _, data := range batch {
			// Try as hook event first, fall back to log entry
			if err := p.ProcessHookEvent(data); err != nil {
				p.ProcessLogEntry(data)
			}
		}
		batch = batch[:0]
		timer.Reset(p.batchTime)
	}

	for {
		select {
		case <-p.stopCh:
			// Flush remaining
			if len(batch) > 0 {
				flush()
			}
			return

		case data, ok := <-eventCh:
			if !ok {
				if len(batch) > 0 {
					flush()
				}
				return
			}
			batch = append(batch, data)
			if len(batch) >= p.batchSize {
				flush()
			}

		case <-timer.C:
			if len(batch) > 0 {
				flush()
			}
			timer.Reset(p.batchTime)
		}
	}
}

// Stop signals the batch processor to stop and waits for it to finish.
func (p *Pipeline) Stop() {
	close(p.stopCh)
	p.wg.Wait()
}

// handleSessionStart creates a new session from hook event data.
func (p *Pipeline) handleSessionStart(raw map[string]interface{}, sessionID string) error {
	cwd, _ := raw["cwd"].(string)
	gitBranch, _ := raw["gitBranch"].(string)
	version, _ := raw["version"].(string)
	entrypoint, _ := raw["entrypoint"].(string)
	permissionMode, _ := raw["permission_mode"].(string)

	projectPath := cwd
	projectName := filepath.Base(cwd)

	session := &models.Session{
		ID:             sessionID,
		ProjectPath:    projectPath,
		ProjectName:    projectName,
		CWD:            cwd,
		GitBranch:      gitBranch,
		StartedAt:      time.Now().UTC(),
		ClaudeVersion:  version,
		EntryPoint:     entrypoint,
		PermissionMode: permissionMode,
	}

	return db.InsertSession(p.database, session)
}

// handleSessionEnd updates the session's ended_at timestamp.
func (p *Pipeline) handleSessionEnd(sessionID string) error {
	now := time.Now().UTC()
	_, err := p.database.Exec(
		"UPDATE sessions SET ended_at = ? WHERE id = ?",
		now.Format(time.RFC3339), sessionID,
	)
	if err != nil {
		return fmt.Errorf("update session ended_at: %w", err)
	}
	return nil
}

// handlePostToolUse inserts a tool call record.
func (p *Pipeline) handlePostToolUse(raw map[string]interface{}, sessionID string) error {
	toolUseID, _ := raw["tool_use_id"].(string)
	toolName, _ := raw["tool_name"].(string)
	toolInput, _ := raw["tool_input"].(string)
	toolResponse, _ := raw["tool_response"].(string)
	errStr, _ := raw["error"].(string)
	messageID, _ := raw["message_id"].(string)

	tc := &models.ToolCall{
		ID:           toolUseID,
		MessageID:    messageID,
		SessionID:    sessionID,
		ToolName:     toolName,
		ToolInput:    toolInput,
		ToolResponse: toolResponse,
		Success:      errStr == "",
		Error:        errStr,
		Timestamp:    time.Now().UTC(),
	}

	return db.InsertToolCall(p.database, tc)
}

// handleSubagentStart inserts a new subagent record.
func (p *Pipeline) handleSubagentStart(raw map[string]interface{}, sessionID string) error {
	agentID, _ := raw["agent_id"].(string)
	agentType, _ := raw["agent_type"].(string)

	sa := &models.Subagent{
		ID:        agentID,
		SessionID: sessionID,
		AgentType: agentType,
		StartedAt: time.Now().UTC(),
	}

	return db.InsertSubagent(p.database, sa)
}

// handleSubagentStop updates the subagent's ended_at timestamp.
func (p *Pipeline) handleSubagentStop(raw map[string]interface{}, sessionID string) error {
	agentID, _ := raw["agent_id"].(string)
	now := time.Now().UTC()

	sa := &models.Subagent{
		ID:        agentID,
		SessionID: sessionID,
		StartedAt: now, // will be ignored by upsert's ON CONFLICT
		EndedAt:   &now,
	}

	return db.InsertSubagent(p.database, sa)
}

// ensureSession creates a session from log entry metadata if it doesn't exist.
func (p *Pipeline) ensureSession(entry *parser.LogEntry) error {
	projectPath := entry.CWD
	projectName := filepath.Base(entry.CWD)
	if projectPath == "" {
		projectPath = "unknown"
		projectName = "unknown"
	}

	session := &models.Session{
		ID:             entry.SessionID,
		ProjectPath:    projectPath,
		ProjectName:    projectName,
		CWD:            entry.CWD,
		GitBranch:      entry.GitBranch,
		StartedAt:      entry.Timestamp,
		ClaudeVersion:  entry.Version,
		EntryPoint:     entry.EntryPoint,
		PermissionMode: entry.PermissionMode,
	}

	return db.InsertSession(p.database, session)
}

// incrementSessionTokens adds usage tokens to the session's running totals
// and updates the estimated cost.
func (p *Pipeline) incrementSessionTokens(sessionID string, usage parser.Usage, model string) {
	cost := p.CalculateCost(model, usage.InputTokens, usage.OutputTokens,
		usage.CacheReadInputTokens, usage.CacheCreationInputTokens)

	p.database.Exec(`
		UPDATE sessions SET
			total_input_tokens = total_input_tokens + ?,
			total_output_tokens = total_output_tokens + ?,
			total_cache_read_tokens = total_cache_read_tokens + ?,
			total_cache_write_tokens = total_cache_write_tokens + ?,
			estimated_cost_usd = estimated_cost_usd + ?
		WHERE id = ?`,
		usage.InputTokens, usage.OutputTokens,
		usage.CacheReadInputTokens, usage.CacheCreationInputTokens,
		cost, sessionID,
	)
}
