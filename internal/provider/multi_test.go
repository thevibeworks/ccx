package provider

import (
	"fmt"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

type mockBackend struct {
	id       string
	homes    []string
	projects []*parser.Project
	sessions map[string]*parser.Session
}

func (m *mockBackend) ID() string      { return m.id }
func (m *mockBackend) Homes() []string { return m.homes }

func (m *mockBackend) DiscoverProjects() ([]*parser.Project, error) {
	return m.projects, nil
}

func (m *mockBackend) FindProject(name string) (*parser.Project, error) {
	for _, p := range m.projects {
		if p.Name == name || p.EncodedName == name {
			return p, nil
		}
	}
	return nil, nil
}

func (m *mockBackend) FindSession(projectName, sessionID string) (*parser.Session, error) {
	key := projectName + "/" + sessionID
	if s, ok := m.sessions[key]; ok {
		return s, nil
	}
	if s, ok := m.sessions[sessionID]; ok {
		return s, nil
	}
	return nil, nil
}

func (m *mockBackend) ParseSession(filePath string) (*parser.Session, error) {
	if s, ok := m.sessions[filePath]; ok {
		return s, nil
	}
	return nil, fmt.Errorf("not found: %s", filePath)
}

func TestMultiID(t *testing.T) {
	m := NewMulti()
	if m.ID() != "multi" {
		t.Fatalf("ID() = %q, want %q", m.ID(), "multi")
	}
}

func TestMultiHomesMergesBackends(t *testing.T) {
	b1 := &mockBackend{homes: []string{"/home/user/.claude"}}
	b2 := &mockBackend{homes: []string{"/home/user/.codex"}}
	m := NewMulti(b1, b2)
	homes := m.Homes()
	if len(homes) != 2 {
		t.Fatalf("len(Homes()) = %d, want 2", len(homes))
	}
}

func TestMultiDiscoverProjectsMergesByEncodedName(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Hour)

	b1 := &mockBackend{
		projects: []*parser.Project{
			{
				Name:         "myproject",
				EncodedName:  "home-user-myproject",
				Provider:     "claude-code",
				LastModified: earlier,
				Sessions: []*parser.Session{
					{ID: "cc-session-1", EndTime: earlier},
				},
			},
		},
	}
	b2 := &mockBackend{
		projects: []*parser.Project{
			{
				Name:         "myproject",
				EncodedName:  "home-user-myproject",
				Provider:     "codex",
				LastModified: now,
				Sessions: []*parser.Session{
					{ID: "cx-session-1", EndTime: now},
				},
			},
		},
	}

	m := NewMulti(b1, b2)
	projects, err := m.DiscoverProjects()
	if err != nil {
		t.Fatalf("DiscoverProjects() error: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 merged project, got %d", len(projects))
	}
	p := projects[0]
	if len(p.Sessions) != 2 {
		t.Fatalf("expected 2 sessions merged, got %d", len(p.Sessions))
	}
	if p.Provider != "" {
		t.Fatalf("merged project provider should be empty, got %q", p.Provider)
	}
	if !p.LastModified.Equal(now) {
		t.Fatal("LastModified should be the later timestamp")
	}
	if p.Sessions[0].ID != "cx-session-1" {
		t.Fatalf("sessions should be sorted by EndTime desc, first = %q", p.Sessions[0].ID)
	}
}

func TestMultiDiscoverProjectsPrefersWorkspacePathForMergedProject(t *testing.T) {
	now := time.Now()

	b1 := &mockBackend{
		projects: []*parser.Project{
			{
				Name:         "repo",
				EncodedName:  "home-user-src-repo",
				Path:         "/home/user/.claude/projects/home-user-src-repo",
				Provider:     "claude-code",
				LastModified: now.Add(-time.Hour),
				Sessions: []*parser.Session{
					{ID: "cc-session-1", Provider: "claude-code", CWD: "/home/user/src/repo", EndTime: now.Add(-time.Hour)},
				},
			},
		},
	}
	b2 := &mockBackend{
		projects: []*parser.Project{
			{
				Name:         "repo",
				EncodedName:  "home-user-src-repo",
				Path:         "/home/user/src/repo",
				Provider:     "codex",
				LastModified: now,
				Sessions: []*parser.Session{
					{ID: "cx-session-1", Provider: "codex", CWD: "/home/user/src/repo", EndTime: now},
				},
			},
		},
	}

	m := NewMulti(b1, b2)

	project, err := m.FindProject("/home/user/src/repo")
	if err != nil {
		t.Fatalf("FindProject() error: %v", err)
	}
	if project == nil {
		t.Fatal("expected merged project to be searchable by workspace path")
	}
	if project.Path != "/home/user/src/repo" {
		t.Fatalf("project.Path = %q, want /home/user/src/repo", project.Path)
	}
}

