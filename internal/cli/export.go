package cli

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/szaher/claude-monitor/internal/config"
	"github.com/szaher/claude-monitor/internal/db"
	"github.com/szaher/claude-monitor/internal/exporter"
	"github.com/szaher/claude-monitor/internal/models"
)

// Export exports session data as JSON, CSV, or HTML.
func Export(args []string) error {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	format := fs.String("format", "json", "Output format: json, csv, html")
	project := fs.String("project", "", "Filter by project path or name")
	from := fs.String("from", "", "Filter sessions starting from (RFC3339 or YYYY-MM-DD)")
	to := fs.String("to", "", "Filter sessions starting before (RFC3339 or YYYY-MM-DD)")
	output := fs.String("output", "", "Output file or directory (default: stdout for json/html, ./export for csv)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Load config and init database
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	baseDir := filepath.Join(home, ".claude-monitor")
	configPath := filepath.Join(baseDir, "config.yaml")

	cfg, err := config.Load(configPath)
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

	// Query sessions with filters
	sessions, err := querySessions(database, *project, *from, *to)
	if err != nil {
		return fmt.Errorf("query sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found matching the given filters.")
		return nil
	}

	exp := exporter.New(database)

	switch *format {
	case "json":
		outputPath := *output
		if outputPath == "" {
			// Write to stdout
			if err := exp.ExportJSON(os.Stdout, sessions); err != nil {
				return fmt.Errorf("export JSON: %w", err)
			}
			return nil
		}
		f, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		defer f.Close()
		if err := exp.ExportJSON(f, sessions); err != nil {
			return fmt.Errorf("export JSON: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Exported %d sessions to %s\n", len(sessions), outputPath)

	case "csv":
		outputDir := *output
		if outputDir == "" {
			outputDir = "export"
		}
		if err := exp.ExportCSV(outputDir, sessions); err != nil {
			return fmt.Errorf("export CSV: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Exported %d sessions to %s/\n", len(sessions), outputDir)

	case "html":
		outputPath := *output
		if outputPath == "" {
			// Write to stdout
			if err := exp.ExportHTML(os.Stdout, sessions); err != nil {
				return fmt.Errorf("export HTML: %w", err)
			}
			return nil
		}
		f, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		defer f.Close()
		if err := exp.ExportHTML(f, sessions); err != nil {
			return fmt.Errorf("export HTML: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Exported %d sessions to %s\n", len(sessions), outputPath)

	default:
		return fmt.Errorf("unsupported format: %s (use json, csv, or html)", *format)
	}

	return nil
}

// querySessions queries sessions from the database with optional filters.
func querySessions(database *sql.DB, project, from, to string) ([]*models.Session, error) {
	query := `SELECT id, project_path, project_name, cwd, git_branch,
		started_at, ended_at, claude_version, entry_point, permission_mode,
		total_input_tokens, total_output_tokens, total_cache_read_tokens,
		total_cache_write_tokens, estimated_cost_usd
		FROM sessions WHERE 1=1`

	var args []interface{}

	if project != "" {
		query += " AND (project_path = ? OR project_name = ?)"
		args = append(args, project, project)
	}
	if from != "" {
		fromTime := normalizeTime(from)
		query += " AND started_at >= ?"
		args = append(args, fromTime)
	}
	if to != "" {
		toTime := normalizeTime(to)
		query += " AND started_at <= ?"
		args = append(args, toTime)
	}

	query += " ORDER BY started_at DESC"

	rows, err := database.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*models.Session
	for rows.Next() {
		s := &models.Session{}
		var startedAt string
		var endedAt sql.NullString
		var cwd, gitBranch, claudeVersion, entryPoint, permissionMode sql.NullString

		if err := rows.Scan(
			&s.ID, &s.ProjectPath, &s.ProjectName,
			&cwd, &gitBranch,
			&startedAt, &endedAt,
			&claudeVersion, &entryPoint, &permissionMode,
			&s.TotalInputTokens, &s.TotalOutputTokens,
			&s.TotalCacheReadTokens, &s.TotalCacheWriteTokens,
			&s.EstimatedCostUSD,
		); err != nil {
			return nil, fmt.Errorf("scan session row: %w", err)
		}

		s.CWD = cwd.String
		s.GitBranch = gitBranch.String
		s.ClaudeVersion = claudeVersion.String
		s.EntryPoint = entryPoint.String
		s.PermissionMode = permissionMode.String

		t, err := time.Parse(time.RFC3339, startedAt)
		if err != nil {
			t, _ = time.Parse("2006-01-02T15:04:05Z07:00", startedAt)
		}
		s.StartedAt = t

		if endedAt.Valid {
			t, err := time.Parse(time.RFC3339, endedAt.String)
			if err != nil {
				t, _ = time.Parse("2006-01-02T15:04:05Z07:00", endedAt.String)
			}
			s.EndedAt = &t
		}

		sessions = append(sessions, s)
	}

	return sessions, rows.Err()
}

// normalizeTime converts a date string to RFC3339 format.
// Accepts YYYY-MM-DD or RFC3339.
func normalizeTime(s string) string {
	// If already RFC3339, return as-is
	if _, err := time.Parse(time.RFC3339, s); err == nil {
		return s
	}
	// Try date-only format
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format(time.RFC3339)
	}
	return s
}
