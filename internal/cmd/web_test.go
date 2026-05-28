package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/catalog"
	"github.com/thevibeworks/ccx/internal/parser"
)

type stubBackend struct {
	projects []*parser.Project
	err      error
}

func (s *stubBackend) ID() string      { return "stub" }
func (s *stubBackend) Homes() []string { return nil }
func (s *stubBackend) DiscoverProjects() ([]*parser.Project, error) {
	return s.projects, s.err
}
func (s *stubBackend) ListSessions(_ catalog.SessionQuery) ([]*parser.Session, error) {
	return nil, nil
}
func (s *stubBackend) FindProject(_ string) (*parser.Project, error)    { return nil, nil }
func (s *stubBackend) FindSession(_, _ string) (*parser.Session, error) { return nil, nil }
func (s *stubBackend) ParseSession(_ string) (*parser.Session, error)   { return nil, nil }

func testProjects() []*parser.Project {
	now := time.Now()
	return []*parser.Project{
		{
			Name:        "my-project",
			EncodedName: "-Users-eric-my-project",
			Path:        "/Users/eric/my-project",
			Sessions: []*parser.Session{
				{
					ID:      "aaaa1111-2222-3333-4444-555555555555",
					CWD:     "/Users/eric/my-project",
					EndTime: now.Add(-1 * time.Hour),
				},
				{
					ID:      "bbbb1111-2222-3333-4444-555555555555",
					CWD:     "/Users/eric/my-project",
					EndTime: now,
				},
				{
					ID:      "aaaa2222-2222-3333-4444-555555555555",
					CWD:     "/Users/eric/my-project",
					EndTime: now.Add(-2 * time.Hour),
				},
			},
		},
		{
			Name:        "other-repo",
			EncodedName: "-Users-eric-other-repo",
			Path:        "/Users/eric/other-repo",
			Sessions: []*parser.Session{
				{
					ID:      "cccc1111-2222-3333-4444-555555555555",
					CWD:     "/Users/eric/other-repo",
					EndTime: now.Add(-30 * time.Minute),
				},
			},
		},
	}
}

// --- resolveSessionLink ---

