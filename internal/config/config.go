package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration for claude-monitor.
type Config struct {
	Server  ServerConfig  `yaml:"server" json:"server"`
	Capture CaptureConfig `yaml:"capture" json:"capture"`
	Storage StorageConfig `yaml:"storage" json:"storage"`
	Cost    CostConfig    `yaml:"cost" json:"cost"`
	UI      UIConfig      `yaml:"ui" json:"ui"`
}

// ServerConfig holds the web server settings.
type ServerConfig struct {
	Port int    `yaml:"port" json:"port"`
	Host string `yaml:"host" json:"host"`
}

// CaptureConfig controls what data is captured from Claude Code sessions.
type CaptureConfig struct {
	Metadata MetadataConfig `yaml:"metadata" json:"metadata"`
	Events   EventsConfig   `yaml:"events" json:"events"`
}

// MetadataConfig controls which metadata fields are captured.
type MetadataConfig struct {
	GitBranch       bool `yaml:"git_branch" json:"git_branch"`
	GitRepo         bool `yaml:"git_repo" json:"git_repo"`
	WorkingDir      bool `yaml:"working_directory" json:"working_directory"`
	ClaudeVersion   bool `yaml:"claude_version" json:"claude_version"`
	EnvironmentVars bool `yaml:"environment_vars" json:"environment_vars"`
	CommandArgs     bool `yaml:"command_args" json:"command_args"`
	SystemInfo      bool `yaml:"system_info" json:"system_info"`
}

// EventsConfig controls which hook events are captured.
type EventsConfig struct {
	SessionStart  bool `yaml:"session_start" json:"session_start"`
	SessionEnd    bool `yaml:"session_end" json:"session_end"`
	PreToolUse    bool `yaml:"pre_tool_use" json:"pre_tool_use"`
	PostToolUse   bool `yaml:"post_tool_use" json:"post_tool_use"`
	SubagentStart bool `yaml:"subagent_start" json:"subagent_start"`
	SubagentStop  bool `yaml:"subagent_stop" json:"subagent_stop"`
	Stop          bool `yaml:"stop" json:"stop"`
}

// StorageConfig controls where and how data is stored.
type StorageConfig struct {
	DatabasePath   string `yaml:"database_path" json:"database_path"`
	ArchivePath    string `yaml:"archive_path" json:"archive_path"`
	ArchiveEnabled bool   `yaml:"archive_enabled" json:"archive_enabled"`
	RetentionDays  int    `yaml:"retention_days" json:"retention_days"`
	MaxDBSizeMB    int    `yaml:"max_db_size_mb" json:"max_db_size_mb"`
}

// CostConfig holds model pricing information for cost estimation.
type CostConfig struct {
	Models map[string]ModelPricing `yaml:"models" json:"models"`
}

// ModelPricing defines the per-token pricing for a model.
// Prices are in dollars per million tokens.
type ModelPricing struct {
	Input      float64 `yaml:"input" json:"input"`
	Output     float64 `yaml:"output" json:"output"`
	CacheRead  float64 `yaml:"cache_read" json:"cache_read"`
	CacheWrite float64 `yaml:"cache_write" json:"cache_write"`
}

// UIConfig controls the web UI appearance and behavior.
type UIConfig struct {
	Theme           string `yaml:"theme" json:"theme"`
	DefaultPage     string `yaml:"default_page" json:"default_page"`
	SessionsPerPage int    `yaml:"sessions_per_page" json:"sessions_per_page"`
}

// DefaultConfig returns a Config populated with sensible defaults.
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	baseDir := filepath.Join(home, ".claude-monitor")

	return &Config{
		Server: ServerConfig{
			Port: 3000,
			Host: "127.0.0.1",
		},
		Capture: CaptureConfig{
			Metadata: MetadataConfig{
				GitBranch:       true,
				GitRepo:         true,
				WorkingDir:      true,
				ClaudeVersion:   true,
				EnvironmentVars: false,
				CommandArgs:     false,
				SystemInfo:      true,
			},
			Events: EventsConfig{
				SessionStart:  true,
				SessionEnd:    true,
				PreToolUse:    true,
				PostToolUse:   true,
				SubagentStart: true,
				SubagentStop:  true,
				Stop:          true,
			},
		},
		Storage: StorageConfig{
			DatabasePath:   filepath.Join(baseDir, "claude-monitor.db"),
			ArchivePath:    filepath.Join(baseDir, "archive"),
			ArchiveEnabled: false,
			RetentionDays:  0,
			MaxDBSizeMB:    0,
		},
		Cost: CostConfig{
			Models: map[string]ModelPricing{
				"opus": {
					Input:      15.0,
					Output:     75.0,
					CacheRead:  1.5,
					CacheWrite: 18.75,
				},
				"sonnet": {
					Input:      3.0,
					Output:     15.0,
					CacheRead:  0.3,
					CacheWrite: 3.75,
				},
				"haiku": {
					Input:      0.25,
					Output:     1.25,
					CacheRead:  0.03,
					CacheWrite: 0.3,
				},
			},
		},
		UI: UIConfig{
			Theme:           "auto",
			DefaultPage:     "dashboard",
			SessionsPerPage: 50,
		},
	}
}

// Load reads a YAML config file from path. If the file does not exist,
// it returns the default configuration. The returned config starts with
// defaults, then overlays values from the file so that unspecified fields
// retain their defaults.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Save marshals the config to YAML and writes it to the given path.
// It creates the parent directory if it does not exist.
func Save(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// ExpandPath replaces a leading ~ with the user's home directory.
func ExpandPath(path string) string {
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
