package server

import (
	"math"
	"net/http"
	"time"

	"github.com/szaher/claude-monitor/internal/db"
)

func (s *Server) handleSessionCompare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	idA := r.URL.Query().Get("a")
	idB := r.URL.Query().Get("b")
	if idA == "" || idB == "" {
		writeError(w, http.StatusBadRequest, "missing required parameters 'a' and 'b'")
		return
	}
	if idA == idB {
		writeError(w, http.StatusBadRequest, "sessions must be different")
		return
	}

	sessA, err := db.GetSessionByID(s.db, idA)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found: "+idA)
		return
	}
	sessB, err := db.GetSessionByID(s.db, idB)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found: "+idB)
		return
	}

	bdA, _ := s.queryBreakdown("tool_calls.session_id = ?", idA)
	bdB, _ := s.queryBreakdown("tool_calls.session_id = ?", idB)
	if bdA == nil {
		bdA = map[string]interface{}{"tools": []interface{}{}, "skills": []interface{}{}, "mcp_servers": []interface{}{}, "agents": []interface{}{}}
	}
	if bdB == nil {
		bdB = map[string]interface{}{"tools": []interface{}{}, "skills": []interface{}{}, "mcp_servers": []interface{}{}, "agents": []interface{}{}}
	}

	metricsA := s.computeSessionMetrics(idA)
	metricsB := s.computeSessionMetrics(idB)

	segA := s.computeTimeSegments(idA)
	segB := s.computeTimeSegments(idB)

	deltas := s.computeDeltas(metricsA, metricsB)

	buildSessionResp := func(sess interface{}, metrics, segments map[string]interface{}, bd map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{
			"session":       sess,
			"metrics":       metrics,
			"breakdown":     bd,
			"time_segments": segments,
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"session_a": buildSessionResp(sessA, metricsA, segA, bdA),
		"session_b": buildSessionResp(sessB, metricsB, segB, bdB),
		"deltas":    deltas,
	})
}

func (s *Server) computeSessionMetrics(sessionID string) map[string]interface{} {
	var cost float64
	var inputTok, outputTok, cacheRead int
	var startedAt, endedAt string

	s.db.QueryRow(`
		SELECT COALESCE(estimated_cost_usd, 0),
			COALESCE(total_input_tokens, 0),
			COALESCE(total_output_tokens, 0),
			COALESCE(total_cache_read_tokens, 0),
			started_at,
			COALESCE(ended_at, '')
		FROM sessions WHERE id = ?
	`, sessionID).Scan(&cost, &inputTok, &outputTok, &cacheRead, &startedAt, &endedAt)

	totalTokens := inputTok + outputTok
	cacheHitRate := 0.0
	if cacheRead+inputTok > 0 {
		cacheHitRate = float64(cacheRead) / float64(cacheRead+inputTok)
	}

	durationMin := 0.0
	if endedAt != "" {
		if startT, err := time.Parse(time.RFC3339, startedAt); err == nil {
			if endT, err := time.Parse(time.RFC3339, endedAt); err == nil {
				durationMin = endT.Sub(startT).Minutes()
			}
		}
	}

	var toolCalls, toolErrors int
	s.db.QueryRow(`SELECT COUNT(*), COUNT(CASE WHEN success = 0 THEN 1 END) FROM tool_calls WHERE session_id = ?`, sessionID).Scan(&toolCalls, &toolErrors)

	errorRate := 0.0
	if toolCalls > 0 {
		errorRate = float64(toolErrors) / float64(toolCalls)
	}

	return map[string]interface{}{
		"cost":             cost,
		"duration_minutes": math.Round(durationMin*10) / 10,
		"total_tokens":     totalTokens,
		"cache_hit_rate":   math.Round(cacheHitRate*1000) / 1000,
		"tool_calls":       toolCalls,
		"error_rate":       math.Round(errorRate*1000) / 1000,
	}
}

