package server

import (
	"fmt"
	"net/http"
	"strings"
)

var promptCategories = map[string][]string{
	"Bug Fix":  {"fix", "bug", "error", "broken", "crash", "failing", "debug"},
	"Feature":  {"add", "implement", "create", "build", "new feature", "scaffold"},
	"Refactor": {"refactor", "rename", "extract", "move", "reorganize", "clean up"},
	"Explain":  {"explain", "what does", "how does", "why", "understand", "walk me through"},
	"Test":     {"test", "spec", "coverage", "assert", "mock", "fixture"},
	"Config":   {"config", "setup", "install", "deploy", "ci", "docker", "env"},
	"Review":   {"review", "check", "audit", "look at", "feedback"},
	"Docs":     {"document", "readme", "comment", "docstring", "jsdoc"},
}

func classifyPrompt(text string) []string {
	lower := strings.ToLower(text)
	var categories []string
	for category, keywords := range promptCategories {
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				categories = append(categories, category)
				break
			}
		}
	}
	return categories
}

func (s *Server) handlePromptPatterns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	project := r.URL.Query().Get("project")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	where := "m.type = 'user' AND m.content_text IS NOT NULL AND m.content_text != ''"
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

	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT m.content_text, m.session_id, date(m.timestamp) as day
		FROM messages m WHERE %s
	`, where), args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query prompts: "+err.Error())
		return
	}
	defer rows.Close()

	type catStats struct {
		Count int
		Days  map[string]int
	}
	stats := map[string]*catStats{}
	totalPrompts := 0

	for rows.Next() {
		var content, sessionID, day string
		if err := rows.Scan(&content, &sessionID, &day); err != nil {
			continue
		}
		totalPrompts++

		categories := classifyPrompt(content)
		for _, cat := range categories {
			if stats[cat] == nil {
				stats[cat] = &catStats{Days: map[string]int{}}
			}
			stats[cat].Count++
			stats[cat].Days[day]++
		}
	}

	categories := []map[string]interface{}{}
	for name, st := range stats {
		pct := 0.0
		if totalPrompts > 0 {
			pct = float64(st.Count) / float64(totalPrompts) * 100
		}
		categories = append(categories, map[string]interface{}{
			"name":       name,
			"count":      st.Count,
			"percentage": pct,
		})
	}

	// Build trend data
	trendMap := map[string]map[string]int{}
	for cat, st := range stats {
		for day, count := range st.Days {
			if trendMap[day] == nil {
				trendMap[day] = map[string]int{}
			}
			trendMap[day][cat] = count
		}
	}

	trends := []map[string]interface{}{}
	for day, cats := range trendMap {
		entry := map[string]interface{}{"date": day}
		for cat, count := range cats {
			entry[cat] = count
		}
		trends = append(trends, entry)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"categories": categories,
		"trends":     trends,
	})
}
