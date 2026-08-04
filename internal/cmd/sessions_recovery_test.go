package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thevibeworks/ccx/internal/catalog"
	"github.com/thevibeworks/ccx/internal/parser"
)

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = old })

	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

type stubSessionLister struct {
	byPath map[string][]*parser.Session
	seen   []catalog.SessionQuery
}

func (s *stubSessionLister) ListSessions(q catalog.SessionQuery) ([]*parser.Session, error) {
	s.seen = append(s.seen, q)
	return s.byPath[q.WorkspacePath], nil
}

func TestSessionsFromParentsFindsAncestorWorkspace(t *testing.T) {
	root := filepath.Join("/", "wrk", "proj")
	lister := &stubSessionLister{byPath: map[string][]*parser.Session{
		root: {{ID: "s1"}},
	}}

	sessions, matched := sessionsFromParents(lister, catalog.SessionQuery{}, filepath.Join(root, "sub", "deeper"))
	if len(sessions) != 1 || sessions[0].ID != "s1" {
		t.Fatalf("sessions = %v, want the ancestor workspace session", sessions)
	}
	if matched != root {
		t.Fatalf("matched = %q, want %q", matched, root)
	}
	for _, q := range lister.seen {
		if q.Scope != catalog.ScopeWorkspace {
			t.Fatalf("walk queried scope %q, want workspace", q.Scope)
		}
	}
}

func TestSessionsFromParentsGivesUpAtRoot(t *testing.T) {
	lister := &stubSessionLister{}
	sessions, matched := sessionsFromParents(lister, catalog.SessionQuery{}, "/nowhere/at/all")
	if sessions != nil || matched != "" {
		t.Fatalf("got %v %q, want no recovery", sessions, matched)
	}
	if len(lister.seen) == 0 {
		t.Fatal("walk never queried any ancestor")
	}
}

func TestWalkStartPath(t *testing.T) {
	wsQuery := catalog.SessionQuery{Scope: catalog.ScopeWorkspace, WorkspacePath: "/wrk/proj/sub"}
	if got := walkStartPath(wsQuery, ""); got != "/wrk/proj/sub" {
		t.Fatalf("bare workspace invocation: got %q", got)
	}
	if got := walkStartPath(catalog.SessionQuery{Scope: catalog.ScopeProject}, "my-slug"); got != "" {
		t.Fatalf("plain slug must not walk: got %q", got)
	}
	if got := walkStartPath(catalog.SessionQuery{Scope: catalog.ScopeProject}, "/abs/path/sub"); got != "/abs/path/sub" {
		t.Fatalf("absolute path arg: got %q", got)
	}
	dir := t.TempDir()
	t.Chdir(filepath.Dir(dir))
	rel := filepath.Base(dir)
	got := walkStartPath(catalog.SessionQuery{Scope: catalog.ScopeProject}, rel)
	if got == "" || !filepath.IsAbs(got) || filepath.Base(got) != rel {
		t.Fatalf("existing relative dir should walk from its abs path: got %q", got)
	}
}

type stubProjectDiscoverer struct {
	projects []*parser.Project
}

func (s *stubProjectDiscoverer) DiscoverProjects() ([]*parser.Project, error) {
	return s.projects, nil
}

func TestSuggestClosestProjectsRanksTokenOverlap(t *testing.T) {
	disc := &stubProjectDiscoverer{projects: []*parser.Project{
		{Name: "260715-ccx-session-watch"},
		{Name: "260324-ccx-codex"},
		{Name: "unrelated-thing"},
	}}

	out := captureStderr(t, func() {
		suggestClosestProjects(disc, "/wrk/WIP/260715_ccx-session-watch")
	})
	if !strings.Contains(out, "260715-ccx-session-watch") {
		t.Fatalf("suggestion missing best match: %q", out)
	}
	if strings.Contains(out, "unrelated-thing") {
		t.Fatalf("suggestion includes zero-overlap project: %q", out)
	}
	first := strings.Index(out, "260715-ccx-session-watch")
	second := strings.Index(out, "260324-ccx-codex")
	if second >= 0 && second < first {
		t.Fatalf("weaker match ranked first: %q", out)
	}
}
