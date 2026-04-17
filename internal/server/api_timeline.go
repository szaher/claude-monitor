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

	// Context compactions
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

	// Duration
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
		"duration_seconds": durationSec,
		"total_tokens":     totalTokens,
	})
}
