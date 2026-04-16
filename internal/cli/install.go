package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/szaher/claude-monitor/internal/config"
	"github.com/szaher/claude-monitor/internal/db"
	"github.com/szaher/claude-monitor/internal/installer"
)

// Install sets up claude-monitor: creates directories, initialises the
// database, writes default config, and installs Claude Code hooks.
func Install(args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	baseDir := filepath.Join(homeDir, ".claude-monitor")

	// 1. Create base directory.
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return fmt.Errorf("create base directory: %w", err)
	}
	fmt.Println("Created directory:", baseDir)

	// 2. Initialise database.
	dbPath := filepath.Join(baseDir, "claude-monitor.db")
	database, err := db.InitDB(dbPath)
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}
	database.Close()
	fmt.Println("Initialised database:", dbPath)

	// 3. Create archive directory.
	archiveDir := filepath.Join(baseDir, "archive")
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return fmt.Errorf("create archive directory: %w", err)
	}
	fmt.Println("Created archive directory:", archiveDir)

	// 4. Write default configuration.
	cfgPath := filepath.Join(baseDir, "config.yaml")
	cfg := config.DefaultConfig()
	if err := config.Save(cfgPath, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	fmt.Println("Wrote default config:", cfgPath)

	// 5. Install Claude Code hooks.
	if err := installer.InstallHooks(); err != nil {
		return fmt.Errorf("install hooks: %w", err)
	}
	fmt.Println("Installed Claude Code hooks")

	fmt.Println()
	fmt.Println("claude-monitor installed successfully!")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Start the daemon:   claude-monitor serve")
	fmt.Println("  2. Import history:     claude-monitor import")
	fmt.Println("  3. Open the dashboard: http://127.0.0.1:3000")

	return nil
}
