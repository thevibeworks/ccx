package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/provider"
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
	searchType     string
	searchLimit    int
	searchJSON     bool
	searchProvider string
	searchAfter    string
	searchBefore   string
	searchModel    string
)

func init() {
	searchCmd.Flags().StringVarP(&searchType, "type", "t", "", "filter by type: project, session")
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 20, "max results")
	searchCmd.Flags().BoolVar(&searchJSON, "json", false, "output as JSON")
	searchCmd.Flags().StringVarP(&searchProvider, "provider", "p", "", "filter by provider: cc, cx, all")
	searchCmd.Flags().StringVar(&searchAfter, "after", "", "sessions after date (YYYY-MM-DD)")
	searchCmd.Flags().StringVar(&searchBefore, "before", "", "sessions before date (YYYY-MM-DD)")
	searchCmd.Flags().StringVar(&searchModel, "model", "", "filter by model name substring")

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
	backend := provider.Default()

	after, err := config.ParseDate(searchAfter)
	if err != nil {
		return fmt.Errorf("invalid --after date: %w", err)
	}
	before, err := config.ParseBeforeDate(searchBefore)
	if err != nil {
		return fmt.Errorf("invalid --before date: %w", err)
	}
	filter := config.SessionFilter{
		Provider: config.NormalizeProvider(searchProvider),
		After:    after,
		Before:   before,
		Model:    searchModel,
	}

	projects, err := backend.DiscoverProjects()
	if err != nil {
		return fmt.Errorf("failed to discover projects: %w", err)
	}

	var results []searchResult

	for _, p := range projects {
		projDisplay := parser.GetProjectDisplayName(p.EncodedName)
		projPath := parser.DecodePath(p.EncodedName)

		// Project name match (skip if filtering to sessions only)
		if searchType != "session" {
			nameMatch := strings.Contains(strings.ToLower(p.EncodedName), query) ||
				strings.Contains(strings.ToLower(projPath), query) ||
				strings.Contains(strings.ToLower(projDisplay), query)

			providerMatch := filter.Provider == ""
			if !providerMatch {
				for _, s := range p.Sessions {
					if s.Provider == filter.Provider {
						providerMatch = true
						break
					}
				}
			}

			if nameMatch && providerMatch {
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
			if !filter.IsEmpty() && !filter.Match(s) {
				continue
			}

			// Session ID match (high priority)
			if strings.HasPrefix(strings.ToLower(s.ID), query) {
				results = append(results, searchResult{
					Type:     "session",
					Project:  projDisplay,
					Session:  truncateID(s.ID, 8),
					Summary:  sessionSummaryPreview(s.Summary, 64),
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
					Summary:  sessionSummaryPreview(s.Summary, 64),
					Time:     formatAge(s.StartTime),
					Priority: 2,
				})
			}
		}
	}

	// Search memory files
	if searchType != "project" && searchType != "session" {
		settings := config.Load()
		for _, home := range []string{settings.ClaudeHome, settings.CodexHome} {
			searchMemoryDir(home, "projects", query, filter, &results)
			// Global files
			for _, name := range []string{"CLAUDE.md", "instructions.md", "AGENTS.md"} {
				path := filepath.Join(home, name)
				if _, err := os.Stat(path); err != nil {
					continue
				}
				if strings.Contains(strings.ToLower(name), query) {
					results = append(results, searchResult{
						Type:     "memory",
						Project:  filepath.Base(home),
						Summary:  name,
						Time:     "-",
						Priority: 1,
					})
				}
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
			r.Type, truncateDisplay(cleanDisplayText(r.Project), 24), session, cleanDisplayText(r.Summary), time)
	}

	return w.Flush()
}

func searchMemoryDir(home, subdir, query string, filter config.SessionFilter, results *[]searchResult) {
	projectsDir := filepath.Join(home, subdir)
	projEntries, err := os.ReadDir(projectsDir)
	if err != nil {
		return
	}
	for _, projEntry := range projEntries {
		if !projEntry.IsDir() {
			continue
		}
		memDir := filepath.Join(projectsDir, projEntry.Name(), "memory")
		entries, err := os.ReadDir(memDir)
		if err != nil {
			continue
		}
		projDisplay := parser.GetProjectDisplayName(projEntry.Name())
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			if strings.Contains(strings.ToLower(entry.Name()), query) {
				*results = append(*results, searchResult{
					Type:     "memory",
					Project:  projDisplay,
					Summary:  entry.Name(),
					Time:     "-",
					Priority: 1,
				})
			}
		}
	}
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
