package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	"github.com/szaher/claude-monitor/internal/models"
)

// InsertSession inserts or updates a session. On conflict, it updates
// ended_at, token counts, and estimated cost (upsert).
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
			ended_at = excluded.ended_at,
			total_input_tokens = excluded.total_input_tokens,
			total_output_tokens = excluded.total_output_tokens,
			total_cache_read_tokens = excluded.total_cache_read_tokens,
			total_cache_write_tokens = excluded.total_cache_write_tokens,
			estimated_cost_usd = excluded.estimated_cost_usd
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

	err := db.QueryRow(`
		SELECT id, project_path, project_name, cwd, git_branch,
			started_at, ended_at, claude_version, entry_point, permission_mode,
			total_input_tokens, total_output_tokens, total_cache_read_tokens,
			total_cache_write_tokens, estimated_cost_usd
		FROM sessions WHERE id = ?
	`, id).Scan(
		&s.ID, &s.ProjectPath, &s.ProjectName,
		&cwd, &gitBranch,
		&startedAt, &endedAt,
		&claudeVersion, &entryPoint, &permissionMode,
		&s.TotalInputTokens, &s.TotalOutputTokens,
		&s.TotalCacheReadTokens, &s.TotalCacheWriteTokens,
		&s.EstimatedCostUSD,
	)
	if err != nil {
		return nil, fmt.Errorf("get session %s: %w", id, err)
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
