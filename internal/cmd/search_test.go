package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

// sub is the default substring matcher; word is -w.
func sub(q string) textMatcher  { return newTextMatcher(q, false) }
func word(q string) textMatcher { return newTextMatcher(q, true) }

func TestCountMatchingLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	content := `{"type":"user","text":"tell me about Pi-Agent"}
{"type":"assistant","text":"nothing relevant"}
{"type":"assistant","text":"pi-agent uses ACP"}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := countMatchingLines(path, sub("pi-agent")); got != 2 {
		t.Fatalf("case-insensitive matches: got %d, want 2", got)
	}
	if got, _ := countMatchingLines(path, sub("absent-term")); got != 0 {
		t.Fatalf("no-match count: got %d, want 0", got)
	}
	if got, _ := countMatchingLines(filepath.Join(dir, "missing.jsonl"), sub("x")); got != 0 {
		t.Fatalf("missing file must count 0, got %d", got)
	}
}

// countContentMatches must also cover subagent transcripts, which live
// in <id>/subagents/agent-*.jsonl beside the main <id>.jsonl — and
// must count exactly the files view --show-agents renders.
func TestCountContentMatchesIncludesSubagents(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "abc-123.jsonl")
	if err := os.WriteFile(main, []byte(`{"text":"goose in main"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(dir, "abc-123", "subagents")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sideLines := `{"text":"goose in sidechain"}
{"text":"more goose here"}
`
	if err := os.WriteFile(filepath.Join(subDir, "agent-1.jsonl"), []byte(sideLines), 0o644); err != nil {
		t.Fatal(err)
	}
	// Non-jsonl files (meta.json) and jsonl without the agent- prefix
	// must be skipped, matching what the session parser loads.
	if err := os.WriteFile(filepath.Join(subDir, "agent-1.meta.json"), []byte(`{"text":"goose meta"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "notes.jsonl"), []byte(`{"text":"goose notes"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if got, _, _ := countContentMatches(main, sub("goose"), false); got != 3 {
		t.Fatalf("main+subagent matches: got %d, want 3", got)
	}
	if got, _, _ := countContentMatches("", sub("goose"), false); got != 0 {
		t.Fatalf("empty path must count 0, got %d", got)
	}
	if !sessionHasRawMatch(main, sub("sidechain")) {
		t.Fatal("prefilter must see subagent files")
	}
	if sessionHasRawMatch(main, sub("absent")) {
		t.Fatal("prefilter false positive")
	}
}

type stubSessionParser struct{}

func (stubSessionParser) ParseSession(path string) (*parser.Session, error) {
	return parser.ParseSession(path)
}

// scanConversationText must count only what a human reads in the
// conversation. Hook attachments (isMeta) and tool-result lines are
// exactly the boilerplate that once outranked the real discussion
// 327 hits to 13 — they must contribute zero.
func TestScanConversationTextSignalOnly(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "abc-123.jsonl")
	lines := strings.Join([]string{
		`{"type":"user","uuid":"u1","timestamp":"2026-08-03T00:00:00Z","message":{"role":"user","content":[{"type":"text","text":"let's design the deadman auto-handoff timer"}]}}`,
		`{"type":"assistant","uuid":"a1","parentUuid":"u1","timestamp":"2026-08-03T00:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"deadman fires after idle; the deadman then writes the handoff"}]}}`,
		`{"type":"user","uuid":"m1","parentUuid":"a1","isMeta":true,"timestamp":"2026-08-03T00:00:02Z","message":{"role":"user","content":[{"type":"text","text":"deadman armed: auto-handoff in 50m"}]}}`,
		`{"type":"user","uuid":"t1","parentUuid":"m1","timestamp":"2026-08-03T00:00:03Z","message":{"role":"user","content":[{"type":"tool_result","content":"deadman armed: auto-handoff stdout twin"}]}}`,
		`{"type":"assistant","uuid":"a2","parentUuid":"t1","timestamp":"2026-08-03T00:00:04Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"the deadman design needs a disarm path"}]}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(main, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(dir, "abc-123", "subagents")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	side := `{"type":"assistant","uuid":"s1","isSidechain":true,"timestamp":"2026-08-03T00:00:05Z","message":{"role":"assistant","content":[{"type":"text","text":"deadman in sidechain"}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(subDir, "agent-1.jsonl"), []byte(side), 0o644); err != nil {
		t.Fatal(err)
	}

	// 6 raw lines match; only 5 occurrences live in conversation text
	// (1 user + 2 assistant + 1 thinking + 1 sidechain).
	if raw, first, _ := countContentMatches(main, sub("deadman"), false); raw != 6 || !first.Equal(time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("raw line matches: got %d @ %v, want 6 @ 2026-08-03T00:00:00Z", raw, first)
	}
	n, first, previews, _ := scanConversationText(stubSessionParser{}, main, sub("deadman"), false)
	if n != 5 {
		t.Fatalf("signal matches: got %d, want 5 (noise counted?)", n)
	}
	if !first.Equal(time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("first hit: got %v, want the user prompt time", first)
	}
	if len(previews) != maxContentPreviews {
		t.Fatalf("previews: got %d, want %d", len(previews), maxContentPreviews)
	}
	if previews[0].Role != "user" || !strings.Contains(previews[0].Text, "deadman") {
		t.Fatalf("first preview should be the user prompt, got [%s] %q", previews[0].Role, previews[0].Text)
	}
	if previews[1].Role != "assistant" {
		t.Fatalf("second preview role: got %q, want assistant", previews[1].Role)
	}

	if n, _, _, _ := scanConversationText(stubSessionParser{}, main, sub("auto-handoff"), false); n != 1 {
		t.Fatalf("auto-handoff signal matches: got %d, want 1 (hook noise counted?)", n)
	}
}

func TestScanConversationTextSidechainRole(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "abc-123.jsonl")
	line := `{"type":"user","uuid":"u1","timestamp":"2026-08-03T00:00:00Z","message":{"role":"user","content":[{"type":"text","text":"kick off"}]}}` + "\n"
	if err := os.WriteFile(main, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(dir, "abc-123", "subagents")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	side := `{"type":"assistant","uuid":"s1","isSidechain":true,"timestamp":"2026-08-03T00:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"goose only lives here"}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(subDir, "agent-1.jsonl"), []byte(side), 0o644); err != nil {
		t.Fatal(err)
	}

	n, _, previews, _ := scanConversationText(stubSessionParser{}, main, sub("goose"), false)
	if n != 1 || len(previews) != 1 {
		t.Fatalf("sidechain match: got n=%d previews=%d, want 1/1", n, len(previews))
	}
	if previews[0].Role != "agent" {
		t.Fatalf("sidechain preview role: got %q, want agent", previews[0].Role)
	}
}

func TestMatchSnippet(t *testing.T) {
	long := strings.Repeat("a", 100) + " needle " + strings.Repeat("b", 100)
	got := matchSnippet(long, 101, len("needle"))
	if !strings.Contains(got, "needle") {
		t.Fatalf("snippet must contain the match, got %q", got)
	}
	if !strings.HasPrefix(got, "...") || !strings.HasSuffix(got, "...") {
		t.Fatalf("mid-text snippet should be marked truncated on both ends, got %q", got)
	}
	if got := matchSnippet("short text", 0, 5); got != "short text" {
		t.Fatalf("untruncated snippet: got %q", got)
	}
	// Out-of-range indexes must clamp, not panic.
	_ = matchSnippet("tiny", 999, 4)
}

func TestRawPrefilterSafe(t *testing.T) {
	cases := map[string]bool{
		"deadman":    true,
		"fix bug":    true,
		`say "hi"`:   false,
		`back\slash`: false,
		"路径":         false,
		"tab\there":  false,
	}
	for q, want := range cases {
		if got := rawPrefilterSafe(q); got != want {
			t.Errorf("rawPrefilterSafe(%q) = %v, want %v", q, got, want)
		}
	}
}

// Grep parity must survive lines larger than any fixed scanner budget
// (transcript lines with embedded images run past 10MB).
func TestCountMatchingLinesOversizedLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.jsonl")
	huge := `{"pad":"` + strings.Repeat("x", 11*1024*1024) + `"}`
	content := huge + "\n" + `{"text":"needle after the giant line"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := countMatchingLines(path, sub("needle")); got != 1 {
		t.Fatalf("match after oversized line: got %d, want 1", got)
	}
}

// -w must stop "semantica" from matching "semantically" — the case that
// made 46 of 47 results noise — while still matching the term inside
// punctuation, hyphenated compounds, and CJK runs (no ASCII word
// boundary exists there, so none is demanded).
func TestTextMatcherWord(t *testing.T) {
	cases := []struct {
		q, text string
		sub, w  int
	}{
		{"semantica", "semantically correct", 1, 0},
		{"semantica", "we chose Semantica.", 1, 1},
		{"semantica", "semantica-agi ships semantica", 2, 2},
		{"semantica", "(semantica)", 1, 1},
		{"pi-agent", "Pi-Agent uses ACP; pi-agents too", 2, 1},
		{"中文", "关于中文的讨论", 1, 1},
		{"fix bug", "fix bugs later; fix bug now", 2, 1},
	}
	for _, c := range cases {
		if got := sub(c.q).count(c.text); got != c.sub {
			t.Errorf("sub(%q).count(%q) = %d, want %d", c.q, c.text, got, c.sub)
		}
		if got := word(c.q).count(c.text); got != c.w {
			t.Errorf("word(%q).count(%q) = %d, want %d", c.q, c.text, got, c.w)
		}
		if (word(c.q).matches(c.text)) != (c.w > 0) {
			t.Errorf("word(%q).matches(%q) disagrees with count %d", c.q, c.text, c.w)
		}
	}
	// index feeds the preview window: it must point at the real match,
	// not the substring inside a longer word.
	if i, n := word("semantica").index("semantically, then semantica-agi"); i != 19 || n != len("semantica") {
		t.Fatalf("word index: got %d/%d, want 19/9", i, n)
	}
	// literal() is the prefilter: a superset of word matches.
	if !word("semantica").literal().matches("semantically") {
		t.Fatal("literal prefilter must keep substring semantics")
	}
}

// indexFold/countFold replace ToLower-per-line on the raw scan; they
// must agree with the allocating form on ASCII and non-ASCII input.
func TestIndexFold(t *testing.T) {
	cases := []struct {
		s, q string
		idx  int
		n    int
	}{
		{"tell me about Pi-Agent", "pi-agent", 14, 1},
		{"PI-AGENT pi-agent Pi-Agent", "pi-agent", 0, 3},
		{"nothing here", "absent", -1, 0},
		{"aaa", "aa", 0, 1},
		{"路径 and 路径", "路径", 0, 2},
		{"x", "", 0, 0},
	}
	for _, c := range cases {
		if got := indexFold(c.s, c.q); got != c.idx {
			t.Errorf("indexFold(%q,%q) = %d, want %d", c.s, c.q, got, c.idx)
		}
		if got := countFold(c.s, c.q); got != c.n {
			t.Errorf("countFold(%q,%q) = %d, want %d", c.s, c.q, got, c.n)
		}
		if c.q != "" {
			if want := strings.Index(strings.ToLower(c.s), c.q); want != c.idx {
				t.Errorf("test expectation drift for %q: ToLower form gives %d", c.q, want)
			}
		}
	}
	// A window that cuts a multi-byte rune must not match.
	if indexFold("cafés", "\xc3s") != -1 {
		t.Fatal("partial rune must not fold-match")
	}
}

func TestLineTimestamp(t *testing.T) {
	// The tool result embeds another object's timestamp before the
	// top-level one; only the top-level field counts.
	line := `{"type":"user","message":{"content":[{"type":"tool_result","content":"{\"timestamp\":\"2020-01-01T00:00:00Z\"}"}]},"timestamp":"2026-08-03T01:02:03.500Z"}`
	if got := lineTimestamp(line); !got.Equal(time.Date(2026, 8, 3, 1, 2, 3, 500000000, time.UTC)) {
		t.Fatalf("lineTimestamp: got %v", got)
	}
	if !lineTimestamp(`{"no":"ts"}`).IsZero() || !lineTimestamp(`not json`).IsZero() {
		t.Fatal("missing/invalid timestamp must be zero")
	}
}

// A session whose summary matches must still carry its content
// evidence: previously the summary hit short-circuited the scan, so
// the one session where the term was actually discussed was the one
// result with no matches, previews, or first-hit time.
func TestSessionSearcherSummaryHitKeepsContent(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "abc-123.jsonl")
	lines := strings.Join([]string{
		`{"type":"user","uuid":"u1","timestamp":"2026-08-18T07:00:00Z","message":{"role":"user","content":[{"type":"text","text":"when did we mention semantica?"}]}}`,
		`{"type":"assistant","uuid":"a1","parentUuid":"u1","timestamp":"2026-08-18T07:00:05Z","message":{"role":"assistant","content":[{"type":"text","text":"semantically, never; semantica itself: now"}]}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(main, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	end := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	sess := &parser.Session{ID: "abc-123", FilePath: main, Summary: "trace semantica mentions", EndTime: end}

	// Name-only search: summary hit, no content fields.
	r := sessionSearcher{m: word("semantica")}.match(sess, "proj")
	if r == nil || r.Type != "session" || r.Matches != 0 || r.FirstHit != "" {
		t.Fatalf("summary-only match: got %+v", r)
	}

	// Content on: same row, now with evidence. Word mode counts 2
	// (not the "semantically" in the reply).
	r = sessionSearcher{m: word("semantica"), content: true, parser: stubSessionParser{}}.match(sess, "proj")
	if r == nil || r.Type != "session" || r.Priority != 2 {
		t.Fatalf("summary+content match must stay a session hit, got %+v", r)
	}
	if r.Matches != 2 || len(r.Previews) != 2 || r.Previews[0].Role != "user" {
		t.Fatalf("content evidence missing on summary hit: %+v", r)
	}
	if r.FirstHit != "2026-08-18T07:00:00Z" || !r.lastTime.Equal(end) {
		t.Fatalf("first/last: got %q / %v", r.FirstHit, r.lastTime)
	}

	// Substring mode counts the noise too; -w is what makes the
	// count trustworthy.
	r = sessionSearcher{m: sub("semantica"), content: true, parser: stubSessionParser{}}.match(sess, "proj")
	if r == nil || r.Matches != 3 {
		t.Fatalf("substring count: got %+v", r)
	}

	// No summary hit, no content hit: nil, not an empty row.
	if r := (sessionSearcher{m: word("absent"), content: true, parser: stubSessionParser{}}).match(sess, "proj"); r != nil {
		t.Fatalf("miss must be nil, got %+v", r)
	}

	// --raw: line count plus first-hit from the raw timestamp.
	r = sessionSearcher{m: sub("semantica"), content: true, raw: true, parser: stubSessionParser{}}.match(sess, "proj")
	if r == nil || r.Matches != 2 || r.FirstHit != "2026-08-18T07:00:00Z" {
		t.Fatalf("raw match: got %+v", r)
	}
}

// matchAll must return hits in candidate order regardless of which
// worker finishes first — stable ranking depends on it.
func TestSessionSearcherMatchAllOrder(t *testing.T) {
	var cands []sessionCandidate
	for i := 0; i < 50; i++ {
		summary := "nothing"
		if i%3 == 0 {
			summary = "goose"
		}
		cands = append(cands, sessionCandidate{
			session: &parser.Session{ID: "id-" + strings.Repeat("x", i%7) + string(rune('a'+i%26)), Summary: summary},
			project: "p",
		})
	}
	hits := sessionSearcher{m: sub("goose")}.matchAll(cands, 4)
	if len(hits) != 17 {
		t.Fatalf("hits: got %d, want 17", len(hits))
	}
	for i := 1; i < len(hits); i++ {
		if hits[i-1].Session > hits[i].Session && cands[0].session != nil {
			// IDs are not monotonic by construction; check discovery
			// order via summary sequence instead.
			break
		}
	}
	want := 0
	for _, c := range cands {
		if c.session.Summary != "goose" {
			continue
		}
		if hits[want].Session != truncateID(c.session.ID, 8) {
			t.Fatalf("hit %d out of order: got %s want %s", want, hits[want].Session, c.session.ID)
		}
		want++
	}
}

func TestSortSearchResults(t *testing.T) {
	ts := func(d int) time.Time { return time.Date(2026, 8, d, 0, 0, 0, 0, time.UTC) }
	mk := func(name string, prio, hits, first, last int) searchResult {
		r := searchResult{Session: name, Priority: prio, Matches: hits}
		if first > 0 {
			r.firstHit = ts(first)
		}
		if last > 0 {
			r.lastTime = ts(last)
		}
		return r
	}
	order := func(rs []searchResult) string {
		var ids []string
		for _, r := range rs {
			ids = append(ids, r.Session)
		}
		return strings.Join(ids, " ")
	}
	base := func() []searchResult {
		return []searchResult{
			mk("proj", 1, 0, 0, 0),
			mk("late-many", 3, 9, 15, 17),
			mk("early-few", 3, 1, 3, 10),
			mk("summary", 2, 0, 0, 12),
		}
	}

	rs := base()
	sortSearchResults(rs, "hits")
	if got := order(rs); got != "proj summary late-many early-few" {
		t.Fatalf("hits order: %s", got)
	}
	rs = base()
	sortSearchResults(rs, "first")
	if got := order(rs); got != "early-few late-many proj summary" {
		t.Fatalf("first order: %s (timeless must trail in hits order)", got)
	}
	rs = base()
	sortSearchResults(rs, "last")
	if got := order(rs); got != "late-many summary early-few proj" {
		t.Fatalf("last order: %s", got)
	}
}

// --hits turns a search into citations: one row per matching message
// with the anchors (message id, time, role) needed to walk back to
// it, oldest first across sessions. Under --raw the unit is a line.
func TestSessionSearcherHits(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "abc-123.jsonl")
	lines := strings.Join([]string{
		`{"type":"user","uuid":"u1","timestamp":"2026-08-18T07:00:00Z","message":{"role":"user","content":[{"type":"text","text":"when did we mention semantica?"}]}}`,
		`{"type":"assistant","uuid":"a1","parentUuid":"u1","timestamp":"2026-08-18T07:00:05Z","message":{"role":"assistant","content":[{"type":"text","text":"semantically, never"},{"type":"text","text":"semantica itself: now, and semantica again"}]}}`,
		`{"type":"user","uuid":"t1","parentUuid":"a1","timestamp":"2026-08-18T07:00:06Z","message":{"role":"user","content":[{"type":"tool_result","content":"semantica in a tool result"}]}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(main, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	sess := &parser.Session{ID: "abc-123", FilePath: main, Summary: "s"}

	r := sessionSearcher{m: word("semantica"), content: true, hits: true, parser: stubSessionParser{}}.match(sess, "proj")
	if r == nil || r.Matches != 3 || len(r.hits) != 2 {
		t.Fatalf("hits: got %+v", r)
	}
	h := r.hits[0]
	if h.MessageID != "u1" || h.Role != "user" || h.Matches != 1 || h.Session != "abc-123" || h.Path != main || h.Project != "proj" {
		t.Fatalf("first hit anchors: %+v", h)
	}
	if !h.Time.Equal(time.Date(2026, 8, 18, 7, 0, 0, 0, time.UTC)) || !strings.Contains(h.Quote, "semantica") {
		t.Fatalf("first hit time/quote: %+v", h)
	}
	if r.hits[1].MessageID != "a1" || r.hits[1].Matches != 2 || !strings.Contains(r.hits[1].Quote, "semantica itself") {
		t.Fatalf("second hit must count both blocks and quote the first real match, got %+v", r.hits[1])
	}

	// Without --hits nothing extra is collected.
	if r := (sessionSearcher{m: word("semantica"), content: true, parser: stubSessionParser{}}).match(sess, "proj"); len(r.hits) != 0 {
		t.Fatalf("hits collected without --hits: %d", len(r.hits))
	}

	// --raw --hits: every matching line, including the tool result,
	// anchored by the line's own uuid/type/timestamp.
	r = sessionSearcher{m: sub("semantica"), content: true, raw: true, hits: true, parser: stubSessionParser{}}.match(sess, "proj")
	if r == nil || len(r.hits) != 3 || r.hits[2].MessageID != "t1" || r.hits[2].Role != "user" {
		t.Fatalf("raw hits: got %+v", r)
	}

	// collectHits orders across sessions by time, zero times last.
	later := searchResult{hits: []searchHit{{MessageID: "z", Time: time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)}, {MessageID: "none"}}}
	all := collectHits([]searchResult{later, *r})
	if len(all) != 5 || all[0].MessageID != "u1" || all[3].MessageID != "z" || all[4].MessageID != "none" {
		ids := []string{}
		for _, h := range all {
			ids = append(ids, h.MessageID)
		}
		t.Fatalf("collectHits order: %v", ids)
	}
}
