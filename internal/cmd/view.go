package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/claude-code/ccx/internal/config"
	"github.com/claude-code/ccx/internal/parser"
	"github.com/claude-code/ccx/internal/render"
)

var viewCmd = &cobra.Command{
	Use:   "view [session]",
	Short: "View a session in terminal",
	Long: `View a Claude Code session in the terminal.

SESSION can be:
  - Full UUID: e38536a2-dbe6-442d-8b69-5bab525796ee
  - Short prefix: e38536
  - Index: @1 (most recent), @2 (second most recent)
  - With project: myproject:e38536

If SESSION is omitted, shows an interactive picker.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runView,
}

var (
	viewProject      string
	viewShowThinking bool
	viewShowAgents   bool
	viewFlat         bool
)

func init() {
	viewCmd.Flags().StringVarP(&viewProject, "project", "p", "", "project name")
	viewCmd.Flags().BoolVar(&viewShowThinking, "show-thinking", false, "show thinking blocks expanded")
	viewCmd.Flags().BoolVar(&viewShowAgents, "show-agents", false, "show agent sidechains")
	viewCmd.Flags().BoolVar(&viewFlat, "flat", false, "disable tree rendering")
}

func runView(cmd *cobra.Command, args []string) error {
	projectsDir := config.ProjectsDir()

	var session *parser.Session
	var err error

	if len(args) == 0 {
		session, err = selectSession(projectsDir)
	} else {
		sessionArg := args[0]
		projectName, sessionID := parseSessionArg(sessionArg)
		if viewProject != "" {
			projectName = viewProject
		}
		session, err = parser.FindSession(projectsDir, projectName, sessionID)
	}

	if err != nil {
		return fmt.Errorf("failed to find session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("session not found")
	}

	fullSession, err := parser.ParseSession(session.FilePath)
	if err != nil {
		return fmt.Errorf("failed to parse session: %w", err)
	}

	opts := render.TerminalOptions{
		ShowThinking: viewShowThinking,
		ShowAgents:   viewShowAgents,
		FlatMode:     viewFlat,
		Theme:        config.Theme(),
	}

	return render.Terminal(fullSession, opts)
}

func parseSessionArg(arg string) (project, session string) {
	if strings.Contains(arg, ":") {
		parts := strings.SplitN(arg, ":", 2)
		return parts[0], parts[1]
	}
	return "", arg
}

func selectSession(projectsDir string) (*parser.Session, error) {
	projects, err := parser.DiscoverProjects(projectsDir)
	if err != nil {
		return nil, err
	}

	var allSessions []*parser.Session
	for _, p := range projects {
		for _, s := range p.Sessions {
			s.ProjectName = p.Name
			allSessions = append(allSessions, s)
		}
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
		summary := s.Summary
		if len(summary) > 50 {
			summary = summary[:47] + "..."
		}
		fmt.Printf("  %d. [%s] %s\n", i+1, s.ProjectName, summary)
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
