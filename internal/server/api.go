package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/szaher/claude-monitor/internal/config"
	"github.com/szaher/claude-monitor/internal/db"
)

// writeJSON writes v as JSON with the given HTTP status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// intParam reads an integer query parameter with a default value.
func intParam(r *http.Request, name string, def int) int {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// handleSessions handles GET /api/sessions — list sessions with filtering.
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	project := r.URL.Query().Get("project")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	limit := intParam(r, "limit", 50)
	offset := intParam(r, "offset", 0)

	// Build query dynamically
	query := `SELECT id, project_path, project_name, cwd, git_branch,
		started_at, ended_at, claude_version, entry_point, permission_mode,
		total_input_tokens, total_output_tokens, total_cache_read_tokens,
		total_cache_write_tokens, estimated_cost_usd
		FROM sessions WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM sessions WHERE 1=1`

	var args []interface{}

	if project != "" {
		query += " AND (project_path = ? OR project_name = ?)"
		countQuery += " AND (project_path = ? OR project_name = ?)"
		args = append(args, project, project)
	}
	if from != "" {
		query += " AND started_at >= ?"
		countQuery += " AND started_at >= ?"
		args = append(args, from)
	}
	if to != "" {
		query += " AND started_at <= ?"
		countQuery += " AND started_at <= ?"
		args = append(args, to)
	}

	// Get total count
	var total int
	err := s.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query count: "+err.Error())
		return
	}

	// Get paginated results
	query += " ORDER BY started_at DESC LIMIT ? OFFSET ?"
	queryArgs := append(args, limit, offset)

	rows, err := s.db.Query(query, queryArgs...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query sessions: "+err.Error())
		return
	}
	defer rows.Close()

	sessions := []map[string]interface{}{}
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "scan session: "+err.Error())
			return
		}
		sessions = append(sessions, sess)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "rows: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sessions": sessions,
		"total":    total,
	})
}

// handleSessionDetail handles GET /api/sessions/{id} — session detail with messages.
func (s *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract session ID from URL path: /api/sessions/{id}
	id := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing session id")
		return
	}

	session, err := db.GetSessionByID(s.db, id)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			writeError(w, http.StatusNotFound, "session not found")
		} else {
			writeError(w, http.StatusInternalServerError, "get session: "+err.Error())
		}
		return
	}

	// Get messages for this session
	messages, err := s.queryMessages(id, "", 1000, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query messages: "+err.Error())
		return
	}

	// Get tool calls for this session
	toolCalls, err := s.queryToolCalls(id, "", 1000, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query tool_calls: "+err.Error())
		return
	}

	// Get subagents for this session
	subagents, err := s.querySubagents(id, 1000, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query subagents: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"session":    session,
		"messages":   messages,
		"tool_calls": toolCalls,
		"subagents":  subagents,
	})
}

// handleMessages handles GET /api/messages — list messages.
func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sessionID := r.URL.Query().Get("session_id")
	msgType := r.URL.Query().Get("type")
	limit := intParam(r, "limit", 50)
	offset := intParam(r, "offset", 0)

	messages, err := s.queryMessages(sessionID, msgType, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query messages: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"messages": messages,
	})
}

// handleToolCalls handles GET /api/tool-calls — list tool calls.
func (s *Server) handleToolCalls(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sessionID := r.URL.Query().Get("session_id")
	toolName := r.URL.Query().Get("tool_name")
	limit := intParam(r, "limit", 50)
	offset := intParam(r, "offset", 0)

	toolCalls, err := s.queryToolCalls(sessionID, toolName, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query tool_calls: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tool_calls": toolCalls,
	})
}

// handleSubagents handles GET /api/subagents — list subagents.
func (s *Server) handleSubagents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sessionID := r.URL.Query().Get("session_id")
	limit := intParam(r, "limit", 50)
	offset := intParam(r, "offset", 0)

	subagents, err := s.querySubagents(sessionID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query subagents: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"subagents": subagents,
	})
}

