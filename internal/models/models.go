package models

import "time"

type Session struct {
	ID                    string     `json:"id"`
	ProjectPath           string     `json:"project_path"`
	ProjectName           string     `json:"project_name"`
	CWD                   string     `json:"cwd"`
	GitBranch             string     `json:"git_branch"`
	StartedAt             time.Time  `json:"started_at"`
	EndedAt               *time.Time `json:"ended_at,omitempty"`
	ClaudeVersion         string     `json:"claude_version"`
	EntryPoint            string     `json:"entry_point"`
	PermissionMode        string     `json:"permission_mode"`
	TotalInputTokens      int        `json:"total_input_tokens"`
	TotalOutputTokens     int        `json:"total_output_tokens"`
	TotalCacheReadTokens  int        `json:"total_cache_read_tokens"`
	TotalCacheWriteTokens int        `json:"total_cache_write_tokens"`
	EstimatedCostUSD      float64    `json:"estimated_cost_usd"`
}

type Message struct {
	ID               string    `json:"id"`
	SessionID        string    `json:"session_id"`
	ParentID         string    `json:"parent_id"`
	Type             string    `json:"type"`
	Role             string    `json:"role"`
	Model            string    `json:"model"`
	ContentText      string    `json:"content_text"`
	ContentJSON      string    `json:"content_json"`
	StopReason       string    `json:"stop_reason"`
	InputTokens      int       `json:"input_tokens"`
	OutputTokens     int       `json:"output_tokens"`
	CacheReadTokens  int       `json:"cache_read_tokens"`
	CacheWriteTokens int       `json:"cache_write_tokens"`
	Timestamp        time.Time `json:"timestamp"`
}

type ToolCall struct {
	ID           string    `json:"id"`
	MessageID    string    `json:"message_id"`
	SessionID    string    `json:"session_id"`
	ToolName     string    `json:"tool_name"`
	ToolInput    string    `json:"tool_input"`
	ToolResponse string    `json:"tool_response"`
	Success      bool      `json:"success"`
	Error        string    `json:"error"`
	DurationMS   int       `json:"duration_ms"`
	Timestamp    time.Time `json:"timestamp"`
}

type Subagent struct {
	ID          string     `json:"id"`
	SessionID   string     `json:"session_id"`
	AgentType   string     `json:"agent_type"`
	Description string     `json:"description"`
	StartedAt   time.Time  `json:"started_at"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
}

type HookEvent struct {
	Event     string                 `json:"event"`
	Timestamp time.Time              `json:"timestamp"`
	SessionID string                 `json:"session_id"`
	Data      map[string]interface{} `json:"data"`
}
