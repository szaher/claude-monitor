package cli

import (
	"bufio"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/szaher/claude-monitor/internal/config"
	"github.com/szaher/claude-monitor/internal/db"
	"github.com/szaher/claude-monitor/internal/models"
)

func GitSync(args []string) error {
	fs := flag.NewFlagSet("git-sync", flag.ExitOnError)
	repoPath := fs.String("repo", "", "Sync a specific repo only")
	since := fs.String("since", "", "Only look at commits after this date (YYYY-MM-DD)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	home, _ := os.UserHomeDir()
	baseDir := filepath.Join(home, ".claude-monitor")
	cfg, err := config.Load(filepath.Join(baseDir, "config.yaml"))
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	dbPath := cfg.Storage.DatabasePath
	if dbPath == "" {
		dbPath = filepath.Join(baseDir, "claude-monitor.db")
	}
	database, err := db.InitDB(dbPath)
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}
	defer database.Close()

	return syncGitCommits(database, *repoPath, *since)
}

func syncGitCommits(database *sql.DB, filterRepo, sinceDate string) error {
	query := "SELECT id, cwd, started_at, ended_at FROM sessions WHERE cwd IS NOT NULL AND cwd != ''"
	var args []interface{}
	if filterRepo != "" {
		query += " AND cwd = ?"
		args = append(args, filterRepo)
	}
	if sinceDate != "" {
		query += " AND started_at >= ?"
		args = append(args, sinceDate)
	}

	rows, err := database.Query(query, args...)
	if err != nil {
		return fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close()

	type sessionInfo struct {
		ID    string
		CWD   string
		Start time.Time
		End   time.Time
	}

	repoSessions := map[string][]sessionInfo{}
	for rows.Next() {
		var id, cwd, startStr string
		var endStr sql.NullString
		rows.Scan(&id, &cwd, &startStr, &endStr)

		start, _ := time.Parse(time.RFC3339, startStr)
		end := time.Now().UTC()
		if endStr.Valid {
			end, _ = time.Parse(time.RFC3339, endStr.String)
		}

		repoSessions[cwd] = append(repoSessions[cwd], sessionInfo{
			ID: id, CWD: cwd, Start: start, End: end,
		})
	}

	totalCommits := 0
	for repo, sessions := range repoSessions {
		earliest := sessions[0].Start
		latest := sessions[0].End
		for _, s := range sessions {
			if s.Start.Before(earliest) {
				earliest = s.Start
			}
			if s.End.After(latest) {
				latest = s.End
			}
		}

		gitRoot := findGitRoot(repo)
		if gitRoot == "" {
			continue
		}

		cmd := exec.Command("git", "-C", gitRoot, "log",
			"--format=%H|%s|%an|%aI",
			"--after="+earliest.Add(-time.Minute).Format(time.RFC3339),
			"--before="+latest.Add(time.Minute).Format(time.RFC3339),
			"--numstat",
		)
		out, err := cmd.Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: git log failed for %s: %v\n", repo, err)
			continue
		}

		scanner := bufio.NewScanner(strings.NewReader(string(out)))
		var currentHash, currentMsg, currentAuthor string
		var currentTime time.Time
		var filesChanged, insertions, deletions int

		flushCommit := func() {
			if currentHash == "" {
				return
			}
			for _, s := range sessions {
				if (currentTime.Equal(s.Start) || currentTime.After(s.Start)) &&
					(currentTime.Equal(s.End) || currentTime.Before(s.End)) {
					commit := &models.SessionCommit{
						SessionID:     s.ID,
						CommitHash:    currentHash,
						CommitMessage: currentMsg,
						Author:        currentAuthor,
						FilesChanged:  filesChanged,
						Insertions:    insertions,
						Deletions:     deletions,
						CommittedAt:   currentTime,
					}
					db.InsertSessionCommit(database, commit)
					totalCommits++
					break
				}
			}
		}

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			parts := strings.SplitN(line, "|", 4)
			if len(parts) == 4 && len(parts[0]) == 40 {
				flushCommit()
				currentHash = parts[0]
				currentMsg = parts[1]
				currentAuthor = parts[2]
				currentTime, _ = time.Parse(time.RFC3339, parts[3])
				filesChanged = 0
				insertions = 0
				deletions = 0
			} else if strings.Contains(line, "\t") {
				numParts := strings.Fields(line)
				if len(numParts) >= 2 {
					ins, _ := strconv.Atoi(numParts[0])
					del, _ := strconv.Atoi(numParts[1])
					insertions += ins
					deletions += del
					filesChanged++
				}
			}
		}
		flushCommit()
	}

	fmt.Printf("Synced %d commits across %d repos\n", totalCommits, len(repoSessions))
	return nil
}

func findGitRoot(path string) string {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
