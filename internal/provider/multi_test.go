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
