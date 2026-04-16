package exporter

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/szaher/claude-monitor/internal/models"
)

// SessionExport is a session with its messages and tool calls for export.
type SessionExport struct {
	Session   *models.Session    `json:"session"`
	Messages  []*models.Message  `json:"messages"`
	ToolCalls []*models.ToolCall `json:"tool_calls"`
}

// Exporter writes session data in various formats.
type Exporter struct {
	db *sql.DB
}

// New creates a new Exporter.
func New(db *sql.DB) *Exporter {
	return &Exporter{db: db}
}

// LoadSessionData loads messages and tool calls for a slice of sessions.
func (e *Exporter) LoadSessionData(sessions []*models.Session) ([]*SessionExport, error) {
	var exports []*SessionExport

	for _, s := range sessions {
		msgs, err := e.queryMessages(s.ID)
		if err != nil {
			return nil, fmt.Errorf("query messages for session %s: %w", s.ID, err)
		}

		tcs, err := e.queryToolCalls(s.ID)
		if err != nil {
			return nil, fmt.Errorf("query tool_calls for session %s: %w", s.ID, err)
		}

		exports = append(exports, &SessionExport{
			Session:   s,
			Messages:  msgs,
			ToolCalls: tcs,
		})
	}

	return exports, nil
}

// ExportJSON writes sessions with their messages and tool calls as a JSON array.
func (e *Exporter) ExportJSON(w io.Writer, sessions []*models.Session) error {
	exports, err := e.LoadSessionData(sessions)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(exports)
}

// ExportCSV writes three CSV files to the given directory:
// sessions.csv, messages.csv, tool_calls.csv.
func (e *Exporter) ExportCSV(dir string, sessions []*models.Session) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	exports, err := e.LoadSessionData(sessions)
	if err != nil {
		return err
	}

	// sessions.csv
	sf, err := os.Create(filepath.Join(dir, "sessions.csv"))
	if err != nil {
		return fmt.Errorf("create sessions.csv: %w", err)
	}
	defer sf.Close()

	sw := csv.NewWriter(sf)
	sw.Write([]string{
		"id", "project_path", "project_name", "cwd", "git_branch",
		"started_at", "ended_at", "claude_version", "entry_point", "permission_mode",
		"total_input_tokens", "total_output_tokens", "total_cache_read_tokens",
		"total_cache_write_tokens", "estimated_cost_usd",
	})
	for _, ex := range exports {
		s := ex.Session
		endedAt := ""
		if s.EndedAt != nil {
			endedAt = s.EndedAt.Format(time.RFC3339)
		}
		sw.Write([]string{
			s.ID, s.ProjectPath, s.ProjectName, s.CWD, s.GitBranch,
			s.StartedAt.Format(time.RFC3339), endedAt,
			s.ClaudeVersion, s.EntryPoint, s.PermissionMode,
			fmt.Sprintf("%d", s.TotalInputTokens),
			fmt.Sprintf("%d", s.TotalOutputTokens),
			fmt.Sprintf("%d", s.TotalCacheReadTokens),
			fmt.Sprintf("%d", s.TotalCacheWriteTokens),
			fmt.Sprintf("%.6f", s.EstimatedCostUSD),
		})
	}
	sw.Flush()
	if err := sw.Error(); err != nil {
		return fmt.Errorf("write sessions.csv: %w", err)
	}

	// messages.csv
	mf, err := os.Create(filepath.Join(dir, "messages.csv"))
	if err != nil {
		return fmt.Errorf("create messages.csv: %w", err)
	}
	defer mf.Close()

	mw := csv.NewWriter(mf)
	mw.Write([]string{
		"id", "session_id", "parent_id", "type", "role", "model",
		"content_text", "stop_reason",
		"input_tokens", "output_tokens", "cache_read_tokens", "cache_write_tokens",
		"timestamp",
	})
	for _, ex := range exports {
		for _, m := range ex.Messages {
			mw.Write([]string{
				m.ID, m.SessionID, m.ParentID, m.Type, m.Role, m.Model,
				m.ContentText, m.StopReason,
				fmt.Sprintf("%d", m.InputTokens),
				fmt.Sprintf("%d", m.OutputTokens),
				fmt.Sprintf("%d", m.CacheReadTokens),
				fmt.Sprintf("%d", m.CacheWriteTokens),
				m.Timestamp.Format(time.RFC3339),
			})
		}
	}
	mw.Flush()
	if err := mw.Error(); err != nil {
		return fmt.Errorf("write messages.csv: %w", err)
	}

	// tool_calls.csv
	tf, err := os.Create(filepath.Join(dir, "tool_calls.csv"))
	if err != nil {
		return fmt.Errorf("create tool_calls.csv: %w", err)
	}
	defer tf.Close()

	tw := csv.NewWriter(tf)
	tw.Write([]string{
		"id", "message_id", "session_id", "tool_name",
		"tool_input", "tool_response", "success", "error", "duration_ms",
		"timestamp",
	})
	for _, ex := range exports {
		for _, tc := range ex.ToolCalls {
			tw.Write([]string{
				tc.ID, tc.MessageID, tc.SessionID, tc.ToolName,
				tc.ToolInput, tc.ToolResponse,
				fmt.Sprintf("%t", tc.Success), tc.Error,
				fmt.Sprintf("%d", tc.DurationMS),
				tc.Timestamp.Format(time.RFC3339),
			})
		}
	}
	tw.Flush()
	if err := tw.Error(); err != nil {
		return fmt.Errorf("write tool_calls.csv: %w", err)
	}

	return nil
}

