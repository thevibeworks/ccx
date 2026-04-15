package parser

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestCanonicalizeWorkspacePath_StripsTrailingSlash(t *testing.T) {
	a := CanonicalizeWorkspacePath("/tmp/foo/")
	b := CanonicalizeWorkspacePath("/tmp/foo")
	if a == "" || a != b {
		t.Errorf("trailing slash: %q vs %q, want equal", a, b)
	}
}

func TestCanonicalizeWorkspacePath_CollapsesDoubleSlash(t *testing.T) {
	got := CanonicalizeWorkspacePath("/tmp//foo//bar")
	want := CanonicalizeWorkspacePath("/tmp/foo/bar")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCanonicalizeWorkspacePath_CaseFoldOnMacOS(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("case fold only active on case-insensitive filesystems")
	}
	a := CanonicalizeWorkspacePath("/Users/Eric/FOO")
	b := CanonicalizeWorkspacePath("/users/eric/foo")
	if a != b {
		t.Errorf("case-fold: %q vs %q, want equal on %s", a, b, runtime.GOOS)
	}
}

func TestCanonicalizeWorkspacePath_EmptyReturnsEmpty(t *testing.T) {
	if got := CanonicalizeWorkspacePath(""); got != "" {
		t.Errorf("empty input, got %q", got)
	}
	if got := CanonicalizeWorkspacePath("   "); got != "" {
		t.Errorf("whitespace input, got %q", got)
	}
}

func TestWorkspaceHashFor_StableAndNonEmpty(t *testing.T) {
	h1 := WorkspaceHashFor("/tmp/foo")
	h2 := WorkspaceHashFor("/tmp/foo")
	if h1 == "" {
		t.Fatal("hash must not be empty for non-empty input")
	}
	if h1 != h2 {
		t.Errorf("hash non-deterministic: %q vs %q", h1, h2)
	}
	if len(h1) != 12 {
		t.Errorf("hash length = %d, want 12", len(h1))
	}
}

func TestWorkspaceHashFor_DifferentForDifferentInputs(t *testing.T) {
	a := WorkspaceHashFor("/tmp/foo")
	b := WorkspaceHashFor("/tmp/bar")
	if a == b {
		t.Errorf("different inputs collided: %q == %q", a, b)
	}
}

func TestGroupProjectsByWorkspace_MergesSameCwdAcrossAgents(t *testing.T) {
	// Two agent projects pointing at the same real directory — maybe
	// one from Claude Code and one from Codex — must land in the
	// SAME Workspace row. Before this change, encoded-name keyed
	// merging would have split them into two rows whenever the two
	// agents encoded slightly differently.
	now := time.Now()
	pCC := &Project{
		Name:         "foo",
		EncodedName:  "-Users-eric-foo",
		Path:         "/Users/eric/foo",
		Provider:     "claude-code",
		LastModified: now,
		Sessions: []*Session{
			{ID: "s1", CWD: "/Users/eric/foo", EndTime: now},
		},
	}
	pCX := &Project{
		Name:         "foo",
		EncodedName:  "users_eric_foo",   // different encoding
		Path:         "/Users/eric/foo/", // trailing slash drift
		Provider:     "codex",
		LastModified: now.Add(-time.Minute),
		Sessions: []*Session{
			{ID: "s2", CWD: "/Users/eric/foo/", EndTime: now.Add(-time.Minute)},
		},
	}

	workspaces := GroupProjectsByWorkspace([]*Project{pCC, pCX})
	if len(workspaces) != 1 {
		t.Fatalf("want 1 workspace (merged), got %d", len(workspaces))
	}
	ws := workspaces[0]
	if len(ws.Projects) != 2 {
		t.Errorf("workspace should subsume 2 projects, got %d", len(ws.Projects))
	}
	if ws.TotalSessions() != 2 {
		t.Errorf("TotalSessions = %d, want 2", ws.TotalSessions())
	}
	if ws.Hash == "" {
		t.Error("Hash must be populated")
	}
	// LastModified tracks the newer of the two.
	if !ws.LastModified.Equal(now) {
		t.Errorf("LastModified = %v, want %v (the newer)", ws.LastModified, now)
	}
}

