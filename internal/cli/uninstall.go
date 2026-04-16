package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/szaher/claude-monitor/internal/installer"
)

// Uninstall removes Claude Code hooks and optionally deletes all
// claude-monitor data.
func Uninstall(args []string) error {
	// 1. Remove Claude Code hooks.
	if err := installer.UninstallHooks(); err != nil {
		return fmt.Errorf("uninstall hooks: %w", err)
	}
	fmt.Println("Removed Claude Code hooks")

	// 2. Ask the user whether to delete data.
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	baseDir := filepath.Join(homeDir, ".claude-monitor")

	fmt.Printf("Delete all claude-monitor data in %s? [y/N] ", baseDir)
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer == "y" || answer == "yes" {
		if err := os.RemoveAll(baseDir); err != nil {
			return fmt.Errorf("remove data directory: %w", err)
		}
		fmt.Println("Deleted:", baseDir)
	} else {
		fmt.Println("Data preserved in:", baseDir)
	}

	fmt.Println("claude-monitor uninstalled successfully.")
	return nil
}
