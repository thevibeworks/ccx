package cmd

import (
	"bufio"
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

With --content, also scan transcript lines inside session files
(including subagent files) — grep parity, but with session identity,
provider abstraction, and date filters.

Examples:
  ccx search auth            # Find sessions about authentication
  ccx search myproject       # Find project by name
  ccx search "fix bug"       # Multi-word search
  ccx search -t session      # Only search sessions
  ccx search --content goose # Scan message content (slower)`,
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
	searchContent  bool
)

func init() {
	searchCmd.Flags().StringVarP(&searchType, "type", "t", "", "filter by type: project, session")
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 20, "max results")
	searchCmd.Flags().BoolVar(&searchJSON, "json", false, "output as JSON")
	searchCmd.Flags().StringVarP(&searchProvider, "provider", "p", "", "filter by provider: cc, cx, all")
	searchCmd.Flags().StringVar(&searchAfter, "after", "", "sessions after date (YYYY-MM-DD)")
	searchCmd.Flags().StringVar(&searchBefore, "before", "", "sessions before date (YYYY-MM-DD)")
	searchCmd.Flags().StringVar(&searchModel, "model", "", "filter by model name substring")
	searchCmd.Flags().BoolVar(&searchContent, "content", false, "also scan message content in session files (slower)")

	rootCmd.AddCommand(searchCmd)
}

type searchResult struct {
	Type     string `json:"type"`
	Project  string `json:"project"`
	Session  string `json:"session,omitempty"`
	Summary  string `json:"summary"`
	Time     string `json:"time,omitempty"`
	Matches  int    `json:"matches,omitempty"`
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
		// Backends set Name to the human-readable form; the encoding
		// heuristics are claude-code specific and mangle grok's
		// url-encoded dirs.
		projDisplay := p.Name
		if projDisplay == "" {
			projDisplay = parser.GetProjectDisplayName(p.EncodedName)
		}
		projPath := p.Path
		if projPath == "" {
			projPath = parser.DecodePath(p.EncodedName)
		}

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
				continue
			}

			// Content scan: raw transcript lines, main file plus subagent
			// files. Grep parity by design — no parse, so it works for
			// every provider's format and misses nothing grep would find.
			if searchContent {
				if n := contentMatches(s.FilePath, query); n > 0 {
					results = append(results, searchResult{
						Type:     "content",
						Project:  projDisplay,
						Session:  truncateID(s.ID, 8),
						Summary:  fmt.Sprintf("%d hits · %s", n, sessionSummaryPreview(s.Summary, 48)),
						Time:     formatAge(s.StartTime),
						Matches:  n,
						Priority: 3,
					})
				}
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

	// Sort by priority, then by match count within content results
	sort.Slice(results, func(i, j int) bool {
		if results[i].Priority != results[j].Priority {
			return results[i].Priority < results[j].Priority
		}
		return results[i].Matches > results[j].Matches
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

func truncateID(id string, max int) string {
	if len(id) <= max {
		return id
	}
	return id[:max]
}

// contentMatches counts transcript lines containing query across the
// main session file and any subagent files beside it (Claude Code
// layout: <id>/subagents/agent-*.jsonl next to <id>.jsonl; other
// providers simply have no such directory).
func contentMatches(sessionPath, query string) int {
	if sessionPath == "" {
		return 0
	}
	count := countMatchingLines(sessionPath, query)
	sessionID := strings.TrimSuffix(filepath.Base(sessionPath), filepath.Ext(sessionPath))
	subDir := filepath.Join(filepath.Dir(sessionPath), sessionID, "subagents")
	entries, err := os.ReadDir(subDir)
	if err != nil {
		return count
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		count += countMatchingLines(filepath.Join(subDir, entry.Name()), query)
	}
	return count
}

// countMatchingLines streams one JSONL file and counts lines matching
// query case-insensitively. A line past the 10MB scanner budget stops
// the scan; the count so far still stands.
func countMatchingLines(path, query string) int {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		if strings.Contains(strings.ToLower(scanner.Text()), query) {
			count++
		}
	}
	return count
}
