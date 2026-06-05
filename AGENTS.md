# Agent Guidelines for claude-monitor

Read [CLAUDE.md](CLAUDE.md) first for project context and architecture.

## Build & Verify

```bash
CGO_ENABLED=1 go build -tags fts5 ./...       # compile check
CGO_ENABLED=1 go test -tags fts5 -v ./...      # run all tests
```

Never skip `-tags fts5` or `CGO_ENABLED=1` — the build will fail without them.

## Tool Availability

- Do NOT assume `gh` (GitHub CLI) is available. Use `git` for all version control operations.
- Do NOT assume `npm`, `node`, or any JS toolchain is available. The web UI has no build step.
- Standard Go toolchain (`go build`, `go test`, `go vet`) and `git` are safe to use.

## Code Conventions

- **Error wrapping**: Use `fmt.Errorf("context: %w", err)` — follow existing patterns in each package
- **No test frameworks**: Use standard `testing` package only
- **No comments unless the "why" is non-obvious**: The codebase is intentionally light on comments
- **Schema changes**: Add migrations to `internal/db/schema.go` in the `migrations` const — never modify the base `schema` const
- **Models**: Add struct definitions in `internal/models/models.go`
- **API endpoints**: Add handlers in `internal/server/api_*.go` files, register routes in `server.go`
- **Web UI**: Vanilla JS in `web/static/js/components/` — no frameworks, no build tools

## File Editing Rules

- Prefer editing existing files over creating new ones
- When adding a new API domain (e.g., budgets, heatmaps), create `internal/server/api_<domain>.go`
- When adding a new CLI subcommand, create `internal/cli/<command>.go` and register it in `cmd/claude-monitor/main.go`

## Before Completing Work

- Run `CGO_ENABLED=1 go build -tags fts5 ./...` to verify compilation
- Run `CGO_ENABLED=1 go test -tags fts5 -v ./...` to verify tests pass
- Do not claim work is done without running both commands
