package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/claude-code/ccx/internal/config"
	"github.com/claude-code/ccx/internal/parser"
)

var sessionsCmd = &cobra.Command{
	Use:     "sessions [project]",
	Aliases: []string{"session", "sess", "s"},
	Short:   "List sessions",
	Long: `List Claude Code sessions.

If PROJECT is specified, show sessions for that project only.
Otherwise, show recent sessions across all projects.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSessions,
}

var (
	sessionsSort  string
	sessionsLimit int
	sessionsAll   bool
	sessionsJSON  bool
)

func init() {
	sessionsCmd.Flags().StringVar(&sessionsSort, "sort", "time", "sort by: time, messages")
	sessionsCmd.Flags().IntVar(&sessionsLimit, "limit", 20, "limit number of sessions (0 = no limit)")
	sessionsCmd.Flags().BoolVar(&sessionsAll, "all", false, "include agent sidechains")
	sessionsCmd.Flags().BoolVar(&sessionsJSON, "json", false, "output as JSON")
}

func runSessions(cmd *cobra.Command, args []string) error {
	projectsDir := config.ProjectsDir()

	var sessions []*parser.Session
	var projectName string

	if len(args) > 0 {
		projectName = args[0]
		project, err := parser.FindProject(projectsDir, projectName)
		if err != nil {
			return fmt.Errorf("failed to find project: %w", err)
		}
		if project == nil {
			return fmt.Errorf("project not found: %s", projectName)
		}
		sessions = project.Sessions
	} else {
		projects, err := parser.DiscoverProjects(projectsDir)
		if err != nil {
			return fmt.Errorf("failed to discover projects: %w", err)
		}
		for _, p := range projects {
			for _, s := range p.Sessions {
				s.ProjectName = p.Name
				sessions = append(sessions, s)
			}
		}
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		return nil
	}

	if sessionsLimit > 0 && len(sessions) > sessionsLimit {
		sessions = sessions[:sessionsLimit]
	}

	if sessionsJSON {
		return printSessionsJSON(sessions)
	}

	return printSessionsTable(sessions, projectName == "")
}

func printSessionsTable(sessions []*parser.Session, showProject bool) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	if showProject {
		fmt.Fprintln(w, "PROJECT\tSESSION\tSTARTED\tSUMMARY")
	} else {
		fmt.Fprintln(w, "SESSION\tSTARTED\tSUMMARY")
	}

	for _, s := range sessions {
		id := s.ID
		if len(id) > 8 {
			id = id[:8]
		}

		summary := s.Summary
		if len(summary) > 50 {
			summary = summary[:47] + "..."
		}

		age := formatAge(s.StartTime)

		if showProject {
			proj := s.ProjectName
			if len(proj) > 20 {
				proj = proj[:17] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", proj, id, age, summary)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\n", id, age, summary)
		}
	}

	return w.Flush()
}

type sessionJSON struct {
	ID        string `json:"id"`
	Project   string `json:"project"`
	Summary   string `json:"summary"`
	StartTime string `json:"start_time"`
}

func printSessionsJSON(sessions []*parser.Session) error {
	items := make([]sessionJSON, len(sessions))
	for i, s := range sessions {
		items[i] = sessionJSON{
			ID:        s.ID,
			Project:   s.ProjectName,
			Summary:   s.Summary,
			StartTime: s.StartTime.Format(time.RFC3339),
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}
