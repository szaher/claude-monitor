package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Server defaults
	if cfg.Server.Port != 3000 {
		t.Errorf("expected port 3000, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %s", cfg.Server.Host)
	}

	// Capture metadata defaults — all true except EnvironmentVars and CommandArgs
	if !cfg.Capture.Metadata.GitBranch {
		t.Error("expected GitBranch true")
	}
	if !cfg.Capture.Metadata.GitRepo {
		t.Error("expected GitRepo true")
	}
	if !cfg.Capture.Metadata.WorkingDir {
		t.Error("expected WorkingDir true")
	}
	if !cfg.Capture.Metadata.ClaudeVersion {
		t.Error("expected ClaudeVersion true")
	}
	if cfg.Capture.Metadata.EnvironmentVars {
		t.Error("expected EnvironmentVars false")
	}
	if cfg.Capture.Metadata.CommandArgs {
		t.Error("expected CommandArgs false")
	}
	if !cfg.Capture.Metadata.SystemInfo {
		t.Error("expected SystemInfo true")
	}

	// Capture events defaults — all true
	if !cfg.Capture.Events.SessionStart {
		t.Error("expected SessionStart true")
	}
	if !cfg.Capture.Events.SessionEnd {
		t.Error("expected SessionEnd true")
	}
	if !cfg.Capture.Events.PreToolUse {
		t.Error("expected PreToolUse true")
	}
	if !cfg.Capture.Events.PostToolUse {
		t.Error("expected PostToolUse true")
	}
	if !cfg.Capture.Events.SubagentStart {
		t.Error("expected SubagentStart true")
	}
	if !cfg.Capture.Events.SubagentStop {
		t.Error("expected SubagentStop true")
	}
	if !cfg.Capture.Events.Stop {
		t.Error("expected Stop true")
	}

	// Storage defaults
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("could not get home dir: %v", err)
	}
	expectedDB := filepath.Join(home, ".claude-monitor", "claude-monitor.db")
	expectedArchive := filepath.Join(home, ".claude-monitor", "archive")
	if cfg.Storage.DatabasePath != expectedDB {
		t.Errorf("expected database_path %s, got %s", expectedDB, cfg.Storage.DatabasePath)
	}
	if cfg.Storage.ArchivePath != expectedArchive {
		t.Errorf("expected archive_path %s, got %s", expectedArchive, cfg.Storage.ArchivePath)
	}
	if cfg.Storage.ArchiveEnabled {
		t.Error("expected archive_enabled false by default")
	}
	if cfg.Storage.RetentionDays != 0 {
		t.Errorf("expected retention_days 0, got %d", cfg.Storage.RetentionDays)
	}
	if cfg.Storage.MaxDBSizeMB != 0 {
		t.Errorf("expected max_db_size_mb 0, got %d", cfg.Storage.MaxDBSizeMB)
	}

	// Cost defaults — should have pricing for opus, sonnet, haiku
	for _, model := range []string{"opus", "sonnet", "haiku"} {
		if _, ok := cfg.Cost.Models[model]; !ok {
			t.Errorf("expected model pricing for %s", model)
		}
	}
	// Spot check a pricing value
	opus := cfg.Cost.Models["opus"]
	if opus.Input <= 0 {
		t.Error("expected opus input pricing > 0")
	}

	// UI defaults
	if cfg.UI.Theme != "auto" {
		t.Errorf("expected theme auto, got %s", cfg.UI.Theme)
	}
	if cfg.UI.DefaultPage != "dashboard" {
		t.Errorf("expected default_page dashboard, got %s", cfg.UI.DefaultPage)
	}
	if cfg.UI.SessionsPerPage != 50 {
		t.Errorf("expected sessions_per_page 50, got %d", cfg.UI.SessionsPerPage)
	}
}

func TestLoad_DefaultConfig(t *testing.T) {
	// Load from a non-existent file should return defaults
	path := filepath.Join(t.TempDir(), "nonexistent", "config.yaml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error for non-existent file: %v", err)
	}

	// Verify it matches defaults
	defaults := DefaultConfig()
	if cfg.Server.Port != defaults.Server.Port {
		t.Errorf("expected default port %d, got %d", defaults.Server.Port, cfg.Server.Port)
	}
	if cfg.Server.Host != defaults.Server.Host {
		t.Errorf("expected default host %s, got %s", defaults.Server.Host, cfg.Server.Host)
	}
	if cfg.UI.SessionsPerPage != defaults.UI.SessionsPerPage {
		t.Errorf("expected default sessions_per_page %d, got %d", defaults.UI.SessionsPerPage, cfg.UI.SessionsPerPage)
	}
}

func TestLoad_ExistingConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Write a partial YAML config that overrides some defaults
	yamlContent := `server:
  port: 8080
  host: "0.0.0.0"
capture:
  metadata:
    environment_vars: true
ui:
  theme: "dark"
  sessions_per_page: 25
`
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	// Check overridden values
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if !cfg.Capture.Metadata.EnvironmentVars {
		t.Error("expected EnvironmentVars true after override")
	}
	if cfg.UI.Theme != "dark" {
		t.Errorf("expected theme dark, got %s", cfg.UI.Theme)
	}
	if cfg.UI.SessionsPerPage != 25 {
		t.Errorf("expected sessions_per_page 25, got %d", cfg.UI.SessionsPerPage)
	}

	// Check that non-overridden defaults are preserved
	if !cfg.Capture.Metadata.GitBranch {
		t.Error("expected GitBranch true (default preserved)")
	}
	if !cfg.Capture.Events.SessionStart {
		t.Error("expected SessionStart true (default preserved)")
	}
}

func TestSave(t *testing.T) {
	dir := t.TempDir()
	// Use a nested path to verify parent directory creation
	path := filepath.Join(dir, "subdir", "config.yaml")

	cfg := DefaultConfig()
	cfg.Server.Port = 9999
	cfg.UI.Theme = "dark"

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	// Verify the file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}

	// Reload and verify roundtrip
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load after Save returned error: %v", err)
	}

	if loaded.Server.Port != 9999 {
		t.Errorf("expected port 9999, got %d", loaded.Server.Port)
	}
	if loaded.UI.Theme != "dark" {
		t.Errorf("expected theme dark, got %s", loaded.UI.Theme)
	}
	// Verify defaults survived roundtrip
	if loaded.Server.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %s", loaded.Server.Host)
	}
	if loaded.UI.SessionsPerPage != 50 {
		t.Errorf("expected sessions_per_page 50, got %d", loaded.UI.SessionsPerPage)
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("could not get home dir: %v", err)
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"~/foo/bar", filepath.Join(home, "foo", "bar")},
		{"~/.claude-monitor/db.sqlite", filepath.Join(home, ".claude-monitor", "db.sqlite")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"~", home},
	}

	for _, tt := range tests {
		result := ExpandPath(tt.input)
		if result != tt.expected {
			t.Errorf("ExpandPath(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(path, []byte("{{invalid yaml::: ["), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}
