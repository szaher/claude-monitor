package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/szaher/claude-monitor/internal/db"
)

func (s *Server) handleSessionPatch(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing session id")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}

	var update struct {
		Notes *string `json:"notes"`
		Tags  *string `json:"tags"`
	}
	if err := json.Unmarshal(body, &update); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	var notes, tags string
	s.db.QueryRow("SELECT COALESCE(notes,''), COALESCE(tags,'') FROM sessions WHERE id = ?", id).Scan(&notes, &tags)

	if update.Notes != nil {
		notes = *update.Notes
	}
	if update.Tags != nil {
		tags = *update.Tags
	}

	if err := db.UpdateSessionNotesTags(s.db, id, notes, tags); err != nil {
		writeError(w, http.StatusInternalServerError, "update: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":    id,
		"notes": notes,
		"tags":  tags,
	})
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rows, err := s.db.Query("SELECT tags FROM sessions WHERE tags != '' AND tags IS NOT NULL")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query tags: "+err.Error())
		return
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var tags string
		if err := rows.Scan(&tags); err != nil {
			continue
		}
		for _, tag := range strings.Split(tags, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				counts[tag]++
			}
		}
	}

	result := []map[string]interface{}{}
	for tag, count := range counts {
		result = append(result, map[string]interface{}{
			"tag":   tag,
			"count": count,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tags": result,
	})
}
