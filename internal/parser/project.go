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

		sessions, err := discoverSessions(projectPath)
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

func discoverSessions(projectPath string) ([]*Session, error) {
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

		summary, startTime, endTime, stats := quickParseSession(sessionPath)
		if summary == "" || summary == "(no summary)" {
			continue
		}
		if strings.ToLower(summary) == "warmup" {
			continue
		}

		session := &Session{
			ID:          strings.TrimSuffix(name, ".jsonl"),
			FilePath:    sessionPath,
			ProjectName: filepath.Base(projectPath),
			Summary:     summary,
			StartTime:   startTime,
			EndTime:     endTime,
			Stats:       stats,
		}
		sessions = append(sessions, session)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].EndTime.After(sessions[j].EndTime)
	})

	return sessions, nil
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
