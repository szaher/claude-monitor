package parser

import (
	"strings"
	"testing"
	"time"
)

func TestParseLogEntry_UserEntry(t *testing.T) {
	data := []byte(`{
		"type": "user",
		"uuid": "abc-123",
		"parentUuid": "",
		"timestamp": "2026-04-15T10:30:00Z",
		"sessionId": "session-001",
		"cwd": "/Users/szaher/my-project",
		"gitBranch": "main",
		"version": "1.2.3",
		"entrypoint": "cli",
		"permissionMode": "default",
		"message": {
			"role": "user",
			"content": "Hello, Claude!"
		}
	}`)

	entry, err := ParseLogEntry(data)
	if err != nil {
		t.Fatalf("ParseLogEntry returned error: %v", err)
	}

	if entry.Type != "user" {
		t.Errorf("expected type 'user', got %q", entry.Type)
	}
	if entry.UUID != "abc-123" {
		t.Errorf("expected uuid 'abc-123', got %q", entry.UUID)
	}
	if entry.ParentUUID != "" {
		t.Errorf("expected empty parentUuid, got %q", entry.ParentUUID)
	}
	expectedTime := time.Date(2026, 4, 15, 10, 30, 0, 0, time.UTC)
	if !entry.Timestamp.Equal(expectedTime) {
		t.Errorf("expected timestamp %v, got %v", expectedTime, entry.Timestamp)
	}
	if entry.SessionID != "session-001" {
		t.Errorf("expected sessionId 'session-001', got %q", entry.SessionID)
	}
	if entry.CWD != "/Users/szaher/my-project" {
		t.Errorf("expected cwd '/Users/szaher/my-project', got %q", entry.CWD)
	}
	if entry.GitBranch != "main" {
		t.Errorf("expected gitBranch 'main', got %q", entry.GitBranch)
	}
	if entry.Version != "1.2.3" {
		t.Errorf("expected version '1.2.3', got %q", entry.Version)
	}
	if entry.EntryPoint != "cli" {
		t.Errorf("expected entrypoint 'cli', got %q", entry.EntryPoint)
	}
	if entry.PermissionMode != "default" {
		t.Errorf("expected permissionMode 'default', got %q", entry.PermissionMode)
	}
	if entry.Message.Role != "user" {
		t.Errorf("expected message role 'user', got %q", entry.Message.Role)
	}

	text := ExtractContentText(entry.Message.Content)
	if text != "Hello, Claude!" {
		t.Errorf("expected content text 'Hello, Claude!', got %q", text)
	}
}

func TestParseLogEntry_AssistantEntry(t *testing.T) {
	data := []byte(`{
		"type": "assistant",
		"uuid": "def-456",
		"parentUuid": "abc-123",
		"timestamp": "2026-04-15T10:30:05Z",
		"sessionId": "session-001",
		"message": {
			"role": "assistant",
			"model": "claude-sonnet-4-20250514",
			"content": [
				{"type": "thinking", "thinking": "Let me think about this..."},
				{"type": "text", "text": "Here is my response."},
				{"type": "tool_use", "id": "tool-1", "name": "Bash", "input": {"command": "ls"}}
			],
			"usage": {
				"input_tokens": 100,
				"output_tokens": 50,
				"cache_read_input_tokens": 80,
				"cache_creation_input_tokens": 20
			}
		}
	}`)

	entry, err := ParseLogEntry(data)
	if err != nil {
		t.Fatalf("ParseLogEntry returned error: %v", err)
	}

	if entry.Type != "assistant" {
		t.Errorf("expected type 'assistant', got %q", entry.Type)
	}
	if entry.UUID != "def-456" {
		t.Errorf("expected uuid 'def-456', got %q", entry.UUID)
	}
	if entry.ParentUUID != "abc-123" {
		t.Errorf("expected parentUuid 'abc-123', got %q", entry.ParentUUID)
	}
	if entry.Message.Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected model 'claude-sonnet-4-20250514', got %q", entry.Message.Model)
	}
	if entry.Message.Usage.InputTokens != 100 {
		t.Errorf("expected input_tokens 100, got %d", entry.Message.Usage.InputTokens)
	}
	if entry.Message.Usage.OutputTokens != 50 {
		t.Errorf("expected output_tokens 50, got %d", entry.Message.Usage.OutputTokens)
	}
	if entry.Message.Usage.CacheReadInputTokens != 80 {
		t.Errorf("expected cache_read_input_tokens 80, got %d", entry.Message.Usage.CacheReadInputTokens)
	}
	if entry.Message.Usage.CacheCreationInputTokens != 20 {
		t.Errorf("expected cache_creation_input_tokens 20, got %d", entry.Message.Usage.CacheCreationInputTokens)
	}

	// Verify content text extraction
	text := ExtractContentText(entry.Message.Content)
	if !strings.Contains(text, "Here is my response.") {
		t.Errorf("expected content text to contain 'Here is my response.', got %q", text)
	}

	// Verify tool call extraction
	toolCalls := ExtractToolCalls(entry.Message.Content)
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].Name != "Bash" {
		t.Errorf("expected tool name 'Bash', got %q", toolCalls[0].Name)
	}
	if toolCalls[0].ID != "tool-1" {
		t.Errorf("expected tool id 'tool-1', got %q", toolCalls[0].ID)
	}
	if toolCalls[0].Type != "tool_use" {
		t.Errorf("expected tool type 'tool_use', got %q", toolCalls[0].Type)
	}
}

