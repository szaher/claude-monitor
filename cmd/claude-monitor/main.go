package main

import (
	"fmt"
	"os"

	"github.com/szaher/claude-monitor/internal/cli"
	"github.com/szaher/claude-monitor/internal/hook"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	args := os.Args[2:]

	switch os.Args[1] {
	case "version":
		cli.Version(version)
	case "hook":
		if err := hook.Execute(); err != nil {
			fmt.Fprintf(os.Stderr, "Hook error: %v\n", err)
			os.Exit(1)
		}
	case "install":
		if err := cli.Install(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "uninstall":
		if err := cli.Uninstall(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "serve":
		if err := cli.Serve(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "import":
		if err := cli.Import(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "export":
		if err := cli.Export(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "config":
		if err := cli.Config(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "status":
		if err := cli.Status(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
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
  hook             Handle hook events (internal use)
  version          Print version info`)
}
