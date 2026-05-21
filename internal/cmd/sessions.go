package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/thevibeworks/ccx/internal/catalog"
	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/provider"
)

var sessionsCmd = &cobra.Command{
	Use:     "sessions [project]",
	Aliases: []string{"session", "sess", "s"},
	Short:   "List sessions",
	Long: `List sessions from the selected provider.

If PROJECT is specified, show sessions for that project only.
Otherwise, show recent sessions for the current workspace.
Use --all to show recent sessions across all projects.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSessions,
}

var (
	sessionsSort     string
	sessionsLimit    int
	sessionsJSON     bool
	sessionsProvider string
	sessionsSearch   string
	sessionsAfter    string
	sessionsBefore   string
	sessionsModel    string
	sessionsAll      bool
)

func init() {
	sessionsCmd.Flags().StringVar(&sessionsSort, "sort", "time", "sort by: time, messages")
	sessionsCmd.Flags().IntVar(&sessionsLimit, "limit", 20, "limit number of sessions (0 = no limit)")
	sessionsCmd.Flags().BoolVar(&sessionsJSON, "json", false, "output as JSON")
	sessionsCmd.Flags().StringVarP(&sessionsProvider, "provider", "p", "", "filter by provider: cc, cx, all")
	sessionsCmd.Flags().StringVarP(&sessionsSearch, "search", "s", "", "search in session summaries")
	sessionsCmd.Flags().StringVar(&sessionsAfter, "after", "", "sessions after date (YYYY-MM-DD)")
	sessionsCmd.Flags().StringVar(&sessionsBefore, "before", "", "sessions before date (YYYY-MM-DD)")
	sessionsCmd.Flags().StringVar(&sessionsModel, "model", "", "filter by model name substring")
	sessionsCmd.Flags().BoolVar(&sessionsAll, "all", false, "list sessions across all projects")
}

func runSessions(cmd *cobra.Command, args []string) error {
	backend := provider.Default()

	after, err := config.ParseDate(sessionsAfter)
	if err != nil {
		return fmt.Errorf("invalid --after date: %w", err)
	}
	before, err := config.ParseBeforeDate(sessionsBefore)
	if err != nil {
		return fmt.Errorf("invalid --before date: %w", err)
	}
	filter := config.SessionFilter{
		Provider: config.NormalizeProvider(sessionsProvider),
		After:    after,
		Before:   before,
		Query:    sessionsSearch,
		Model:    sessionsModel,
	}

	query := catalog.SessionQuery{
		Filter: filter,
		Sort:   catalog.SessionSort(sessionsSort),
		Limit:  sessionsLimit,
	}
	if err := catalog.ValidateSessionSort(query.Sort); err != nil {
		return err
	}
	var projectName string
	if len(args) > 0 {
		projectName = args[0]
		query.Scope = catalog.ScopeProject
		query.ProjectName = projectName
	} else if sessionsAll {
		query.Scope = catalog.ScopeAll
	} else {
		current, err := currentWorkspaceQuery()
		if err != nil {
			return err
		}
		query.Scope = current.Scope
		query.WorkspacePath = current.WorkspacePath
	}

	sessions, err := backend.ListSessions(query)
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}
	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		return nil
	}

	if sessionsJSON {
		return printSessionsJSON(sessions)
	}

	return printSessionsTable(sessions, projectName == "" && sessionsAll)
}

func providerTag(p string) string {
	switch p {
	case "claude-code":
		return "[CC]"
	case "codex":
		return "[CX]"
	default:
		return ""
	}
}

func printSessionsTable(sessions []*parser.Session, showProject bool) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	if showProject {
		fmt.Fprintln(w, "SRC\tPROJECT\tSESSION\tSTARTED\tSUMMARY")
	} else {
		fmt.Fprintln(w, "SRC\tSESSION\tSTARTED\tSUMMARY")
	}

	for _, s := range sessions {
		id := s.ID
		if len(id) > 8 {
			id = id[:8]
		}

		summary := sessionSummaryPreview(s.Summary, 64)

		age := formatAge(s.StartTime)
		tag := providerTag(s.Provider)

		if showProject {
			proj := truncateDisplay(cleanDisplayText(s.ProjectName), 24)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", tag, proj, id, age, summary)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", tag, id, age, summary)
		}
	}

	return w.Flush()
}

type sessionJSON struct {
	ID        string `json:"id"`
	Provider  string `json:"provider"`
	Project   string `json:"project"`
	Summary   string `json:"summary"`
	StartTime string `json:"start_time"`
}

func printSessionsJSON(sessions []*parser.Session) error {
	items := make([]sessionJSON, len(sessions))
	for i, s := range sessions {
		items[i] = sessionJSON{
			ID:        s.ID,
			Provider:  s.Provider,
			Project:   s.ProjectName,
			Summary:   s.Summary,
			StartTime: s.StartTime.Format(time.RFC3339),
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}
