# Contributing to ccx

Get set up and ship code in 5 minutes. No ceremony.

## Quick Start

```bash
git clone https://github.com/thevibeworks/ccx.git
cd ccx
make build      # Builds ./bin/ccx
make test       # Runs tests
./bin/ccx --help
```

Requirements: Go 1.24+ (pure Go, no CGO/C compiler needed)

## Running Tests

```bash
make test                      # All tests
go test ./internal/parser/...  # Specific package
go test -v -run TestFoo        # Specific test
```

## Code Structure

```
cmd/ccx/main.go          Entry point
internal/
├── cmd/                 Cobra commands
│   ├── root.go
│   ├── projects.go
│   ├── sessions.go
│   ├── view.go
│   ├── export.go
│   └── web.go
├── parser/              JSONL parsing + tree building
├── render/              Output formats (HTML, MD, Org)
├── web/                 HTTP server + templates
├── db/                  SQLite persistence (stars, cache)
└── config/              Configuration paths
```

**Pattern:**
- Commands in `internal/cmd/`
- Core logic in `internal/{domain}/`
- No external dependencies in parser (stdlib only)

## Adding a New Command

Example: Adding `ccx stats`

1. Create the command file:
```bash
touch internal/cmd/stats.go
```

2. Write the command:
```go
package cmd

import "github.com/spf13/cobra"

var statsCmd = &cobra.Command{
    Use:   "stats",
    Short: "Show session statistics",
    RunE:  runStats,
}

func init() {
    rootCmd.AddCommand(statsCmd)
}

func runStats(cmd *cobra.Command, args []string) error {
    // Implementation
    return nil
}
```

3. Test it:
```bash
make build && ./bin/ccx stats
```

## Adding a New Export Format

1. Add to `internal/render/`:
```go
// internal/render/csv.go
func RenderCSV(session *parser.Session) ([]byte, error) {
    // Implementation
}
```

2. Register in export command (`internal/cmd/export.go`)

## PR Guidelines

**Before submitting:**
- `make test` passes
- `make build` succeeds
- Manual test with real Claude Code sessions

**PR description:**
```
What: Added `ccx stats` command
Why: Users want to see token usage across sessions
Tested: Ran against my ~/.claude sessions
```

No templates. Just tell us what you did and why.

## What We Want

### High Priority
- Full-text search within session content
- Session comparison/diff view
- Cost estimation display
- More export formats (PDF, EPUB)

### Nice to Have
- MCP server mode
- Session replay
- Custom tagging

### Not Interested
- Modifying Claude Code sessions (read-only by design)
- Syncing to cloud services
- Anything that requires network access by default

## Code Style

Follow Go conventions:
- `gofmt` your code
- Meaningful names: `sessionID` not `sid`
- Comments explain WHY, not WHAT
- If it needs a comment to understand WHAT it does, refactor it

**Error messages:**
```go
BAD:  return fmt.Errorf("error")
GOOD: return fmt.Errorf("failed to parse session %s: %w", path, err)
```

## Testing Philosophy

Test logic, not I/O.

```go
// Good - tests parsing logic
func TestParseMessage_ExtractsContent(t *testing.T) {
    msg := parseMessage(`{"type":"user","content":"hello"}`)
    if msg.Content != "hello" {
        t.Errorf("expected hello, got %s", msg.Content)
    }
}

// Bad - tests filesystem
func TestReadSession_ReadsFile(t *testing.T) {
    session := ReadSession("/path/to/session.jsonl")
    // ...
}
```

Use `testdata/` for fixture files. Keep them minimal.

## Questions?

Open an issue. We're friendly, just direct.

---

Inspired by Simon Willison's claude-code-transcripts. Rebuilt in Go.
