package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/provider"
)

var searchCmd = &cobra.Command{
	Use:   "search QUERY",
	Short: "Search across projects and sessions",
	Long: `Search for projects and sessions by name or summary.

With --content, also scan conversation text inside session files
(including subagent files): user prompts and assistant replies,
ranked by hit count with a matched-text preview. Injected noise —
tool results, hook attachments, command echoes — doesn't count.

Add --raw to match every raw transcript line instead: grep parity,
no parse, misses nothing grep would find.

Examples:
  ccx search auth              # Find sessions about authentication
  ccx search myproject         # Find project by name
  ccx search "fix bug"         # Multi-word search
  ccx search -t session        # Only search sessions
  ccx search --content goose   # Scan conversation text (slower)
  ccx search --raw goose       # Grep parity over raw lines`,
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
	searchRaw      bool
)

func init() {
	searchCmd.Flags().StringVarP(&searchType, "type", "t", "", "filter by type: project, session")
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 20, "max results")
	searchCmd.Flags().BoolVar(&searchJSON, "json", false, "output as JSON")
	searchCmd.Flags().StringVarP(&searchProvider, "provider", "p", "", "filter by provider: cc, cx, all")
	searchCmd.Flags().StringVar(&searchAfter, "after", "", "sessions after date (YYYY-MM-DD)")
	searchCmd.Flags().StringVar(&searchBefore, "before", "", "sessions before date (YYYY-MM-DD)")
	searchCmd.Flags().StringVar(&searchModel, "model", "", "filter by model name substring")
	searchCmd.Flags().BoolVar(&searchContent, "content", false, "also scan conversation text in session files (slower)")
	searchCmd.Flags().BoolVar(&searchRaw, "raw", false, "content scan matches every raw transcript line (grep parity; implies --content)")

	rootCmd.AddCommand(searchCmd)
}

type searchResult struct {
	Type     string           `json:"type"`
	Project  string           `json:"project"`
	Session  string           `json:"session,omitempty"`
	Path     string           `json:"path,omitempty"`
	Summary  string           `json:"summary"`
	Time     string           `json:"time,omitempty"`
	Matches  int              `json:"matches,omitempty"`
	Previews []contentPreview `json:"previews,omitempty"`
	Priority int              `json:"-"`
}

// contentPreview is one matched conversation snippet, role-labeled so
// noise is distinguishable from signal without leaving ccx.
type contentPreview struct {
	Role string `json:"role"` // "user" | "assistant" | "agent"
	Text string `json:"text"`
}

const maxContentPreviews = 3

