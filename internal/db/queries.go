package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	"github.com/szaher/claude-monitor/internal/models"
)

// InsertSession inserts a session or updates metadata on conflict.
// Token counts and cost are NOT touched on conflict — they are managed
// exclusively by incrementSessionTokens (real-time) or UpdateSessionTokens (import).
func InsertSession(db *sql.DB, s *models.Session) error {
	if s.ProjectName == "" {
		s.ProjectName = filepath.Base(s.ProjectPath)
	}

	var endedAt *string
	if s.EndedAt != nil {
		t := s.EndedAt.UTC().Format(time.RFC3339)
		endedAt = &t
	}

	_, err := db.Exec(`
		INSERT INTO sessions (
			id, project_path, project_name, cwd, git_branch,
			started_at, ended_at, claude_version, entry_point, permission_mode,
			total_input_tokens, total_output_tokens, total_cache_read_tokens,
			total_cache_write_tokens, estimated_cost_usd
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			ended_at = COALESCE(excluded.ended_at, ended_at),
			cwd = CASE WHEN excluded.cwd != '' THEN excluded.cwd ELSE cwd END,
			git_branch = CASE WHEN excluded.git_branch != '' THEN excluded.git_branch ELSE git_branch END,
			claude_version = CASE WHEN excluded.claude_version != '' THEN excluded.claude_version ELSE claude_version END
	`,
		s.ID, s.ProjectPath, s.ProjectName, s.CWD, s.GitBranch,
		s.StartedAt.UTC().Format(time.RFC3339), endedAt,
		s.ClaudeVersion, s.EntryPoint, s.PermissionMode,
		s.TotalInputTokens, s.TotalOutputTokens,
		s.TotalCacheReadTokens, s.TotalCacheWriteTokens,
		s.EstimatedCostUSD,
	)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// UpdateSessionTokens sets the accumulated token counts and cost for a session.
// Used by the import command after processing all messages.
func UpdateSessionTokens(db *sql.DB, sessionID string, inputTokens, outputTokens, cacheRead, cacheWrite int, cost float64) error {
	_, err := db.Exec(`
		UPDATE sessions SET
			total_input_tokens = ?,
			total_output_tokens = ?,
			total_cache_read_tokens = ?,
			total_cache_write_tokens = ?,
			estimated_cost_usd = ?
		WHERE id = ?`,
		inputTokens, outputTokens, cacheRead, cacheWrite, cost, sessionID,
	)
	if err != nil {
		return fmt.Errorf("update session tokens: %w", err)
	}
	return nil
}

// InsertMessage inserts a message. Duplicates (same id) are silently ignored.
func InsertMessage(db *sql.DB, m *models.Message) error {
	_, err := db.Exec(`
		INSERT INTO messages (
			id, session_id, parent_id, type, role, model,
			content_text, content_json, stop_reason,
			input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
			timestamp
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`,
		m.ID, m.SessionID, m.ParentID, m.Type, m.Role, m.Model,
		m.ContentText, m.ContentJSON, m.StopReason,
		m.InputTokens, m.OutputTokens, m.CacheReadTokens, m.CacheWriteTokens,
		m.Timestamp.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	return nil
}

// InsertToolCall inserts a tool call. Duplicates (same id) are silently ignored.
func InsertToolCall(db *sql.DB, tc *models.ToolCall) error {
	_, err := db.Exec(`
		INSERT INTO tool_calls (
			id, message_id, session_id, tool_name,
			tool_input, tool_response, success, error, duration_ms,
			timestamp
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`,
		tc.ID, tc.MessageID, tc.SessionID, tc.ToolName,
		tc.ToolInput, tc.ToolResponse, tc.Success, tc.Error, tc.DurationMS,
		tc.Timestamp.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert tool call: %w", err)
	}
	return nil
}

// InsertSubagent inserts or updates a subagent. On conflict, it updates ended_at.
func InsertSubagent(db *sql.DB, sa *models.Subagent) error {
	var endedAt *string
	if sa.EndedAt != nil {
		t := sa.EndedAt.UTC().Format(time.RFC3339)
		endedAt = &t
	}

	_, err := db.Exec(`
		INSERT INTO subagents (
			id, session_id, agent_type, description, started_at, ended_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			ended_at = excluded.ended_at
	`,
		sa.ID, sa.SessionID, sa.AgentType, sa.Description,
		sa.StartedAt.UTC().Format(time.RFC3339), endedAt,
	)
	if err != nil {
		return fmt.Errorf("insert subagent: %w", err)
	}
	return nil
}

// GetSessionByID retrieves a single session by its ID.
func GetSessionByID(db *sql.DB, id string) (*models.Session, error) {
	s := &models.Session{}
	var startedAt string
	var endedAt sql.NullString
	var cwd, gitBranch, claudeVersion, entryPoint, permissionMode sql.NullString
	var notes, tags sql.NullString

	err := db.QueryRow(`
		SELECT id, project_path, project_name, cwd, git_branch,
			started_at, ended_at, claude_version, entry_point, permission_mode,
			total_input_tokens, total_output_tokens, total_cache_read_tokens,
			total_cache_write_tokens, estimated_cost_usd,
			COALESCE(notes,'') as notes, COALESCE(tags,'') as tags
		FROM sessions WHERE id = ?
	`, id).Scan(
		&s.ID, &s.ProjectPath, &s.ProjectName,
		&cwd, &gitBranch,
		&startedAt, &endedAt,
		&claudeVersion, &entryPoint, &permissionMode,
		&s.TotalInputTokens, &s.TotalOutputTokens,
		&s.TotalCacheReadTokens, &s.TotalCacheWriteTokens,
		&s.EstimatedCostUSD,
		&notes, &tags,
	)
	if err != nil {
		return nil, fmt.Errorf("get session %s: %w", id, err)
	}

	s.CWD = cwd.String
	s.GitBranch = gitBranch.String
	s.ClaudeVersion = claudeVersion.String
	s.EntryPoint = entryPoint.String
	s.PermissionMode = permissionMode.String
	s.Notes = notes.String
	s.Tags = tags.String

	t, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return nil, fmt.Errorf("parse started_at: %w", err)
	}
	s.StartedAt = t

	if endedAt.Valid {
		t, err := time.Parse(time.RFC3339, endedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse ended_at: %w", err)
		}
		s.EndedAt = &t
	}

	return s, nil
}

// GetSessionsByProject returns sessions for a given project path,
// ordered by started_at DESC, with pagination via limit and offset.
func GetSessionsByProject(db *sql.DB, projectPath string, limit, offset int) ([]*models.Session, error) {
	rows, err := db.Query(`
		SELECT id, project_path, project_name, cwd, git_branch,
			started_at, ended_at, claude_version, entry_point, permission_mode,
			total_input_tokens, total_output_tokens, total_cache_read_tokens,
			total_cache_write_tokens, estimated_cost_usd
		FROM sessions
		WHERE project_path = ?
		ORDER BY started_at DESC
		LIMIT ? OFFSET ?
	`, projectPath, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query sessions by project: %w", err)
	}
	defer rows.Close()

	var sessions []*models.Session
	for rows.Next() {
		s := &models.Session{}
		var startedAt string
		var endedAt sql.NullString
		var cwd, gitBranch, claudeVersion, entryPoint, permissionMode sql.NullString

		if err := rows.Scan(
			&s.ID, &s.ProjectPath, &s.ProjectName,
			&cwd, &gitBranch,
			&startedAt, &endedAt,
			&claudeVersion, &entryPoint, &permissionMode,
			&s.TotalInputTokens, &s.TotalOutputTokens,
			&s.TotalCacheReadTokens, &s.TotalCacheWriteTokens,
			&s.EstimatedCostUSD,
		); err != nil {
			return nil, fmt.Errorf("scan session row: %w", err)
		}

		s.CWD = cwd.String
		s.GitBranch = gitBranch.String
		s.ClaudeVersion = claudeVersion.String
		s.EntryPoint = entryPoint.String
		s.PermissionMode = permissionMode.String

		t, err := time.Parse(time.RFC3339, startedAt)
		if err != nil {
			return nil, fmt.Errorf("parse started_at: %w", err)
		}
		s.StartedAt = t

		if endedAt.Valid {
			t, err := time.Parse(time.RFC3339, endedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse ended_at: %w", err)
			}
			s.EndedAt = &t
		}

		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return sessions, nil
}

// SearchMessages performs a full-text search on message content using FTS5.
func SearchMessages(db *sql.DB, query string, limit int) ([]*models.Message, error) {
	rows, err := db.Query(`
		SELECT m.id, m.session_id, m.parent_id, m.type, m.role, m.model,
			m.content_text, m.content_json, m.stop_reason,
			m.input_tokens, m.output_tokens, m.cache_read_tokens, m.cache_write_tokens,
			m.timestamp
		FROM messages m
		JOIN messages_fts fts ON m.id = fts.id
		WHERE messages_fts MATCH ?
		LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search messages: %w", err)
	}
	defer rows.Close()

	var messages []*models.Message
	for rows.Next() {
		m := &models.Message{}
		var ts string
		var parentID, role, model, contentText, contentJSON, stopReason sql.NullString

		if err := rows.Scan(
			&m.ID, &m.SessionID, &parentID, &m.Type, &role, &model,
			&contentText, &contentJSON, &stopReason,
			&m.InputTokens, &m.OutputTokens, &m.CacheReadTokens, &m.CacheWriteTokens,
			&ts,
		); err != nil {
			return nil, fmt.Errorf("scan message row: %w", err)
		}

		m.ParentID = parentID.String
		m.Role = role.String
		m.Model = model.String
		m.ContentText = contentText.String
		m.ContentJSON = contentJSON.String
		m.StopReason = stopReason.String

		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return nil, fmt.Errorf("parse timestamp: %w", err)
		}
		m.Timestamp = t

		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return messages, nil
}

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
