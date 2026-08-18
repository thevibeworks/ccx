package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thevibeworks/ccx/internal/catalog"
	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/provider"
	"github.com/thevibeworks/ccx/internal/render"
)

var viewCmd = &cobra.Command{
	Use:   "view [session]",
	Short: "View a session in terminal",
	Long: `View a session in the terminal.

SESSION can be:
  - Full UUID: e38536a2-dbe6-442d-8b69-5bab525796ee
  - Short prefix: e38536
  - Index: @1 (most recent), @2 (second most recent)
  - With project: myproject:e38536

If SESSION is omitted, shows an interactive picker.

--at MESSAGE_ID walks from a citation to its context: the message
with that id (a search --hits message_id, a trace step message_id;
prefixes work) plus --context N messages before and after it,
flattened. The target is always shown, even under --brief.

Examples:
  ccx view e38536                     Whole session
  ccx view e38536 --brief             Conversation only
  ccx view e38536 --at c8bd2144       Around one cited message
  ccx view e38536 --at c8bd2144 --context 8`,
	Args: cobra.MaximumNArgs(1),
	RunE: runView,
}

var (
	viewProject      string
	viewShowThinking bool
	viewShowAgents   bool
	viewFlat         bool
	viewBrief        bool
	viewAll          bool
	viewColor        string
	viewAt           string
	viewContext      int
)

func init() {
	viewCmd.Flags().StringVarP(&viewProject, "project", "p", "", "project name")
	viewCmd.Flags().BoolVar(&viewAll, "all", false, "search across all projects")
	viewCmd.Flags().BoolVar(&viewShowThinking, "show-thinking", false, "show thinking blocks expanded")
	viewCmd.Flags().BoolVar(&viewShowAgents, "show-agents", false, "show agent sidechains")
	viewCmd.Flags().BoolVar(&viewFlat, "flat", false, "disable tree rendering")
	viewCmd.Flags().BoolVarP(&viewBrief, "brief", "b", false, "conversation only: human input, agent responses, compactions")
	viewCmd.Flags().StringVar(&viewColor, "color", "auto", "colorize output: auto, always, never")
	viewCmd.Flags().StringVar(&viewAt, "at", "", "show the message with this id (or unique prefix) and its context")
	viewCmd.Flags().IntVar(&viewContext, "context", 3, "with --at: messages of context before and after the target")
}

// resolveColorMode maps --color to a concrete decision; auto follows
// whether stdout is a terminal, so piped output stays grep-clean.
func resolveColorMode(mode string) (bool, error) {
	switch mode {
	case "auto":
		info, err := os.Stdout.Stat()
		return err == nil && info.Mode()&os.ModeCharDevice != 0, nil
	case "always":
		return true, nil
	case "never":
		return false, nil
	default:
		return false, fmt.Errorf("invalid --color %q (valid: auto, always, never)", mode)
	}
}

func runView(cmd *cobra.Command, args []string) error {
	backend := provider.Default()

	var session *parser.Session
	var err error

	if len(args) == 0 {
		session, err = selectSession(backend, viewAll)
	} else {
		sessionArg := args[0]
		projectName, sessionID := parseSessionArg(sessionArg)
		if viewProject != "" {
			projectName = viewProject
		}
		query, qErr := sessionLookupQuery(projectName, viewAll)
		if qErr != nil {
			return qErr
		}
		session, err = resolveSessionInQuery(backend, query, sessionID)
	}

	if err != nil {
		return fmt.Errorf("failed to find session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("session not found")
	}

	fullSession, err := backend.ParseSession(session.FilePath)
	if err != nil {
		return fmt.Errorf("failed to parse session: %w", err)
	}

	if viewAt != "" {
		window, index, total, err := render.WindowSession(fullSession, viewAt, viewContext, viewContext)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "message %d of %d in %s · %d before / %d after\n", index, total, truncateID(fullSession.ID, 8), viewContext, viewContext)
		if viewBrief {
			window = briefKeeping(window, viewAt)
		}
		fullSession = window
	} else if viewBrief {
		fullSession = render.BriefSession(fullSession)
	}

	color, err := resolveColorMode(viewColor)
	if err != nil {
		return err
	}

	opts := render.TerminalOptions{
		ShowThinking: viewShowThinking,
		ShowAgents:   viewShowAgents,
		FlatMode:     viewFlat,
		Color:        color,
		Theme:        config.Theme(),
	}

	return render.Terminal(fullSession, opts)
}

// briefKeeping applies the brief filter to a window but never drops
// the cited target itself: the point of --at is to see that message.
func briefKeeping(window *parser.Session, target string) *parser.Session {
	brief := render.BriefSession(window)
	for _, m := range brief.RootMessages {
		if strings.HasPrefix(m.UUID, target) {
			return brief
		}
	}
	// Re-insert the target at its wire position among the kept ones.
	var out []*parser.Message
	inserted := false
	for _, m := range window.RootMessages {
		if strings.HasPrefix(m.UUID, target) {
			out = append(out, m)
			inserted = true
			continue
		}
		for _, k := range brief.RootMessages {
			if k.UUID == m.UUID {
				out = append(out, k)
				break
			}
		}
	}
	if !inserted {
		return brief
	}
	brief.RootMessages = out
	return brief
}

func sessionLookupQuery(projectName string, all bool) (catalog.SessionQuery, error) {
	if projectName != "" {
		return catalog.SessionQuery{
			Scope:       catalog.ScopeProject,
			ProjectName: projectName,
		}, nil
	}
	if all {
		return allSessionsQuery(), nil
	}
	return currentWorkspaceQuery()
}

func parseSessionArg(arg string) (project, session string) {
	if strings.Contains(arg, ":") {
		parts := strings.SplitN(arg, ":", 2)
		return parts[0], parts[1]
	}
	return "", arg
}

func selectSession(backend provider.Backend, all bool) (*parser.Session, error) {
	query := catalog.SessionQuery{
		Sort:  catalog.SortTime,
		Limit: 10,
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

	allSessions, err := backend.ListSessions(query)
	if err != nil {
		return nil, err
	}

	if len(allSessions) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}

	fmt.Println("Recent sessions:")
	limit := 10
	if len(allSessions) < limit {
		limit = len(allSessions)
	}

	for i, s := range allSessions[:limit] {
		summary := sessionSummaryPreview(s.Summary, 64)
		tag := providerTag(s.Provider)
		project := truncateDisplay(cleanDisplayText(s.ProjectName), 24)
		fmt.Printf("  %d. %s [%s] %s\n", i+1, tag, project, summary)
	}

	fmt.Printf("\nSelect session (1-%d): ", limit)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return nil, fmt.Errorf("no input")
	}
	input := strings.TrimSpace(scanner.Text())
	choice, err := strconv.Atoi(input)
	if err != nil || choice < 1 || choice > limit {
		return nil, fmt.Errorf("invalid selection: %s", input)
	}

	return allSessions[choice-1], nil
}