func runSearch(cmd *cobra.Command, args []string) error {
	query := strings.ToLower(strings.Join(args, " "))
	backend := provider.Default()

	if searchRaw {
		searchContent = true
	}

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
					Path:     s.FilePath,
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
					Path:     s.FilePath,
					Summary:  sessionSummaryPreview(s.Summary, 64),
					Time:     formatAge(s.StartTime),
					Priority: 2,
				})
				continue
			}

			// Content scan. The default counts conversation text only —
			// user prompts and assistant replies — so ranking follows
			// discussion, not injected boilerplate (a hook line fired
			// every turn once outranked the real answer 327 hits to 13;
			// docs/devlog/2026-08-03-content-search-noise.org). --raw
			// keeps grep parity over raw transcript lines, main file
			// plus subagent files: no parse, works for every provider's
			// format, misses nothing grep would find.
			if searchContent {
				if searchRaw {
					if n := countContentMatches(s.FilePath, query); n > 0 {
						results = append(results, searchResult{
							Type:     "content",
							Project:  projDisplay,
							Session:  truncateID(s.ID, 8),
							Path:     s.FilePath,
							Summary:  fmt.Sprintf("%d hits · %s", n, sessionSummaryPreview(s.Summary, 48)),
							Time:     formatAge(s.StartTime),
							Matches:  n,
							Priority: 3,
						})
					}
					continue
				}

				// Cheap line-scan prefilter before the full parse; only
				// trustworthy when JSON escaping can't hide the query.
				if rawPrefilterSafe(query) && countContentMatches(s.FilePath, query) == 0 {
					continue
				}
				n, previews := scanConversationText(backend, s.FilePath, query)
				if n == 0 {
					continue
				}
				summary := fmt.Sprintf("%d hits · %s", n, sessionSummaryPreview(s.Summary, 48))
				if len(previews) > 0 {
					summary = fmt.Sprintf("%d hits · [%s] %s", n, previews[0].Role, truncateDisplay(previews[0].Text, 56))
				}
				results = append(results, searchResult{
					Type:     "content",
					Project:  projDisplay,
					Session:  truncateID(s.ID, 8),
					Path:     s.FilePath,
					Summary:  summary,
					Time:     formatAge(s.StartTime),
					Matches:  n,
					Previews: previews,
					Priority: 3,
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

	// Sort by priority, then by match count within content results.
	// Stable so equal-rank results keep discovery order across runs.
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Priority != results[j].Priority {
			return results[i].Priority < results[j].Priority
		}
		return results[i].Matches > results[j].Matches
	})

	// Limit results — never silently.
	if searchLimit > 0 && len(results) > searchLimit {
		fmt.Fprintf(os.Stderr, "showing %d of %d results (raise with -n)\n", searchLimit, len(results))
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

// sessionParser is the slice of provider.Backend the conversation
// scan needs; narrowed so tests can stub it.
type sessionParser interface {
	ParseSession(filePath string) (*parser.Session, error)
}

// scanConversationText parses one session (the parser loads sidechain
// files too) and searches only conversation text: text and thinking
// blocks of user prompts and assistant messages. Tool results, hook
// attachments, command echoes, and meta lines never count — that's
// what --raw is for. Returns total occurrences plus up to
// maxContentPreviews role-labeled snippets around the earliest
// matches. query must already be lowercase.
func scanConversationText(p sessionParser, path, query string) (int, []contentPreview) {
	sess, err := p.ParseSession(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: skipping unparseable %s: %v\n", filepath.Base(path), err)
		return 0, nil
	}

	count := 0
	var previews []contentPreview
	var walk func(msgs []*parser.Message)
	walk = func(msgs []*parser.Message) {
		for _, m := range msgs {
			if m.Kind == parser.KindUserPrompt || m.Kind == parser.KindAssistant {
				role := m.Type
				if m.IsSidechain {
					role = "agent"
				}
				for _, b := range m.Content {
					if b.Type != "text" && b.Type != "thinking" {
						continue
					}
					lower := strings.ToLower(b.Text)
					n := strings.Count(lower, query)
					if n == 0 {
						continue
					}
					count += n
					if len(previews) < maxContentPreviews {
						previews = append(previews, contentPreview{
							Role: role,
							Text: matchSnippet(b.Text, strings.Index(lower, query), len(query)),
						})
					}
				}
			}
			walk(m.Children)
		}
	}
	walk(sess.RootMessages)
	return count, previews
}

// matchSnippet cuts a display window around a match, clamped to rune
// boundaries. idx indexes the lowered copy of text; byte positions can
// drift on the rare rune whose lowercase form changes width, so bounds
// are clamped rather than trusted.
func matchSnippet(text string, idx, qlen int) string {
	if idx < 0 {
		idx = 0
	}
	if idx > len(text) {
		idx = len(text)
	}
	start := idx - 32
	if start < 0 {
		start = 0
	}
	end := idx + qlen + 56
	if end > len(text) {
		end = len(text)
	}
	for start > 0 && !utf8.RuneStart(text[start]) {
		start--
	}
	for end < len(text) && !utf8.RuneStart(text[end]) {
		end++
	}
	out := cleanDisplayText(text[start:end])
	if start > 0 {
		out = "..." + out
	}
	if end < len(text) {
		out += "..."
	}
	return out
}

// rawPrefilterSafe reports whether a zero-hit raw line scan proves a
// zero-hit conversation scan. JSON writers escape `"`, `\`, control
// chars, and sometimes non-ASCII, so those queries must skip the
// cheap prefilter and parse every candidate session instead.
func rawPrefilterSafe(query string) bool {
	for i := 0; i < len(query); i++ {
		if query[i] < 0x20 || query[i] > 0x7e || query[i] == '"' || query[i] == '\\' {
			return false
		}
	}
	return true
}

// countContentMatches counts transcript lines containing query across
// the main session file and any subagent files beside it (layout
// knowledge lives in parser.SubagentFiles; providers without subagent
// files simply contribute none).
func countContentMatches(sessionPath, query string) int {
	if sessionPath == "" {
		return 0
	}
	count := countMatchingLines(sessionPath, query)
	for _, f := range parser.SubagentFiles(sessionPath) {
		count += countMatchingLines(f, query)
	}
	return count
}

// countMatchingLines streams one JSONL file and counts lines matching
// query case-insensitively. bufio.Reader, not Scanner: transcript
// lines carrying embedded images exceed any fixed budget, and a
// silent early stop is exactly the false-negative class --content
// exists to kill. Unreadable files warn instead of lying "0 hits".
func countMatchingLines(path, query string) int {
	file, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: skipping unreadable %s: %v\n", filepath.Base(path), err)
		return 0
	}
	defer file.Close()

	count := 0
	reader := bufio.NewReaderSize(file, 64*1024)
	for {
		line, err := reader.ReadString('\n')
		if line != "" && strings.Contains(strings.ToLower(line), query) {
			count++
		}
		if err != nil {
			if err != io.EOF {
				fmt.Fprintf(os.Stderr, "warning: read error in %s: %v\n", filepath.Base(path), err)
			}
			return count
		}
	}
}
