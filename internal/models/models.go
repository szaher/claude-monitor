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
	Notes                 string     `json:"notes"`
	Tags                  string     `json:"tags"`
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
	ID            string    `json:"id"`
	MessageID     string    `json:"message_id"`
	SessionID     string    `json:"session_id"`
	ToolName      string    `json:"tool_name"`
	ToolInput     string    `json:"tool_input"`
	ToolResponse  string    `json:"tool_response"`
	Success       bool      `json:"success"`
	Error         string    `json:"error"`
	DurationMS    int       `json:"duration_ms"`
	Timestamp     time.Time `json:"timestamp"`
	Stderr        string    `json:"stderr"`
	StdoutPreview string    `json:"stdout_preview"`
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

type SessionMetric struct {
	ID                     int       `json:"id"`
	SessionID              string    `json:"session_id"`
	MessageID              string    `json:"message_id"`
	Speed                  string    `json:"speed"`
	ServiceTier            string    `json:"service_tier"`
	InferenceGeo           string    `json:"inference_geo"`
	CacheEphemeral5mTokens int       `json:"cache_ephemeral_5m_tokens"`
	CacheEphemeral1hTokens int       `json:"cache_ephemeral_1h_tokens"`
	Timestamp              time.Time `json:"timestamp"`
}

type ContextCompaction struct {
	ID            int       `json:"id"`
	SessionID     string    `json:"session_id"`
	PreTokens     int       `json:"pre_tokens"`
	PostTokens    int       `json:"post_tokens"`
	TriggerReason string    `json:"trigger_reason"`
	DurationMS    int       `json:"duration_ms"`
	Timestamp     time.Time `json:"timestamp"`
}

type SessionAttachment struct {
	ID             int       `json:"id"`
	SessionID      string    `json:"session_id"`
	AttachmentType string    `json:"attachment_type"`
	Content        string    `json:"content"`
	Timestamp      time.Time `json:"timestamp"`
}

type SessionCommit struct {
	ID            int       `json:"id"`
	SessionID     string    `json:"session_id"`
	CommitHash    string    `json:"commit_hash"`
	CommitMessage string    `json:"commit_message"`
	Author        string    `json:"author"`
	FilesChanged  int       `json:"files_changed"`
	Insertions    int       `json:"insertions"`
	Deletions     int       `json:"deletions"`
	CommittedAt   time.Time `json:"committed_at"`
}

type Budget struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	ProjectPath string  `json:"project_path"`
	Period      string  `json:"period"`
	AmountUSD   float64 `json:"amount_usd"`
	Enabled     bool    `json:"enabled"`
}
