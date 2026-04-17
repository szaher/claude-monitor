package server

import (
	"database/sql"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/szaher/claude-monitor/internal/db"
	"github.com/szaher/claude-monitor/internal/models"
)

func (s *Server) handleSessionCommits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	sessionID := parts[0]

	rows, err := s.db.Query(`
		SELECT commit_hash, commit_message, author, files_changed, insertions, deletions, committed_at
		FROM session_commits WHERE session_id = ?
		ORDER BY committed_at ASC
	`, sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query commits: "+err.Error())
		return
	}
	defer rows.Close()

	commits := []map[string]interface{}{}
	for rows.Next() {
		var hash, message, author, committedAt string
		var filesChanged, insertions, deletions int
		rows.Scan(&hash, &message, &author, &filesChanged, &insertions, &deletions, &committedAt)
		commits = append(commits, map[string]interface{}{
			"commit_hash":    hash,
			"commit_message": message,
			"author":         author,
			"files_changed":  filesChanged,
			"insertions":     insertions,
			"deletions":      deletions,
			"committed_at":   committedAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"commits": commits})
}

func (s *Server) handleGitSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sessionID := r.URL.Query().Get("session_id")
	repo := ""

	if sessionID != "" {
		var cwd sql.NullString
		s.db.QueryRow("SELECT cwd FROM sessions WHERE id = ?", sessionID).Scan(&cwd)
		if cwd.Valid {
			repo = cwd.String
		}
	}

	go func() {
		database := s.db
		syncGitCommitsForRepo(database, repo)
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "syncing"})
}

func syncGitCommitsForRepo(database *sql.DB, repo string) {
	query := "SELECT id, cwd, started_at, ended_at FROM sessions WHERE cwd IS NOT NULL AND cwd != ''"
	var args []interface{}
	if repo != "" {
		query += " AND cwd = ?"
		args = append(args, repo)
	}

	rows, err := database.Query(query, args...)
	if err != nil {
		return
	}
	defer rows.Close()

	type sessionInfo struct {
		ID    string
		CWD   string
		Start time.Time
		End   time.Time
	}

	var sessions []sessionInfo
	for rows.Next() {
		var id, cwd, startStr string
		var endStr sql.NullString
		rows.Scan(&id, &cwd, &startStr, &endStr)
		start, _ := time.Parse(time.RFC3339, startStr)
		end := time.Now().UTC()
		if endStr.Valid {
			end, _ = time.Parse(time.RFC3339, endStr.String)
		}
		sessions = append(sessions, sessionInfo{ID: id, CWD: cwd, Start: start, End: end})
	}

	for _, s := range sessions {
		gitRoot := findGitRootPath(s.CWD)
		if gitRoot == "" {
			continue
		}
		cmd := exec.Command("git", "-C", gitRoot, "log",
			"--format=%H|%s|%an|%aI",
			"--after="+s.Start.Add(-time.Minute).Format(time.RFC3339),
			"--before="+s.End.Add(time.Minute).Format(time.RFC3339),
		)
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			parts := strings.SplitN(line, "|", 4)
			if len(parts) != 4 || len(parts[0]) != 40 {
				continue
			}
			commitTime, _ := time.Parse(time.RFC3339, parts[3])
			commit := &models.SessionCommit{
				SessionID:     s.ID,
				CommitHash:    parts[0],
				CommitMessage: parts[1],
				Author:        parts[2],
				CommittedAt:   commitTime,
			}
			db.InsertSessionCommit(database, commit)
		}
	}
}

func findGitRootPath(path string) string {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
