package claude

import (
	"path/filepath"
	"regexp"

	"github.com/thevibeworks/ccx/internal/catalog"
	"github.com/thevibeworks/ccx/internal/parser"
)

const ProviderID = "claude-code"

type Backend struct {
	home        string
	projectsDir string
}

func New(home string) *Backend {
	return NewWithProjectsDir(home, filepath.Join(home, "projects"))
}

func NewWithProjectsDir(home, projectsDir string) *Backend {
	return &Backend{
		home:        filepath.Clean(home),
		projectsDir: filepath.Clean(projectsDir),
	}
}

func (b *Backend) ID() string { return ProviderID }

func (b *Backend) Homes() []string { return []string{b.home} }

func (b *Backend) DiscoverProjects() ([]*parser.Project, error) {
	projects, err := parser.DiscoverProjects(b.projectsDir)
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		p.Provider = ProviderID
		for _, s := range p.Sessions {
			s.Provider = ProviderID
		}
	}
	return projects, nil
}

func (b *Backend) ListSessions(query catalog.SessionQuery) ([]*parser.Session, error) {
	if query.Scope == catalog.ScopeWorkspace {
		return b.listWorkspaceSessions(query)
	}

	if query.Scope == catalog.ScopeProject && query.ProjectName != "" {
		project, err := b.FindProject(query.ProjectName)
		if err != nil || project == nil {
			return nil, err
		}
		return catalog.ApplySessionQuery([]*parser.Project{project}, query), nil
	}

	projects, err := b.DiscoverProjects()
	if err != nil {
		return nil, err
	}
	return catalog.ApplySessionQuery(projects, query), nil
}

func (b *Backend) listWorkspaceSessions(query catalog.SessionQuery) ([]*parser.Session, error) {
	encoded := claudeEncodeWorkspacePath(query.WorkspacePath)
	if encoded != "" {
		projectPath := filepath.Join(b.projectsDir, encoded)
		if sessions, err := parser.DiscoverProjectSessions(projectPath); err == nil && len(sessions) > 0 {
			project := &parser.Project{
				Name:         parser.GetProjectDisplayName(encoded),
				EncodedName:  encoded,
				Path:         query.WorkspacePath,
				Provider:     ProviderID,
				Sessions:     sessions,
				LastModified: sessions[0].EndTime,
			}
			for _, session := range sessions {
				session.Provider = ProviderID
				session.ProjectName = project.Name
			}
			return catalog.ApplySessionQuery([]*parser.Project{project}, query), nil
		}
	}

	projects, err := b.DiscoverProjects()
	if err != nil {
		return nil, err
	}
	return catalog.ApplySessionQuery(projects, query), nil
}

func (b *Backend) FindProject(name string) (*parser.Project, error) {
	p, err := parser.FindProject(b.projectsDir, name)
	if err != nil || p == nil {
		return p, err
	}
	p.Provider = ProviderID
	for _, s := range p.Sessions {
		s.Provider = ProviderID
	}
	return p, nil
}

func (b *Backend) FindSession(projectName, sessionID string) (*parser.Session, error) {
	s, err := parser.FindSession(b.projectsDir, projectName, sessionID)
	if err != nil || s == nil {
		return s, err
	}
	s.Provider = ProviderID
	return s, nil
}

func claudeEncodeWorkspacePath(path string) string {
	if path == "" {
		return ""
	}
	return claudeNonAlphanumeric.ReplaceAllString(filepath.Clean(path), "-")
}

var claudeNonAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9]`)

func (b *Backend) ParseSession(filePath string) (*parser.Session, error) {
	s, err := parser.ParseSession(filePath)
	if err != nil || s == nil {
		return s, err
	}
	s.Provider = ProviderID
	return s, nil
}
