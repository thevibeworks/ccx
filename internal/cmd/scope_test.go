package cmd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/catalog"
	"github.com/thevibeworks/ccx/internal/parser"
)

type scopeBackend struct {
	sessions []*parser.Session
}

func (b *scopeBackend) ID() string      { return "scope" }
func (b *scopeBackend) Homes() []string { return nil }
func (b *scopeBackend) DiscoverProjects() ([]*parser.Project, error) {
	return nil, nil
}
func (b *scopeBackend) ListSessions(query catalog.SessionQuery) ([]*parser.Session, error) {
	project := &parser.Project{Name: "repo", Path: query.WorkspacePath, Sessions: b.sessions}
	if query.Scope == catalog.ScopeAll {
		project.Path = ""
	}
	return catalog.ApplySessionQuery([]*parser.Project{project}, query), nil
}
func (b *scopeBackend) FindProject(string) (*parser.Project, error) { return nil, nil }
func (b *scopeBackend) FindSession(string, string) (*parser.Session, error) {
	return nil, nil
}
func (b *scopeBackend) ParseSession(string) (*parser.Session, error) { return nil, nil }

func TestResolveSessionInQueryDoesNotEscapeWorkspaceScope(t *testing.T) {
	dir := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	query, err := currentWorkspaceQuery()
	if err != nil {
		t.Fatalf("currentWorkspaceQuery() error: %v", err)
	}

	backend := &scopeBackend{sessions: []*parser.Session{
		{ID: "abcdef-current", CWD: dir, EndTime: time.Now()},
		{ID: "abcdef-other", CWD: "/tmp/other", EndTime: time.Now().Add(time.Minute)},
	}}

	got, err := resolveSessionInQuery(backend, query, "abcdef")
	if err != nil {
		t.Fatalf("resolveSessionInQuery() error: %v", err)
	}
	if got == nil || got.ID != "abcdef-current" {
		t.Fatalf("got %+v, want current workspace match", got)
	}
}

func TestResolveSessionInQueryReportsScopedAmbiguity(t *testing.T) {
	dir := t.TempDir()
	backend := &scopeBackend{sessions: []*parser.Session{
		{ID: "abcdef123456-one", CWD: dir},
		{ID: "abcdef123456-two", CWD: dir},
		{ID: "abcdef123456-other", CWD: "/tmp/other"},
	}}

	_, err := resolveSessionInQuery(backend, catalog.SessionQuery{
		Scope:         catalog.ScopeWorkspace,
		WorkspacePath: dir,
	}, "abcdef123456")

	var ambiguous *catalog.AmbiguousSessionError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("err = %v, want AmbiguousSessionError", err)
	}
	if len(ambiguous.Matches) != 2 {
		t.Fatalf("matches = %d, want 2 scoped matches", len(ambiguous.Matches))
	}
	if !strings.Contains(err.Error(), "abcdef123456-one") || !strings.Contains(err.Error(), "abcdef123456-two") {
		t.Fatalf("ambiguous error should list scoped matches, got %q", err.Error())
	}
}

func TestResolveSessionInQuerySupportsScopedIndex(t *testing.T) {
	now := time.Now()
	backend := &scopeBackend{sessions: []*parser.Session{
		{ID: "old", CWD: "/tmp/repo", EndTime: now.Add(-time.Hour)},
		{ID: "new", CWD: "/tmp/repo", EndTime: now},
	}}

	got, err := resolveSessionInQuery(backend, catalog.SessionQuery{
		Scope:         catalog.ScopeWorkspace,
		WorkspacePath: "/tmp/repo",
	}, "@1")
	if err != nil {
		t.Fatalf("resolveSessionInQuery(@1) error: %v", err)
	}
	if got == nil || got.ID != "new" {
		t.Fatalf("got %+v, want newest session", got)
	}
}

func TestLatestTraceSessionIsNonInteractiveLatest(t *testing.T) {
	now := time.Now()
	backend := &scopeBackend{sessions: []*parser.Session{
		{ID: "old", CWD: "/tmp/repo", EndTime: now.Add(-time.Hour)},
		{ID: "new", CWD: "/tmp/repo", EndTime: now},
	}}

	got, err := latestTraceSession(backend, true)
	if err != nil {
		t.Fatalf("latestTraceSession() error: %v", err)
	}
	if got == nil || got.ID != "new" {
		t.Fatalf("got %+v, want newest session", got)
	}
}

func TestFindGitRootForSessionFallsBackWithWarning(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	dir := t.TempDir()
	runGit(t, dir, "init")

	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	stale := filepath.Join(dir, "missing")
	root, warnings := findGitRootForSession(&parser.Session{CWD: stale})
	if root != dir {
		t.Fatalf("root = %q, want %q", root, dir)
	}
	if len(warnings) != 1 || warnings[0].Kind != "session_git_root_missing" {
		t.Fatalf("warnings = %+v, want session_git_root_missing", warnings)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