func TestMultiDiscoverProjectsKeepsSeparate(t *testing.T) {
	b1 := &mockBackend{
		projects: []*parser.Project{
			{Name: "alpha", EncodedName: "home-user-alpha", Provider: "claude-code"},
		},
	}
	b2 := &mockBackend{
		projects: []*parser.Project{
			{Name: "beta", EncodedName: "home-user-beta", Provider: "codex"},
		},
	}

	m := NewMulti(b1, b2)
	projects, err := m.DiscoverProjects()
	if err != nil {
		t.Fatalf("DiscoverProjects() error: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 separate projects, got %d", len(projects))
	}
}

func TestMultiFindProjectCaseInsensitive(t *testing.T) {
	b := &mockBackend{
		projects: []*parser.Project{
			{Name: "MyProject", EncodedName: "home-user-MyProject"},
		},
	}
	m := NewMulti(b)

	p, err := m.FindProject("myproject")
	if err != nil {
		t.Fatalf("FindProject() error: %v", err)
	}
	if p == nil {
		t.Fatal("expected to find project case-insensitively")
	}
}

func TestMultiFindProjectSubstring(t *testing.T) {
	b := &mockBackend{
		projects: []*parser.Project{
			{Name: "my-cool-project", EncodedName: "home-user-my-cool-project", Path: "/home/user/my-cool-project"},
		},
	}
	m := NewMulti(b)

	p, err := m.FindProject("cool")
	if err != nil {
		t.Fatalf("FindProject() error: %v", err)
	}
	if p == nil {
		t.Fatal("expected to find project by substring")
	}
}

func TestMultiFindProjectReturnsNilOnMiss(t *testing.T) {
	b := &mockBackend{projects: []*parser.Project{}}
	m := NewMulti(b)

	p, err := m.FindProject("nonexistent")
	if err != nil {
		t.Fatalf("FindProject() error: %v", err)
	}
	if p != nil {
		t.Fatal("expected nil for nonexistent project")
	}
}

func TestMultiFindSessionFirstBackendWins(t *testing.T) {
	s1 := &parser.Session{ID: "sess-1", Provider: "claude-code"}
	s2 := &parser.Session{ID: "sess-1", Provider: "codex"}

	b1 := &mockBackend{sessions: map[string]*parser.Session{"sess-1": s1}}
	b2 := &mockBackend{sessions: map[string]*parser.Session{"sess-1": s2}}

	m := NewMulti(b1, b2)
	s, err := m.FindSession("", "sess-1")
	if err != nil {
		t.Fatalf("FindSession() error: %v", err)
	}
	if s.Provider != "claude-code" {
		t.Fatalf("expected first backend to win, got provider %q", s.Provider)
	}
}

func TestMultiFindSessionReturnsNilOnMiss(t *testing.T) {
	b := &mockBackend{sessions: map[string]*parser.Session{}}
	m := NewMulti(b)

	s, err := m.FindSession("proj", "nonexistent")
	if err != nil {
		t.Fatalf("FindSession() error: %v", err)
	}
	if s != nil {
		t.Fatal("expected nil for nonexistent session")
	}
}

