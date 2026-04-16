package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/szaher/claude-monitor/internal/config"
	"github.com/szaher/claude-monitor/internal/db"
	"github.com/szaher/claude-monitor/internal/ingestion"
	"github.com/szaher/claude-monitor/internal/models"
	"github.com/szaher/claude-monitor/internal/parser"
)

// Import imports historical Claude Code session logs into the database.
func Import(args []string) error {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	projectsPath := fs.String("path", "", "Path to Claude Code projects directory (default ~/.claude/projects)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Determine projects directory
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	scanPath := *projectsPath
	if scanPath == "" {
		scanPath = filepath.Join(home, ".claude", "projects")
	}
	scanPath = config.ExpandPath(scanPath)

	if _, err := os.Stat(scanPath); os.IsNotExist(err) {
		return fmt.Errorf("projects directory not found: %s", scanPath)
	}

	// Load config and init database
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

	// Create pipeline for cost calculation
	costPipeline := ingestion.NewPipeline(database, nil, cfg.Cost.Models)

	fmt.Printf("Scanning %s for session logs...\n", scanPath)

	var totalSessions, totalMessages, totalToolCalls int

	// Walk the projects directory recursively to find .jsonl files
	err = filepath.Walk(scanPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // skip inaccessible paths
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		// Skip subagent files — they live under <session-id>/subagents/
		relPath, _ := filepath.Rel(scanPath, path)
		if strings.Contains(relPath, string(filepath.Separator)+"subagents"+string(filepath.Separator)) ||
			strings.Contains(relPath, "/subagents/") {
			return nil
		}

		// Extract session ID from filename (filename without .jsonl extension)
		sessionID := strings.TrimSuffix(info.Name(), ".jsonl")

		// Extract project path from parent directory name
		// The parent directory of the .jsonl file represents the encoded project path
		projectDirName := filepath.Base(filepath.Dir(path))
		projectPath := parser.DecodeProjectPath(projectDirName)
		projectName := filepath.Base(projectPath)

		// Open and parse the log file
		f, err := os.Open(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not open %s: %v\n", path, err)
			return nil
		}
		defer f.Close()

		entries, err := parser.ParseLogFile(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not parse %s: %v\n", path, err)
			return nil
		}

		if len(entries) == 0 {
			return nil
		}

		// Create session from first entry's metadata
		first := entries[0]
		session := &models.Session{
			ID:             sessionID,
			ProjectPath:    projectPath,
			ProjectName:    projectName,
			CWD:            first.CWD,
			GitBranch:      first.GitBranch,
			StartedAt:      first.Timestamp,
			ClaudeVersion:  first.Version,
			EntryPoint:     first.EntryPoint,
			PermissionMode: first.PermissionMode,
		}

		// Scan entries to determine the effective session ID and metadata
		for _, entry := range entries {
			if entry.SessionID != "" {
				session.ID = entry.SessionID
				break
			}
		}
		for _, entry := range entries {
			if session.CWD == "" && entry.CWD != "" {
				session.CWD = entry.CWD
			}
			if session.GitBranch == "" && entry.GitBranch != "" {
				session.GitBranch = entry.GitBranch
			}
		}

		// Prefer CWD from entries over decoded directory name for project path,
		// since DecodeProjectPath mangles paths containing hyphens or dots.
		if session.CWD != "" {
			session.ProjectPath = session.CWD
			session.ProjectName = filepath.Base(session.CWD)
		}

		// Find the last entry timestamp for ended_at
		last := entries[len(entries)-1]
		if last.Timestamp.After(first.Timestamp) {
			session.EndedAt = &last.Timestamp
		}

		// Insert session FIRST so foreign key constraints are satisfied
		if err := db.InsertSession(database, session); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: insert session %s: %v\n", session.ID, err)
			return nil
		}

		// Process all entries
		var sessionInputTokens, sessionOutputTokens int
		var sessionCacheRead, sessionCacheWrite int
		var sessionCost float64
		var msgCount, tcCount int
		effectiveSessionID := session.ID

		for _, entry := range entries {
			// Only process user and assistant messages
			if entry.Type != "user" && entry.Type != "assistant" {
				continue
			}

			contentText := parser.ExtractContentText(entry.Message.Content)
			contentJSON, _ := json.Marshal(entry.Message.Content)

			msg := &models.Message{
				ID:               entry.UUID,
				SessionID:        effectiveSessionID,
				ParentID:         entry.ParentUUID,
				Type:             entry.Type,
				Role:             entry.Message.Role,
				Model:            entry.Message.Model,
				ContentText:      contentText,
				ContentJSON:      string(contentJSON),
				InputTokens:      entry.Message.Usage.InputTokens,
				OutputTokens:     entry.Message.Usage.OutputTokens,
				CacheReadTokens:  entry.Message.Usage.CacheReadInputTokens,
				CacheWriteTokens: entry.Message.Usage.CacheCreationInputTokens,
				Timestamp:        entry.Timestamp,
			}

			if err := db.InsertMessage(database, msg); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: insert message %s: %v\n", msg.ID, err)
				continue
			}
			msgCount++

			// Extract tool calls from assistant messages
			if entry.Type == "assistant" {
				sessionInputTokens += entry.Message.Usage.InputTokens
				sessionOutputTokens += entry.Message.Usage.OutputTokens
				sessionCacheRead += entry.Message.Usage.CacheReadInputTokens
				sessionCacheWrite += entry.Message.Usage.CacheCreationInputTokens

				// Calculate cost for this message
				sessionCost += costPipeline.CalculateCost(
					entry.Message.Model,
					entry.Message.Usage.InputTokens,
					entry.Message.Usage.OutputTokens,
					entry.Message.Usage.CacheReadInputTokens,
					entry.Message.Usage.CacheCreationInputTokens,
				)

				toolCalls := parser.ExtractToolCalls(entry.Message.Content)
				for _, tc := range toolCalls {
					inputJSON, _ := json.Marshal(tc.Input)
					toolCall := &models.ToolCall{
						ID:        tc.ID,
						MessageID: entry.UUID,
						SessionID: effectiveSessionID,
						ToolName:  tc.Name,
						ToolInput: string(inputJSON),
						Success:   true,
						Timestamp: entry.Timestamp,
					}
					if err := db.InsertToolCall(database, toolCall); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: insert tool call %s: %v\n", tc.ID, err)
						continue
					}
					tcCount++
				}
			}
		}

		// Update session with accumulated token totals and cost
		session.TotalInputTokens = sessionInputTokens
		session.TotalOutputTokens = sessionOutputTokens
		session.TotalCacheReadTokens = sessionCacheRead
		session.TotalCacheWriteTokens = sessionCacheWrite
		session.EstimatedCostUSD = sessionCost

		// Re-insert to update token counts and cost (upsert)
		if err := db.InsertSession(database, session); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: update session %s: %v\n", session.ID, err)
		}

		totalSessions++
		totalMessages += msgCount
		totalToolCalls += tcCount

		return nil
	})

	if err != nil {
		return fmt.Errorf("walk projects directory: %w", err)
	}

	fmt.Printf("Imported %d sessions, %d messages, %d tool calls\n",
		totalSessions, totalMessages, totalToolCalls)

	return nil
}