// ExportHTML writes a self-contained HTML report with inline CSS.
func (e *Exporter) ExportHTML(w io.Writer, sessions []*models.Session) error {
	exports, err := e.LoadSessionData(sessions)
	if err != nil {
		return err
	}

	tmpl, err := template.New("report").Funcs(template.FuncMap{
		"formatTime": func(t time.Time) string {
			return t.Format("2006-01-02 15:04:05")
		},
		"formatTimePtr": func(t *time.Time) string {
			if t == nil {
				return "ongoing"
			}
			return t.Format("2006-01-02 15:04:05")
		},
		"formatCost": func(c float64) string {
			return fmt.Sprintf("$%.4f", c)
		},
		"truncate": func(s string, n int) string {
			if len(s) <= n {
				return s
			}
			return s[:n] + "..."
		},
	}).Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("parse HTML template: %w", err)
	}

	data := struct {
		GeneratedAt string
		Sessions    []*SessionExport
		TotalCost   float64
	}{
		GeneratedAt: time.Now().Format("2006-01-02 15:04:05"),
		Sessions:    exports,
	}
	for _, ex := range exports {
		data.TotalCost += ex.Session.EstimatedCostUSD
	}

	return tmpl.Execute(w, data)
}

// queryMessages retrieves all messages for a session.
func (e *Exporter) queryMessages(sessionID string) ([]*models.Message, error) {
	rows, err := e.db.Query(`
		SELECT id, session_id, parent_id, type, role, model,
			content_text, content_json, stop_reason,
			input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
			timestamp
		FROM messages
		WHERE session_id = ?
		ORDER BY timestamp ASC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []*models.Message
	for rows.Next() {
		m := &models.Message{}
		var ts string
		var parentID, role, model, contentText, contentJSON, stopReason sql.NullString

		if err := rows.Scan(
			&m.ID, &m.SessionID, &parentID, &m.Type, &role, &model,
			&contentText, &contentJSON, &stopReason,
			&m.InputTokens, &m.OutputTokens, &m.CacheReadTokens, &m.CacheWriteTokens,
			&ts,
		); err != nil {
			return nil, err
		}

		m.ParentID = parentID.String
		m.Role = role.String
		m.Model = model.String
		m.ContentText = contentText.String
		m.ContentJSON = contentJSON.String
		m.StopReason = stopReason.String

		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			// Try other formats
			t, err = time.Parse("2006-01-02T15:04:05Z07:00", ts)
			if err != nil {
				t = time.Time{}
			}
		}
		m.Timestamp = t

		msgs = append(msgs, m)
	}

	return msgs, rows.Err()
}

// queryToolCalls retrieves all tool calls for a session.
func (e *Exporter) queryToolCalls(sessionID string) ([]*models.ToolCall, error) {
	rows, err := e.db.Query(`
		SELECT id, message_id, session_id, tool_name,
			tool_input, tool_response, success, error, duration_ms,
			timestamp
		FROM tool_calls
		WHERE session_id = ?
		ORDER BY timestamp ASC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tcs []*models.ToolCall
	for rows.Next() {
		tc := &models.ToolCall{}
		var ts string
		var toolInput, toolResponse, errStr sql.NullString
		var durationMS sql.NullInt64

		if err := rows.Scan(
			&tc.ID, &tc.MessageID, &tc.SessionID, &tc.ToolName,
			&toolInput, &toolResponse, &tc.Success, &errStr, &durationMS,
			&ts,
		); err != nil {
			return nil, err
		}

		tc.ToolInput = toolInput.String
		tc.ToolResponse = toolResponse.String
		tc.Error = errStr.String
		tc.DurationMS = int(durationMS.Int64)

		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			t = time.Time{}
		}
		tc.Timestamp = t

		tcs = append(tcs, tc)
	}

	return tcs, rows.Err()
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Claude Monitor Export Report</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
         line-height: 1.6; color: #333; background: #f5f5f5; padding: 2rem; }
  .container { max-width: 1200px; margin: 0 auto; }
  h1 { color: #1a1a2e; margin-bottom: 0.5rem; }
  .meta { color: #666; margin-bottom: 2rem; font-size: 0.9rem; }
  .summary { display: flex; gap: 1rem; margin-bottom: 2rem; flex-wrap: wrap; }
  .summary-card { background: #fff; border-radius: 8px; padding: 1rem 1.5rem;
                   box-shadow: 0 1px 3px rgba(0,0,0,0.1); flex: 1; min-width: 150px; }
  .summary-card .label { font-size: 0.8rem; color: #666; text-transform: uppercase; }
  .summary-card .value { font-size: 1.5rem; font-weight: 700; color: #1a1a2e; }
  .session { background: #fff; border-radius: 8px; margin-bottom: 1.5rem;
              box-shadow: 0 1px 3px rgba(0,0,0,0.1); overflow: hidden; }
  .session-header { padding: 1rem 1.5rem; background: #1a1a2e; color: #fff; }
  .session-header h2 { font-size: 1.1rem; margin-bottom: 0.25rem; }
  .session-header .details { font-size: 0.8rem; opacity: 0.8; }
  .session-body { padding: 1rem 1.5rem; }
  .messages-table, .tools-table { width: 100%; border-collapse: collapse; margin-top: 0.5rem; font-size: 0.85rem; }
  .messages-table th, .tools-table th { text-align: left; padding: 0.5rem;
      background: #f8f9fa; border-bottom: 2px solid #dee2e6; }
  .messages-table td, .tools-table td { padding: 0.5rem; border-bottom: 1px solid #eee;
      vertical-align: top; }
  .role-user { color: #0066cc; }
  .role-assistant { color: #28a745; }
  .section-title { font-size: 0.95rem; font-weight: 600; margin: 1rem 0 0.5rem; color: #444; }
  .badge { display: inline-block; padding: 0.15rem 0.5rem; border-radius: 4px;
            font-size: 0.75rem; font-weight: 500; }
  .badge-success { background: #d4edda; color: #155724; }
  .badge-error { background: #f8d7da; color: #721c24; }
  .content-preview { max-width: 500px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
</head>
<body>
<div class="container">
  <h1>Claude Monitor Export Report</h1>
  <p class="meta">Generated: {{.GeneratedAt}}</p>
  <div class="summary">
    <div class="summary-card">
      <div class="label">Sessions</div>
      <div class="value">{{len .Sessions}}</div>
    </div>
    <div class="summary-card">
      <div class="label">Total Cost</div>
      <div class="value">{{formatCost .TotalCost}}</div>
    </div>
  </div>

  {{range .Sessions}}
  <div class="session">
    <div class="session-header">
      <h2>{{.Session.ProjectName}} &mdash; {{.Session.ID}}</h2>
      <div class="details">
        {{formatTime .Session.StartedAt}} to {{formatTimePtr .Session.EndedAt}}
        | {{.Session.ProjectPath}}
        {{if .Session.GitBranch}}| branch: {{.Session.GitBranch}}{{end}}
        | cost: {{formatCost .Session.EstimatedCostUSD}}
      </div>
    </div>
    <div class="session-body">
      {{if .Messages}}
      <div class="section-title">Messages ({{len .Messages}})</div>
      <table class="messages-table">
        <thead><tr><th>Time</th><th>Role</th><th>Model</th><th>Content</th><th>Tokens</th></tr></thead>
        <tbody>
        {{range .Messages}}
        <tr>
          <td>{{formatTime .Timestamp}}</td>
          <td class="role-{{.Role}}">{{.Role}}</td>
          <td>{{.Model}}</td>
          <td class="content-preview">{{truncate .ContentText 120}}</td>
          <td>{{.InputTokens}}/{{.OutputTokens}}</td>
        </tr>
        {{end}}
        </tbody>
      </table>
      {{end}}

      {{if .ToolCalls}}
      <div class="section-title">Tool Calls ({{len .ToolCalls}})</div>
      <table class="tools-table">
        <thead><tr><th>Time</th><th>Tool</th><th>Status</th><th>Duration</th></tr></thead>
        <tbody>
        {{range .ToolCalls}}
        <tr>
          <td>{{formatTime .Timestamp}}</td>
          <td>{{.ToolName}}</td>
          <td>{{if .Success}}<span class="badge badge-success">OK</span>{{else}}<span class="badge badge-error">Error</span>{{end}}</td>
          <td>{{.DurationMS}}ms</td>
        </tr>
        {{end}}
        </tbody>
      </table>
      {{end}}
    </div>
  </div>
  {{end}}
</div>
</body>
</html>
`