func TestMultiParseSessionRoutesByPath(t *testing.T) {
	s1 := &parser.Session{ID: "cc-sess", Provider: "claude-code"}
	s2 := &parser.Session{ID: "cx-sess", Provider: "codex"}

	b1 := &mockBackend{
		homes:    []string{"/home/user/.claude"},
		sessions: map[string]*parser.Session{"/home/user/.claude/projects/test/sess.jsonl": s1},
	}
	b2 := &mockBackend{
		homes:    []string{"/home/user/.codex"},
		sessions: map[string]*parser.Session{"/home/user/.codex/sessions/sess.jsonl": s2},
	}

	m := NewMulti(b1, b2)

	s, err := m.ParseSession("/home/user/.claude/projects/test/sess.jsonl")
	if err != nil {
		t.Fatalf("ParseSession(claude path) error: %v", err)
	}
	if s.Provider != "claude-code" {
		t.Fatalf("expected claude-code, got %q", s.Provider)
	}

	s, err = m.ParseSession("/home/user/.codex/sessions/sess.jsonl")
	if err != nil {
		t.Fatalf("ParseSession(codex path) error: %v", err)
	}
	if s.Provider != "codex" {
		t.Fatalf("expected codex, got %q", s.Provider)
	}
}

func TestMultiParseSessionFallbackOnUnknownPath(t *testing.T) {
	s := &parser.Session{ID: "found", Provider: "codex"}
	b1 := &mockBackend{
		homes:    []string{"/home/user/.claude"},
		sessions: map[string]*parser.Session{},
	}
	b2 := &mockBackend{
		homes:    []string{"/home/user/.codex"},
		sessions: map[string]*parser.Session{"/tmp/random/sess.jsonl": s},
	}

	m := NewMulti(b1, b2)
	result, err := m.ParseSession("/tmp/random/sess.jsonl")
	if err != nil {
		t.Fatalf("ParseSession(fallback) error: %v", err)
	}
	if result == nil {
		t.Fatal("expected fallback to find session")
	}
	if result.Provider != "codex" {
		t.Fatalf("expected codex via fallback, got %q", result.Provider)
	}
}

func TestMultiParseSessionReturnsNilOnMiss(t *testing.T) {
	b := &mockBackend{
		homes:    []string{"/home/user/.claude"},
		sessions: map[string]*parser.Session{},
	}
	m := NewMulti(b)
	result, err := m.ParseSession("/nonexistent/path.jsonl")
	if err != nil {
		t.Fatalf("ParseSession() error: %v", err)
	}
	if result != nil {
		t.Fatal("expected nil for missing session")
	}
}

