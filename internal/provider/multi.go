package provider

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/thevibeworks/ccx/internal/parser"
)

type Multi struct {
	backends []Backend
	cache    *sessionCache
}

func NewMulti(backends ...Backend) *Multi {
	return &Multi{
		backends: backends,
		cache:    newSessionCache(16),
	}
}

// ClearSessionCache drops all in-memory parsed-session entries. Used
// by diagnostics and tests. Not part of a stable API.
func (m *Multi) ClearSessionCache() {
	if m.cache != nil {
		m.cache.clear()
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

func (m *Multi) DiscoverProjects() ([]*parser.Project, error) {
	merged := make(map[string]*parser.Project)

	for _, b := range m.backends {
		projects, err := b.DiscoverProjects()
		if err != nil {
			return nil, err
		}

		for _, p := range projects {
			key := p.EncodedName
			existing := merged[key]
			if existing == nil {
				cp := *p
				cp.Path = projectLookupPath(&cp)
				merged[key] = &cp
				continue
			}

			existing.Sessions = append(existing.Sessions, p.Sessions...)
			if p.LastModified.After(existing.LastModified) {
				existing.LastModified = p.LastModified
			}
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

func (m *Multi) FindProject(name string) (*parser.Project, error) {
	projects, err := m.DiscoverProjects()
	if err != nil {
		return nil, err
	}

	query := strings.ToLower(strings.TrimSpace(name))
	for _, p := range projects {
		if strings.ToLower(p.Name) == query || strings.ToLower(p.EncodedName) == query {
			return p, nil
		}
		if strings.Contains(strings.ToLower(p.Name), query) || strings.Contains(strings.ToLower(p.Path), query) {
			return p, nil
		}
	}

	return nil, nil
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

func (m *Multi) FindSession(projectName, sessionID string) (*parser.Session, error) {
	for _, b := range m.backends {
		session, err := b.FindSession(projectName, sessionID)
		if err != nil {
			return nil, err
		}
		if session != nil {
			return session, nil
		}
	}
	return nil, nil
}

func (m *Multi) ParseSession(filePath string) (*parser.Session, error) {
	absPath, _ := filepath.Abs(filePath)

	// Cache layer: if we parsed this file recently and mtime/size are
	// unchanged, return the cached tree. Otherwise delegate to the
	// appropriate backend and cache the result.
	if m.cache != nil {
		return m.cache.getOrLoad(filePath, func() (*parser.Session, error) {
			return m.parseThroughBackends(filePath, absPath)
		})
	}
	return m.parseThroughBackends(filePath, absPath)
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
