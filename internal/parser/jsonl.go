package parser

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"time"
)

// LogEntry represents a single line from a Claude Code JSONL session log.
type LogEntry struct {
	Type           string      `json:"type"`
	UUID           string      `json:"uuid"`
	ParentUUID     string      `json:"parentUuid"`
	Timestamp      time.Time   `json:"timestamp"`
	SessionID      string      `json:"sessionId"`
	CWD            string      `json:"cwd"`
	GitBranch      string      `json:"gitBranch"`
	Version        string      `json:"version"`
	EntryPoint     string      `json:"entrypoint"`
	PermissionMode string      `json:"permissionMode"`
	Message        MessageData `json:"message"`
}

// MessageData holds the message payload within a log entry.
type MessageData struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string for user, []ContentBlock for assistant
	Model   string      `json:"model"`
	Usage   Usage       `json:"usage"`
}

// Usage tracks token counts for a single message.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

// ContentBlock represents a single block within an assistant message's content array.
type ContentBlock struct {
	Type     string                 `json:"type"`
	Text     string                 `json:"text,omitempty"`
	Thinking string                 `json:"thinking,omitempty"`
	Name     string                 `json:"name,omitempty"`
	ID       string                 `json:"id,omitempty"`
	Input    map[string]interface{} `json:"input,omitempty"`
}

// ParseLogEntry unmarshals a single JSON line into a LogEntry.
func ParseLogEntry(data []byte) (*LogEntry, error) {
	var entry LogEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// ParseLogFile reads a JSONL stream line-by-line, returning all successfully parsed entries.
// Empty lines and malformed JSON lines are silently skipped.
func ParseLogFile(r io.Reader) ([]*LogEntry, error) {
	var entries []*LogEntry

	scanner := bufio.NewScanner(r)
	// Set a large buffer to handle big log lines (up to 1MB).
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		entry, err := ParseLogEntry([]byte(line))
		if err != nil {
			// Skip malformed lines.
			continue
		}
		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return entries, err
	}

	return entries, nil
}

// ExtractContentText returns the searchable text from a message's content field.
// If content is a string, it is returned directly.
// If content is an array of content blocks, all "text" fields are extracted and joined with a space.
func ExtractContentText(content interface{}) string {
	if content == nil {
		return ""
	}

	// String content (typical for user messages).
	if s, ok := content.(string); ok {
		return s
	}

	// Array content (typical for assistant messages).
	arr, ok := content.([]interface{})
	if !ok {
		return ""
	}

	var texts []string
	for _, item := range arr {
		block, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if text, ok := block["text"].(string); ok && text != "" {
			texts = append(texts, text)
		}
	}

	return strings.Join(texts, " ")
}

// ExtractToolCalls extracts all content blocks of type "tool_use" from a message's content.
// Returns an empty slice if content is not an array or contains no tool_use blocks.
func ExtractToolCalls(content interface{}) []ContentBlock {
	if content == nil {
		return nil
	}

	arr, ok := content.([]interface{})
	if !ok {
		return nil
	}

	var tools []ContentBlock
	for _, item := range arr {
		block, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		blockType, _ := block["type"].(string)
		if blockType != "tool_use" {
			continue
		}

		cb := ContentBlock{
			Type: blockType,
		}
		if name, ok := block["name"].(string); ok {
			cb.Name = name
		}
		if id, ok := block["id"].(string); ok {
			cb.ID = id
		}
		if input, ok := block["input"].(map[string]interface{}); ok {
			cb.Input = input
		}

		tools = append(tools, cb)
	}

	return tools
}

// DecodeProjectPath converts an encoded project directory name back to a filesystem path.
// The encoding replaces "/" with "-", so "-Users-szaher-my-project" becomes "/Users/szaher/my/project".
// Since hyphens within directory names are indistinguishable from path separators in this encoding,
// all hyphens are replaced with "/".
func DecodeProjectPath(encoded string) string {
	if encoded == "" {
		return ""
	}
	return strings.ReplaceAll(encoded, "-", "/")
}
