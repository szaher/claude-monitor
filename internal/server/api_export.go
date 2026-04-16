package server

import (
	"archive/zip"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/szaher/claude-monitor/internal/db"
	"github.com/szaher/claude-monitor/internal/exporter"
	"github.com/szaher/claude-monitor/internal/models"
)

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}
	project := r.URL.Query().Get("project")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	sessionID := r.URL.Query().Get("session_id")

	var sessions []*models.Session

	if sessionID != "" {
		sess, err := db.GetSessionByID(s.db, sessionID)
		if err != nil {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		sessions = []*models.Session{sess}
	} else {
		query := `SELECT id FROM sessions WHERE 1=1`
		var args []interface{}

		if project != "" {
			query += " AND (project_path = ? OR project_name = ?)"
			args = append(args, project, project)
		}
		if from != "" {
			query += " AND started_at >= ?"
			args = append(args, from)
		}
		if to != "" {
			query += " AND started_at <= ?"
			args = append(args, to)
		}
		query += " ORDER BY started_at DESC LIMIT 1000"

		rows, err := s.db.Query(query, args...)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "query sessions: "+err.Error())
			return
		}
		defer rows.Close()

		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				continue
			}
			fullSess, err := db.GetSessionByID(s.db, id)
			if err == nil {
				sessions = append(sessions, fullSess)
			}
		}
	}

	if len(sessions) == 0 {
		writeError(w, http.StatusNotFound, "no sessions found")
		return
	}

	exp := exporter.New(s.db)

	switch format {
	case "json":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=claude-monitor-export.json")
		exp.ExportJSON(w, sessions)

	case "csv":
		tmpDir, err := os.MkdirTemp("", "claude-monitor-csv-*")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create temp dir: "+err.Error())
			return
		}
		defer os.RemoveAll(tmpDir)

		if err := exp.ExportCSV(tmpDir, sessions); err != nil {
			writeError(w, http.StatusInternalServerError, "export csv: "+err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", "attachment; filename=claude-monitor-export.zip")

		zw := zip.NewWriter(w)
		defer zw.Close()

		for _, name := range []string{"sessions.csv", "messages.csv", "tool_calls.csv"} {
			data, err := os.ReadFile(filepath.Join(tmpDir, name))
			if err != nil {
				continue
			}
			fw, err := zw.Create(name)
			if err != nil {
				continue
			}
			fw.Write(data)
		}

	case "html":
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Disposition", "attachment; filename=claude-monitor-report.html")
		exp.ExportHTML(w, sessions)

	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported format: %s", format))
	}
}
