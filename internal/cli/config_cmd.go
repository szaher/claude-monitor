package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/szaher/claude-monitor/internal/config"
	"gopkg.in/yaml.v3"
)

// Config manages the claude-monitor configuration.
func Config(args []string) error {
	if len(args) == 0 {
		printConfigUsage()
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}
	configPath := filepath.Join(home, ".claude-monitor", "config.yaml")

	switch args[0] {
	case "get":
		return configGet(configPath, args[1:])
	case "set":
		return configSet(configPath, args[1:])
	case "list":
		return configList(configPath)
	case "reset":
		return configReset(configPath)
	case "edit":
		return configEdit(configPath)
	default:
		return fmt.Errorf("unknown config subcommand: %s", args[0])
	}
}

func printConfigUsage() {
	fmt.Println(`Usage: claude-monitor config <subcommand> [args]

Subcommands:
  get <key>            Get a config value (dot-notation, e.g. "server.port")
  set <key> <value>    Set a config value
  list                 Print entire config as YAML
  reset                Reset config to defaults
  edit                 Open config in $EDITOR`)
}

// configGet prints the value for a given dot-notation key.
func configGet(configPath string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: claude-monitor config get <key>")
	}
	key := args[0]

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Marshal to a generic map for flexible key access
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	var m map[string]interface{}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	val, err := getNestedValue(m, key)
	if err != nil {
		return err
	}

	// Print the value
	switch v := val.(type) {
	case map[string]interface{}:
		out, _ := yaml.Marshal(v)
		fmt.Print(string(out))
	default:
		fmt.Println(v)
	}

	return nil
}

// configSet sets a config value given a dot-notation key and value string.
func configSet(configPath string, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: claude-monitor config set <key> <value>")
	}
	key := args[0]
	value := args[1]

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Marshal to a generic map
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	var m map[string]interface{}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	// Convert value to appropriate type
	typedValue := parseValue(value)

	// Set the nested value
	if err := setNestedValue(m, key, typedValue); err != nil {
		return err
	}

	// Marshal the map back to YAML, then unmarshal into Config struct
	data, err = yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal updated config: %w", err)
	}

	var updatedCfg config.Config
	if err := yaml.Unmarshal(data, &updatedCfg); err != nil {
		return fmt.Errorf("unmarshal updated config: %w", err)
	}

	if err := config.Save(configPath, &updatedCfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("Set %s = %v\n", key, typedValue)
	return nil
}

// configList prints the entire config as YAML.
func configList(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// configReset resets the config to defaults and writes it to disk.
func configReset(configPath string) error {
	cfg := config.DefaultConfig()
	if err := config.Save(configPath, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Println("Config reset to defaults.")
	fmt.Println("Saved to:", configPath)
	return nil
}

// configEdit opens the config file in the user's $EDITOR.
func configEdit(configPath string) error {
	// Ensure the file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		cfg := config.DefaultConfig()
		if err := config.Save(configPath, cfg); err != nil {
			return fmt.Errorf("create default config: %w", err)
		}
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "vi"
	}

	cmd := exec.Command(editor, configPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// getNestedValue retrieves a value from a nested map using a dot-notation key.
func getNestedValue(m map[string]interface{}, key string) (interface{}, error) {
	parts := strings.Split(key, ".")
	var current interface{} = m

	for _, part := range parts {
		cm, ok := current.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("key %q not found (path element %q is not a map)", key, part)
		}
		val, exists := cm[part]
		if !exists {
			return nil, fmt.Errorf("key %q not found", key)
		}
		current = val
	}

	return current, nil
}

// setNestedValue sets a value in a nested map using a dot-notation key.
func setNestedValue(m map[string]interface{}, key string, value interface{}) error {
	parts := strings.Split(key, ".")

	current := m
	for i := 0; i < len(parts)-1; i++ {
		val, exists := current[parts[i]]
		if !exists {
			return fmt.Errorf("key %q not found (missing path element %q)", key, parts[i])
		}
		nested, ok := val.(map[string]interface{})
		if !ok {
			return fmt.Errorf("key %q not found (path element %q is not a map)", key, parts[i])
		}
		current = nested
	}

	lastPart := parts[len(parts)-1]
	if _, exists := current[lastPart]; !exists {
		return fmt.Errorf("key %q not found", key)
	}
	current[lastPart] = value
	return nil
}

// parseValue converts a string value to the appropriate Go type.
func parseValue(s string) interface{} {
	// Boolean
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}

	// Integer
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}

	// Float
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}

	// String
	return s
}
