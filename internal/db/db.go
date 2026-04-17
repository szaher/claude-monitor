// Package db provides SQLite persistence for claude-monitor.
//
// Build with the "fts5" tag to enable full-text search:
//
//	CGO_ENABLED=1 go build -tags fts5 ./...
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// InitDB opens (or creates) a SQLite database at the given path,
// enables WAL mode and foreign keys, and applies the schema.
func InitDB(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=ON")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("execute schema: %w", err)
	}

	// Run migrations (idempotent — ALTER TABLE errors are ignored)
	for _, stmt := range strings.Split(migrations, ";") {
		// Strip leading comment lines from the statement
		lines := strings.Split(stmt, "\n")
		var cleaned []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "--") {
				continue
			}
			cleaned = append(cleaned, line)
		}
		stmt = strings.TrimSpace(strings.Join(cleaned, "\n"))
		if stmt == "" {
			continue
		}
		_, err := db.Exec(stmt)
		if err != nil && !strings.Contains(err.Error(), "duplicate column") {
			db.Close()
			return nil, fmt.Errorf("migration %q: %w", stmt, err)
		}
	}

	return db, nil
}
