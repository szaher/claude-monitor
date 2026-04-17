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
		total_cache_write_tokens, estimated_cost_usd,
		COALESCE(notes,'') as notes, COALESCE(tags,'') as tags
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
	tag := r.URL.Query().Get("tag")
	if tag != "" {
		query += " AND (',' || tags || ',' LIKE '%,' || ? || ',%')"
		countQuery += " AND (',' || tags || ',' LIKE '%,' || ? || ',%')"
		args = append(args, tag)
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

	if r.Method == http.MethodPatch {
		s.handleSessionPatch(w, r)
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract session ID from URL path: /api/sessions/{id}
	id := path
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
// Supports optional query params: project, from, to.
// When filters are set, "filtered" stats replace "today" stats.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	project := r.URL.Query().Get("project")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	hasFilter := project != "" || from != "" || to != ""

	// Build dynamic WHERE clauses for sessions and tool_calls
	sessWhere := "1=1"
	tcWhere := "1=1"
	var sessArgs, tcArgs []interface{}

	if project != "" {
		sessWhere += " AND project_path = ?"
		sessArgs = append(sessArgs, project)
		tcWhere += " AND session_id IN (SELECT id FROM sessions WHERE project_path = ?)"
		tcArgs = append(tcArgs, project)
	}
	if from != "" {
		sessWhere += " AND started_at >= ?"
		sessArgs = append(sessArgs, from)
		tcWhere += " AND timestamp >= ?"
		tcArgs = append(tcArgs, from)
	}
	if to != "" {
		sessWhere += " AND started_at <= ?"
		sessArgs = append(sessArgs, to)
		tcWhere += " AND timestamp <= ?"
		tcArgs = append(tcArgs, to)
	}

	// Period stats — either filtered range or today
	var periodSessions, periodToolCalls, periodTokens int
	var periodCost float64
	periodLabel := "today"

	if hasFilter {
		periodLabel = "filtered"
		s.db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM sessions WHERE %s`, sessWhere), sessArgs...).Scan(&periodSessions)
		s.db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM tool_calls WHERE %s`, tcWhere), tcArgs...).Scan(&periodToolCalls)
		s.db.QueryRow(fmt.Sprintf(`SELECT COALESCE(SUM(total_input_tokens + total_output_tokens), 0) FROM sessions WHERE %s`, sessWhere), sessArgs...).Scan(&periodTokens)
		s.db.QueryRow(fmt.Sprintf(`SELECT COALESCE(SUM(estimated_cost_usd), 0) FROM sessions WHERE %s`, sessWhere), sessArgs...).Scan(&periodCost)
	} else {
		today := time.Now().UTC().Format("2006-01-02")
		s.db.QueryRow(`
			SELECT COUNT(*) FROM (
				SELECT id FROM sessions WHERE date(started_at) = ?
				UNION
				SELECT DISTINCT session_id FROM tool_calls WHERE date(timestamp) = ?
			)`, today, today).Scan(&periodSessions)
		s.db.QueryRow(`SELECT COUNT(*) FROM tool_calls WHERE date(timestamp) = ?`, today).Scan(&periodToolCalls)
		s.db.QueryRow(`SELECT COALESCE(SUM(total_input_tokens + total_output_tokens), 0) FROM sessions WHERE date(started_at) = ?`, today).Scan(&periodTokens)
		s.db.QueryRow(`SELECT COALESCE(SUM(estimated_cost_usd), 0) FROM sessions WHERE date(started_at) = ?`, today).Scan(&periodCost)
	}

	// Active sessions
	var activeSessions int
	s.db.QueryRow(`
		SELECT COUNT(DISTINCT session_id) FROM tool_calls
		WHERE timestamp >= datetime('now', '-15 minutes')
	`).Scan(&activeSessions)

	// Total stats (always unfiltered)
	var totalSessions, totalToolCalls, totalTokens int
	var totalCost float64

	s.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&totalSessions)
	s.db.QueryRow(`SELECT COUNT(*) FROM tool_calls`).Scan(&totalToolCalls)
	s.db.QueryRow(`SELECT COALESCE(SUM(total_input_tokens + total_output_tokens), 0) FROM sessions`).Scan(&totalTokens)
	s.db.QueryRow(`SELECT COALESCE(SUM(estimated_cost_usd), 0) FROM sessions`).Scan(&totalCost)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"period_label": periodLabel,
		"today": map[string]interface{}{
			"sessions":   periodSessions,
			"tool_calls": periodToolCalls,
			"tokens":     periodTokens,
			"cost":       periodCost,
		},
		"total": map[string]interface{}{
			"sessions":   totalSessions,
			"tool_calls": totalToolCalls,
			"tokens":     totalTokens,
			"cost":       totalCost,
		},
		"active_sessions": activeSessions,
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

