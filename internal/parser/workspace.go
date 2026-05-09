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
//   - Resolves symlinks via filepath.EvalSymlinks when the path
//     actually exists on disk. EvalSymlinks on a case-insensitive
//     filesystem returns the case the FS actually stores, which
//     defuses case drift at the source without us having to
//     unconditionally lowercase.
//   - Cleans the result (collapses //, strips trailing /)
//   - For paths that DON'T exist on disk (deleted projects), applies
//     a platform-aware case-fold as a fallback so old sessions for
//     the same real directory still group together. On darwin /
//     windows where the default FS is case-insensitive we lowercase;
//     on linux we preserve case.
//
// Note on APFS case-sensitive volumes: APFS supports per-volume and
// per-directory case sensitivity. Two truly distinct directories
// differing only in case would still exist on such a volume, and
// EvalSymlinks returns their literal cased paths — so they remain
// distinct after this function. The fallback lowercase only kicks
// in for deleted paths, where distinguishing two gone-from-disk
// directories that differed only in case is inherently lossy
// regardless.
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
	// Try to resolve symlinks. On success, trust the FS-returned
	// casing — a case-insensitive FS will fold for us, a case-
	// sensitive FS will preserve the distinction we care about.
	resolved, resolvedOK := evalSymlinks(path)
	if resolvedOK {
		path = resolved
	}
	path = filepath.Clean(path)
	if len(path) > 1 {
		path = strings.TrimRight(path, string(filepath.Separator))
	}
	// Fallback case-fold ONLY when we couldn't resolve via the FS —
	// otherwise we'd collapse case-sensitive volumes' distinct
	// directories into one Workspace row.
	if !resolvedOK && (runtime.GOOS == "darwin" || runtime.GOOS == "windows") {
		path = strings.ToLower(path)
	}
	return path
}

func evalSymlinks(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved == "" {
		return "", false
	}
	return resolved, true
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
// Project and canonicalizes it.
//
// Preference order:
//  1. First session's CWD (most reliable — comes from the JSONL metadata)
//  2. Project.Path when it looks like a real cwd rather than an
//     encoded-name projects folder
//  3. Decoded EncodedName (lossy fallback for dash-encoded names)
//
// CWD first means we sidestep the fragility of hardcoded
// "/.claude/projects/" / "/.codex/sessions/" string checks that used
// to fail for XDG-relocated homes (CLAUDE_CODE_HOME=/srv/claude) or
// Windows path separators.
func workspaceKeyFor(p *Project) string {
	if p == nil {
		return ""
	}
	for _, s := range p.Sessions {
		if s != nil && strings.TrimSpace(s.CWD) != "" {
			return CanonicalizeWorkspacePath(s.CWD)
		}
	}
	if isRealCwd(p.Path) {
		return CanonicalizeWorkspacePath(p.Path)
	}
	if p.EncodedName != "" {
		return CanonicalizeWorkspacePath(decodeProjectName(p.EncodedName))
	}
	return ""
}

// isRealCwd reports whether a path looks like the user's actual
// working directory rather than an agent-home encoded projects
// folder. Used as a secondary source when the Project has no session
// CWD metadata.
func isRealCwd(path string) bool {
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	// Normalize separators before checking so Windows paths with
	// backslashes match too.
	normalized := filepath.ToSlash(path)
	if strings.Contains(normalized, "/.claude/projects/") {
		return false
	}
	if strings.Contains(normalized, "/.codex/sessions/") {
		return false
	}
	return true
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
