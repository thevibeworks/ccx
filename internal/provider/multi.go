package provider

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thevibeworks/ccx/internal/catalog"
	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/parser"
)

type Multi struct {
	backends []Backend
	cache    *sessionCache // in-memory LRU (fast, volatile)
	disk     *diskCache    // persistent gob store (survives restart); may be nil
}

func NewMulti(backends ...Backend) *Multi {
	m := &Multi{
		backends: backends,
		cache:    newSessionCache(16),
	}
	// Best-effort disk cache. Failure (unwriteable data dir, permission
	// issues) just drops to memory-only — not a fatal condition.
	if dc, err := newDiskCache(filepath.Join(config.DataDir(), "session-cache")); err == nil {
		m.disk = dc
	}
	return m
}

// NewMultiWithDiskCache is used by tests that need a specific cache
// directory. Production callers should use NewMulti.
func NewMultiWithDiskCache(diskDir string, backends ...Backend) *Multi {
	m := &Multi{
		backends: backends,
		cache:    newSessionCache(16),
	}
	if diskDir != "" {
		if dc, err := newDiskCache(diskDir); err == nil {
			m.disk = dc
		}
	}
	return m
}

// ClearSessionCache drops all parsed-session entries from both the
// in-memory LRU and the persistent disk cache. Used by diagnostics
// and tests. Not part of a stable API.
func (m *Multi) ClearSessionCache() {
	if m.cache != nil {
		m.cache.clear()
	}
	if m.disk != nil {
		_ = m.disk.clear()
	}
}

func (m *Multi) ID() string { return "multi" }

func (m *Multi) Homes() []string {
	var homes []string
	for _, b := range m.backends {
		homes = append(homes, b.Homes()...)
	}
	return homes
}

// DiscoverProjects returns the agent-level projects across all
// backends, merged by CANONICAL workspace path. Two backends that
// both know about the same real cwd produce one merged Project —
// even if the encoded-name differs or case/symlinks drift.
//
// The merge key used to be Project.EncodedName, which silently split
// the same real directory into multiple rows whenever macOS case
// drift, a trailing slash, or a symlink snuck in between two agents.
// Now the key is parser.CanonicalizeWorkspacePath of the cwd, which
// is what a user actually means when they think "the project I'm
// working in".
func (m *Multi) DiscoverProjects() ([]*parser.Project, error) {
	merged := make(map[string]*parser.Project)

	for _, b := range m.backends {
		projects, err := b.DiscoverProjects()
		if err != nil {
			return nil, err
		}

		for _, p := range projects {
			// Pick the best cwd source for this provider-level project
			// before computing a canonical merge key.
			cp := *p
			cp.Path = projectLookupPath(&cp)
			key := parser.CanonicalizeWorkspacePath(cp.Path)
			if key == "" {
				// Canonicalization failed: fall back to encoded name,
				// BUT namespace it per provider so Claude Code's
				// "-Users-eric-foo" doesn't silently merge with Codex's
				// "-Users-eric-foo" when they're not actually the same
				// cwd (e.g. one or both have no cwd metadata at all).
				key = "enc:" + b.ID() + ":" + cp.EncodedName
			}

			existing := merged[key]
			if existing == nil {
				merged[key] = &cp
				continue
			}

			existing.Sessions = append(existing.Sessions, p.Sessions...)
			if p.LastModified.After(existing.LastModified) {
				existing.LastModified = p.LastModified
			}
			// Merged row is no longer provider-specific
			existing.Provider = ""
			existing.Path = projectLookupPath(existing)
		}
	}

	projects := make([]*parser.Project, 0, len(merged))
	for _, p := range merged {
		sort.Slice(p.Sessions, func(i, j int) bool {
			return p.Sessions[i].EndTime.After(p.Sessions[j].EndTime)
		})
		projects = append(projects, p)
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].LastModified.After(projects[j].LastModified)
	})

	return projects, nil
}

// DiscoverWorkspaces returns the canonicalized top-level Workspace
// objects. Each Workspace groups one or more agent-level Projects
// that share the same real directory. Use this for sidebar-style
// navigation that wants to show "one row per place you've been
// working" rather than "one row per backend encoding."
func (m *Multi) DiscoverWorkspaces() ([]*parser.Workspace, error) {
	projects, err := m.DiscoverProjects()
	if err != nil {
		return nil, err
	}
	return parser.GroupProjectsByWorkspace(projects), nil
}

