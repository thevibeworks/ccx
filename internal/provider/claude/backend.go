package claude

import (
	"path/filepath"

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

func (b *Backend) ParseSession(filePath string) (*parser.Session, error) {
	s, err := parser.ParseSession(filePath)
	if err != nil || s == nil {
		return s, err
	}
	s.Provider = ProviderID
	return s, nil
}