// handleStats handles GET /api/stats — dashboard aggregate stats.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	today := time.Now().UTC().Format("2006-01-02")

	// Today's stats
	var todaySessions, todayToolCalls, todayTokens int
	var todayCost float64

	s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE date(started_at) = ?`, today).Scan(&todaySessions)
	s.db.QueryRow(`SELECT COUNT(*) FROM tool_calls WHERE date(timestamp) = ?`, today).Scan(&todayToolCalls)
	s.db.QueryRow(`SELECT COALESCE(SUM(total_input_tokens + total_output_tokens), 0) FROM sessions WHERE date(started_at) = ?`, today).Scan(&todayTokens)
	s.db.QueryRow(`SELECT COALESCE(SUM(estimated_cost_usd), 0) FROM sessions WHERE date(started_at) = ?`, today).Scan(&todayCost)

	// Total stats
	var totalSessions, totalToolCalls, totalTokens int
	var totalCost float64

	s.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&totalSessions)
	s.db.QueryRow(`SELECT COUNT(*) FROM tool_calls`).Scan(&totalToolCalls)
	s.db.QueryRow(`SELECT COALESCE(SUM(total_input_tokens + total_output_tokens), 0) FROM sessions`).Scan(&totalTokens)
	s.db.QueryRow(`SELECT COALESCE(SUM(estimated_cost_usd), 0) FROM sessions`).Scan(&totalCost)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"today": map[string]interface{}{
			"sessions":   todaySessions,
			"tool_calls": todayToolCalls,
			"tokens":     todayTokens,
			"cost":       todayCost,
		},
		"total": map[string]interface{}{
			"sessions":   totalSessions,
			"tool_calls": totalToolCalls,
			"tokens":     totalTokens,
			"cost":       totalCost,
		},
	})
}

// handleDailyStats handles GET /api/stats/daily — daily activity for charts.
func (s *Server) handleDailyStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	days := intParam(r, "days", 90)

	rows, err := s.db.Query(`
		SELECT date(started_at) as day,
			COUNT(*) as sessions,
			COALESCE(SUM(total_input_tokens + total_output_tokens), 0) as tokens,
			COALESCE(SUM(estimated_cost_usd), 0) as cost
		FROM sessions
		WHERE started_at >= datetime('now', ?)
		GROUP BY day
		ORDER BY day ASC
	`, fmt.Sprintf("-%d days", days))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query daily stats: "+err.Error())
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var day string
		var sessions, tokens int
		var cost float64
		if err := rows.Scan(&day, &sessions, &tokens, &cost); err != nil {
			writeError(w, http.StatusInternalServerError, "scan daily stat: "+err.Error())
			return
		}
		result = append(result, map[string]interface{}{
			"date":     day,
			"sessions": sessions,
			"tokens":   tokens,
			"cost":     cost,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"days": result,
	})
}

// handleToolStats handles GET /api/stats/tools — tool usage breakdown.
func (s *Server) handleToolStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rows, err := s.db.Query(`
		SELECT tool_name,
			COUNT(*) as count,
			CASE WHEN COUNT(*) > 0
				THEN CAST(SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) AS REAL) / COUNT(*)
				ELSE 0
			END as success_rate
		FROM tool_calls
		GROUP BY tool_name
		ORDER BY count DESC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query tool stats: "+err.Error())
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var name string
		var count int
		var successRate float64
		if err := rows.Scan(&name, &count, &successRate); err != nil {
			writeError(w, http.StatusInternalServerError, "scan tool stat: "+err.Error())
			return
		}
		result = append(result, map[string]interface{}{
			"name":         name,
			"count":        count,
			"success_rate": successRate,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tools": result,
	})
}