func (m *Multi) ListSessions(query catalog.SessionQuery) ([]*parser.Session, error) {
	if query.Scope == catalog.ScopeProject {
		project, err := m.FindProject(query.ProjectName)
		if err != nil || project == nil {
			return nil, err
		}
		projectQuery := query
		projectQuery.Scope = catalog.ScopeAll
		projectQuery.ProjectName = ""
		return catalog.ApplySessionQuery([]*parser.Project{project}, projectQuery), nil
	}

	var sessions []*parser.Session
	for _, b := range m.backends {
		if query.Filter.Provider != "" && query.Filter.Provider != b.ID() {
			continue
		}
		backendQuery := query.WithoutProviderFilter().WithoutLimit()
		backendSessions, err := b.ListSessions(backendQuery)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, backendSessions...)
	}
	if !query.Filter.IsEmpty() {
		var filtered []*parser.Session
		for _, session := range sessions {
			if query.Filter.Match(session) {
				filtered = append(filtered, session)
			}
		}
		sessions = filtered
	}
	catalog.SortSessions(sessions, query.Sort)
	if query.Limit > 0 && len(sessions) > query.Limit {
		sessions = sessions[:query.Limit]
	}
	return sessions, nil
}

func (m *Multi) FindProject(name string) (*parser.Project, error) {
	projects, err := m.DiscoverProjects()
	if err != nil {
		return nil, err
	}

	query := strings.ToLower(strings.TrimSpace(name))
	if query == "" {
		return nil, nil
	}

	// Pass 1: exact match against the merged row's display name or
	// the winner-take-all EncodedName.
	for _, p := range projects {
		if strings.ToLower(p.Name) == query || strings.ToLower(p.EncodedName) == query {
			return p, nil
		}
	}

	// Pass 2: match by canonical-path equivalence. A URL built from
	// Claude Code's encoded name (e.g. "-Users-eric-foo") needs to
	// find the same row when the row merged sessions from Codex
	// (whose backend uses a different encoding). We canonicalise the
	// incoming query and compare to the merged row's canonical form.
	queryCanonical := parser.CanonicalizeWorkspacePath(decodeEncodedProjectName(name))
	if queryCanonical != "" {
		for _, p := range projects {
			if parser.CanonicalizeWorkspacePath(p.Path) == queryCanonical {
				return p, nil
			}
		}
	}

	// Pass 3: substring match — preserves the old fuzzy-find behaviour
	// for `ccx view` interactive pickers.
	for _, p := range projects {
		if strings.Contains(strings.ToLower(p.Name), query) || strings.Contains(strings.ToLower(p.Path), query) {
			return p, nil
		}
	}

	return nil, nil
}

// decodeEncodedProjectName reverses the agent-side cwd encoding as
// best we can. Both Claude Code and Codex replace "/" with "-", but
// Claude Code also replaces "_" and "." with "-" (non-reversible)
// while Codex uses parser.EncodePath (reversible only for "/"). We
// accept whichever form comes in on the URL and produce a path that
// canonicalises the same as the stored cwd, so URL matching works
// regardless of which backend's encoding the caller used.
func decodeEncodedProjectName(encoded string) string {
	if encoded == "" {
		return ""
	}
	s := strings.ReplaceAll(encoded, "-", "/")
	if !strings.HasPrefix(s, "/") {
		s = "/" + s
	}
	return s
}

func projectLookupPath(project *parser.Project) string {
	for _, session := range project.Sessions {
		if strings.TrimSpace(session.CWD) != "" {
			return filepath.Clean(session.CWD)
		}
	}
	if strings.TrimSpace(project.Path) == "" {
		return ""
	}
	return filepath.Clean(project.Path)
}

