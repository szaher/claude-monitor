package cli

import "fmt"

// Version prints the build version to stdout.
func Version(version string) {
	fmt.Printf("claude-monitor %s\n", version)
}
