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

var projectsCmd = &cobra.Command{
	Use:     "projects",
	Aliases: []string{"project", "proj", "p"},
	Short:   "List all projects",
	Long:    `List all Claude Code projects found in CLAUDE_CODE_HOME.`,
	RunE:    runProjects,
}

var (
	projectsSort  string
	projectsLimit int
	projectsJSON  bool
)

func init() {
	projectsCmd.Flags().StringVar(&projectsSort, "sort", "time", "sort by: name, time, sessions")
	projectsCmd.Flags().IntVar(&projectsLimit, "limit", 0, "limit number of projects (0 = no limit)")
	projectsCmd.Flags().BoolVar(&projectsJSON, "json", false, "output as JSON")
}

func runProjects(cmd *cobra.Command, args []string) error {
	projectsDir := config.ProjectsDir()

	projects, err := parser.DiscoverProjects(projectsDir)
	if err != nil {
		return fmt.Errorf("failed to discover projects: %w", err)
	}

	if len(projects) == 0 {
		fmt.Println("No projects found.")
		fmt.Printf("Looked in: %s\n", projectsDir)
		return nil
	}

	if projectsLimit > 0 && len(projects) > projectsLimit {
		projects = projects[:projectsLimit]
	}

	if projectsJSON {
		return printProjectsJSON(projects)
	}

	return printProjectsTable(projects)
}

func printProjectsTable(projects []*parser.Project) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROJECT\tSESSIONS\tLAST MODIFIED")

	for _, p := range projects {
		age := formatAge(p.LastModified)
		fmt.Fprintf(w, "%s\t%d\t%s\n", p.Name, len(p.Sessions), age)
	}

	return w.Flush()
}

type projectJSON struct {
	Name         string `json:"name"`
	EncodedName  string `json:"encoded_name"`
	Sessions     int    `json:"sessions"`
	LastModified string `json:"last_modified"`
}

func printProjectsJSON(projects []*parser.Project) error {
	items := make([]projectJSON, len(projects))
	for i, p := range projects {
		items[i] = projectJSON{
			Name:         p.Name,
			EncodedName:  p.EncodedName,
			Sessions:     len(p.Sessions),
			LastModified: p.LastModified.Format(time.RFC3339),
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

func formatAge(t time.Time) string {
	if t.IsZero() {
		return "N/A"
	}

	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	default:
		return t.Format("2006-01-02")
	}
}