func TestParseLogFile(t *testing.T) {
	jsonl := `{"type":"user","uuid":"u1","timestamp":"2026-04-15T10:00:00Z","sessionId":"s1","message":{"role":"user","content":"Hello"}}
{"type":"assistant","uuid":"a1","parentUuid":"u1","timestamp":"2026-04-15T10:00:01Z","sessionId":"s1","message":{"role":"assistant","content":[{"type":"text","text":"Hi there"}],"model":"claude-sonnet-4-20250514","usage":{"input_tokens":10,"output_tokens":5}}}

{"type":"user","uuid":"u2","timestamp":"2026-04-15T10:00:02Z","sessionId":"s1","message":{"role":"user","content":"Another message"}}
not valid json at all
{"type":"system","uuid":"s1","timestamp":"2026-04-15T10:00:03Z","sessionId":"s1","message":{"role":"system","content":"System info"}}`

	reader := strings.NewReader(jsonl)
	entries, err := ParseLogFile(reader)
	if err != nil {
		t.Fatalf("ParseLogFile returned error: %v", err)
	}

	// Should have 4 valid entries (empty line and invalid JSON are skipped)
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	if entries[0].Type != "user" {
		t.Errorf("entry 0: expected type 'user', got %q", entries[0].Type)
	}
	if entries[0].UUID != "u1" {
		t.Errorf("entry 0: expected uuid 'u1', got %q", entries[0].UUID)
	}

	if entries[1].Type != "assistant" {
		t.Errorf("entry 1: expected type 'assistant', got %q", entries[1].Type)
	}
	if entries[1].Message.Model != "claude-sonnet-4-20250514" {
		t.Errorf("entry 1: expected model 'claude-sonnet-4-20250514', got %q", entries[1].Message.Model)
	}

	if entries[2].Type != "user" {
		t.Errorf("entry 2: expected type 'user', got %q", entries[2].Type)
	}

	if entries[3].Type != "system" {
		t.Errorf("entry 3: expected type 'system', got %q", entries[3].Type)
	}
}

func TestExtractContentText_String(t *testing.T) {
	text := ExtractContentText("plain text content")
	if text != "plain text content" {
		t.Errorf("expected 'plain text content', got %q", text)
	}
}

func TestExtractContentText_Array(t *testing.T) {
	// Simulate the interface{} that JSON unmarshaling produces for an array of content blocks
	content := []interface{}{
		map[string]interface{}{"type": "thinking", "thinking": "Let me think..."},
		map[string]interface{}{"type": "text", "text": "First part."},
		map[string]interface{}{"type": "text", "text": "Second part."},
		map[string]interface{}{"type": "tool_use", "name": "Bash", "id": "t1", "input": map[string]interface{}{"command": "ls"}},
	}

	text := ExtractContentText(content)
	if !strings.Contains(text, "First part.") {
		t.Errorf("expected text to contain 'First part.', got %q", text)
	}
	if !strings.Contains(text, "Second part.") {
		t.Errorf("expected text to contain 'Second part.', got %q", text)
	}
	// Tool use blocks should not appear in text
	if strings.Contains(text, "Bash") {
		t.Errorf("expected text to not contain tool name, got %q", text)
	}
}

