// Package hook implements the hook command that relays Claude Code
// hook events to the monitoring daemon via a Unix domain socket.
package hook

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
)

// Execute reads JSON hook data from stdin and forwards it to the
// monitoring daemon over the Unix domain socket. If the daemon is not
// running the function returns nil so that Claude Code is never blocked.
func Execute() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	sockPath := filepath.Join(homeDir, ".claude-monitor", "monitor.sock")

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	if len(data) == 0 {
		return nil
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		// Daemon not running — exit silently so we never block Claude Code.
		return nil
	}
	defer conn.Close()

	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("write to socket: %w", err)
	}

	return nil
}
