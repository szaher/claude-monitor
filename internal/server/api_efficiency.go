package server

import (
	"fmt"
	"net/http"
)

func (s *Server) handleTokenEfficiency(w http.ResponseWriter, r *http.Request) {
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
		where += " AND m.session_id IN (SELECT id FROM sessions WHERE project_path = ?)"
		args = append(args, project)
	}
	if from != "" {
		where += " AND m.timestamp >= ?"
		args = append(args, from)
	}
	if to != "" {
		where += " AND m.timestamp <= ?"
		args = append(args, to)
	}

	// Cache hit rate
	var totalCacheRead, totalInput int
	s.db.QueryRow(fmt.Sprintf(
		"SELECT COALESCE(SUM(cache_read_tokens),0), COALESCE(SUM(input_tokens),0) FROM messages m WHERE %s", where,
	), args...).Scan(&totalCacheRead, &totalInput)

	cacheHitRate := 0.0
	if totalCacheRead+totalInput > 0 {
		cacheHitRate = float64(totalCacheRead) / float64(totalCacheRead+totalInput)
	}

	// Average tokens per message
	var avgInput, avgOutput float64
	var msgCount int
	s.db.QueryRow(fmt.Sprintf(
		"SELECT COALESCE(AVG(input_tokens),0), COALESCE(AVG(output_tokens),0), COUNT(*) FROM messages m WHERE %s AND type='assistant'", where,
	), args...).Scan(&avgInput, &avgOutput, &msgCount)

	// Average tokens per tool call
	var avgTokensPerTool float64
	s.db.QueryRow(fmt.Sprintf(`
		SELECT COALESCE(CAST(SUM(m.output_tokens) AS REAL) / NULLIF(COUNT(DISTINCT tc.id), 0), 0)
		FROM messages m
		JOIN tool_calls tc ON tc.session_id = m.session_id
		WHERE %s AND m.type = 'assistant'
	`, where), args...).Scan(&avgTokensPerTool)

	// Cache savings calculation
	cacheSavingsTokens := totalCacheRead
	var cacheSavingsUSD float64
	cacheSavingsUSD = float64(cacheSavingsTokens) / 1_000_000 * 10.0

	// Efficiency by model
	modelRows, err := s.db.Query(fmt.Sprintf(`
		SELECT COALESCE(model, 'unknown'),
			COALESCE(SUM(cache_read_tokens), 0) as cr,
			COALESCE(SUM(input_tokens), 0) as inp,
			COALESCE(AVG(output_tokens), 0) as avg_out,
			COALESCE(AVG(input_tokens + output_tokens), 0) as avg_total
		FROM messages m
		WHERE %s AND model IS NOT NULL AND model != ''
		GROUP BY model
	`, where), args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query model efficiency: "+err.Error())
		return
	}
	defer modelRows.Close()

	effByModel := []map[string]interface{}{}
	for modelRows.Next() {
		var model string
		var cr, inp int
		var avgOut, avgTotal float64
		modelRows.Scan(&model, &cr, &inp, &avgOut, &avgTotal)
		modelHitRate := 0.0
		if cr+inp > 0 {
			modelHitRate = float64(cr) / float64(cr+inp)
		}
		outputRatio := 0.0
		if avgTotal > 0 {
			outputRatio = avgOut / avgTotal
		}
		effByModel = append(effByModel, map[string]interface{}{
			"model":            model,
			"cache_hit_rate":   modelHitRate,
			"avg_output_ratio": outputRatio,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"cache_hit_rate":                cacheHitRate,
		"avg_tokens_per_tool_call":      avgTokensPerTool,
		"avg_input_tokens_per_message":  avgInput,
		"avg_output_tokens_per_message": avgOutput,
		"cache_savings_tokens":          cacheSavingsTokens,
		"cache_savings_usd":             cacheSavingsUSD,
		"efficiency_by_model":           effByModel,
	})
}