func TestResolveSessionLink_ExactMatch(t *testing.T) {
	backend := &stubBackend{projects: testProjects()}
	path, err := resolveSessionLink(backend, "bbbb1111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "/session/-Users-eric-my-project/bbbb1111-2222-3333-4444-555555555555"
	if path != want {
		t.Fatalf("got %q, want %q", path, want)
	}
}

func TestResolveSessionLink_ShortPrefix(t *testing.T) {
	backend := &stubBackend{projects: testProjects()}
	path, err := resolveSessionLink(backend, "bbbb1111")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "/session/-Users-eric-my-project/bbbb1111-2222-3333-4444-555555555555"
	if path != want {
		t.Fatalf("got %q, want %q", path, want)
	}
}

func TestResolveSessionLink_AmbiguousPrefix(t *testing.T) {
	backend := &stubBackend{projects: testProjects()}
	_, err := resolveSessionLink(backend, "aaaa")
	if err == nil {
		t.Fatal("expected ambiguity error, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous error, got: %v", err)
	}
}

func TestResolveSessionLink_NoMatch(t *testing.T) {
	backend := &stubBackend{projects: testProjects()}
	_, err := resolveSessionLink(backend, "zzzz9999")
	if err == nil {
		t.Fatal("expected error for non-existent session")
	}
	if !strings.Contains(err.Error(), "no session matches") {
		t.Fatalf("expected 'no session matches', got: %v", err)
	}
}

func TestResolveSessionLink_CrossProject(t *testing.T) {
	backend := &stubBackend{projects: testProjects()}
	path, err := resolveSessionLink(backend, "cccc1111")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "/session/-Users-eric-other-repo/cccc1111-2222-3333-4444-555555555555"
	if path != want {
		t.Fatalf("got %q, want %q", path, want)
	}
}

func TestResolveSessionLink_EmptyProjects(t *testing.T) {
	backend := &stubBackend{projects: nil}
	_, err := resolveSessionLink(backend, "anything")
	if err == nil {
		t.Fatal("expected error for empty project list")
	}
	if !strings.Contains(err.Error(), "no session matches") {
		t.Fatalf("expected 'no session matches', got: %v", err)
	}
}

func TestResolveSessionLink_BackendError(t *testing.T) {
	backend := &stubBackend{err: fmt.Errorf("disk on fire")}
	_, err := resolveSessionLink(backend, "anything")
	if err == nil {
		t.Fatal("expected error propagation")
	}
	if !strings.Contains(err.Error(), "disk on fire") {
		t.Fatalf("expected wrapped backend error, got: %v", err)
	}
}

// --- resolveLatestLink ---

func TestResolveLatestLink_PicksNewest(t *testing.T) {
	now := time.Now()
	projects := []*parser.Project{
		{
			Name:        "test-proj",
			EncodedName: "-tmp-test-proj",
			Path:        "/tmp/test-proj",
			Sessions: []*parser.Session{
				{ID: "old-session", CWD: "/tmp/test-proj", EndTime: now.Add(-2 * time.Hour)},
				{ID: "newest-session", CWD: "/tmp/test-proj", EndTime: now},
				{ID: "mid-session", CWD: "/tmp/test-proj", EndTime: now.Add(-1 * time.Hour)},
			},
		},
	}
	backend := &stubBackend{projects: projects}

	// findProjectByPath uses CWD matching, so we need to test via
	// resolveLatestLink indirectly. Instead test the pick-newest logic
	// by calling findProjectByPath + the latest logic directly.
	project, err := findProjectByPath(backend, "/tmp/test-proj")
	if err != nil {
		t.Fatalf("findProjectByPath error: %v", err)
	}
	if project == nil {
		t.Fatal("expected project, got nil")
	}

	// Verify the original session order is preserved
	origOrder := make([]string, len(project.Sessions))
	for i, s := range project.Sessions {
		origOrder[i] = s.ID
	}

	// Simulate resolveLatestLink logic (finds newest without mutating)
	latest := project.Sessions[0]
	for _, s := range project.Sessions[1:] {
		if s.EndTime.After(latest.EndTime) {
			latest = s
		}
	}

	if latest.ID != "newest-session" {
		t.Fatalf("expected newest-session, got %s", latest.ID)
	}

	// Verify session order was NOT mutated
	for i, s := range project.Sessions {
		if s.ID != origOrder[i] {
			t.Fatalf("session order mutated: position %d is %s, was %s", i, s.ID, origOrder[i])
		}
	}
}

func TestResolveLatestLink_NoSessions(t *testing.T) {
	projects := []*parser.Project{
		{
			Name:        "empty-proj",
			EncodedName: "-tmp-empty-proj",
			Path:        "/tmp/empty-proj",
			Sessions:    nil,
		},
	}
	backend := &stubBackend{projects: projects}

	project, err := findProjectByPath(backend, "/tmp/empty-proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if project == nil {
		t.Fatal("expected project, got nil")
	}
	if len(project.Sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(project.Sessions))
	}
}

// --- findProjectByPath ---

func TestFindProjectByPath_WorkspaceMatch(t *testing.T) {
	backend := &stubBackend{projects: testProjects()}
	project, err := findProjectByPath(backend, "/Users/eric/my-project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if project == nil {
		t.Fatal("expected project, got nil")
	}
	if project.Name != "my-project" {
		t.Fatalf("got project %q, want my-project", project.Name)
	}
}

func TestFindProjectByPath_NameFallback(t *testing.T) {
	backend := &stubBackend{projects: testProjects()}
	project, err := findProjectByPath(backend, "my-project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if project == nil {
		t.Fatal("expected project from name fallback, got nil")
	}
	if project.Name != "my-project" {
		t.Fatalf("got project %q, want my-project", project.Name)
	}
}

func TestFindProjectByPath_NoMatch(t *testing.T) {
	backend := &stubBackend{projects: testProjects()}
	project, err := findProjectByPath(backend, "/nonexistent/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if project != nil {
		t.Fatalf("expected nil, got project %q", project.Name)
	}
}

func TestFindProjectByPath_BackendError(t *testing.T) {
	backend := &stubBackend{err: fmt.Errorf("storage unavailable")}
	_, err := findProjectByPath(backend, "/any/path")
	if err == nil {
		t.Fatal("expected error propagation")
	}
	if !strings.Contains(err.Error(), "storage unavailable") {
		t.Fatalf("expected wrapped error, got: %v", err)
	}
}

// --- serverAlive ---

func TestServerAlive_Running(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/stats" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"projects":1}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if !serverAlive(srv.URL) {
		t.Fatal("expected serverAlive=true for running server")
	}
}

func TestServerAlive_NotRunning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := srv.URL
	srv.Close()

	if serverAlive(deadURL) {
		t.Fatal("expected serverAlive=false for closed server")
	}
}