func TestGroupProjectsByWorkspace_SeparatesDifferentCwds(t *testing.T) {
	pA := &Project{
		Name: "alpha",
		Path: "/Users/eric/alpha",
		Sessions: []*Session{
			{ID: "s1", CWD: "/Users/eric/alpha", EndTime: time.Now()},
		},
	}
	pB := &Project{
		Name: "beta",
		Path: "/Users/eric/beta",
		Sessions: []*Session{
			{ID: "s2", CWD: "/Users/eric/beta", EndTime: time.Now()},
		},
	}
	ws := GroupProjectsByWorkspace([]*Project{pA, pB})
	if len(ws) != 2 {
		t.Errorf("different cwds: want 2 workspaces, got %d", len(ws))
	}
}

func TestGroupProjectsByWorkspace_AllSessionsSortedByEndTime(t *testing.T) {
	now := time.Now()
	p := &Project{
		Path: "/tmp/x",
		Sessions: []*Session{
			{ID: "old", CWD: "/tmp/x", EndTime: now.Add(-1 * time.Hour)},
			{ID: "new", CWD: "/tmp/x", EndTime: now},
			{ID: "mid", CWD: "/tmp/x", EndTime: now.Add(-30 * time.Minute)},
		},
	}
	workspaces := GroupProjectsByWorkspace([]*Project{p})
	if len(workspaces) != 1 {
		t.Fatalf("want 1 workspace, got %d", len(workspaces))
	}
	sessions := workspaces[0].AllSessions()
	if len(sessions) != 3 {
		t.Fatalf("want 3 sessions, got %d", len(sessions))
	}
	if sessions[0].ID != "new" || sessions[1].ID != "mid" || sessions[2].ID != "old" {
		t.Errorf("sessions not sorted newest-first: %v", []string{sessions[0].ID, sessions[1].ID, sessions[2].ID})
	}
}

func TestGroupProjectsByWorkspace_IgnoresClaudeHomeFallbackPath(t *testing.T) {
	// A project whose Path is the encoded folder under ~/.claude/projects
	// should use its session CWD as the canonical source, not the
	// encoded folder path.
	p := &Project{
		EncodedName: "-Users-eric-real",
		Path:        "/home/user/.claude/projects/-Users-eric-real",
		Sessions: []*Session{
			{ID: "s1", CWD: "/Users/eric/real", EndTime: time.Now()},
		},
	}
	ws := GroupProjectsByWorkspace([]*Project{p})
	if len(ws) != 1 {
		t.Fatalf("want 1 workspace, got %d", len(ws))
	}
	wantKey := CanonicalizeWorkspacePath("/Users/eric/real")
	if ws[0].CanonicalPath != wantKey {
		t.Errorf("CanonicalPath = %q, want %q (from session CWD, not encoded folder)", ws[0].CanonicalPath, wantKey)
	}
}

func TestDecodeProjectName_LeadingSlashAndDashes(t *testing.T) {
	if got := decodeProjectName("-Users-eric-foo"); got != "/Users/eric/foo" {
		t.Errorf("decodeProjectName = %q, want /Users/eric/foo", got)
	}
	if got := decodeProjectName(""); got != "" {
		t.Errorf("empty input should stay empty, got %q", got)
	}
}

func TestWorkspace_NilSafe(t *testing.T) {
	var w *Workspace
	if w.TotalSessions() != 0 {
		t.Error("nil Workspace.TotalSessions should be 0")
	}
	if w.AllSessions() != nil {
		t.Error("nil Workspace.AllSessions should be nil")
	}
}

func TestCanonicalizeWorkspacePath_ExpandsTilde(t *testing.T) {
	got := CanonicalizeWorkspacePath("~/some-nonexistent-path-for-ccx-test")
	if got == "" || got == "~/some-nonexistent-path-for-ccx-test" {
		t.Errorf("~ not expanded: got %q", got)
	}
	// The result should be absolute
	if !filepath.IsAbs(got) {
		t.Errorf("expanded path not absolute: %q", got)
	}
}
