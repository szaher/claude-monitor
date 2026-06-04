package server

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleInsightsStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "daily"
	}
	project := r.URL.Query().Get("project")
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	now := time.Now().UTC()
	to := now
	from := now.AddDate(0, 0, -30)

	if fromStr != "" {
		if t, err := time.Parse("2006-01-02", fromStr); err == nil {
			from = t
		} else if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			from = t
		}
	}
	if toStr != "" {
		if t, err := time.Parse("2006-01-02", toStr); err == nil {
			to = t
		} else if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			to = t
		}
	}

	duration := to.Sub(from)
	prevTo := from
	prevFrom := prevTo.Add(-duration)

	fromFmt := from.Format(time.RFC3339)
	toFmt := to.Format(time.RFC3339)
	prevFromFmt := prevFrom.Format(time.RFC3339)
	prevToFmt := prevTo.Format(time.RFC3339)

	// Build project filter
	projectFilter := ""
	var projectArgs []interface{}
	if project != "" {
		projectFilter = " AND (project_path = ? OR project_name = ?)"
		projectArgs = []interface{}{project, project}
	}

	// Helper to build args for a period query
	buildArgs := func(periodFrom, periodTo string) []interface{} {
		args := []interface{}{periodFrom, periodTo}
		args = append(args, projectArgs...)
		return args
	}

	type periodAgg struct {
		sessions       int
		totalCost      float64
		totalTokens    int
		cacheRead      int
		totalInput     int
		durationMinSum float64
		durationCount  int
		toolCalls      int
		toolErrors     int
		days           int
	}

	queryAgg := func(periodFrom, periodTo string) periodAgg {
		var agg periodAgg
		args := buildArgs(periodFrom, periodTo)

		s.db.QueryRow(fmt.Sprintf(`
			SELECT COUNT(*),
				COALESCE(SUM(estimated_cost_usd), 0),
				COALESCE(SUM(total_input_tokens + total_output_tokens), 0),
				COALESCE(SUM(total_cache_read_tokens), 0),
				COALESCE(SUM(total_input_tokens), 0),
				COALESCE(SUM(CASE WHEN ended_at IS NOT NULL AND ended_at != '' THEN (julianday(ended_at) - julianday(started_at)) * 24 * 60 ELSE 0 END), 0),
				COUNT(CASE WHEN ended_at IS NOT NULL AND ended_at != '' THEN 1 END),
				COUNT(DISTINCT date(started_at))
			FROM sessions
			WHERE started_at >= ? AND started_at <= ?%s
		`, projectFilter), args...).Scan(
			&agg.sessions, &agg.totalCost, &agg.totalTokens,
			&agg.cacheRead, &agg.totalInput,
			&agg.durationMinSum, &agg.durationCount, &agg.days,
		)

		tcArgs := []interface{}{periodFrom, periodTo}
		tcProjectFilter := ""
		if project != "" {
			tcProjectFilter = " AND session_id IN (SELECT id FROM sessions WHERE project_path = ? OR project_name = ?)"
			tcArgs = append(tcArgs, project, project)
		}
		s.db.QueryRow(fmt.Sprintf(`
			SELECT COUNT(*),
				COUNT(CASE WHEN success = 0 THEN 1 END)
			FROM tool_calls
			WHERE timestamp >= ? AND timestamp <= ?%s
		`, tcProjectFilter), tcArgs...).Scan(&agg.toolCalls, &agg.toolErrors)

		return agg
	}

	cur := queryAgg(fromFmt, toFmt)
	prev := queryAgg(prevFromFmt, prevToFmt)

	// Compute metric values
	type metricDef struct {
		key       string
		curVal    float64
		prevVal   float64
		improving string // "decreasing", "increasing", "neutral"
	}

	safeDiv := func(a, b float64) float64 {
		if b == 0 {
			return 0
		}
		return a / b
	}

	metrics := []metricDef{
		{"avg_cost_per_session", safeDiv(cur.totalCost, float64(cur.sessions)), safeDiv(prev.totalCost, float64(prev.sessions)), "decreasing"},
		{"cache_hit_rate", safeDiv(float64(cur.cacheRead), float64(cur.cacheRead+cur.totalInput)), safeDiv(float64(prev.cacheRead), float64(prev.cacheRead+prev.totalInput)), "increasing"},
		{"avg_tokens_per_session", safeDiv(float64(cur.totalTokens), float64(cur.sessions)), safeDiv(float64(prev.totalTokens), float64(prev.sessions)), "decreasing"},
		{"avg_duration_minutes", safeDiv(cur.durationMinSum, float64(cur.durationCount)), safeDiv(prev.durationMinSum, float64(prev.durationCount)), "decreasing"},
		{"error_rate", safeDiv(float64(cur.toolErrors), float64(cur.toolCalls)), safeDiv(float64(prev.toolErrors), float64(prev.toolCalls)), "decreasing"},
		{"sessions_per_day", safeDiv(float64(cur.sessions), float64(cur.days)), safeDiv(float64(prev.sessions), float64(prev.days)), "neutral"},
		{"tool_calls_per_session", safeDiv(float64(cur.toolCalls), float64(cur.sessions)), safeDiv(float64(prev.toolCalls), float64(prev.sessions)), "neutral"},
		{"total_cost", cur.totalCost, prev.totalCost, "neutral"},
	}

	// Date bucket expression for series
	var dateBucket string
	switch period {
	case "weekly":
		dateBucket = "strftime('%Y-W%W', started_at)"
	case "monthly":
		dateBucket = "strftime('%Y-%m', started_at)"
	default:
		dateBucket = "date(started_at)"
	}

	// Query series for each metric
	type dataPoint struct {
		Date  string  `json:"date"`
		Value float64 `json:"value"`
	}

	querySeries := func(periodFrom, periodTo string) []map[string]interface{} {
		args := buildArgs(periodFrom, periodTo)
		tcArgs := []interface{}{periodFrom, periodTo}
		tcProjectFilter := ""
		if project != "" {
			tcProjectFilter = " AND session_id IN (SELECT id FROM sessions WHERE project_path = ? OR project_name = ?)"
			tcArgs = append(tcArgs, project, project)
		}

		rows, err := s.db.Query(fmt.Sprintf(`
			SELECT %s as bucket,
				COUNT(*) as sessions,
				COALESCE(SUM(estimated_cost_usd), 0) as total_cost,
				COALESCE(SUM(total_input_tokens + total_output_tokens), 0) as total_tokens,
				COALESCE(SUM(total_cache_read_tokens), 0) as cache_read,
				COALESCE(SUM(total_input_tokens), 0) as total_input,
				COALESCE(SUM(CASE WHEN ended_at IS NOT NULL AND ended_at != '' THEN (julianday(ended_at) - julianday(started_at)) * 24 * 60 ELSE 0 END), 0) as dur_sum,
				COUNT(CASE WHEN ended_at IS NOT NULL AND ended_at != '' THEN 1 END) as dur_count
			FROM sessions
			WHERE started_at >= ? AND started_at <= ?%s
			GROUP BY bucket
			ORDER BY bucket ASC
		`, dateBucket, projectFilter), args...)
		if err != nil {
			return nil
		}
		defer rows.Close()

		type bucketData struct {
			bucket    string
			sessions  int
			totalCost float64
			tokens    int
			cacheRead int
			input     int
			durSum    float64
			durCount  int
		}
		var buckets []bucketData
		for rows.Next() {
			var b bucketData
			rows.Scan(&b.bucket, &b.sessions, &b.totalCost, &b.tokens, &b.cacheRead, &b.input, &b.durSum, &b.durCount)
			buckets = append(buckets, b)
		}

		// Tool call series per bucket
		tcRows, err := s.db.Query(fmt.Sprintf(`
			SELECT %s as bucket,
				COUNT(*) as tc,
				COUNT(CASE WHEN success = 0 THEN 1 END) as errs
			FROM tool_calls
			JOIN sessions ON sessions.id = tool_calls.session_id
			WHERE tool_calls.timestamp >= ? AND tool_calls.timestamp <= ?%s
			GROUP BY bucket
			ORDER BY bucket ASC
		`, strings.Replace(dateBucket, "started_at", "tool_calls.timestamp", 1), tcProjectFilter), tcArgs...)

		tcMap := map[string][2]int{}
		if err == nil {
			defer tcRows.Close()
			for tcRows.Next() {
				var bucket string
				var tc, errs int
				tcRows.Scan(&bucket, &tc, &errs)
				tcMap[bucket] = [2]int{tc, errs}
			}
		}

		// Build per-metric series
		seriesMap := make([][]dataPoint, 8)
		for i := range seriesMap {
			seriesMap[i] = []dataPoint{}
		}

		for _, b := range buckets {
			tc := tcMap[b.bucket]
			seriesMap[0] = append(seriesMap[0], dataPoint{b.bucket, safeDiv(b.totalCost, float64(b.sessions))})
			seriesMap[1] = append(seriesMap[1], dataPoint{b.bucket, safeDiv(float64(b.cacheRead), float64(b.cacheRead+b.input))})
			seriesMap[2] = append(seriesMap[2], dataPoint{b.bucket, safeDiv(float64(b.tokens), float64(b.sessions))})
			seriesMap[3] = append(seriesMap[3], dataPoint{b.bucket, safeDiv(b.durSum, float64(b.durCount))})
			seriesMap[4] = append(seriesMap[4], dataPoint{b.bucket, safeDiv(float64(tc[1]), float64(tc[0]))})
			seriesMap[5] = append(seriesMap[5], dataPoint{b.bucket, float64(b.sessions)})
			seriesMap[6] = append(seriesMap[6], dataPoint{b.bucket, safeDiv(float64(tc[0]), float64(b.sessions))})
			seriesMap[7] = append(seriesMap[7], dataPoint{b.bucket, b.totalCost})
		}

		result := make([]map[string]interface{}, 8)
		for i, s := range seriesMap {
			pts := []map[string]interface{}{}
			for _, p := range s {
				pts = append(pts, map[string]interface{}{"date": p.Date, "value": p.Value})
			}
			result[i] = map[string]interface{}{"points": pts}
		}
		return result
	}

	curSeries := querySeries(fromFmt, toFmt)
	prevSeries := querySeries(prevFromFmt, prevToFmt)

	// Build response
	metricsResp := map[string]interface{}{}
	for i, m := range metrics {
		deltaPct := 0.0
		if m.prevVal != 0 {
			deltaPct = ((m.curVal - m.prevVal) / math.Abs(m.prevVal)) * 100
		}

		trend := "stable"
		if math.Abs(deltaPct) > 1.0 {
			switch m.improving {
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
		} else if m.improving == "neutral" {
			trend = "neutral"
		}

		entry := map[string]interface{}{
			"current":   m.curVal,
			"previous":  m.prevVal,
			"delta_pct": math.Round(deltaPct*100) / 100,
			"trend":     trend,
		}

		if curSeries != nil && i < len(curSeries) {
			entry["series"] = curSeries[i]["points"]
		} else {
			entry["series"] = []interface{}{}
		}
		if prevSeries != nil && i < len(prevSeries) {
			entry["previous_series"] = prevSeries[i]["points"]
		} else {
			entry["previous_series"] = []interface{}{}
		}

		metricsResp[m.key] = entry
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"period":  period,
		"from":    from.Format("2006-01-02"),
		"to":      to.Format("2006-01-02"),
		"metrics": metricsResp,
	})
}
