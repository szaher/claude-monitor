package main

import (
	"fmt"
	"os"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "version":
		fmt.Printf("claude-monitor %s\n", version)
	case "install", "uninstall", "serve", "import", "export", "config", "status", "hook":
		fmt.Printf("Command '%s' not yet implemented\n", command)
		os.Exit(1)
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Usage: claude-monitor <command> [flags]

Commands:
  install          Install hooks and initialize database
  uninstall        Remove hooks, optionally delete data
  serve            Start daemon + web UI
  import           Import all historical Claude Code logs
  config           Manage configuration
  status           Show daemon status, database stats
  export           Export sessions as JSON/CSV/HTML
  hook             Handle hook events (called by Claude Code)
  version          Print version info

Run 'claude-monitor <command> --help' for more information on a command.`)
}
