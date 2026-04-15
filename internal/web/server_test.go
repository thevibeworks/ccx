package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/db"
	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/provider/claude"
)

func setupTestDir(t *testing.T) string {
	dir := t.TempDir()

	// Create a test project directory
	projectDir := filepath.Join(dir, "projects", "-test-project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a test session file
	sessionFile := filepath.Join(projectDir, "test-session-123.jsonl")
	content := `{"type":"user","timestamp":"2024-01-01T10:00:00Z","uuid":"u1","message":{"content":"Hello"}}
{"type":"assistant","timestamp":"2024-01-01T10:00:01Z","uuid":"a1","parentUuid":"u1","message":{"content":"Hi there!"}}
`
	if err := os.WriteFile(sessionFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func setTestBackend(dir string) {
	backend := claude.NewWithProjectsDir(dir, filepath.Join(dir, "projects"))
	providerHomes = backend.Homes()
	sessionProvider = backend
}

func TestHandleIndex(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handleIndex(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleIndex returned %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if len(body) == 0 {
		t.Error("handleIndex returned empty body")
	}
}

func TestHandleIndex_NotFoundForOtherPaths(t *testing.T) {
	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()

	handleIndex(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("handleIndex for /nonexistent returned %d, want %d", w.Code, http.StatusNotFound)
	}
}

// TestHandleIndex_NotFoundRendersReportBugPage — when the handler
// returns a 404 to a human-facing route, the body should be the
// styled "report a bug" page, not net/http's plain text default.
// The page must include the failing URL AND a pre-filled GitHub
// issue link so users can report breakage in one click.
func TestHandleIndex_NotFoundRendersReportBugPage(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/totally-not-a-route", nil)
	w := httptest.NewRecorder()
	handleIndex(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body := w.Body.String()

	// Must be the styled page, not net/http's default "404 page not found\n".
	if !strings.Contains(body, "nf-box") {
		t.Error("response missing styled .nf-box container")
	}
	if !strings.Contains(body, "Report this bug") {
		t.Error("response missing 'Report this bug' CTA")
	}
	// The pre-filled issue URL must point at the ccx repo.
	if !strings.Contains(body, "github.com/thevibeworks/ccx/issues/new") {
		t.Error("response missing github issue link")
	}
	// The failing URL must be visible so the user (and the maintainer
	// reading the bug report) can see what was attempted.
	if !strings.Contains(body, "/totally-not-a-route") {
		t.Error("response missing the failing URL in the body")
	}
	// The issue title/body parameters must include the failing URL
	// URL-encoded. %2F = "/" means the path shows up in the query
	// string.
	if !strings.Contains(body, "totally-not-a-route") {
		t.Error("issue body missing failing path")
	}
}

// TestHandleSession_NotFoundRendersReportBugPageWithSessionDetail —
// when a session lookup fails, the 404 page's detail line should
// mention the session and project so users (and maintainers reading
// a bug report) know what was being looked for.
func TestHandleSession_NotFoundRendersReportBugPageWithSessionDetail(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/session/-no-such-project/019d8f43-8d53-7263-8e70-7729495e2b95", nil)
	w := httptest.NewRecorder()
	handleSession(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Can't find this session") {
		t.Errorf("missing session headline, got: %s", truncateForLog(body))
	}
	if !strings.Contains(body, "019d8f43-8d5") {
		t.Errorf("detail line should include truncated session id, got: %s", truncateForLog(body))
	}
	if !strings.Contains(body, "-no-such-project") {
		t.Errorf("detail line should include project name, got: %s", truncateForLog(body))
	}
	if !strings.Contains(body, "github.com/thevibeworks/ccx/issues/new") {
		t.Error("response missing github issue link")
	}
}

func truncateForLog(s string) string {
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}

// TestCliSpinnerLayout_FixedWidth asserts the spinner CSS pins the
// icon to a stable screen position — icon absolute-positioned at
// left, block with a fixed width — so rotating the verb doesn't make
// the icon jitter horizontally.
func TestCliSpinnerLayout_FixedWidth(t *testing.T) {
	css := cssStyles()
	if !strings.Contains(css, ".cli-spinner-char {\n  position: absolute;") {
		t.Error("cli-spinner-char must be position:absolute (otherwise icon jitters as verb changes)")
	}
	if !strings.Contains(css, ".cli-spinner {") || !strings.Contains(css, "width: 220px;") {
		t.Error("cli-spinner must have a fixed width so the centered overlay doesn't re-center with each verb")
	}
	if !strings.Contains(css, `body[data-ccx-provider="codex"] .cli-spinner { color: var(--accent-cx); }`) {
		t.Error("codex sessions should swap the spinner color to the Codex accent")
	}
}

// TestRenderSessionPage_EmitsProviderDataAttribute verifies that a
// Codex session page sets body.dataset.ccxProvider = "codex" so the
// CSS rule above actually fires. Claude Code sessions should set it
// to "claude-code", and anything with an empty provider should
// simply omit the hint.
func TestRenderSessionPage_EmitsProviderDataAttribute(t *testing.T) {
	session := &parser.Session{
		ID:        "019d8f43-8d53",
		Provider:  "codex",
		StartTime: time.Now(),
	}
	html := renderSessionPage(session, "test-project", nil, 0, false, false, false, "light")
	if !strings.Contains(html, `document.body.dataset.ccxProvider="codex"`) {
		t.Error("expected codex provider hint in body dataset, got: ", truncateForLog(html))
	}

	sessionCC := &parser.Session{
		ID:        "abcdef",
		Provider:  "claude-code",
		StartTime: time.Now(),
	}
	htmlCC := renderSessionPage(sessionCC, "test-project", nil, 0, false, false, false, "light")
	if !strings.Contains(htmlCC, `document.body.dataset.ccxProvider="claude-code"`) {
		t.Error("expected claude-code provider hint in body dataset")
	}

	sessionNoProvider := &parser.Session{
		ID:        "abcdef",
		StartTime: time.Now(),
	}
	htmlNone := renderSessionPage(sessionNoProvider, "test-project", nil, 0, false, false, false, "light")
	if strings.Contains(htmlNone, "dataset.ccxProvider") {
		t.Error("empty provider should NOT emit the hint")
	}
}

func TestHandleAPIProjects(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/api/projects", nil)
	w := httptest.NewRecorder()

	handleAPIProjects(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleAPIProjects returned %d, want %d", w.Code, http.StatusOK)
	}

	var projects []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &projects); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(projects) != 1 {
		t.Errorf("expected 1 project, got %d", len(projects))
	}
}

func TestHandleAPISessions(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/api/sessions/-test-project", nil)
	w := httptest.NewRecorder()

	handleAPISessions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleAPISessions returned %d, want %d", w.Code, http.StatusOK)
	}

	var sessions []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}
}

func TestHandleAPISessions_NotFound(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/api/sessions/-nonexistent-project", nil)
	w := httptest.NewRecorder()

	handleAPISessions(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("handleAPISessions for nonexistent project returned %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleAPIStats(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()

	handleAPIStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleAPIStats returned %d, want %d", w.Code, http.StatusOK)
	}

	var stats map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if _, ok := stats["projects"]; !ok {
		t.Error("stats missing projects")
	}
	if _, ok := stats["sessions"]; !ok {
		t.Error("stats missing sessions")
	}
}

func TestHandleAPISearch(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/api/search?q=Hello", nil)
	w := httptest.NewRecorder()

	handleAPISearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleAPISearch returned %d, want %d", w.Code, http.StatusOK)
	}

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if _, ok := result["results"]; !ok {
		t.Error("search result missing 'results' field")
	}
}

func TestHandleAPISearch_EmptyQuery(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/search?q=", nil)
	w := httptest.NewRecorder()

	handleAPISearch(w, req)

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	results, ok := result["results"].([]any)
	if !ok {
		t.Fatal("results is not an array")
	}
	if len(results) != 0 {
		t.Errorf("expected empty results for empty query, got %d", len(results))
	}
}

func TestHandleProject(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/project/-test-project", nil)
	w := httptest.NewRecorder()

	handleProject(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleProject returned %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if len(body) == 0 {
		t.Error("handleProject returned empty body")
	}
}

func TestHandleProject_NotFound(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/project/-nonexistent", nil)
	w := httptest.NewRecorder()

	handleProject(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("handleProject for nonexistent returned %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleSession(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/session/-test-project/test-session-123", nil)
	w := httptest.NewRecorder()

	handleSession(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleSession returned %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if len(body) == 0 {
		t.Error("handleSession returned empty body")
	}
}

func TestHandleSession_NotFound(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/session/-test-project/nonexistent-session", nil)
	w := httptest.NewRecorder()

	handleSession(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("handleSession for nonexistent returned %d, want %d", w.Code, http.StatusNotFound)
	}
}

// setupLargeSessionDir creates a project with a session large enough (>500 msgs)
// to trigger progressive loading. Used to test the load-earlier hash-nav fix.
func setupLargeSessionDir(t *testing.T, msgCount int) string {
	t.Helper()
	dir := t.TempDir()

	projectDir := filepath.Join(dir, "projects", "-large-project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	var b strings.Builder
	for i := 0; i < msgCount; i++ {
		uuid := "m" + strings.Repeat("0", 8-len(itoa(i))) + itoa(i)
		parent := ""
		if i > 0 {
			prev := "m" + strings.Repeat("0", 8-len(itoa(i-1))) + itoa(i-1)
			parent = `,"parentUuid":"` + prev + `"`
		}
		kind := "user"
		if i%2 == 1 {
			kind = "assistant"
		}
		b.WriteString(`{"type":"` + kind + `","timestamp":"2024-01-01T10:00:00Z","uuid":"` + uuid + `"` + parent + `,"message":{"content":"msg ` + itoa(i) + `"}}` + "\n")
	}

	sessionFile := filepath.Join(projectDir, "large-session-abc.jsonl")
	if err := os.WriteFile(sessionFile, []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}

func TestHandleSession_ProgressiveLoadingMarkersPresent(t *testing.T) {
	dir := setupLargeSessionDir(t, 600)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/session/-large-project/large-session-abc", nil)
	w := httptest.NewRecorder()

	handleSession(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("handleSession returned %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()

	// The load-earlier button must be rendered (progressive loading is active)
	if !strings.Contains(body, `id="load-earlier"`) {
		t.Error("expected load-earlier element to be rendered for 600-msg session")
	}
	// The hidden-reload branch must be wired up in the hash handler
	if !strings.Contains(body, `document.getElementById('load-earlier')`) {
		t.Error("expected hash handler to check for load-earlier when target not found")
	}
	if !strings.Contains(body, `jumpToHashTarget`) {
		t.Error("expected jumpToHashTarget function in session page JS")
	}
	// The pending-search persistence must be in loadEarlierMessages
	if !strings.Contains(body, `ccx-pending-search`) {
		t.Error("expected sessionStorage ccx-pending-search key for search persistence across reload")
	}
}

func TestHandleSession_LoadAllBypassesProgressive(t *testing.T) {
	dir := setupLargeSessionDir(t, 600)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/session/-large-project/large-session-abc?all=1", nil)
	w := httptest.NewRecorder()

	handleSession(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("handleSession returned %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()

	// With ?all=1, the load-earlier button must not be rendered (no hidden sections)
	if strings.Contains(body, `id="load-earlier"`) {
		t.Error("load-earlier element should not be present when ?all=1 is set")
	}
	// But the hash handler JS is still present — we just don't need to trigger it
	if !strings.Contains(body, `jumpToHashTarget`) {
		t.Error("jumpToHashTarget should still be wired up even when loadAll is active")
	}
}

// setupPricedSessionDir creates a project with a session containing real
// per-message usage and a known model so the spend section renders with
// non-zero cost. Used to test the per-turn breakdown UI (issue #2).
func setupPricedSessionDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	projectDir := filepath.Join(dir, "projects", "-priced-project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Two turns, known model, known usage. Turn 2 intentionally more expensive
	// than turn 1 so sorting by cost desc is observable.
	content := `{"type":"user","timestamp":"2026-04-01T10:00:00Z","uuid":"u1","message":{"content":"cheap turn"}}
{"type":"assistant","timestamp":"2026-04-01T10:00:01Z","uuid":"a1","parentUuid":"u1","message":{"role":"assistant","content":"reply","model":"claude-sonnet-4-5","usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}
{"type":"user","timestamp":"2026-04-01T10:01:00Z","uuid":"u2","parentUuid":"a1","message":{"content":"expensive turn with way more tokens"}}
{"type":"assistant","timestamp":"2026-04-01T10:01:02Z","uuid":"a2","parentUuid":"u2","message":{"role":"assistant","content":"big reply","model":"claude-sonnet-4-5","usage":{"input_tokens":5000,"output_tokens":3000,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}
`

	sessionFile := filepath.Join(projectDir, "priced-session-xyz.jsonl")
	if err := os.WriteFile(sessionFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestHandleSession_SpendSectionRendersWithCost(t *testing.T) {
	dir := setupPricedSessionDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/session/-priced-project/priced-session-xyz", nil)
	w := httptest.NewRecorder()

	handleSession(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("handleSession returned %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()

	// Spend section must be present with a total
	if !strings.Contains(body, `info-section-spend`) {
		t.Error("expected info-section-spend in rendered info panel")
	}
	if !strings.Contains(body, `Per-turn spend`) {
		t.Error("expected 'Per-turn spend' header")
	}
	if !strings.Contains(body, `spend-row`) {
		t.Error("expected spend-row elements linking to turn anchors")
	}
	if !strings.Contains(body, `Session total:`) {
		t.Error("expected session total footer in spend section")
	}
	// Cost row must appear in the Tokens section
	if !strings.Contains(body, `"info-row info-cost"`) {
		t.Error("expected info-cost row in Tokens section")
	}
	// The expensive turn's anchor must be linked (#msg-u2)
	if !strings.Contains(body, `href="#msg-u2"`) {
		t.Error("expected spend row linking to turn anchor #msg-u2")
	}
}

func TestHandleSession_TimelineRailRendered(t *testing.T) {
	dir := setupPricedSessionDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/session/-priced-project/priced-session-xyz", nil)
	w := httptest.NewRecorder()
	handleSession(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("handleSession returned %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()

	// Rail structure
	if !strings.Contains(body, `id="timeline-rail"`) {
		t.Error("expected timeline-rail aside in rendered page")
	}
	if !strings.Contains(body, `id="timeline-spine"`) {
		t.Error("expected timeline-spine id on the rail")
	}
	if !strings.Contains(body, `id="timeline-current"`) {
		t.Error("expected timeline-current scroll indicator")
	}
	if !strings.Contains(body, `id="timeline-playhead"`) {
		t.Error("expected timeline-playhead hover indicator")
	}
	if !strings.Contains(body, `id="timeline-tooltip"`) {
		t.Error("expected timeline-tooltip sibling element")
	}

	// Ticks are spans with data-* attributes (no native anchor nav;
	// click is dispatched via the rail's click handler and JS hash nav)
	if !strings.Contains(body, `tick-user`) {
		t.Error("expected tick-user class on user-prompt ticks")
	}
	if !strings.Contains(body, `data-uuid="u1"`) {
		t.Error("expected data-uuid=u1 on a timeline tick")
	}
	if !strings.Contains(body, `data-uuid="u2"`) {
		t.Error("expected data-uuid=u2 on a timeline tick")
	}
	if !strings.Contains(body, `data-offset=`) {
		t.Error("expected data-offset attribute on ticks for hover tooltip")
	}
	if !strings.Contains(body, `data-snippet=`) {
		t.Error("expected data-snippet attribute on ticks for hover tooltip")
	}

	// JS handlers wired up
	if !strings.Contains(body, `jumpTickRelative`) {
		t.Error("expected jumpTickRelative function for [/] keyboard nav")
	}
	if !strings.Contains(body, `handleRailMouse`) {
		t.Error("expected handleRailMouse for hover-to-scrub interaction")
	}
	if !strings.Contains(body, `nearestTickIndex`) {
		t.Error("expected nearestTickIndex binary-search helper")
	}

	// Cost-weighted integration: ticks must carry data-cost and --heat
	// inline, wired from the matching TurnStats.
	if !strings.Contains(body, `data-cost=`) {
		t.Error("expected data-cost attribute on at least one priced tick")
	}
	if !strings.Contains(body, `data-tokens=`) {
		t.Error("expected data-tokens attribute on at least one priced tick")
	}
	if !strings.Contains(body, `data-cumulative=`) {
		t.Error("expected data-cumulative attribute for running total in tooltip")
	}
	if !strings.Contains(body, `data-index=`) {
		t.Error("expected data-index attribute for turn ordinal in tooltip")
	}
	if !strings.Contains(body, `--heat:`) {
		t.Error("expected --heat CSS var inline on timeline ticks")
	}

	// Fisheye zoom + hysteresis + rAF-throttled interaction model
	if !strings.Contains(body, `zoom-0`) {
		t.Error("expected zoom-0 class for fisheye-nearest tick")
	}
	if !strings.Contains(body, `applyFisheyeZoom`) {
		t.Error("expected applyFisheyeZoom JS helper")
	}
	if !strings.Contains(body, `selectWithHysteresis`) {
		t.Error("expected selectWithHysteresis helper to prevent tooltip flicker between adjacent ticks")
	}
	if !strings.Contains(body, `requestAnimationFrame(processRailFrame)`) {
		t.Error("expected rAF-throttled mousemove processing")
	}
	if !strings.Contains(body, `TIMELINE_LEAVE_GRACE_MS`) {
		t.Error("expected mouseleave grace-period constant")
	}
}

func TestHandleSession_TimelineRailEmptyFallback(t *testing.T) {
	// Session with a single message (zero duration span) should still
	// render the rail but with the empty spine class — not crash.
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "projects", "-solo-project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `{"type":"user","timestamp":"2026-04-01T10:00:00Z","uuid":"u1","message":{"content":"only message"}}` + "\n"
	if err := os.WriteFile(filepath.Join(projectDir, "solo-session-abc.jsonl"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/session/-solo-project/solo-session-abc", nil)
	w := httptest.NewRecorder()
	handleSession(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("handleSession returned %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()

	if !strings.Contains(body, `id="timeline-rail"`) {
		t.Error("expected timeline-rail even for single-message session")
	}
	if !strings.Contains(body, `timeline-empty`) {
		t.Error("expected timeline-empty class when session has no span")
	}
}

func TestHandleSession_SessionNavIsNarrowByDefault(t *testing.T) {
	// The session-nav sidebar should emit the narrow-by-default CSS
	// rules (panel-nav.session-nav width:56px, hover → 260px expand).
	// This is a smoke test — the actual layout behaviour is CSS-only
	// and can't be exercised from Go, but we can verify the right
	// selectors are in the rendered page.
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/session/-test-project/test-session-123", nil)
	w := httptest.NewRecorder()
	handleSession(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("handleSession returned %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()

	if !strings.Contains(body, `.panel-nav.session-nav {`) {
		t.Error("expected .panel-nav.session-nav CSS block (narrow default state)")
	}
	if !strings.Contains(body, `width: 56px;`) {
		t.Error("expected collapsed 56px width for session-nav")
	}
	if !strings.Contains(body, `.panel-nav.session-nav:hover`) {
		t.Error("expected hover-expand rule on session-nav")
	}
	if !strings.Contains(body, `width: 260px;`) {
		t.Error("expected expanded 260px width on hover")
	}
}

func TestHandleSession_SpendSectionShowsTokenBreakdownForUnpricedModel(t *testing.T) {
	// Session with an unknown model still renders the breakdown (token-only),
	// because seeing which turns used the most tokens is still useful even
	// when cost can't be computed.
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "projects", "-unpriced-project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `{"type":"user","timestamp":"2026-04-01T10:00:00Z","uuid":"u1","message":{"content":"hi"}}
{"type":"assistant","timestamp":"2026-04-01T10:00:01Z","uuid":"a1","parentUuid":"u1","message":{"role":"assistant","content":"hi","model":"gpt-4","usage":{"input_tokens":100,"output_tokens":50}}}
`
	if err := os.WriteFile(filepath.Join(projectDir, "unpriced-session-abc.jsonl"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/session/-unpriced-project/unpriced-session-abc", nil)
	w := httptest.NewRecorder()
	handleSession(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("handleSession returned %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()

	if !strings.Contains(body, `info-section-spend`) {
		t.Error("expected info-section-spend even for unpriced models (token-only breakdown is useful)")
	}
	if !strings.Contains(body, `No pricing match`) {
		t.Error("expected 'No pricing match' note when model is unknown")
	}
	// Cost row in Tokens section should NOT appear (no cost resolved)
	if strings.Contains(body, `"info-row info-cost"`) {
		t.Error("info-cost row should NOT render when session cost is 0")
	}
}

func TestHandleAPIExport_JSON(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/api/export/-test-project/test-session-123?format=json", nil)
	w := httptest.NewRecorder()

	handleAPIExport(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleAPIExport returned %d, want %d", w.Code, http.StatusOK)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
}

func TestHandleAPIExport_Markdown(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/api/export/-test-project/test-session-123?format=md", nil)
	w := httptest.NewRecorder()

	handleAPIExport(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleAPIExport returned %d, want %d", w.Code, http.StatusOK)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/markdown") {
		t.Errorf("Content-Type = %q, want text/markdown*", contentType)
	}
}

func TestHandleAPIExport_NotFound(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/api/export/-test-project/nonexistent?format=json", nil)
	w := httptest.NewRecorder()

	handleAPIExport(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("handleAPIExport for nonexistent returned %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleStar(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	// Initialize db for stars
	dbPath := filepath.Join(dir, "ccx.db")
	if err := db.Init(dbPath); err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	body := strings.NewReader(`{"action":"add","type":"session","target_id":"test-session-123","project_id":"-test-project"}`)
	req := httptest.NewRequest("POST", "/api/star", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handleStar(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleStar returned %d, want %d. Body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestHandleGetStars(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	// Initialize db for stars
	dbPath := filepath.Join(dir, "ccx.db")
	if err := db.Init(dbPath); err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	req := httptest.NewRequest("GET", "/api/stars", nil)
	w := httptest.NewRecorder()

	handleGetStars(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleGetStars returned %d, want %d", w.Code, http.StatusOK)
	}

	var stars []any
	if err := json.Unmarshal(w.Body.Bytes(), &stars); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
}

func TestHandleAPISessions_WithProviderFilter(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/api/sessions/-test-project?provider=codex", nil)
	w := httptest.NewRecorder()

	handleAPISessions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("returned %d, want %d", w.Code, http.StatusOK)
	}

	var sessions []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	// claude backend sessions filtered by codex → 0 results
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions (wrong provider), got %d", len(sessions))
	}
}

func TestHandleAPISessions_WithQueryFilter(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/api/sessions/-test-project?q=nonexistent-query-xyz", nil)
	w := httptest.NewRecorder()

	handleAPISessions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("returned %d, want %d", w.Code, http.StatusOK)
	}

	var sessions []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions (query miss), got %d", len(sessions))
	}
}

func TestHandleAPISessions_NoFilterReturnsAll(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/api/sessions/-test-project", nil)
	w := httptest.NewRecorder()

	handleAPISessions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("returned %d, want %d", w.Code, http.StatusOK)
	}

	var sessions []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session (no filter), got %d", len(sessions))
	}
	// Check new fields are present
	s := sessions[0]
	if _, ok := s["provider"]; !ok {
		t.Error("missing provider field")
	}
	if _, ok := s["messages"]; !ok {
		t.Error("missing messages field")
	}
}

func TestParseSessionFilter(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		wantProv string
		wantQ    string
	}{
		{"empty", "", "", ""},
		{"provider cc", "provider=cc", "claude-code", ""},
		{"provider codex", "provider=codex", "codex", ""},
		{"query", "q=auth+bug", "", "auth bug"},
		{"combined", "provider=cx&q=test&model=opus", "codex", "test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, _ := url.Parse("/api/sessions/proj?" + tt.query)
			req := &http.Request{URL: u}
			f := parseSessionFilter(req)
			if f.Provider != tt.wantProv {
				t.Errorf("Provider = %q, want %q", f.Provider, tt.wantProv)
			}
			if f.Query != tt.wantQ {
				t.Errorf("Query = %q, want %q", f.Query, tt.wantQ)
			}
		})
	}
}

func TestHandleSettings(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/settings", nil)
	w := httptest.NewRecorder()

	handleSettings(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("returned %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Providers") {
		t.Error("settings page should contain Providers section")
	}
	if !strings.Contains(body, "ccx Configuration") {
		t.Error("settings page should contain ccx Configuration section")
	}
	if !strings.Contains(body, "Claude Code") {
		t.Error("settings page should mention Claude Code provider")
	}
	if !strings.Contains(body, "Codex") {
		t.Error("settings page should mention Codex provider")
	}
}

func TestHandleAPIFile_AllowsAgentsFile(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	agentsDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}
	agentFile := filepath.Join(agentsDir, "agent.md")
	if err := os.WriteFile(agentFile, []byte("agent content"), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/file?path="+url.QueryEscape(agentFile), nil)
	w := httptest.NewRecorder()

	handleAPIFile(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("handleAPIFile returned %d, want %d. Body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["content"] != "agent content" {
		t.Fatalf("content = %q, want %q", resp["content"], "agent content")
	}
}

func TestHandleAPIFile_AllowsConfigFile(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	configFile := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configFile, []byte("model = \"gpt-5.4\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/file?path="+url.QueryEscape(configFile), nil)
	w := httptest.NewRecorder()

	handleAPIFile(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("handleAPIFile returned %d, want %d. Body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["content"] != "model = \"gpt-5.4\"\n" {
		t.Fatalf("content = %q, want config.toml content", resp["content"])
	}
}

func TestHandleAPISearchFindsMemory(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	// Create a CLAUDE.md in the test home
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Project rules\nAlways use snake_case\n"), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/search?q=CLAUDE", nil)
	w := httptest.NewRecorder()

	handleAPISearch(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("returned %d", w.Code)
	}

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	results, ok := result["results"].([]any)
	if !ok {
		t.Fatal("results not an array")
	}

	found := false
	for _, r := range results {
		rm := r.(map[string]any)
		if rm["type"] == "memory" {
			found = true
			break
		}
	}
	if !found {
		t.Error("search for 'CLAUDE' should find memory file CLAUDE.md")
	}
}

func TestHandleAPISearchMemoryContent(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	memDir := filepath.Join(dir, "projects", "-test-project", "memory")
	if err := os.MkdirAll(memDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(memDir, "topic.md"), []byte("# Unique Search Term xyzzy42\n"), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/search?q=xyzzy42", nil)
	w := httptest.NewRecorder()

	handleAPISearch(w, req)

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	results := result["results"].([]any)

	found := false
	for _, r := range results {
		rm := r.(map[string]any)
		if rm["type"] == "memory" && rm["snippet"] != nil && rm["snippet"] != "" {
			found = true
		}
	}
	if !found {
		t.Error("search for 'xyzzy42' should find memory file by content with snippet")
	}
}

func TestHandleProjectWithMemory(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	memDir := filepath.Join(dir, "projects", "-test-project", "memory")
	if err := os.MkdirAll(memDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(memDir, "MEMORY.md"), []byte("# Memory Index\n- [topic](topic.md)\n"), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/project/-test-project", nil)
	w := httptest.NewRecorder()

	handleProject(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("returned %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "mem-section") {
		t.Error("project page should contain memory section")
	}
	if !strings.Contains(body, "MEMORY.md") {
		t.Error("project page should list MEMORY.md")
	}
}

func TestHandleProjectWithoutMemory(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/project/-test-project", nil)
	w := httptest.NewRecorder()

	handleProject(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("returned %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if strings.Contains(body, "mem-section") {
		t.Error("project page without memory files should NOT contain memory section")
	}
}

func TestHandleMemory(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	// Create a memory file
	memDir := filepath.Join(dir, "projects", "-test-project", "memory")
	if err := os.MkdirAll(memDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(memDir, "MEMORY.md"), []byte("- [test](test.md) — test memory\n"), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/memory", nil)
	w := httptest.NewRecorder()

	handleMemory(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("handleMemory returned %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Memory") {
		t.Error("page should contain Memory heading")
	}
	if !strings.Contains(body, "MEMORY.md") {
		t.Error("page should list MEMORY.md file")
	}
	if !strings.Contains(body, "Project Memory") {
		t.Error("page should have Project Memory section")
	}
}

func TestHandleMemoryEmpty(t *testing.T) {
	dir := t.TempDir()
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/memory", nil)
	w := httptest.NewRecorder()

	handleMemory(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("handleMemory returned %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "No memory") {
		t.Error("empty state should show 'No memory' message")
	}
}

func TestLoadMemories(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	// Create global instruction
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Global\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create rules dir
	rulesDir := filepath.Join(dir, "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "style.md"), []byte("# Style\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create project memory
	memDir := filepath.Join(dir, "projects", "-test-project", "memory")
	if err := os.MkdirAll(memDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(memDir, "MEMORY.md"), []byte("index\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(memDir, "topic.md"), []byte("topic\n"), 0644); err != nil {
		t.Fatal(err)
	}

	data := loadMemories()

	if len(data.Global) < 1 {
		t.Error("expected at least 1 global instruction (CLAUDE.md)")
	}
	if len(data.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(data.Rules))
	}
	if len(data.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(data.Projects))
	}
	if len(data.Projects[0].Files) != 2 {
		t.Errorf("expected 2 memory files, got %d", len(data.Projects[0].Files))
	}
	if data.TotalFiles < 4 {
		t.Errorf("TotalFiles = %d, expected at least 4", data.TotalFiles)
	}
}

func TestHandleAPIFile_AllowsProjectsFile(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	// projects/ is now an allowed root (for memory file access)
	sessionFile := filepath.Join(dir, "projects", "-test-project", "test-session-123.jsonl")
	req := httptest.NewRequest("GET", "/api/file?path="+url.QueryEscape(sessionFile), nil)
	w := httptest.NewRecorder()

	handleAPIFile(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("handleAPIFile returned %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleAPIFile_DeniesPrefixConfusion(t *testing.T) {
	dir := t.TempDir()
	setTestBackend(dir)

	agentsDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	evilDir := dir + "_evil"
	if err := os.MkdirAll(evilDir, 0755); err != nil {
		t.Fatal(err)
	}
	evilFile := filepath.Join(evilDir, "evil.md")
	if err := os.WriteFile(evilFile, []byte("nope"), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/file?path="+url.QueryEscape(evilFile), nil)
	w := httptest.NewRecorder()

	handleAPIFile(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("handleAPIFile returned %d, want %d. Body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestHandleAPIFile_DeniesSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	setTestBackend(dir)

	agentsDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	outsideDir := filepath.Join(dir, "outside")
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}

	linkPath := filepath.Join(agentsDir, "link.txt")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/file?path="+url.QueryEscape(linkPath), nil)
	w := httptest.NewRecorder()

	handleAPIFile(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("handleAPIFile returned %d, want %d. Body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}
