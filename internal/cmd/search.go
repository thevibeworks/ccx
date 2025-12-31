package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/claude-code/ccx/internal/config"
	"github.com/claude-code/ccx/internal/parser"
)

var searchCmd = &cobra.Command{
	Use:   "search QUERY",
	Short: "Search across projects and sessions",
	Long: `Search for projects and sessions by name or summary.

Examples:
  ccx search auth          # Find sessions about authentication
  ccx search myproject     # Find project by name
  ccx search "fix bug"     # Multi-word search
  ccx search -t session    # Only search sessions`,
	Args: cobra.MinimumNArgs(1),
	RunE: runSearch,
}

var (
	searchType  string
	searchLimit int
	searchJSON  bool
)

func init() {
	searchCmd.Flags().StringVarP(&searchType, "type", "t", "", "filter by type: project, session")
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 20, "max results")
	searchCmd.Flags().BoolVar(&searchJSON, "json", false, "output as JSON")

	rootCmd.AddCommand(searchCmd)
}

type searchResult struct {
	Type     string `json:"type"`
	Project  string `json:"project"`
	Session  string `json:"session,omitempty"`
	Summary  string `json:"summary"`
	Time     string `json:"time,omitempty"`
	Priority int    `json:"-"`
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := strings.ToLower(strings.Join(args, " "))
	projectsDir := config.ProjectsDir()

	projects, err := parser.DiscoverProjects(projectsDir)
	if err != nil {
		return fmt.Errorf("failed to discover projects: %w", err)
	}

	var results []searchResult

	for _, p := range projects {
		projDisplay := parser.GetProjectDisplayName(p.EncodedName)
		projPath := parser.DecodePath(p.EncodedName)

		// Project name match (skip if filtering to sessions only)
		if searchType != "session" {
			if strings.Contains(strings.ToLower(p.EncodedName), query) ||
				strings.Contains(strings.ToLower(projPath), query) ||
				strings.Contains(strings.ToLower(projDisplay), query) {
				results = append(results, searchResult{
					Type:     "project",
					Project:  projDisplay,
					Summary:  projPath,
					Priority: 1,
				})
			}
		}

		// Session search (skip if filtering to projects only)
		if searchType == "project" {
			continue
		}

		for _, s := range p.Sessions {
			// Session ID match (high priority)
			if strings.HasPrefix(strings.ToLower(s.ID), query) {
				results = append(results, searchResult{
					Type:     "session",
					Project:  projDisplay,
					Session:  truncateID(s.ID, 8),
					Summary:  truncate(s.Summary, 60),
					Time:     formatAge(s.StartTime),
					Priority: 0,
				})
				continue
			}

			// Summary match
			if strings.Contains(strings.ToLower(s.Summary), query) {
				results = append(results, searchResult{
					Type:     "session",
					Project:  projDisplay,
					Session:  truncateID(s.ID, 8),
					Summary:  truncate(s.Summary, 60),
					Time:     formatAge(s.StartTime),
					Priority: 2,
				})
			}
		}
	}

	// Sort by priority
	sort.Slice(results, func(i, j int) bool {
		return results[i].Priority < results[j].Priority
	})

	// Limit results
	if searchLimit > 0 && len(results) > searchLimit {
		results = results[:searchLimit]
	}

	if len(results) == 0 {
		fmt.Println("No results found.")
		return nil
	}

	if searchJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	return printSearchResults(results)
}

func printSearchResults(results []searchResult) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TYPE\tPROJECT\tSESSION\tSUMMARY\tTIME")

	for _, r := range results {
		session := r.Session
		if session == "" {
			session = "-"
		}
		time := r.Time
		if time == "" {
			time = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			r.Type, truncate(r.Project, 20), session, r.Summary, time)
	}

	return w.Flush()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func truncateID(id string, max int) string {
	if len(id) <= max {
		return id
	}
	return id[:max]
}
