package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"
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

The query is one case-insensitive phrase: multiple words must appear
adjacent and in order ("fix bug" won't match "bug ... fix"). For
term-level matching, run one search per term. Matching is substring
by default; -w/--word requires whole words, so "semantica" no longer
matches "semantically". Exits 0 either way; zero matches just prints
"No results found."

With --content, also scan conversation text inside session files
(including subagent files): user prompts and assistant replies,
ranked by hit count with a matched-text preview and the time of the
earliest match (FIRST). Injected noise — tool results, hook
attachments, command echoes — doesn't count. Prompt history
(~/.claude/history.jsonl, ~/.codex/history.jsonl) is scanned too and
surfaces as type "prompt" for prompts whose session file is gone —
the longest-lived evidence, past session cleanup.

Add --raw to match every raw transcript line instead: grep parity,
no parse, misses nothing grep would find.

--sort orders results: hits (default: match kind, then hit count),
first (earliest match first — "when did we first mention X"), last
(most recently active session first).

--hits lists every matching message instead of one row per session:
time, session, role, message id, and the quote around the match,
oldest first. Each row is a citation — the same anchors ccx trace
and view use — so a claim built on a search can point at its
evidence. -n caps the rows.

Examples:
  ccx search auth              # Find sessions about authentication
  ccx search myproject         # Find project by name
  ccx search "fix bug"         # Phrase match: adjacent words, in order
  ccx search -t session        # Only search sessions
  ccx search --content goose   # Scan conversation text (slower)
  ccx search --raw goose       # Grep parity over raw lines
  ccx search --content -w --sort first semantica
                               # Whole-word, earliest mention first
  ccx search --content -w --hits semantica
                               # Every mention, quoted and anchored`,
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
	searchWord     bool
	searchSort     string
	searchHits     bool
)

func init() {
	searchCmd.Flags().StringVarP(&searchType, "type", "t", "", "filter by type: project, session")
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 20, "max results")
	searchCmd.Flags().BoolVar(&searchJSON, "json", false, "output as JSON")
	searchCmd.Flags().StringVarP(&searchProvider, "provider", "p", "", "filter by provider: cc, cx, gx, all")
	searchCmd.Flags().StringVar(&searchAfter, "after", "", "sessions after date (YYYY-MM-DD)")
	searchCmd.Flags().StringVar(&searchBefore, "before", "", "sessions before date (YYYY-MM-DD)")
	searchCmd.Flags().StringVar(&searchModel, "model", "", "filter by model name substring")
	searchCmd.Flags().BoolVar(&searchContent, "content", false, "also scan conversation text in session files (slower)")
	searchCmd.Flags().BoolVar(&searchRaw, "raw", false, "content scan matches every raw transcript line (grep parity; implies --content)")
	searchCmd.Flags().BoolVarP(&searchWord, "word", "w", false, "match whole words only (\"semantica\" won't match \"semantically\")")
	searchCmd.Flags().StringVar(&searchSort, "sort", "hits", "order results: hits, first, last")
	searchCmd.Flags().BoolVar(&searchHits, "hits", false, "list every matching message with time, role, message id, and quote (implies --content)")

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
	FirstHit string           `json:"first_hit,omitempty"` // RFC3339 time of the earliest match
	Previews []contentPreview `json:"previews,omitempty"`
	Priority int              `json:"-"`

	firstHit time.Time
	lastTime time.Time
	hits     []searchHit
}

// contentPreview is one matched conversation snippet, role-labeled so
// noise is distinguishable from signal without leaving ccx.
type contentPreview struct {
	Role string `json:"role"` // "user" | "assistant" | "agent"
	Text string `json:"text"`
}

// searchHit is one matching message: a quote plus the anchors needed
// to walk back to it (session file, message id, time). Under --raw the
// unit is a transcript line and Role is the line's top-level type.
type searchHit struct {
	Project   string    `json:"project"`
	Session   string    `json:"session"`
	Path      string    `json:"path"`
	MessageID string    `json:"message_id,omitempty"`
	Time      time.Time `json:"time"`
	Role      string    `json:"role"`
	Matches   int       `json:"matches"` // occurrences within this message
	Quote     string    `json:"quote"`
}

const maxContentPreviews = 3

