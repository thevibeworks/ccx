package claude

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thevibeworks/ccx/internal/catalog"
)

func TestListSessionsScopesClaudeSessionsByWorkspace(t *testing.T) {
	home := t.TempDir()
	projectsDir := filepath.Join(home, "projects")

	currentDir := filepath.Join(projectsDir, "-tmp-work-current")
	otherDir := filepath.Join(projectsDir, "-tmp-work-other")
	writeClaudeSession(t, filepath.Join(currentDir, "current.jsonl"), "/tmp/work/current", "current session")
	writeClaudeSession(t, filepath.Join(otherDir, "other.jsonl"), "/tmp/work/other", "other session")

	backend := NewWithProjectsDir(home, projectsDir)
	got, err := backend.ListSessions(catalog.SessionQuery{
		Scope:         catalog.ScopeWorkspace,
		WorkspacePath: "/tmp/work/current",
	})
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "current" {
		t.Fatalf("got %+v, want only current session", got)
	}
	if got[0].Provider != ProviderID {
		t.Fatalf("Provider = %q, want %q", got[0].Provider, ProviderID)
	}
}

func TestListSessionsClaudeAllPreservesGlobalBehavior(t *testing.T) {
	home := t.TempDir()
	projectsDir := filepath.Join(home, "projects")
	writeClaudeSession(t, filepath.Join(projectsDir, "-tmp-work-current", "current.jsonl"), "/tmp/work/current", "current session")
	writeClaudeSession(t, filepath.Join(projectsDir, "-tmp-work-other", "other.jsonl"), "/tmp/work/other", "other session")

	backend := NewWithProjectsDir(home, projectsDir)
	got, err := backend.ListSessions(catalog.SessionQuery{Scope: catalog.ScopeAll})
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

func TestListSessionsClaudeWorkspaceFastPathUsesClaudeSanitizer(t *testing.T) {
	home := t.TempDir()
	projectsDir := filepath.Join(home, "projects")
	writeClaudeSession(t, filepath.Join(projectsDir, "-tmp-work-a-b", "current.jsonl"), "/tmp/work/a+b", "current session")

	backend := NewWithProjectsDir(home, projectsDir)
	got, err := backend.ListSessions(catalog.SessionQuery{
		Scope:         catalog.ScopeWorkspace,
		WorkspacePath: "/tmp/work/a+b",
	})
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "current" {
		t.Fatalf("got %+v, want session from sanitized -tmp-work-a-b directory", got)
	}
}

func writeClaudeSession(t *testing.T, path, cwd, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	data := `{"type":"user","timestamp":"2026-05-21T10:00:00Z","uuid":"u1","cwd":"` + cwd + `","message":{"role":"user","content":"` + content + `"}}
{"type":"assistant","timestamp":"2026-05-21T10:00:01Z","uuid":"a1","parentUuid":"u1","cwd":"` + cwd + `","message":{"role":"assistant","content":"ok"}}
`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}