// FindSession resolves (projectName, sessionID) → *Session across
// both the direct per-backend path AND the merged-discovery path.
//
// Why two paths:
//
// Each backend (Claude Code, Codex) encodes the cwd differently.
// Claude Code uses the real on-disk folder name from ~/.claude/
// projects/ (leading dash, "_"/"." replaced with "-"). Codex uses
// parser.EncodePath (no leading dash, "_" preserved). For the same
// real directory, the two encodings are not equal strings.
//
// Multi.DiscoverProjects merges the two into one row keyed on
// CanonicalizeWorkspacePath — correct. But the merged row can only
// carry ONE EncodedName (whichever backend lands first), so URLs
// built from that merged row use just one backend's encoding. When
// the URL targets a session from the OTHER backend, per-backend
// FindSession lookups miss it because its encoding doesn't match.
//
// Fix: try each backend directly first (fast path, preserves old
// behaviour for non-merged cases), then fall back to the merged
// project list and find the session by ID within any Sessions slice
// whose merged project matches `projectName` via FindProject.
func (m *Multi) FindSession(projectName, sessionID string) (*parser.Session, error) {
	// Fast path: each backend's direct lookup.
	for _, b := range m.backends {
		session, err := b.FindSession(projectName, sessionID)
		if err != nil {
			return nil, err
		}
		if session != nil {
			return session, nil
		}
	}

	// Fallback: search the merged discovery list. This is what makes
	// cross-agent Workspace URLs resolvable.
	if sessionID == "" {
		return nil, nil
	}
	project, err := m.FindProject(projectName)
	if err != nil {
		return nil, err
	}
	if project != nil {
		if s := findSessionInProject(project, sessionID); s != nil {
			return s, nil
		}
	}

	// Last resort: session ID might be unique across the whole index
	// even when the project lookup drifted. Scan everything.
	projects, err := m.DiscoverProjects()
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		if s := findSessionInProject(p, sessionID); s != nil {
			return s, nil
		}
	}
	return nil, nil
}

func findSessionInProject(p *parser.Project, sessionID string) *parser.Session {
	if p == nil {
		return nil
	}
	for _, s := range p.Sessions {
		if s == nil {
			continue
		}
		if s.ID == sessionID || strings.HasPrefix(s.ID, sessionID) {
			return s
		}
	}
	return nil
}

func (m *Multi) ParseSession(filePath string) (*parser.Session, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil || absPath == "" {
		// If we can't canonicalize the path, fall back to the raw form
		// so a degenerate input (empty string, bad symlink) still gets
		// the error from parseThroughBackends instead of a silent
		// cache-miss loop.
		absPath = filePath
	}

	// Two-tier cache: in-memory LRU first (hot path), then disk cache
	// (cold but still avoids re-parsing). Both invalidate on mtime+size
	// mismatch so external edits to the session file are picked up.
	//
	// The cache key is the ABSOLUTE path so two callers with different
	// representations of the same file ("./a.jsonl" vs "/wd/a.jsonl")
	// hit the same slot instead of redundantly parsing twice.
	if m.cache == nil {
		return m.parseThroughBackends(filePath, absPath)
	}

	return m.cache.getOrLoad(absPath, func() (*parser.Session, error) {
		// Memory miss — try disk if available before falling through
		// to a live parse. Stat the source file once; reuse for both
		// disk lookup and the backend's cache write.
		info, statErr := os.Stat(filePath)

		if m.disk != nil && statErr == nil {
			if sess, hit := m.disk.get(absPath, info.ModTime(), info.Size()); hit {
				return sess, nil
			}
		}

		sess, err := m.parseThroughBackends(filePath, absPath)
		if err != nil || sess == nil {
			return sess, err
		}

		// Persist to disk for next restart. Silently ignore write errors.
		if m.disk != nil && statErr == nil {
			m.disk.put(absPath, sess, info.ModTime(), info.Size())
		}

		return sess, nil
	})
}

// parseThroughBackends is the original backend-dispatch logic. Pulled
// out so the cache layer can invoke it on miss without duplicating the
// dispatch rules.
func (m *Multi) parseThroughBackends(filePath, absPath string) (*parser.Session, error) {
	for _, b := range m.backends {
		for _, home := range b.Homes() {
			absHome, _ := filepath.Abs(home)
			if strings.HasPrefix(absPath, absHome+string(filepath.Separator)) {
				return b.ParseSession(filePath)
			}
		}
	}
	for _, b := range m.backends {
		session, err := b.ParseSession(filePath)
		if err == nil && session != nil {
			return session, nil
		}
	}
	return nil, nil
}
