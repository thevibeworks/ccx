package parser

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func DiscoverProjects(projectsDir string) ([]*Project, error) {
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var projects []*Project
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		encodedName := entry.Name()
		projectPath := filepath.Join(projectsDir, encodedName)

		sessions, err := DiscoverProjectSessions(projectPath)
		if err != nil {
			continue
		}
		if len(sessions) == 0 {
			continue
		}

		info, _ := entry.Info()
		lastMod := info.ModTime()
		if len(sessions) > 0 {
			lastMod = sessions[0].EndTime
		}

		project := &Project{
			Name:         GetProjectDisplayName(encodedName),
			EncodedName:  encodedName,
			Path:         projectPath,
			Sessions:     sessions,
			LastModified: lastMod,
		}
		projects = append(projects, project)
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].LastModified.After(projects[j].LastModified)
	})

	return projects, nil
}

func DiscoverProjectSessions(projectPath string) ([]*Session, error) {
	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return nil, err
	}

	var sessions []*Session
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		if strings.HasPrefix(name, "agent-") {
			continue
		}

		sessionPath := filepath.Join(projectPath, name)

		info, err := entry.Info()
		if err != nil {
			continue
		}
		if cached, hit := MetaLookup(sessionPath, info.ModTime(), info.Size()); hit {
			if cached != nil {
				sessions = append(sessions, cached)
			}
			continue
		}

		session := quickParseListableSession(sessionPath, name, projectPath)
		MetaStore(sessionPath, info.ModTime(), info.Size(), session)
		if session != nil {
			sessions = append(sessions, session)
		}
	}
	FlushMetaCache()

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].EndTime.After(sessions[j].EndTime)
	})

	return sessions, nil
}

// quickParseListableSession runs the metadata quick-parse and applies
// the listability rules (skip empty/no-summary/warmup files). Returns
// nil when the file should not appear in session lists — a result the
// metadata cache records so skipped files aren't re-scanned every
// discovery pass.
func quickParseListableSession(sessionPath, fileName, projectPath string) *Session {
	summary, startTime, endTime, stats, meta := quickParseSession(sessionPath)
	if summary == "" || summary == "(no summary)" {
		return nil
	}
	if strings.ToLower(summary) == "warmup" {
		return nil
	}

	return &Session{
		ID:          strings.TrimSuffix(fileName, ".jsonl"),
		FilePath:    sessionPath,
		ProjectName: filepath.Base(projectPath),
		Summary:     summary,
		StartTime:   startTime,
		EndTime:     endTime,
		Stats:       stats,
		Slug:        meta.Slug,
		Version:     meta.Version,
		GitBranch:   meta.GitBranch,
		CWD:         meta.CWD,
		Model:       meta.Model,
	}
}

func FindProject(projectsDir, name string) (*Project, error) {
	projects, err := DiscoverProjects(projectsDir)
	if err != nil {
		return nil, err
	}

	name = strings.ToLower(name)
	for _, p := range projects {
		if strings.ToLower(p.Name) == name || strings.ToLower(p.EncodedName) == name {
			return p, nil
		}
		if strings.Contains(strings.ToLower(p.Name), name) {
			return p, nil
		}
	}
	return nil, nil
}

func FindSession(projectsDir, projectName, sessionID string) (*Session, error) {
	var project *Project
	var err error

	if projectName != "" {
		project, err = FindProject(projectsDir, projectName)
		if err != nil || project == nil {
			return nil, err
		}
	}

	if project != nil {
		for _, s := range project.Sessions {
			if matchSession(s, sessionID) {
				return s, nil
			}
		}
		return nil, nil
	}

	projects, err := DiscoverProjects(projectsDir)
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		for _, s := range p.Sessions {
			if matchSession(s, sessionID) {
				return s, nil
			}
		}
	}
	return nil, nil
}

func matchSession(s *Session, query string) bool {
	if s.ID == query {
		return true
	}
	if strings.HasPrefix(s.ID, query) {
		return true
	}
	return false
}