// textMatcher decides how QUERY matches text. Default: case-insensitive
// substring. Word mode adds ASCII word boundaries on each side of the
// query that starts/ends with a word character, so "semantica" no
// longer matches "semantically" while "中文" still matches inside a CJK
// run (no \w on either side, so no boundary is demanded there).
type textMatcher struct {
	query string         // lowercase literal
	re    *regexp.Regexp // nil in substring mode
}

func newTextMatcher(query string, word bool) textMatcher {
	m := textMatcher{query: strings.ToLower(query)}
	if !word || m.query == "" {
		return m
	}
	pat := regexp.QuoteMeta(m.query)
	if isWordByte(m.query[0]) {
		pat = `\b` + pat
	}
	if isWordByte(m.query[len(m.query)-1]) {
		pat += `\b`
	}
	m.re = regexp.MustCompile(`(?i)` + pat)
	return m
}

func isWordByte(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// literal is the substring matcher for the same query: a superset of
// word matches, so it is a valid cheap prefilter.
func (m textMatcher) literal() textMatcher { return textMatcher{query: m.query} }

// index returns the byte offset and length of the first match in
// text, or -1, 0.
func (m textMatcher) index(text string) (int, int) {
	if m.re == nil {
		return indexFold(text, m.query), len(m.query)
	}
	loc := m.re.FindStringIndex(text)
	if loc == nil {
		return -1, 0
	}
	return loc[0], loc[1] - loc[0]
}

func (m textMatcher) matches(text string) bool {
	i, _ := m.index(text)
	return i >= 0
}

// count returns the number of non-overlapping matches in text.
func (m textMatcher) count(text string) int {
	if m.re == nil {
		return countFold(text, m.query)
	}
	return len(m.re.FindAllStringIndex(text, -1))
}

// indexFold is a case-insensitive strings.Index for a lowercase query
// that does not allocate a lowered copy of s: the raw scan runs it
// over every transcript line, and lowering gigabytes was the scan's
// dominant cost. Non-ASCII queries fall back to the lowered copy
// (Unicode case folding is not byte-stable).
func indexFold(s, query string) int {
	if query == "" {
		return 0
	}
	if !isASCII(query) {
		return strings.Index(strings.ToLower(s), query)
	}
	n := len(query)
	first := query[0]
	for i := 0; i+n <= len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != first {
			continue
		}
		if strings.EqualFold(s[i:i+n], query) {
			return i
		}
	}
	return -1
}

