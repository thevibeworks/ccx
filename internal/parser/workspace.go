package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Workspace is a real directory on disk that one or more coding-agent
// projects point at. It's the top-level IA object ccx navigates by.
//
// Why Workspace and not Project?
//
// "Project" as Claude Code and Codex store it is really "encoded cwd
// folder on disk under ~/.claude/projects/ or ~/.codex/sessions/."
// That encoding is fragile: the same real directory can show up under
// multiple encoded names because of trailing slashes, case drift on
// macOS, symlinks, or the two agents encoding differently. Merging
// by encoded name leaves the user with duplicate "projects" that are
// in fact the same place.
//
// A Workspace keys on the CANONICAL path — real-path resolved, case-
// folded on macOS, trailing-slash-stripped. Same directory, same
// Workspace. Same Workspace, one row in the sidebar, one merged
// session list across Claude Code and Codex.
type Workspace struct {
	// CanonicalPath is the post-resolve filesystem path. Used as the
	// merge key across agents.
	CanonicalPath string

	// DisplayName is the leaf directory name, suitable for a sidebar
	// label. Falls back to CanonicalPath when the leaf is empty.
	DisplayName string

	// Hash is a stable short fingerprint of the canonical path, used
	// in URLs. ccx doesn't try to make these pretty — the user never
	// types them.
	Hash string

	// Projects are the agent-specific Project objects this Workspace
	// subsumes. Typically one per provider (Claude Code, Codex). Kept
	// around because each provider has its own on-disk layout and
	// FindSession still needs to dispatch through them.
	Projects []*Project

	// LastModified is the newest EndTime across all subsumed projects.
	LastModified time.Time
}

// TotalSessions returns how many sessions this Workspace contains
// across all subsumed agent projects.
func (w *Workspace) TotalSessions() int {
	if w == nil {
		return 0
	}
	n := 0
	for _, p := range w.Projects {
		n += len(p.Sessions)
	}
	return n
}

// AllSessions flattens the Workspace's subsumed project sessions into
// a single slice sorted by EndTime descending (newest first).
func (w *Workspace) AllSessions() []*Session {
	if w == nil {
		return nil
	}
	var out []*Session
	for _, p := range w.Projects {
		out = append(out, p.Sessions...)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].EndTime.After(out[j].EndTime)
	})
	return out
}

// CanonicalizeWorkspacePath normalizes a path so two equivalent forms
// produce the same result.
//
//   - Expands ~ to the user home
//   - Resolves symlinks via filepath.EvalSymlinks (best effort)
//   - Cleans the result (collapses //, strips trailing /)
//   - Case-folds on macOS (HFS+/APFS are case-insensitive by default)
//
// Returns the input cleaned if the path doesn't exist on disk, so
// old sessions for deleted workspaces still group together.
func CanonicalizeWorkspacePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	// Try to resolve symlinks; fall back to Clean on failure.
	if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != "" {
		path = resolved
	}
	path = filepath.Clean(path)
	// Strip any remaining trailing separator (Clean leaves / as /).
	if len(path) > 1 {
		path = strings.TrimRight(path, string(filepath.Separator))
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}

// WorkspaceHashFor returns a short stable fingerprint of the canonical
// path, used as the URL segment for a Workspace. Short enough to keep
// URLs readable, long enough to avoid collisions at the scale one
// machine ever encounters.
func WorkspaceHashFor(canonicalPath string) string {
	if canonicalPath == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(canonicalPath))
	return hex.EncodeToString(sum[:6]) // 12 hex chars
}

// GroupProjectsByWorkspace takes a flat list of agent-level Projects
// (potentially from multiple providers) and merges them under their
// canonical workspace. The input slice is not modified.
//
// Merge rule: two Projects go into the same Workspace if they share
// the same CanonicalizeWorkspacePath value. The source path comes
// from Project.Path when non-empty; otherwise the first session's
// CWD; otherwise the EncodedName decoded (legacy fallback).
func GroupProjectsByWorkspace(projects []*Project) []*Workspace {
	if len(projects) == 0 {
		return nil
	}
	byCanonical := make(map[string]*Workspace)
	for _, p := range projects {
		if p == nil {
			continue
		}
		canonical := workspaceKeyFor(p)
		if canonical == "" {
			continue
		}
		ws, ok := byCanonical[canonical]
		if !ok {
			ws = &Workspace{
				CanonicalPath: canonical,
				DisplayName:   workspaceDisplayFor(canonical, p),
				Hash:          WorkspaceHashFor(canonical),
			}
			byCanonical[canonical] = ws
		}
		ws.Projects = append(ws.Projects, p)
		if p.LastModified.After(ws.LastModified) {
			ws.LastModified = p.LastModified
		}
	}
	workspaces := make([]*Workspace, 0, len(byCanonical))
	for _, ws := range byCanonical {
		workspaces = append(workspaces, ws)
	}
	sort.Slice(workspaces, func(i, j int) bool {
		return workspaces[i].LastModified.After(workspaces[j].LastModified)
	})
	return workspaces
}

// workspaceKeyFor picks the best available real-path source for a
// Project and canonicalizes it. Preference order:
//  1. Project.Path (if it points at a real cwd, not the encoded folder)
//  2. First session's CWD
//  3. Decoded EncodedName
func workspaceKeyFor(p *Project) string {
	if p == nil {
		return ""
	}
	// Project.Path in the legacy model is the encoded projects/xxx
	// folder. Only use it when it looks like a real cwd (starts with /
	// and isn't under the claude/codex home).
	if strings.HasPrefix(p.Path, "/") &&
		!strings.Contains(p.Path, "/.claude/projects/") &&
		!strings.Contains(p.Path, "/.codex/sessions/") {
		return CanonicalizeWorkspacePath(p.Path)
	}
	for _, s := range p.Sessions {
		if s != nil && strings.TrimSpace(s.CWD) != "" {
			return CanonicalizeWorkspacePath(s.CWD)
		}
	}
	if p.EncodedName != "" {
		return CanonicalizeWorkspacePath(decodeProjectName(p.EncodedName))
	}
	return ""
}

func workspaceDisplayFor(canonical string, p *Project) string {
	if canonical == "" {
		if p != nil {
			return p.Name
		}
		return "(unknown)"
	}
	leaf := filepath.Base(canonical)
	if leaf == "." || leaf == string(filepath.Separator) || leaf == "" {
		return canonical
	}
	return leaf
}

// decodeProjectName reverses the agent-side cwd encoding: dashes to
// slashes, with an implied leading slash. "-Users-eric-foo" becomes
// "/Users/eric/foo". Best-effort — does not round-trip perfectly
// because the original encoding is lossy on directories that contain
// dashes, but it's close enough to satisfy the canonical-merge check.
func decodeProjectName(encoded string) string {
	if encoded == "" {
		return ""
	}
	s := strings.ReplaceAll(encoded, "-", "/")
	if !strings.HasPrefix(s, "/") {
		s = "/" + s
	}
	return s
}
