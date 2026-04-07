package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverProjects(t *testing.T) {
	dir := t.TempDir()
	projectsDir := filepath.Join(dir, "projects")

	projDir := filepath.Join(projectsDir, "-home-dev-myapp")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	sessionFile := filepath.Join(projDir, "sess-001.jsonl")
	content := `{"type":"user","timestamp":"2026-01-01T10:00:00Z","uuid":"u1","message":{"role":"user","content":"hello"}}
{"type":"assistant","timestamp":"2026-01-01T10:00:01Z","uuid":"a1","parentUuid":"u1","message":{"role":"assistant","content":"hi"}}
`
	if err := os.WriteFile(sessionFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	projects, err := DiscoverProjects(projectsDir)
	if err != nil {
		t.Fatalf("DiscoverProjects() error: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].EncodedName != "-home-dev-myapp" {
		t.Errorf("EncodedName = %q", projects[0].EncodedName)
	}
	if len(projects[0].Sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(projects[0].Sessions))
	}
}

func TestDiscoverProjectsEmpty(t *testing.T) {
	dir := t.TempDir()
	projectsDir := filepath.Join(dir, "projects")
	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatal(err)
	}

	projects, err := DiscoverProjects(projectsDir)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(projects))
	}
}

func TestDiscoverProjectsNonexistentDir(t *testing.T) {
	projects, err := DiscoverProjects("/nonexistent/path")
	if err != nil {
		t.Fatalf("should return empty, not error: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected nil/empty, got %d", len(projects))
	}
}

func TestDiscoverProjectsSkipsAgentFiles(t *testing.T) {
	dir := t.TempDir()
	projectsDir := filepath.Join(dir, "projects")
	projDir := filepath.Join(projectsDir, "-home-dev-proj")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Main session
	main := filepath.Join(projDir, "sess-001.jsonl")
	if err := os.WriteFile(main, []byte(`{"type":"user","timestamp":"2026-01-01T10:00:00Z","uuid":"u1","message":{"role":"user","content":"hello"}}
`), 0644); err != nil {
		t.Fatal(err)
	}

	// Agent sidechain file (should be skipped)
	agent := filepath.Join(projDir, "agent-sess-001.jsonl")
	if err := os.WriteFile(agent, []byte(`{"type":"user","timestamp":"2026-01-01T10:00:00Z","uuid":"u2","message":{"role":"user","content":"agent"}}
`), 0644); err != nil {
		t.Fatal(err)
	}

	projects, err := DiscoverProjects(projectsDir)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if len(projects[0].Sessions) != 1 {
		t.Errorf("expected 1 session (agent file skipped), got %d", len(projects[0].Sessions))
	}
}

func TestFindProjectExact(t *testing.T) {
	dir := t.TempDir()
	projectsDir := filepath.Join(dir, "projects")
	projDir := filepath.Join(projectsDir, "-home-dev-myapp")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, "s.jsonl"), []byte(`{"type":"user","timestamp":"2026-01-01T10:00:00Z","uuid":"u1","message":{"role":"user","content":"hi"}}
`), 0644); err != nil {
		t.Fatal(err)
	}

	p, err := FindProject(projectsDir, "-home-dev-myapp")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if p == nil {
		t.Fatal("expected to find project by exact name")
	}
}

func TestFindProjectSubstring(t *testing.T) {
	dir := t.TempDir()
	projectsDir := filepath.Join(dir, "projects")
	projDir := filepath.Join(projectsDir, "-home-dev-myapp")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, "s.jsonl"), []byte(`{"type":"user","timestamp":"2026-01-01T10:00:00Z","uuid":"u1","message":{"role":"user","content":"hi"}}
`), 0644); err != nil {
		t.Fatal(err)
	}

	p, err := FindProject(projectsDir, "myapp")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if p == nil {
		t.Fatal("expected to find project by substring")
	}
}

func TestFindProjectNotFound(t *testing.T) {
	dir := t.TempDir()
	projectsDir := filepath.Join(dir, "projects")
	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatal(err)
	}

	p, err := FindProject(projectsDir, "nonexistent")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if p != nil {
		t.Fatal("expected nil for nonexistent project")
	}
}

func TestFindSession(t *testing.T) {
	dir := t.TempDir()
	projectsDir := filepath.Join(dir, "projects")
	projDir := filepath.Join(projectsDir, "-home-dev-proj")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, "abc12345.jsonl"), []byte(`{"type":"user","timestamp":"2026-01-01T10:00:00Z","uuid":"u1","message":{"role":"user","content":"hi"}}
`), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := FindSession(projectsDir, "-home-dev-proj", "abc12345")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if s == nil {
		t.Fatal("expected to find session")
	}
	if s.ID != "abc12345" {
		t.Errorf("ID = %q, want abc12345", s.ID)
	}
}

func TestFindSessionByPrefix(t *testing.T) {
	dir := t.TempDir()
	projectsDir := filepath.Join(dir, "projects")
	projDir := filepath.Join(projectsDir, "-home-dev-proj")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, "abc12345-long-uuid.jsonl"), []byte(`{"type":"user","timestamp":"2026-01-01T10:00:00Z","uuid":"u1","message":{"role":"user","content":"hi"}}
`), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := FindSession(projectsDir, "-home-dev-proj", "abc12345")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if s == nil {
		t.Fatal("expected to find session by prefix")
	}
}

func TestFindSessionGlobal(t *testing.T) {
	dir := t.TempDir()
	projectsDir := filepath.Join(dir, "projects")
	projDir := filepath.Join(projectsDir, "-home-dev-proj")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, "xyz789.jsonl"), []byte(`{"type":"user","timestamp":"2026-01-01T10:00:00Z","uuid":"u1","message":{"role":"user","content":"hi"}}
`), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := FindSession(projectsDir, "", "xyz789")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if s == nil {
		t.Fatal("expected to find session without project scope")
	}
}

func TestMatchSession(t *testing.T) {
	s := &Session{ID: "abc12345-full-uuid"}

	if !matchSession(s, "abc12345-full-uuid") {
		t.Error("exact match should pass")
	}
	if !matchSession(s, "abc12345") {
		t.Error("prefix match should pass")
	}
	if matchSession(s, "xyz") {
		t.Error("non-matching should fail")
	}
}

func TestDiscoverProjectsSkipsWarmup(t *testing.T) {
	dir := t.TempDir()
	projectsDir := filepath.Join(dir, "projects")
	projDir := filepath.Join(projectsDir, "-home-dev-proj")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Session with "warmup" summary — should be filtered
	if err := os.WriteFile(filepath.Join(projDir, "warmup.jsonl"), []byte(`{"type":"user","timestamp":"2026-01-01T10:00:00Z","uuid":"u1","message":{"role":"user","content":"warmup"}}
`), 0644); err != nil {
		t.Fatal(err)
	}

	// Normal session
	if err := os.WriteFile(filepath.Join(projDir, "real.jsonl"), []byte(`{"type":"user","timestamp":"2026-01-01T10:00:00Z","uuid":"u1","message":{"role":"user","content":"real request"}}
`), 0644); err != nil {
		t.Fatal(err)
	}

	projects, err := DiscoverProjects(projectsDir)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	// warmup session should be filtered out
	if len(projects[0].Sessions) != 1 {
		t.Errorf("expected 1 session (warmup filtered), got %d", len(projects[0].Sessions))
	}
}
