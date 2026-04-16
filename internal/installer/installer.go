// Package installer manages the installation and removal of Claude Code
// hooks in ~/.claude/settings.json.
package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// hookEntry represents a single hook command entry in settings.json.
type hookEntry struct {
	Matcher string `json:"matcher"`
	Command string `json:"command"`
	Async   bool   `json:"async,omitempty"`
	Timeout int    `json:"timeout"`
}

// InstallHooks reads ~/.claude/settings.json (creating it if it does not
// exist), backs it up, and adds the claude-monitor hook entries for all
// seven hook types. Existing non-hook settings are preserved.
func InstallHooks() error {
	claudeDir, err := claudeSettingsDir()
	if err != nil {
		return err
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	backupPath := settingsPath + ".backup"

	// 1. Read existing settings or start with an empty object.
	settings, err := readSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("read settings: %w", err)
	}

	// 2. Backup the current file (if it exists on disk).
	if _, statErr := os.Stat(settingsPath); statErr == nil {
		data, _ := os.ReadFile(settingsPath)
		if writeErr := os.WriteFile(backupPath, data, 0644); writeErr != nil {
			return fmt.Errorf("write backup: %w", writeErr)
		}
	}

	// 3. Build the hooks map.
	hooks := map[string][]hookEntry{
		"PreToolUse": {
			{Matcher: "", Command: "claude-monitor hook", Async: true, Timeout: 1000},
		},
		"PostToolUse": {
			{Matcher: "", Command: "claude-monitor hook", Async: true, Timeout: 1000},
		},
		"SessionStart": {
			{Matcher: "", Command: "claude-monitor hook", Async: false, Timeout: 2000},
		},
		"SessionEnd": {
			{Matcher: "", Command: "claude-monitor hook", Async: true, Timeout: 1000},
		},
		"SubagentStart": {
			{Matcher: "", Command: "claude-monitor hook", Async: true, Timeout: 1000},
		},
		"SubagentStop": {
			{Matcher: "", Command: "claude-monitor hook", Async: true, Timeout: 1000},
		},
		"Stop": {
			{Matcher: "", Command: "claude-monitor hook", Async: true, Timeout: 1000},
		},
	}

	// 4. Merge hooks into existing settings.
	//    We need to preserve any existing hooks and add ours alongside them.
	existingHooks, _ := settings["hooks"].(map[string]any)
	if existingHooks == nil {
		existingHooks = make(map[string]any)
	}

	for hookType, entries := range hooks {
		// Convert our entries to []any so they merge cleanly into the
		// generic JSON map.
		var entryList []any
		// Preserve existing entries for this hook type that are not ours.
		if existing, ok := existingHooks[hookType]; ok {
			if arr, ok := existing.([]any); ok {
				for _, item := range arr {
					if m, ok := item.(map[string]any); ok {
						cmd, _ := m["command"].(string)
						if cmd != "claude-monitor hook" {
							entryList = append(entryList, item)
						}
					}
				}
			}
		}
		// Append our entries.
		for _, e := range entries {
			entryList = append(entryList, map[string]any{
				"matcher": e.Matcher,
				"command": e.Command,
				"async":   e.Async,
				"timeout": e.Timeout,
			})
		}
		existingHooks[hookType] = entryList
	}

	settings["hooks"] = existingHooks

	// 5. Write back.
	return writeSettings(settingsPath, settings)
}

// UninstallHooks restores the settings backup if one exists. Otherwise
// it loads the current settings, removes the "hooks" key, and writes back.
func UninstallHooks() error {
	claudeDir, err := claudeSettingsDir()
	if err != nil {
		return err
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	backupPath := settingsPath + ".backup"

	// If a backup exists, restore it and remove the backup.
	if _, statErr := os.Stat(backupPath); statErr == nil {
		data, readErr := os.ReadFile(backupPath)
		if readErr != nil {
			return fmt.Errorf("read backup: %w", readErr)
		}
		if writeErr := os.WriteFile(settingsPath, data, 0644); writeErr != nil {
			return fmt.Errorf("restore backup: %w", writeErr)
		}
		os.Remove(backupPath)
		return nil
	}

	// No backup — just remove our hook entries.
	settings, err := readSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("read settings: %w", err)
	}

	// Remove only our entries from each hook type.
	if hooksRaw, ok := settings["hooks"]; ok {
		if hooksMap, ok := hooksRaw.(map[string]any); ok {
			for hookType, entries := range hooksMap {
				if arr, ok := entries.([]any); ok {
					var filtered []any
					for _, item := range arr {
						if m, ok := item.(map[string]any); ok {
							cmd, _ := m["command"].(string)
							if cmd != "claude-monitor hook" {
								filtered = append(filtered, item)
							}
						}
					}
					if len(filtered) > 0 {
						hooksMap[hookType] = filtered
					} else {
						delete(hooksMap, hookType)
					}
				}
			}
			if len(hooksMap) == 0 {
				delete(settings, "hooks")
			}
		}
	}

	return writeSettings(settingsPath, settings)
}

// claudeSettingsDir returns the path to ~/.claude, creating it if needed.
func claudeSettingsDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	dir := filepath.Join(homeDir, ".claude")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create claude dir: %w", err)
	}
	return dir, nil
}

// readSettings reads and parses a JSON settings file. If the file does
// not exist, an empty map is returned.
func readSettings(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]any), nil
		}
		return nil, err
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parse settings JSON: %w", err)
	}
	return settings, nil
}

// writeSettings marshals the settings map and writes it to path with
// human-readable indentation.
func writeSettings(path string, settings map[string]any) error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}