// handleSkillStats handles GET /api/stats/skills — global skills usage stats.
func (s *Server) handleSkillStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rows, err := s.db.Query(`SELECT tool_input FROM tool_calls WHERE tool_name = 'Skill'`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query skill stats: "+err.Error())
		return
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var toolInput sql.NullString
		if err := rows.Scan(&toolInput); err != nil {
			writeError(w, http.StatusInternalServerError, "scan skill stat: "+err.Error())
			return
		}
		if !toolInput.Valid || toolInput.String == "" {
			continue
		}
		var parsed struct {
			Skill string `json:"skill"`
		}
		if err := json.Unmarshal([]byte(toolInput.String), &parsed); err != nil || parsed.Skill == "" {
			continue
		}
		counts[parsed.Skill]++
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "rows: "+err.Error())
		return
	}

	result := []map[string]interface{}{}
	for name, count := range counts {
		result = append(result, map[string]interface{}{
			"name":  name,
			"count": count,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"skills": result,
	})
}

// handleMCPStats handles GET /api/stats/mcp — global MCP server usage stats.
func (s *Server) handleMCPStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rows, err := s.db.Query(`
		SELECT tool_name, COUNT(*) as count
		FROM tool_calls
		WHERE tool_name LIKE 'mcp__%'
		GROUP BY tool_name
		ORDER BY count DESC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query mcp stats: "+err.Error())
		return
	}
	defer rows.Close()

	type mcpTool struct {
		Name  string
		Count int
	}
	serverTools := map[string][]mcpTool{}
	serverCounts := map[string]int{}

	for rows.Next() {
		var toolName string
		var count int
		if err := rows.Scan(&toolName, &count); err != nil {
			writeError(w, http.StatusInternalServerError, "scan mcp stat: "+err.Error())
			return
		}
		parts := strings.Split(toolName, "__")
		if len(parts) < 3 {
			continue
		}
		serverName := parts[1]
		tool := strings.Join(parts[2:], "__")
		serverTools[serverName] = append(serverTools[serverName], mcpTool{Name: tool, Count: count})
		serverCounts[serverName] += count
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "rows: "+err.Error())
		return
	}

	servers := []map[string]interface{}{}
	for name, totalCount := range serverCounts {
		tools := []map[string]interface{}{}
		for _, t := range serverTools[name] {
			tools = append(tools, map[string]interface{}{
				"name":  t.Name,
				"count": t.Count,
			})
		}
		servers = append(servers, map[string]interface{}{
			"name":  name,
			"count": totalCount,
			"tools": tools,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"servers": servers,
	})
}

// handleSessionBreakdown handles GET /api/stats/session-breakdown — per-session breakdown.
func (s *Server) handleSessionBreakdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "missing required parameter 'session_id'")
		return
	}

	breakdown, err := s.queryBreakdown("tool_calls.session_id = ?", sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query session breakdown: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, breakdown)
}

// handleProjectBreakdown handles GET /api/stats/project-breakdown — per-project breakdown.
func (s *Server) handleProjectBreakdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	project := r.URL.Query().Get("project")
	if project == "" {
		writeError(w, http.StatusBadRequest, "missing required parameter 'project'")
		return
	}

	breakdown, err := s.queryBreakdownByProject(project)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query project breakdown: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, breakdown)
}

// queryBreakdown queries tools, skills, MCP servers, and agents for a given WHERE clause.
func (s *Server) queryBreakdown(whereClause string, arg string) (map[string]interface{}, error) {
	// Tools breakdown (excluding Skill and mcp__ tools to avoid double-counting)
	toolRows, err := s.db.Query(fmt.Sprintf(`
		SELECT tool_name, COUNT(*) as count
		FROM tool_calls
		WHERE %s AND tool_name != 'Skill' AND tool_name NOT LIKE 'mcp__%%'
		GROUP BY tool_name
		ORDER BY count DESC
	`, whereClause), arg)
	if err != nil {
		return nil, fmt.Errorf("query tools: %w", err)
	}
	defer toolRows.Close()

	tools := []map[string]interface{}{}
	for toolRows.Next() {
		var name string
		var count int
		if err := toolRows.Scan(&name, &count); err != nil {
			return nil, fmt.Errorf("scan tool: %w", err)
		}
		tools = append(tools, map[string]interface{}{
			"name":  name,
			"count": count,
		})
	}
	if err := toolRows.Err(); err != nil {
		return nil, fmt.Errorf("tools rows: %w", err)
	}

	// Skills breakdown
	skillRows, err := s.db.Query(fmt.Sprintf(`
		SELECT tool_input FROM tool_calls
		WHERE %s AND tool_name = 'Skill'
	`, whereClause), arg)
	if err != nil {
		return nil, fmt.Errorf("query skills: %w", err)
	}
	defer skillRows.Close()

	skillCounts := map[string]int{}
	for skillRows.Next() {
		var toolInput sql.NullString
		if err := skillRows.Scan(&toolInput); err != nil {
			return nil, fmt.Errorf("scan skill: %w", err)
		}
		if !toolInput.Valid || toolInput.String == "" {
			continue
		}
		var parsed struct {
			Skill string `json:"skill"`
		}
		if err := json.Unmarshal([]byte(toolInput.String), &parsed); err != nil || parsed.Skill == "" {
			continue
		}
		skillCounts[parsed.Skill]++
	}
	if err := skillRows.Err(); err != nil {
		return nil, fmt.Errorf("skills rows: %w", err)
	}

	skills := []map[string]interface{}{}
	for name, count := range skillCounts {
		skills = append(skills, map[string]interface{}{
			"name":  name,
			"count": count,
		})
	}

	// MCP breakdown
	mcpRows, err := s.db.Query(fmt.Sprintf(`
		SELECT tool_name, COUNT(*) as count
		FROM tool_calls
		WHERE %s AND tool_name LIKE 'mcp__%%'
		GROUP BY tool_name
		ORDER BY count DESC
	`, whereClause), arg)
	if err != nil {
		return nil, fmt.Errorf("query mcp: %w", err)
	}
	defer mcpRows.Close()

	type mcpTool struct {
		Name  string
		Count int
	}
	serverToolsMap := map[string][]mcpTool{}
	serverCountsMap := map[string]int{}

	for mcpRows.Next() {
		var toolName string
		var count int
		if err := mcpRows.Scan(&toolName, &count); err != nil {
			return nil, fmt.Errorf("scan mcp: %w", err)
		}
		parts := strings.Split(toolName, "__")
		if len(parts) < 3 {
			continue
		}
		serverName := parts[1]
		tool := strings.Join(parts[2:], "__")
		serverToolsMap[serverName] = append(serverToolsMap[serverName], mcpTool{Name: tool, Count: count})
		serverCountsMap[serverName] += count
	}
	if err := mcpRows.Err(); err != nil {
		return nil, fmt.Errorf("mcp rows: %w", err)
	}

	mcpServers := []map[string]interface{}{}
	for name, totalCount := range serverCountsMap {
		mcpToolsList := []map[string]interface{}{}
		for _, t := range serverToolsMap[name] {
			mcpToolsList = append(mcpToolsList, map[string]interface{}{
				"name":  t.Name,
				"count": t.Count,
			})
		}
		mcpServers = append(mcpServers, map[string]interface{}{
			"name":  name,
			"count": totalCount,
			"tools": mcpToolsList,
		})
	}

	// Agents breakdown
	agentRows, err := s.db.Query(fmt.Sprintf(`
		SELECT agent_type, COUNT(*) as count
		FROM subagents
		WHERE %s
		GROUP BY agent_type
		ORDER BY count DESC
	`, strings.Replace(whereClause, "tool_calls.", "subagents.", 1)), arg)
	if err != nil {
		return nil, fmt.Errorf("query agents: %w", err)
	}
	defer agentRows.Close()

	agents := []map[string]interface{}{}
	for agentRows.Next() {
		var agentType string
		var count int
		if err := agentRows.Scan(&agentType, &count); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, map[string]interface{}{
			"agent_type": agentType,
			"count":      count,
		})
	}
	if err := agentRows.Err(); err != nil {
		return nil, fmt.Errorf("agents rows: %w", err)
	}

	return map[string]interface{}{
		"tools":       tools,
		"skills":      skills,
		"mcp_servers": mcpServers,
		"agents":      agents,
	}, nil
}

// queryBreakdownByProject queries the breakdown for a specific project by joining with sessions.
func (s *Server) queryBreakdownByProject(project string) (map[string]interface{}, error) {
	// Tools breakdown
	toolRows, err := s.db.Query(`
		SELECT tc.tool_name, COUNT(*) as count
		FROM tool_calls tc
		JOIN sessions s ON tc.session_id = s.id
		WHERE s.project_path = ? AND tc.tool_name != 'Skill' AND tc.tool_name NOT LIKE 'mcp__%'
		GROUP BY tc.tool_name
		ORDER BY count DESC
	`, project)
	if err != nil {
		return nil, fmt.Errorf("query tools: %w", err)
	}
	defer toolRows.Close()

	tools := []map[string]interface{}{}
	for toolRows.Next() {
		var name string
		var count int
		if err := toolRows.Scan(&name, &count); err != nil {
			return nil, fmt.Errorf("scan tool: %w", err)
		}
		tools = append(tools, map[string]interface{}{
			"name":  name,
			"count": count,
		})
	}
	if err := toolRows.Err(); err != nil {
		return nil, fmt.Errorf("tools rows: %w", err)
	}

	// Skills breakdown
	skillRows, err := s.db.Query(`
		SELECT tc.tool_input FROM tool_calls tc
		JOIN sessions s ON tc.session_id = s.id
		WHERE s.project_path = ? AND tc.tool_name = 'Skill'
	`, project)
	if err != nil {
		return nil, fmt.Errorf("query skills: %w", err)
	}
	defer skillRows.Close()

	skillCounts := map[string]int{}
	for skillRows.Next() {
		var toolInput sql.NullString
		if err := skillRows.Scan(&toolInput); err != nil {
			return nil, fmt.Errorf("scan skill: %w", err)
		}
		if !toolInput.Valid || toolInput.String == "" {
			continue
		}
		var parsed struct {
			Skill string `json:"skill"`
		}
		if err := json.Unmarshal([]byte(toolInput.String), &parsed); err != nil || parsed.Skill == "" {
			continue
		}
		skillCounts[parsed.Skill]++
	}
	if err := skillRows.Err(); err != nil {
		return nil, fmt.Errorf("skills rows: %w", err)
	}

	skills := []map[string]interface{}{}
	for name, count := range skillCounts {
		skills = append(skills, map[string]interface{}{
			"name":  name,
			"count": count,
		})
	}

	// MCP breakdown
	mcpRows, err := s.db.Query(`
		SELECT tc.tool_name, COUNT(*) as count
		FROM tool_calls tc
		JOIN sessions s ON tc.session_id = s.id
		WHERE s.project_path = ? AND tc.tool_name LIKE 'mcp__%'
		GROUP BY tc.tool_name
		ORDER BY count DESC
	`, project)
	if err != nil {
		return nil, fmt.Errorf("query mcp: %w", err)
	}
	defer mcpRows.Close()

	type mcpTool struct {
		Name  string
		Count int
	}
	serverToolsMap := map[string][]mcpTool{}
	serverCountsMap := map[string]int{}

	for mcpRows.Next() {
		var toolName string
		var count int
		if err := mcpRows.Scan(&toolName, &count); err != nil {
			return nil, fmt.Errorf("scan mcp: %w", err)
		}
		parts := strings.Split(toolName, "__")
		if len(parts) < 3 {
			continue
		}
		serverName := parts[1]
		tool := strings.Join(parts[2:], "__")
		serverToolsMap[serverName] = append(serverToolsMap[serverName], mcpTool{Name: tool, Count: count})
		serverCountsMap[serverName] += count
	}
	if err := mcpRows.Err(); err != nil {
		return nil, fmt.Errorf("mcp rows: %w", err)
	}

	mcpServers := []map[string]interface{}{}
	for name, totalCount := range serverCountsMap {
		mcpToolsList := []map[string]interface{}{}
		for _, t := range serverToolsMap[name] {
			mcpToolsList = append(mcpToolsList, map[string]interface{}{
				"name":  t.Name,
				"count": t.Count,
			})
		}
		mcpServers = append(mcpServers, map[string]interface{}{
			"name":  name,
			"count": totalCount,
			"tools": mcpToolsList,
		})
	}

	// Agents breakdown
	agentRows, err := s.db.Query(`
		SELECT sa.agent_type, COUNT(*) as count
		FROM subagents sa
		JOIN sessions s ON sa.session_id = s.id
		WHERE s.project_path = ?
		GROUP BY sa.agent_type
		ORDER BY count DESC
	`, project)
	if err != nil {
		return nil, fmt.Errorf("query agents: %w", err)
	}
	defer agentRows.Close()

	agents := []map[string]interface{}{}
	for agentRows.Next() {
		var agentType string
		var count int
		if err := agentRows.Scan(&agentType, &count); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, map[string]interface{}{
			"agent_type": agentType,
			"count":      count,
		})
	}
	if err := agentRows.Err(); err != nil {
		return nil, fmt.Errorf("agents rows: %w", err)
	}

	return map[string]interface{}{
		"tools":       tools,
		"skills":      skills,
		"mcp_servers": mcpServers,
		"agents":      agents,
	}, nil
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
	var notes, tags sql.NullString
	var totalInput, totalOutput, totalCacheRead, totalCacheWrite int
	var estimatedCost float64

	if err := rows.Scan(&id, &projectPath, &projectName, &cwd, &gitBranch,
		&startedAt, &endedAt, &claudeVersion, &entryPoint, &permissionMode,
		&totalInput, &totalOutput, &totalCacheRead, &totalCacheWrite,
		&estimatedCost, &notes, &tags); err != nil {
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
		"notes":                    notes.String,
		"tags":                     tags.String,
	}

	return sess, nil
}