func TestExtractToolCalls(t *testing.T) {
	content := []interface{}{
		map[string]interface{}{"type": "thinking", "thinking": "Let me think..."},
		map[string]interface{}{"type": "text", "text": "Some text"},
		map[string]interface{}{
			"type":  "tool_use",
			"name":  "Read",
			"id":    "tool-read-1",
			"input": map[string]interface{}{"file_path": "/tmp/test.go"},
		},
		map[string]interface{}{
			"type":  "tool_use",
			"name":  "Edit",
			"id":    "tool-edit-1",
			"input": map[string]interface{}{"file_path": "/tmp/test.go", "old_string": "foo", "new_string": "bar"},
		},
	}

	toolCalls := ExtractToolCalls(content)
	if len(toolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(toolCalls))
	}

	if toolCalls[0].Name != "Read" {
		t.Errorf("expected first tool name 'Read', got %q", toolCalls[0].Name)
	}
	if toolCalls[0].ID != "tool-read-1" {
		t.Errorf("expected first tool id 'tool-read-1', got %q", toolCalls[0].ID)
	}
	if toolCalls[0].Input["file_path"] != "/tmp/test.go" {
		t.Errorf("expected first tool input file_path '/tmp/test.go', got %v", toolCalls[0].Input["file_path"])
	}

	if toolCalls[1].Name != "Edit" {
		t.Errorf("expected second tool name 'Edit', got %q", toolCalls[1].Name)
	}
	if toolCalls[1].ID != "tool-edit-1" {
		t.Errorf("expected second tool id 'tool-edit-1', got %q", toolCalls[1].ID)
	}

	// Test with string content (no tool calls)
	noTools := ExtractToolCalls("just a string")
	if len(noTools) != 0 {
		t.Errorf("expected 0 tool calls for string content, got %d", len(noTools))
	}

	// Test with nil content
	nilTools := ExtractToolCalls(nil)
	if len(nilTools) != 0 {
		t.Errorf("expected 0 tool calls for nil content, got %d", len(nilTools))
	}
}

func TestParseLogEntry_NewFields(t *testing.T) {
	raw := `{"type":"assistant","uuid":"abc","timestamp":"2026-04-16T10:00:00Z","sessionId":"s1","speed":"fast","server_tool_use":{"web_search_count":2,"web_fetch_count":1},"message":{"role":"assistant","content":"hello","model":"claude-opus-4-6","usage":{"input_tokens":100,"output_tokens":50,"service_tier":"standard","cache_creation":{"ephemeral_5m_input_tokens":500,"ephemeral_1h_input_tokens":0}},"stop_reason":"end_turn"}}`

	entry, err := ParseLogEntry([]byte(raw))
	if err != nil {
		t.Fatalf("ParseLogEntry: %v", err)
	}

	if entry.Speed != "fast" {
		t.Errorf("Speed = %q, want %q", entry.Speed, "fast")
	}
	if entry.ServerToolUse == nil || entry.ServerToolUse.WebSearchCount != 2 {
		t.Errorf("ServerToolUse.WebSearchCount = %v, want 2", entry.ServerToolUse)
	}
	if entry.Message.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want %q", entry.Message.StopReason, "end_turn")
	}
	if entry.Message.Usage.ServiceTier != "standard" {
		t.Errorf("ServiceTier = %q, want %q", entry.Message.Usage.ServiceTier, "standard")
	}
	if entry.Message.Usage.CacheCreation == nil || entry.Message.Usage.CacheCreation.Ephemeral5mTokens != 500 {
		t.Errorf("CacheCreation.Ephemeral5mTokens = %v, want 500", entry.Message.Usage.CacheCreation)
	}
}

func TestDecodeProjectPath(t *testing.T) {
	tests := []struct {
		encoded  string
		expected string
	}{
		{"-Users-szaher-my-project", "/Users/szaher/my/project"},
		{"-Users-szaher-go-src", "/Users/szaher/go/src"},
		{"-home-user-code", "/home/user/code"},
		{"", ""},
		{"-", "/"},
	}

	for _, tt := range tests {
		result := DecodeProjectPath(tt.encoded)
		if result != tt.expected {
			t.Errorf("DecodeProjectPath(%q) = %q, want %q", tt.encoded, result, tt.expected)
		}
	}
}
