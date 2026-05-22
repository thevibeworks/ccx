package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/thevibeworks/ccx/internal/catalog"
	"github.com/thevibeworks/ccx/internal/fold"
	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/provider"
)

var traceCmd = &cobra.Command{
	Use:   "trace [session]",
	Short: "Extract a factual context trace for a session",
	Long: `Extract a deterministic evidence bundle for a coding-agent session.

The trace contains session exchanges, tool calls, file evidence, git state,
workspace context documents, and explicit warnings for missing evidence.
It does not decide what mattered. Feed this JSON to the ccx-context-fold
skill to produce an auditable decision trail and knowledge-base patch.

Examples:
  ccx trace                         # Latest workspace session, JSON to stdout
  ccx trace e38536                  # Specific session
  ccx trace -o trace.json           # Write trace JSON
  ccx trace --html                  # Also generate HTML evidence review`,
	Args: cobra.MaximumNArgs(1),
	RunE: runTrace,
}

var (
	traceOutput  string
	traceProject string
	traceHTML    bool
	traceAll     bool
)

func init() {
	addTraceFlags(traceCmd)
}

func addTraceFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&traceOutput, "output", "o", "", "output file (default: stdout)")
	cmd.Flags().StringVarP(&traceProject, "project", "p", "", "project name")
	cmd.Flags().BoolVar(&traceAll, "all", false, "search across all projects")
	cmd.Flags().BoolVar(&traceHTML, "html", false, "also generate HTML evidence review in temp directory")
}

func runTrace(cmd *cobra.Command, args []string) error {
	backend := provider.Default()

	session, err := resolveTraceSession(backend, args)
	if err != nil {
		return err
	}
	if session == nil {
		return fmt.Errorf("no session found")
	}

	fullSession, err := backend.ParseSession(session.FilePath)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	result := fold.Analyze(fullSession)

	repoDir, gitRootWarnings := findGitRootForSession(fullSession)
	result.Warnings = append(result.Warnings, gitRootWarnings...)
	if repoDir != "" {
		if err := fold.CorrelateGit(result, repoDir); err != nil {
			result.Warnings = append(result.Warnings, fold.TraceWarning{
				Kind:    "git_correlation_failed",
				Message: err.Error(),
			})
			fmt.Fprintf(os.Stderr, "warning: git correlation failed: %v\n", err)
		}
		if err := fold.CollectWorkspaceContext(result, repoDir); err != nil {
			result.Warnings = append(result.Warnings, fold.TraceWarning{
				Kind:    "workspace_context_failed",
				Message: err.Error(),
			})
			fmt.Fprintf(os.Stderr, "warning: workspace context failed: %v\n", err)
		}
	} else {
		result.Warnings = append(result.Warnings, fold.TraceWarning{
			Kind:    "git_root_missing",
			Message: "no git repository found from session cwd or current working directory",
		})
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	if traceOutput != "" {
		dir := filepath.Dir(traceOutput)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("mkdir: %w", err)
			}
		}
		if err := os.WriteFile(traceOutput, jsonBytes, 0600); err != nil {
			return fmt.Errorf("write: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Trace written to: %s\n", traceOutput)
	} else {
		fmt.Println(string(jsonBytes))
	}

	if traceHTML {
		if err := generateTraceHTML(result, fullSession); err != nil {
			fmt.Fprintf(os.Stderr, "warning: HTML generation failed: %v\n", err)
		}
	}

	return nil
}

func resolveTraceSession(backend provider.Backend, args []string) (*parser.Session, error) {
	if len(args) == 0 {
		session, err := latestTraceSession(backend, traceAll)
		if err != nil {
			return nil, fmt.Errorf("session: %w", err)
		}
		return session, nil
	}

	projectName, sessionID := parseSessionArg(args[0])
	if traceProject != "" {
		projectName = traceProject
	}
	query, err := sessionLookupQuery(projectName, traceAll)
	if err != nil {
		return nil, err
	}
	session, err := resolveSessionInQuery(backend, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session: %w", err)
	}
	return session, nil
}

func generateTraceHTML(result *fold.TraceResult, session *parser.Session) error {
	tmpDir := os.TempDir()

	slug := session.Slug
	if slug == "" && len(session.ID) > 8 {
		slug = session.ID[:8]
	}
	filename := fmt.Sprintf("ccx-trace-%s-%s.html",
		time.Now().Format("20060102-150405"), sanitizeFilename(slug))
	htmlPath := filepath.Join(tmpDir, filename)

	html := fold.RenderHTML(result)

	if err := os.WriteFile(htmlPath, []byte(html), 0600); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "HTML evidence review: %s\n", htmlPath)
	openBrowser(htmlPath)
	return nil
}

func findGitRootForSession(session *parser.Session) (string, []fold.TraceWarning) {
	var warnings []fold.TraceWarning
	if session != nil && session.CWD != "" {
		if root := findGitRootFrom(session.CWD); root != "" {
			return root, nil
		}
		warnings = append(warnings, fold.TraceWarning{
			Kind:    "session_git_root_missing",
			Message: fmt.Sprintf("session cwd %q is missing or not inside a git repository; falling back to current working directory", session.CWD),
		})
	}
	cwd, err := os.Getwd()
	if err == nil {
		if root := findGitRootFrom(cwd); root != "" {
			return root, warnings
		}
	}
	return "", warnings
}

func findGitRootFrom(dir string) string {
	if dir == "" {
		return ""
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return ""
	}
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return filepath.Clean(strings.TrimSpace(string(out)))
}

func sanitizeFilename(name string) string {
	if name == "" {
		return "session"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func latestTraceSession(backend provider.Backend, all bool) (*parser.Session, error) {
	query := catalog.SessionQuery{
		Sort:  catalog.SortTime,
		Limit: 1,
	}
	if all {
		query.Scope = catalog.ScopeAll
	} else {
		current, err := currentWorkspaceQuery()
		if err != nil {
			return nil, err
		}
		query.Scope = current.Scope
		query.WorkspacePath = current.WorkspacePath
	}

	sessions, err := backend.ListSessions(query)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}
	return sessions[0], nil
}

// openBrowser is defined in web.go
