package cli

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/szaher/claude-monitor/internal/config"
	"github.com/szaher/claude-monitor/internal/db"
)

// Status shows the daemon status and database statistics.
func Status(args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	baseDir := filepath.Join(home, ".claude-monitor")
	configPath := filepath.Join(baseDir, "config.yaml")

	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	dbPath := cfg.Storage.DatabasePath
	if dbPath == "" {
		dbPath = filepath.Join(baseDir, "claude-monitor.db")
	}

	fmt.Println("Claude Monitor Status")
	fmt.Println("=====================")
	fmt.Println()

	// Config file
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Config:     %s\n", configPath)
	} else {
		fmt.Printf("Config:     %s (not found)\n", configPath)
	}

	// Database
	dbInfo, err := os.Stat(dbPath)
	if err != nil {
		fmt.Printf("Database:   %s (not found)\n", dbPath)
		fmt.Println()
		fmt.Println("Run 'claude-monitor install' to set up the database.")
		return nil
	}

	fmt.Printf("Database:   %s (%s)\n", dbPath, formatSize(dbInfo.Size()))

	// Open database to get counts
	database, err := db.InitDB(dbPath)
	if err != nil {
		fmt.Printf("  (could not open database: %v)\n", err)
	} else {
		defer database.Close()

		var sessionCount, messageCount, toolCallCount int

		database.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&sessionCount)
		database.QueryRow("SELECT COUNT(*) FROM messages").Scan(&messageCount)
		database.QueryRow("SELECT COUNT(*) FROM tool_calls").Scan(&toolCallCount)

		fmt.Printf("Sessions:   %d\n", sessionCount)
		fmt.Printf("Messages:   %d\n", messageCount)
		fmt.Printf("Tool Calls: %d\n", toolCallCount)

		// Total cost
		var totalCost float64
		database.QueryRow("SELECT COALESCE(SUM(estimated_cost_usd), 0) FROM sessions").Scan(&totalCost)
		if totalCost > 0 {
			fmt.Printf("Total Cost: $%.4f\n", totalCost)
		}
	}

	// Daemon status - check if socket exists and is connectable
	socketPath := filepath.Join(baseDir, "monitor.sock")
	daemonStatus := "not running"
	if _, err := os.Stat(socketPath); err == nil {
		// Try to connect to the socket to verify the daemon is actually running
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			conn.Close()
			daemonStatus = "running"
		} else {
			daemonStatus = "not running (stale socket)"
		}
	}
	fmt.Printf("Daemon:     %s\n", daemonStatus)

	fmt.Println()
	return nil
}

// formatSize returns a human-readable file size string.
func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
