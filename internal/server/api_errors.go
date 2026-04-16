package server

import (
	"fmt"
	"net/http"
	"strings"
)

func (s *Server) handleErrors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	project := r.URL.Query().Get("project")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	where := "1=1"
	var args []interface{}

	if project != "" {
		where += " AND tc.session_id IN (SELECT id FROM sessions WHERE project_path = ?)"
		args = append(args, project)
	}
	if from != "" {
		where += " AND tc.timestamp >= ?"
		args = append(args, from)
	}
	if to != "" {
		where += " AND tc.timestamp <= ?"
		args = append(args, to)
	}

	// Total errors and error rate
	var totalErrors, totalCalls int
	s.db.QueryRow(fmt.Sprintf(
		"SELECT COUNT(*) FROM tool_calls tc WHERE %s AND success = 0", where), args...,
	).Scan(&totalErrors)
	s.db.QueryRow(fmt.Sprintf(
		"SELECT COUNT(*) FROM tool_calls tc WHERE %s", where), args...,
	).Scan(&totalCalls)

	errorRate := 0.0
	if totalCalls > 0 {
		errorRate = float64(totalErrors) / float64(totalCalls)
	}

	// Errors by tool
	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT tc.tool_name,
			SUM(CASE WHEN tc.success = 0 THEN 1 ELSE 0 END) as errors,
			COUNT(*) as total
		FROM tool_calls tc
		WHERE %s
		GROUP BY tc.tool_name
		HAVING errors > 0
		ORDER BY errors DESC
	`, where), args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query errors by tool: "+err.Error())
		return
	}
	defer rows.Close()

	errorsByTool := []map[string]interface{}{}
	for rows.Next() {
		var tool string
		var errors, total int
		rows.Scan(&tool, &errors, &total)
		rate := 0.0
		if total > 0 {
			rate = float64(errors) / float64(total)
		}
		errorsByTool = append(errorsByTool, map[string]interface{}{
			"tool": tool, "errors": errors, "total": total, "rate": rate,
		})
	}

	// Common error patterns
	errorRows, err := s.db.Query(fmt.Sprintf(`
		SELECT tc.error, COUNT(*) as count
		FROM tool_calls tc
		WHERE %s AND tc.success = 0 AND tc.error IS NOT NULL AND tc.error != ''
		GROUP BY tc.error
		ORDER BY count DESC
		LIMIT 20
	`, where), args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query error patterns: "+err.Error())
		return
	}
	defer errorRows.Close()

	commonErrors := []map[string]interface{}{}
	for errorRows.Next() {
		var errMsg string
		var count int
		errorRows.Scan(&errMsg, &count)
		preview := errMsg
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		commonErrors = append(commonErrors, map[string]interface{}{
			"pattern": preview, "count": count,
		})
	}

	// Error trend (last 30 days)
	trendRows, err := s.db.Query(fmt.Sprintf(`
		SELECT date(tc.timestamp) as day,
			SUM(CASE WHEN tc.success = 0 THEN 1 ELSE 0 END) as errors,
			COUNT(*) as total
		FROM tool_calls tc
		WHERE %s AND tc.timestamp >= datetime('now', '-30 days')
		GROUP BY day
		ORDER BY day ASC
	`, strings.Replace(where, "tc.", "tc.", -1)), args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query error trend: "+err.Error())
		return
	}
	defer trendRows.Close()

	errorTrend := []map[string]interface{}{}
	for trendRows.Next() {
		var day string
		var errors, total int
		trendRows.Scan(&day, &errors, &total)
		errorTrend = append(errorTrend, map[string]interface{}{
			"date": day, "errors": errors, "total": total,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_errors":   totalErrors,
		"error_rate":     errorRate,
		"errors_by_tool": errorsByTool,
		"common_errors":  commonErrors,
		"error_trend":    errorTrend,
	})
}