// handleModelStats handles GET /api/stats/models — model usage breakdown.
func (s *Server) handleModelStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rows, err := s.db.Query(`
		SELECT COALESCE(model, 'unknown') as model_name,
			COALESCE(SUM(input_tokens + output_tokens), 0) as tokens,
			0 as cost
		FROM messages
		WHERE model IS NOT NULL AND model != ''
		GROUP BY model
		ORDER BY tokens DESC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query model stats: "+err.Error())
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var name string
		var tokens int
		var cost float64
		if err := rows.Scan(&name, &tokens, &cost); err != nil {
			writeError(w, http.StatusInternalServerError, "scan model stat: "+err.Error())
			return
		}
		result = append(result, map[string]interface{}{
			"name":   name,
			"tokens": tokens,
			"cost":   cost,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"models": result,
	})
}

// handleProjectStats handles GET /api/stats/projects — project activity breakdown.
func (s *Server) handleProjectStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rows, err := s.db.Query(`
		SELECT project_name, project_path,
			COUNT(*) as sessions,
			COALESCE(SUM(estimated_cost_usd), 0) as cost
		FROM sessions
		GROUP BY project_path
		ORDER BY sessions DESC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query project stats: "+err.Error())
		return
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var name, path string
		var sessions int
		var cost float64
		if err := rows.Scan(&name, &path, &sessions, &cost); err != nil {
			writeError(w, http.StatusInternalServerError, "scan project stat: "+err.Error())
			return
		}
		result = append(result, map[string]interface{}{
			"name":     name,
			"path":     path,
			"sessions": sessions,
			"cost":     cost,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"projects": result,
	})
}

// handleConfig handles GET/POST /api/config — get or update config.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.config)

	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "read body: "+err.Error())
			return
		}

		// Unmarshal into the existing config (overlay)
		if err := json.Unmarshal(body, s.config); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
			return
		}

		// Save to disk
		home, _ := os.UserHomeDir()
		configPath := filepath.Join(home, ".claude-monitor", "config.yaml")
		if err := config.Save(configPath, s.config); err != nil {
			writeError(w, http.StatusInternalServerError, "save config: "+err.Error())
			return
		}

		writeJSON(w, http.StatusOK, s.config)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleSearch handles GET /api/search — full-text search on messages.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "missing query parameter 'q'")
		return
	}

	limit := intParam(r, "limit", 50)

	messages, err := db.SearchMessages(s.db, q, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"messages": messages,
	})
}

// --- Helper query functions ---

// queryMessages queries messages with optional session_id and type filters.
func (s *Server) queryMessages(sessionID, msgType string, limit, offset int) ([]map[string]interface{}, error) {
	query := `SELECT id, session_id, parent_id, type, role, model,
		content_text, content_json, stop_reason,
		input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
		timestamp
		FROM messages WHERE 1=1`
	var args []interface{}

	if sessionID != "" {
		query += " AND session_id = ?"
		args = append(args, sessionID)
	}
	if msgType != "" {
		query += " AND type = ?"
		args = append(args, msgType)
	}

	query += " ORDER BY timestamp ASC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id, sessionID, msgType, timestamp string
		var parentID, role, model, contentText, contentJSON, stopReason sql.NullString
		var inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int

		if err := rows.Scan(&id, &sessionID, &parentID, &msgType, &role, &model,
			&contentText, &contentJSON, &stopReason,
			&inputTokens, &outputTokens, &cacheReadTokens, &cacheWriteTokens,
			&timestamp); err != nil {
			return nil, err
		}

		msg := map[string]interface{}{
			"id":                 id,
			"session_id":        sessionID,
			"parent_id":         parentID.String,
			"type":              msgType,
			"role":              role.String,
			"model":             model.String,
			"content_text":      contentText.String,
			"content_json":      contentJSON.String,
			"stop_reason":       stopReason.String,
			"input_tokens":      inputTokens,
			"output_tokens":     outputTokens,
			"cache_read_tokens": cacheReadTokens,
			"cache_write_tokens": cacheWriteTokens,
			"timestamp":         timestamp,
		}
		result = append(result, msg)
	}
	return result, rows.Err()
}