func countFold(s, query string) int {
	if query == "" {
		return 0
	}
	if !isASCII(query) {
		return strings.Count(strings.ToLower(s), query)
	}
	n := 0
	for {
		i := indexFold(s, query)
		if i < 0 {
			return n
		}
		n++
		s = s[i+len(query):]
	}
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func runSearch(cmd *cobra.Command, args []string) error {
	m := newTextMatcher(strings.Join(args, " "), searchWord)
	query := m.query
	backend := provider.Default()

	if searchRaw || searchHits {
		searchContent = true
	}
	switch searchSort {
	case "hits", "first", "last":
	default:
		return fmt.Errorf("invalid --sort %q: want hits, first, or last", searchSort)
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
	var candidates []sessionCandidate

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
			nameMatch := m.matches(p.EncodedName) || m.matches(projPath) || m.matches(projDisplay)

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
			candidates = append(candidates, sessionCandidate{session: s, project: projDisplay})
		}
	}

	searcher := sessionSearcher{m: m, content: searchContent, raw: searchRaw, hits: searchHits, parser: backend}
	for _, r := range searcher.matchAll(candidates, searchWorkers(searchContent)) {
		results = append(results, *r)
	}

	// Prompt history: prompts whose session file is gone (cleanup)
	// still exist here; the only place "when did we first say X" can
	// be answered past the session horizon.
	if searchContent && searchType != "project" {
		settings := config.Load()
		known := knownSessionIDs(candidates)
		for _, h := range scanPromptHistory(promptHistoryFiles(settings.ClaudeHome, settings.CodexHome), m, known) {
			results = append(results, promptResult(h))
		}
	}

	if searchHits {
		return printSearchHits(collectHits(results))
	}

	// Search memory files
	if searchType != "project" && searchType != "session" {
		settings := config.Load()
		for _, home := range []string{settings.ClaudeHome, settings.CodexHome} {
			searchMemoryDir(home, "projects", m, filter, &results)
			// Global files
			for _, name := range []string{"CLAUDE.md", "instructions.md", "AGENTS.md"} {
				path := filepath.Join(home, name)
				if _, err := os.Stat(path); err != nil {
					continue
				}
				if m.matches(name) {
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

	sortSearchResults(results, searchSort)

	// Limit results — never silently.
	if searchLimit > 0 && len(results) > searchLimit {
		fmt.Fprintf(os.Stderr, "showing %d of %d results (raise with -n)\n", searchLimit, len(results))
		results = results[:searchLimit]
	}

	if len(results) == 0 {
		fmt.Println("No results found.")
		if strings.Contains(query, " ") {
			fmt.Fprintln(os.Stderr, "hint: multi-word queries match as one exact phrase; try a single term")
		}
		if !searchContent {
			fmt.Fprintln(os.Stderr, "hint: --content scans conversation text inside sessions")
		}
		return nil
	}

	if searchJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	return printSearchResults(results, searchContent)
}

// sortSearchResults orders results. "hits": match kind (Priority),
// then hit count — stable, so equal-rank results keep discovery order
// across runs. "first": earliest match first; results with no match
// time (projects, memory, name-only hits) trail in hits order.
// "last": most recently active session first, timeless trailing.
func sortSearchResults(results []searchResult, mode string) {
	byHits := func(a, b searchResult) bool {
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		return a.Matches > b.Matches
	}
	sort.SliceStable(results, func(i, j int) bool {
		a, b := results[i], results[j]
		switch mode {
		case "first":
			switch {
			case a.firstHit.IsZero() != b.firstHit.IsZero():
				return !a.firstHit.IsZero()
			case !a.firstHit.IsZero() && !a.firstHit.Equal(b.firstHit):
				return a.firstHit.Before(b.firstHit)
			}
		case "last":
			switch {
			case a.lastTime.IsZero() != b.lastTime.IsZero():
				return !a.lastTime.IsZero()
			case !a.lastTime.IsZero() && !a.lastTime.Equal(b.lastTime):
				return a.lastTime.After(b.lastTime)
			}
		}
		return byHits(a, b)
	})
}

// searchWorkers sizes the content-scan pool. The scan is I/O plus
// per-line matching; one goroutine per CPU (capped) keeps a cold
// store busy without thrashing it.
func searchWorkers(content bool) int {
	if !content {
		return 1
	}
	n := runtime.NumCPU()
	if n > 8 {
		n = 8
	}
	if n < 1 {
		n = 1
	}
	return n
}

func printSearchResults(results []searchResult, withFirst bool) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	// LAST = last activity (session end time): the same timestamp
	// --after/--before filter on and results sort by, so a filtered
	// row never displays a date outside the requested window. FIRST =
	// earliest matching message, shown when content was scanned.
	if withFirst {
		fmt.Fprintln(w, "TYPE\tPROJECT\tSESSION\tSUMMARY\tFIRST\tLAST")
	} else {
		fmt.Fprintln(w, "TYPE\tPROJECT\tSESSION\tSUMMARY\tLAST")
	}

	for _, r := range results {
		session := r.Session
		if session == "" {
			session = "-"
		}
		last := r.Time
		if last == "" {
			last = "-"
		}
		if withFirst {
			first := "-"
			if !r.firstHit.IsZero() {
				first = r.firstHit.Local().Format("2006-01-02 15:04")
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				r.Type, truncateDisplay(cleanDisplayText(r.Project), 24), session, cleanDisplayText(r.Summary), first, last)
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			r.Type, truncateDisplay(cleanDisplayText(r.Project), 24), session, cleanDisplayText(r.Summary), last)
	}

	return w.Flush()
}

func searchMemoryDir(home, subdir string, m textMatcher, filter config.SessionFilter, results *[]searchResult) {
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
			if m.matches(entry.Name()) {
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

// sessionCandidate is one session that passed the provider/date/model
// filters and still has to be matched against the query.
type sessionCandidate struct {
	session *parser.Session
	project string
}

// sessionSearcher matches one session against the query: ID prefix,
// summary, and — with content on — conversation text or raw lines.
type sessionSearcher struct {
	m       textMatcher
	content bool
	raw     bool
	hits    bool // collect every matching message, not just previews
	parser  sessionParser
}

// matchAll runs match over candidates on `workers` goroutines and
// returns the hits in candidate order (so equal-rank results keep
// discovery order). Progress goes to stderr when it is a terminal.
func (ss sessionSearcher) matchAll(candidates []sessionCandidate, workers int) []*searchResult {
	out := make([]*searchResult, len(candidates))
	if len(candidates) == 0 {
		return nil
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(candidates) {
		workers = len(candidates)
	}

	progress := newScanProgress(len(candidates), ss.content)
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				out[i] = ss.match(candidates[i].session, candidates[i].project)
				progress.tick()
			}
		}()
	}
	for i := range candidates {
		next <- i
	}
	close(next)
	wg.Wait()
	progress.done()

	hits := out[:0]
	for _, r := range out {
		if r != nil {
			hits = append(hits, r)
		}
	}
	return hits
}

// match returns the result for one session, or nil. A summary hit
// stays typed "session" but still carries content evidence (matches,
// previews, first hit) when content is on — the session where the
// term was actually discussed must not be the one result without
// its discussion.
func (ss sessionSearcher) match(s *parser.Session, projDisplay string) *searchResult {
	// Session ID match (high priority)
	if strings.HasPrefix(strings.ToLower(s.ID), ss.m.query) {
		return &searchResult{
			Type:     "session",
			Project:  projDisplay,
			Session:  truncateID(s.ID, 8),
			Path:     s.FilePath,
			Summary:  sessionSummaryPreview(s.Summary, 64),
			Time:     formatAge(s.EndTime),
			Priority: 0,
			lastTime: s.EndTime,
		}
	}

	var res *searchResult
	if ss.m.matches(s.Summary) {
		res = &searchResult{
			Type:     "session",
			Project:  projDisplay,
			Session:  truncateID(s.ID, 8),
			Path:     s.FilePath,
			Summary:  sessionSummaryPreview(s.Summary, 64),
			Time:     formatAge(s.EndTime),
			Priority: 2,
			lastTime: s.EndTime,
		}
	}
	if !ss.content {
		return res
	}

	// Content scan. The default counts conversation text only —
	// user prompts and assistant replies — so ranking follows
	// discussion, not injected boilerplate (a hook line fired
	// every turn once outranked the real answer 327 hits to 13;
	// docs/devlog/2026-08-03-content-search-noise.org). --raw
	// keeps grep parity over raw transcript lines, main file
	// plus subagent files: no parse, works for every provider's
	// format, misses nothing grep would find.
	var (
		n        int
		first    time.Time
		previews []contentPreview
		hits     []searchHit
	)
	if ss.raw {
		var lines []rawHit
		n, first, lines = countContentMatches(s.FilePath, ss.m, ss.hits)
		for _, l := range lines {
			hits = append(hits, searchHit{MessageID: l.id, Time: l.time, Role: l.role, Matches: 1, Quote: l.quote})
		}
	} else {
		// Cheap line-scan prefilter before the full parse; only
		// trustworthy when JSON escaping can't hide the query. The
		// literal matcher is a superset of word matches, so it
		// prefilters both modes.
		if rawPrefilterSafe(ss.m.query) && !sessionHasRawMatch(s.FilePath, ss.m.literal()) {
			return res
		}
		n, first, previews, hits = scanConversationText(ss.parser, s.FilePath, ss.m, ss.hits)
	}
	if n == 0 {
		return res
	}
	for i := range hits {
		hits[i].Project = projDisplay
		hits[i].Session = truncateID(s.ID, 8)
		hits[i].Path = s.FilePath
	}

	summary := fmt.Sprintf("%d hits · %s", n, sessionSummaryPreview(s.Summary, 48))
	if len(previews) > 0 {
		summary = fmt.Sprintf("%d hits · [%s] %s", n, previews[0].Role, truncateDisplay(previews[0].Text, 56))
	}
	if res == nil {
		res = &searchResult{
			Type:     "content",
			Project:  projDisplay,
			Session:  truncateID(s.ID, 8),
			Path:     s.FilePath,
			Summary:  summary,
			Time:     formatAge(s.EndTime),
			Priority: 3,
			lastTime: s.EndTime,
		}
	}
	res.Matches = n
	res.Previews = previews
	res.firstHit = first
	res.hits = hits
	if !first.IsZero() {
		res.FirstHit = first.UTC().Format(time.RFC3339)
	}
	return res
}

// collectHits flattens per-session hits into one timeline, oldest
// first; hits without a timestamp trail in discovery order.
func collectHits(results []searchResult) []searchHit {
	var hits []searchHit
	for _, r := range results {
		hits = append(hits, r.hits...)
	}
	sort.SliceStable(hits, func(i, j int) bool {
		a, b := hits[i].Time, hits[j].Time
		if a.IsZero() != b.IsZero() {
			return !a.IsZero()
		}
		return a.Before(b)
	})
	return hits
}

func printSearchHits(hits []searchHit) error {
	if searchLimit > 0 && len(hits) > searchLimit {
		fmt.Fprintf(os.Stderr, "showing %d of %d hits (raise with -n)\n", searchLimit, len(hits))
		hits = hits[:searchLimit]
	}
	if len(hits) == 0 {
		fmt.Println("No hits found.")
		return nil
	}
	if searchJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(hits)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TIME\tSESSION\tROLE\tMESSAGE\tQUOTE")
	for _, h := range hits {
		t := "-"
		if !h.Time.IsZero() {
			t = h.Time.Local().Format("2006-01-02 15:04")
		}
		id := h.MessageID
		if id == "" {
			id = "-"
		}
		session := h.Session
		if session == "" {
			session = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", t, session, h.Role, truncateID(id, 8), cleanDisplayText(h.Quote))
	}
	return w.Flush()
}

// scanProgress reports content-scan progress on stderr when stderr is
// a terminal, so a multi-minute cold scan is visibly alive. Silent
// when piped: agents and scripts read stdout, and a stream of \r
// lines is noise there.
type scanProgress struct {
	mu    sync.Mutex
	total int
	done_ int
	last  time.Time
	on    bool
}

func newScanProgress(total int, on bool) *scanProgress {
	if on {
		info, err := os.Stderr.Stat()
		on = err == nil && info.Mode()&os.ModeCharDevice != 0
	}
	return &scanProgress{total: total, on: on}
}

func (p *scanProgress) tick() {
	if !p.on {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.done_++
	if now := time.Now(); now.Sub(p.last) >= 200*time.Millisecond {
		p.last = now
		fmt.Fprintf(os.Stderr, "\rscanning %d/%d sessions", p.done_, p.total)
	}
}

func (p *scanProgress) done() {
	if !p.on {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintf(os.Stderr, "\r%*s\r", len(fmt.Sprintf("scanning %d/%d sessions", p.total, p.total)), "")
}

// scanConversationText parses one session (the parser loads sidechain
// files too) and searches only conversation text: text and thinking
// blocks of user prompts and assistant messages. Tool results, hook
// attachments, command echoes, and meta lines never count — that's
// what --raw is for. Returns total occurrences, the timestamp of the
// earliest matching message, and up to maxContentPreviews role-labeled
// snippets around the earliest matches.
func scanConversationText(p sessionParser, path string, m textMatcher, wantHits bool) (int, time.Time, []contentPreview, []searchHit) {
	sess, err := p.ParseSession(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: skipping unparseable %s: %v\n", filepath.Base(path), err)
		return 0, time.Time{}, nil, nil
	}

	count := 0
	var first time.Time
	var previews []contentPreview
	var hits []searchHit
	var walk func(msgs []*parser.Message)
	walk = func(msgs []*parser.Message) {
		for _, msg := range msgs {
			if msg.Kind == parser.KindUserPrompt || msg.Kind == parser.KindAssistant {
				role := msg.Type
				if msg.IsSidechain {
					role = "agent"
				}
				msgHits := 0
				quote := ""
				for _, b := range msg.Content {
					if b.Type != "text" && b.Type != "thinking" {
						continue
					}
					n := m.count(b.Text)
					if n == 0 {
						continue
					}
					count += n
					msgHits += n
					if !msg.Timestamp.IsZero() && (first.IsZero() || msg.Timestamp.Before(first)) {
						first = msg.Timestamp
					}
					if len(previews) < maxContentPreviews || (wantHits && quote == "") {
						idx, qlen := m.index(b.Text)
						snippet := matchSnippet(b.Text, idx, qlen)
						if quote == "" {
							quote = snippet
						}
						if len(previews) < maxContentPreviews {
							previews = append(previews, contentPreview{Role: role, Text: snippet})
						}
					}
				}
				if wantHits && msgHits > 0 {
					hits = append(hits, searchHit{
						MessageID: msg.UUID,
						Time:      msg.Timestamp,
						Role:      role,
						Matches:   msgHits,
						Quote:     quote,
					})
				}
			}
			walk(msg.Children)
		}
	}
	walk(sess.RootMessages)
	return count, first, previews, hits
}

// matchSnippet cuts a display window around a match, clamped to rune
// boundaries. idx may index a lowered copy of text; byte positions can
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

// sessionHasRawMatch reports whether any raw transcript line of the
// session — main file or subagent files — matches. Stops at the first
// hit: it is a prefilter, not a count.
func sessionHasRawMatch(sessionPath string, m textMatcher) bool {
	if sessionPath == "" {
		return false
	}
	if n, _, _ := scanRawLines(sessionPath, m, true, false); n > 0 {
		return true
	}
	for _, f := range parser.SubagentFiles(sessionPath) {
		if n, _, _ := scanRawLines(f, m, true, false); n > 0 {
			return true
		}
	}
	return false
}

// countContentMatches counts transcript lines matching m across the
// main session file and any subagent files beside it (layout
// knowledge lives in parser.SubagentFiles; providers without subagent
// files simply contribute none), and returns the earliest timestamp
// among matching lines.
func countContentMatches(sessionPath string, m textMatcher, wantHits bool) (int, time.Time, []rawHit) {
	if sessionPath == "" {
		return 0, time.Time{}, nil
	}
	count, first, hits := scanRawLines(sessionPath, m, false, wantHits)
	for _, f := range parser.SubagentFiles(sessionPath) {
		n, t, h := scanRawLines(f, m, false, wantHits)
		count += n
		first = earlier(first, t)
		hits = append(hits, h...)
	}
	return count, first, hits
}

// countMatchingLines streams one JSONL file and counts lines matching
// m, returning the earliest timestamp among them.
func countMatchingLines(path string, m textMatcher) (int, time.Time) {
	n, first, _ := scanRawLines(path, m, false, false)
	return n, first
}

// rawHit is one matching raw transcript line: its top-level anchors
// plus a quote around the match.
type rawHit struct {
	id    string
	role  string
	time  time.Time
	quote string
}

// scanRawLines streams one JSONL file and counts lines matching m
// (stopping at the first when stopAtFirst). bufio.Reader, not
// Scanner: transcript lines carrying embedded images exceed any fixed
// budget, and a silent early stop is exactly the false-negative class
// --content exists to kill. Unreadable files warn instead of lying
// "0 hits". The earliest top-level "timestamp" among matching lines
// is returned when counting (every provider stamps its lines).
func scanRawLines(path string, m textMatcher, stopAtFirst, wantHits bool) (int, time.Time, []rawHit) {
	file, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: skipping unreadable %s: %v\n", filepath.Base(path), err)
		return 0, time.Time{}, nil
	}
	defer file.Close()

	count := 0
	var first time.Time
	var hits []rawHit
	reader := bufio.NewReaderSize(file, 64*1024)
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			if idx, qlen := m.index(line); idx >= 0 {
				count++
				if stopAtFirst {
					return count, first, nil
				}
				head := lineHead(line)
				first = earlier(first, head.time)
				if wantHits {
					hits = append(hits, rawHit{id: head.id, role: head.role, time: head.time, quote: matchSnippet(line, idx, qlen)})
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				fmt.Fprintf(os.Stderr, "warning: read error in %s: %v\n", filepath.Base(path), err)
			}
			return count, first, hits
		}
	}
}

// lineHead is the top-level anchor set of one transcript line.
type lineHead_ struct {
	id   string
	role string
	time time.Time
}

// lineHead extracts the top-level "timestamp", "uuid", and "type" of
// one transcript line; zero/empty when absent or unparseable. A full
// decode, not a substring hunt: tool results embed other objects'
// timestamps.
func lineHead(line string) lineHead_ {
	var head struct {
		Timestamp string `json:"timestamp"`
		UUID      string `json:"uuid"`
		Type      string `json:"type"`
	}
	if err := json.Unmarshal([]byte(line), &head); err != nil {
		return lineHead_{}
	}
	out := lineHead_{id: head.UUID, role: head.Type}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, head.Timestamp); err == nil {
			out.time = t
			break
		}
	}
	return out
}

func lineTimestamp(line string) time.Time { return lineHead(line).time }

// earlier returns the earlier of two times, ignoring zero values.
func earlier(a, b time.Time) time.Time {
	switch {
	case a.IsZero():
		return b
	case b.IsZero():
		return a
	case b.Before(a):
		return b
	}
	return a
}