// TestMultiFindSession_CrossAgentMergedURL reproduces the regression
// where clicking a Codex session in a Workspace-merged row returned
// 404. The two backends encode the same cwd differently:
//
//	Claude Code: "-Users-eric-foo-bar"   (from ~/.claude/projects/ folder name)
//	Codex:       "Users-eric-foo_bar"    (from parser.EncodePath)
//
// DiscoverProjects merges them under one row, whose EncodedName is
// whichever backend landed first (Claude Code). URLs built from the
// merged row use Claude Code's form. When the user clicks a Codex
// session, per-backend FindSession can't resolve because Codex's
// FindProject(name) is looking up a name that doesn't match.
//
// After the fix, Multi.FindSession falls back to searching the
// merged project list by sessionID, so cross-agent URLs resolve
// to the right session regardless of which backend stored it.
func TestMultiFindSession_CrossAgentMergedURL(t *testing.T) {
	claudeSession := &parser.Session{
		ID:          "aaaaaaaa-cc",
		FilePath:    "/home/user/.claude/projects/-Users-eric-foo-bar/aaaa.jsonl",
		ProjectName: "-Users-eric-foo-bar",
		CWD:         "/Users/eric/foo_bar",
		Provider:    "claude-code",
		EndTime:     time.Now(),
	}
	codexSession := &parser.Session{
		ID:          "019d8f43-cx",
		FilePath:    "/home/user/.codex/sessions/2026/04/019d8f43.jsonl",
		ProjectName: "Users-eric-foo_bar",
		CWD:         "/Users/eric/foo_bar",
		Provider:    "codex",
		EndTime:     time.Now().Add(-time.Minute),
	}

	claudeBackend := &mockBackend{
		id:    "claude-code",
		homes: []string{"/home/user/.claude"},
		projects: []*parser.Project{{
			Name:         "foo_bar",
			EncodedName:  "-Users-eric-foo-bar", // Claude Code's encoding
			Path:         "/Users/eric/foo_bar",
			Sessions:     []*parser.Session{claudeSession},
			LastModified: claudeSession.EndTime,
		}},
		sessions: map[string]*parser.Session{
			"-Users-eric-foo-bar/aaaaaaaa-cc": claudeSession,
		},
	}
	codexBackend := &mockBackend{
		id:    "codex",
		homes: []string{"/home/user/.codex"},
		projects: []*parser.Project{{
			Name:         "foo_bar",
			EncodedName:  "Users-eric-foo_bar", // Codex's encoding (no leading dash, keeps _)
			Path:         "/Users/eric/foo_bar",
			Sessions:     []*parser.Session{codexSession},
			LastModified: codexSession.EndTime,
		}},
		sessions: map[string]*parser.Session{
			// Deliberately keyed under Codex's own encoded name, so
			// looking up by Claude Code's encoded name will MISS the
			// direct per-backend path and exercise the fallback.
			"Users-eric-foo_bar/019d8f43-cx": codexSession,
		},
	}

	m := NewMulti(claudeBackend, codexBackend)

	// 1. Merged discovery should produce ONE row for this cwd.
	projects, err := m.DiscoverProjects()
	if err != nil {
		t.Fatalf("DiscoverProjects: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("want 1 merged project, got %d", len(projects))
	}
	merged := projects[0]
	if len(merged.Sessions) != 2 {
		t.Errorf("merged project should carry both sessions, got %d", len(merged.Sessions))
	}

	// 2. URL built from the merged row uses Claude Code's encoded name.
	urlProjectName := merged.EncodedName
	if urlProjectName == "" {
		t.Fatal("merged EncodedName empty")
	}

	// 3. Fetching the Codex session via that URL must resolve. Before
	//    the fix, the per-backend FindSession calls would both miss
	//    (Claude Code doesn't know the Codex session ID; Codex doesn't
	//    know the Claude Code encoded name) and the handler 404'd.
	got, err := m.FindSession(urlProjectName, "019d8f43-cx")
	if err != nil {
		t.Fatalf("FindSession: %v", err)
	}
	if got == nil {
		t.Fatalf("FindSession returned nil for cross-agent URL (project=%q, session=019d8f43-cx)", urlProjectName)
	}
	if got.ID != "019d8f43-cx" {
		t.Errorf("got session ID %q, want 019d8f43-cx", got.ID)
	}
	if got.Provider != "codex" {
		t.Errorf("got provider %q, want codex", got.Provider)
	}

	// 4. The Claude Code session in the same merged row must ALSO
	//    still resolve via the same URL project name.
	got2, err := m.FindSession(urlProjectName, "aaaaaaaa-cc")
	if err != nil {
		t.Fatalf("FindSession claude: %v", err)
	}
	if got2 == nil {
		t.Fatal("claude code session should still resolve via merged URL")
	}
	if got2.Provider != "claude-code" {
		t.Errorf("got provider %q, want claude-code", got2.Provider)
	}
}

// TestMultiFindProject_MatchesAlternateEncoding ensures FindProject
// resolves a merged row when queried with EITHER backend's encoded
// name, not just whichever one landed first.
func TestMultiFindProject_MatchesAlternateEncoding(t *testing.T) {
	s := &parser.Session{
		ID:       "s1",
		FilePath: "/home/user/.codex/sessions/2026/s1.jsonl",
		CWD:      "/Users/eric/foo",
		EndTime:  time.Now(),
	}
	b := &mockBackend{
		id:    "codex",
		homes: []string{"/home/user/.codex"},
		projects: []*parser.Project{{
			Name:         "foo",
			EncodedName:  "Users-eric-foo",
			Path:         "/Users/eric/foo",
			Sessions:     []*parser.Session{s},
			LastModified: s.EndTime,
		}},
	}
	m := NewMulti(b)

	// Query using the OTHER (Claude Code-style) encoding.
	got, err := m.FindProject("-Users-eric-foo")
	if err != nil {
		t.Fatalf("FindProject: %v", err)
	}
	if got == nil {
		t.Fatal("FindProject should match the alternate encoding via canonical-path pass")
	}
	if got.Path != "/Users/eric/foo" {
		t.Errorf("got path %q, want /Users/eric/foo", got.Path)
	}
}