func (s *Server) computeTimeSegments(sessionID string) map[string]interface{} {
	var startedAt, endedAt string
	s.db.QueryRow(`SELECT started_at, COALESCE(ended_at, '') FROM sessions WHERE id = ?`, sessionID).Scan(&startedAt, &endedAt)

	empty := map[string]interface{}{
		"user_input_pct":     0.0,
		"assistant_pct":      0.0,
		"tool_execution_pct": 0.0,
		"other_pct":          100.0,
	}

	if endedAt == "" {
		return empty
	}

	startT, err1 := time.Parse(time.RFC3339, startedAt)
	endT, err2 := time.Parse(time.RFC3339, endedAt)
	if err1 != nil || err2 != nil {
		return empty
	}

	totalMs := endT.Sub(startT).Milliseconds()
	if totalMs <= 0 {
		return empty
	}

	var toolMs int64
	s.db.QueryRow(`SELECT COALESCE(SUM(duration_ms), 0) FROM tool_calls WHERE session_id = ?`, sessionID).Scan(&toolMs)

	type msgEvent struct {
		eventType string
		ts        time.Time
	}
	var events []msgEvent

	rows, err := s.db.Query(`
		SELECT type, timestamp FROM messages
		WHERE session_id = ? AND type IN ('user', 'assistant')
		ORDER BY timestamp ASC
	`, sessionID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var msgType, ts string
			rows.Scan(&msgType, &ts)
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				events = append(events, msgEvent{msgType, t})
			}
		}
	}

	var userMs, assistantMs int64
	for i := 0; i < len(events)-1; i++ {
		gap := events[i+1].ts.Sub(events[i].ts).Milliseconds()
		if gap < 0 {
			gap = 0
		}
		if events[i].eventType == "user" {
			assistantMs += gap
		} else {
			userMs += gap
		}
	}

	if toolMs > totalMs {
		toolMs = totalMs
	}

	totalMsgMs := userMs + assistantMs
	if totalMsgMs == 0 {
		totalMsgMs = 1
	}

	nonToolMs := totalMs - toolMs
	if nonToolMs < 0 {
		nonToolMs = 0
	}

	userPct := float64(userMs) / float64(totalMsgMs) * float64(nonToolMs) / float64(totalMs) * 100
	assistantPct := float64(assistantMs) / float64(totalMsgMs) * float64(nonToolMs) / float64(totalMs) * 100
	toolPct := float64(toolMs) / float64(totalMs) * 100
	otherPct := 100.0 - userPct - assistantPct - toolPct
	if otherPct < 0 {
		otherPct = 0
	}

	return map[string]interface{}{
		"user_input_pct":     math.Round(userPct*10) / 10,
		"assistant_pct":      math.Round(assistantPct*10) / 10,
		"tool_execution_pct": math.Round(toolPct*10) / 10,
		"other_pct":          math.Round(otherPct*10) / 10,
	}
}

func (s *Server) computeDeltas(metricsA, metricsB map[string]interface{}) map[string]interface{} {
	type metricDir struct {
		key       string
		improving string
	}

	defs := []metricDir{
		{"cost", "decreasing"},
		{"duration_minutes", "decreasing"},
		{"total_tokens", "decreasing"},
		{"cache_hit_rate", "increasing"},
		{"tool_calls", "neutral"},
		{"error_rate", "decreasing"},
	}

	toFloat := func(v interface{}) float64 {
		switch val := v.(type) {
		case float64:
			return val
		case int:
			return float64(val)
		case int64:
			return float64(val)
		default:
			return 0
		}
	}

	deltas := map[string]interface{}{}
	for _, d := range defs {
		a := toFloat(metricsA[d.key])
		b := toFloat(metricsB[d.key])
		deltaPct := 0.0
		if a != 0 {
			deltaPct = ((b - a) / math.Abs(a)) * 100
		}

		trend := "stable"
		if math.Abs(deltaPct) > 1.0 {
			switch d.improving {
			case "decreasing":
				if deltaPct < 0 {
					trend = "improving"
				} else {
					trend = "worsening"
				}
			case "increasing":
				if deltaPct > 0 {
					trend = "improving"
				} else {
					trend = "worsening"
				}
			case "neutral":
				trend = "neutral"
			}
		} else if d.improving == "neutral" {
			trend = "neutral"
		}

		deltas[d.key] = map[string]interface{}{
			"delta_pct": math.Round(deltaPct*100) / 100,
			"trend":     trend,
		}
	}
	return deltas
}