// queryToolCalls queries tool_calls with optional session_id and tool_name filters.
func (s *Server) queryToolCalls(sessionID, toolName string, limit, offset int) ([]map[string]interface{}, error) {
	query := `SELECT id, message_id, session_id, tool_name,
		tool_input, tool_response, success, error, duration_ms, timestamp
		FROM tool_calls WHERE 1=1`
	var args []interface{}

	if sessionID != "" {
		query += " AND session_id = ?"
		args = append(args, sessionID)
	}
	if toolName != "" {
		query += " AND tool_name = ?"
		args = append(args, toolName)
	}

	query += " ORDER BY timestamp ASC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id, messageID, sessionID, toolName, timestamp string
		var toolInput, toolResponse, errStr sql.NullString
		var success bool
		var durationMS sql.NullInt64

		if err := rows.Scan(&id, &messageID, &sessionID, &toolName,
			&toolInput, &toolResponse, &success, &errStr, &durationMS,
			&timestamp); err != nil {
			return nil, err
		}

		tc := map[string]interface{}{
			"id":            id,
			"message_id":    messageID,
			"session_id":    sessionID,
			"tool_name":     toolName,
			"tool_input":    toolInput.String,
			"tool_response": toolResponse.String,
			"success":       success,
			"error":         errStr.String,
			"duration_ms":   durationMS.Int64,
			"timestamp":     timestamp,
		}
		result = append(result, tc)
	}
	return result, rows.Err()
}

// querySubagents queries subagents with an optional session_id filter.
func (s *Server) querySubagents(sessionID string, limit, offset int) ([]map[string]interface{}, error) {
	query := `SELECT id, session_id, agent_type, description, started_at, ended_at
		FROM subagents WHERE 1=1`
	var args []interface{}

	if sessionID != "" {
		query += " AND session_id = ?"
		args = append(args, sessionID)
	}

	query += " ORDER BY started_at ASC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id, sessionID, agentType, startedAt string
		var description, endedAt sql.NullString

		if err := rows.Scan(&id, &sessionID, &agentType, &description, &startedAt, &endedAt); err != nil {
			return nil, err
		}

		sa := map[string]interface{}{
			"id":          id,
			"session_id":  sessionID,
			"agent_type":  agentType,
			"description": description.String,
			"started_at":  startedAt,
			"ended_at":    endedAt.String,
		}
		result = append(result, sa)
	}
	return result, rows.Err()
}

// scanSession scans a session row into a map.
func scanSession(rows *sql.Rows) (map[string]interface{}, error) {
	var id, projectPath, projectName, startedAt string
	var cwd, gitBranch, claudeVersion, entryPoint, permissionMode, endedAt sql.NullString
	var totalInput, totalOutput, totalCacheRead, totalCacheWrite int
	var estimatedCost float64

	if err := rows.Scan(&id, &projectPath, &projectName, &cwd, &gitBranch,
		&startedAt, &endedAt, &claudeVersion, &entryPoint, &permissionMode,
		&totalInput, &totalOutput, &totalCacheRead, &totalCacheWrite,
		&estimatedCost); err != nil {
		return nil, err
	}

	sess := map[string]interface{}{
		"id":                       id,
		"project_path":             projectPath,
		"project_name":             projectName,
		"cwd":                      cwd.String,
		"git_branch":               gitBranch.String,
		"started_at":               startedAt,
		"ended_at":                 endedAt.String,
		"claude_version":           claudeVersion.String,
		"entry_point":              entryPoint.String,
		"permission_mode":          permissionMode.String,
		"total_input_tokens":       totalInput,
		"total_output_tokens":      totalOutput,
		"total_cache_read_tokens":  totalCacheRead,
		"total_cache_write_tokens": totalCacheWrite,
		"estimated_cost_usd":       estimatedCost,
	}

	return sess, nil
}
