package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thevibeworks/ccx/internal/catalog"
	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/provider"
	"github.com/thevibeworks/ccx/internal/trace"
)

var traceCmd = &cobra.Command{
	Use:   "trace [session]",
	Short: "Show what the agent did: turn/step outline with drill-down",
	Long: `Deterministic factual record of a coding-agent session.

Default output is the outline: every turn (user intent) broken into
steps (the agent's own narration of what it did and why), with tool,
edit, error, and cost rollups. Small enough to read whole, even for
monster sessions.

Drill down from there:
  ccx trace                     Outline of the latest workspace session
  ccx trace e38536              Outline of a specific session
  ccx trace --json              Outline as JSON (for skills/scripts)
  ccx trace --turn 133          Full evidence for one turn (JSON)
  ccx trace --full              Complete trace bundle (JSON, large)

The trace records facts only. Interpretation — what mattered, what
went wrong, what to learn — is the job of the /recap and /retro
skills, which read the outline first and drill into specific turns.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runTrace,
}

var (
	traceOutput  string
	traceProject string
	traceAll     bool
	traceJSON    bool
	traceTurn    int
	traceFull    bool
	traceWidth   int
)

func init() {
	addTraceFlags(traceCmd)
}

func addTraceFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&traceOutput, "output", "o", "", "output file (default: stdout)")
	cmd.Flags().StringVarP(&traceProject, "project", "p", "", "project name")
	cmd.Flags().BoolVar(&traceAll, "all", false, "search across all projects")
	cmd.Flags().BoolVar(&traceJSON, "json", false, "outline as JSON")
	cmd.Flags().IntVar(&traceTurn, "turn", 0, "full evidence for one turn (JSON)")
	cmd.Flags().BoolVar(&traceFull, "full", false, "complete trace bundle (JSON)")
	cmd.Flags().IntVar(&traceWidth, "width", trace.DefaultHeadlineWidth, "outline headline width in runes (0 = untruncated)")
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

	result := trace.Analyze(fullSession)

	repoDir, resolvedFrom, gitRootWarnings := findGitRootForSession(fullSession)
	result.Warnings = append(result.Warnings, gitRootWarnings...)
	if repoDir != "" {
		result.Git.ResolvedFrom = resolvedFrom
		if err := trace.CorrelateGit(result, repoDir); err != nil {
			result.Warnings = append(result.Warnings, trace.TraceWarning{
				Kind:    "git_correlation_failed",
				Message: err.Error(),
			})
			fmt.Fprintf(os.Stderr, "warning: git correlation failed: %v\n", err)
		}
		// Workspace docs are analyst context, only worth carrying in
		// the full bundle.
		if traceFull {
			if err := trace.CollectWorkspaceContext(result, repoDir); err != nil {
				result.Warnings = append(result.Warnings, trace.TraceWarning{
					Kind:    "workspace_context_failed",
					Message: err.Error(),
				})
				fmt.Fprintf(os.Stderr, "warning: workspace context failed: %v\n", err)
			}
		}
	} else if len(gitRootWarnings) == 0 {
		// The session-cwd warning already says everything this one
		// would; emit the generic line only when there was no session
		// cwd to blame (one line per run, not two).
		result.Warnings = append(result.Warnings, trace.TraceWarning{
			Kind:    "git_root_missing",
			Message: "no git repository found from session cwd or current working directory",
		})
	}

	// Session connections cost a parse of every workspace session, so
	// they ride only in the full bundle (docs/design/0006).
	if traceFull {
		related, warnings, err := relateSession(backend, session)
		result.Warnings = append(result.Warnings, warnings...)
		if err != nil {
			result.Warnings = append(result.Warnings, trace.TraceWarning{Kind: "related_failed", Message: err.Error()})
			fmt.Fprintf(os.Stderr, "warning: session connections failed: %v\n", err)
		} else {
			result.Related = related
		}
	}

	output, err := renderTrace(result)
	if err != nil {
		return err
	}

	if traceOutput != "" {
		dir := filepath.Dir(traceOutput)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("mkdir: %w", err)
			}
		}
		if err := os.WriteFile(traceOutput, output, 0600); err != nil {
			return fmt.Errorf("write: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Trace written to: %s\n", traceOutput)
		return nil
	}
	fmt.Println(strings.TrimRight(string(output), "\n"))
	return nil
}

func renderTrace(result *trace.TraceResult) ([]byte, error) {
	switch {
	case traceTurn > 0:
		return renderTurnJSON(result, traceTurn)
	case traceFull:
		return json.MarshalIndent(result, "", "  ")
	case traceJSON:
		return json.MarshalIndent(trace.BuildOutline(result, traceWidth), "", "  ")
	default:
		return []byte(trace.RenderOutlineText(trace.BuildOutline(result, traceWidth))), nil
	}
}

// renderTurnJSON emits one turn with full step evidence, plus the full
// sidechain entries the turn references (steps only carry light refs).
func renderTurnJSON(result *trace.TraceResult, index int) ([]byte, error) {
	for _, turn := range result.Turns {
		if turn.Index != index {
			continue
		}
		referenced := make(map[string]bool)
		for _, step := range turn.Steps {
			for _, sc := range step.Sidechains {
				referenced[sc.AgentID] = true
			}
		}
		var sidechains []trace.Sidechain
		for _, sc := range result.Sidechains {
			if referenced[sc.AgentID] {
				sidechains = append(sidechains, sc)
			}
		}
		return json.MarshalIndent(map[string]any{
			"kind":       "ccx.turn.v1",
			"session":    result.Session,
			"turn":       turn,
			"sidechains": sidechains,
			"warnings":   result.Warnings,
		}, "", "  ")
	}
	return nil, fmt.Errorf("turn %d not found (session has %d turns; run `ccx trace` for the outline)", index, len(result.Turns))
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

// findGitRootForSession resolves the repo for git correlation: the
// session's recorded cwd first, then the process cwd. A successful
// fallback is normal in containerized/remote setups (the session was
// recorded under a host path), so it is reported as provenance on the
// git block, not as a warning. Warnings fire only when nothing resolves.
func findGitRootForSession(session *parser.Session) (string, string, []trace.TraceWarning) {
	sessionCWD := ""
	if session != nil {
		sessionCWD = session.CWD
	}
	if root := findGitRootFrom(sessionCWD); root != "" {
		return root, "session_cwd", nil
	}
	if cwd, err := os.Getwd(); err == nil {
		if root := findGitRootFrom(cwd); root != "" {
			return root, "process_cwd", nil
		}
	}
	var warnings []trace.TraceWarning
	if sessionCWD != "" {
		warnings = append(warnings, trace.TraceWarning{
			Kind:    "session_git_root_missing",
			Message: fmt.Sprintf("session cwd %q is missing or not inside a git repository", sessionCWD),
		})
	}
	return "", "", warnings
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
