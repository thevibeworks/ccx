package catalog

import (
	"fmt"
	"sort"
	"strings"

	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/parser"
)

type SessionScope string

const (
	ScopeAll       SessionScope = "all"
	ScopeWorkspace SessionScope = "workspace"
	ScopeProject   SessionScope = "project"
)

type SessionSort string

const (
	SortTime     SessionSort = "time"
	SortMessages SessionSort = "messages"
)

type SessionQuery struct {
	Scope         SessionScope
	WorkspacePath string
	ProjectName   string
	Filter        config.SessionFilter
	Sort          SessionSort
	Limit         int
}

func (q SessionQuery) WithoutLimit() SessionQuery {
	q.Limit = 0
	return q
}

func (q SessionQuery) WithoutProviderFilter() SessionQuery {
	q.Filter.Provider = ""
	return q
}

func ApplySessionQuery(projects []*parser.Project, q SessionQuery) []*parser.Session {
	var sessions []*parser.Session
	for _, project := range projects {
		if project == nil || !projectMatchesScope(project, q) {
			continue
		}
		for _, session := range project.Sessions {
			if session == nil {
				continue
			}
			if q.Scope == ScopeWorkspace && !sessionMatchesWorkspace(project, session, q.WorkspacePath) {
				continue
			}
			if session.ProjectName == "" {
				session.ProjectName = project.Name
			}
			if !q.Filter.IsEmpty() && !q.Filter.Match(session) {
				continue
			}
			sessions = append(sessions, session)
		}
	}
	SortSessions(sessions, q.Sort)
	if q.Limit > 0 && len(sessions) > q.Limit {
		sessions = sessions[:q.Limit]
	}
	return sessions
}

func SortSessions(sessions []*parser.Session, sortBy SessionSort) {
	switch sortBy {
	case SortMessages:
		sort.SliceStable(sessions, func(i, j int) bool {
			return sessions[i].Stats.MessageCount > sessions[j].Stats.MessageCount
		})
	default:
		sort.SliceStable(sessions, func(i, j int) bool {
			return sessions[i].EndTime.After(sessions[j].EndTime)
		})
	}
}

func ValidateSessionSort(sortBy SessionSort) error {
	switch sortBy {
	case "", SortTime, SortMessages:
		return nil
	default:
		return fmt.Errorf("invalid sort %q (want time or messages)", sortBy)
	}
}

func projectMatchesScope(project *parser.Project, q SessionQuery) bool {
	switch q.Scope {
	case ScopeProject:
		return ProjectMatchesName(project, q.ProjectName)
	case ScopeWorkspace:
		return ProjectMatchesWorkspace(project, q.WorkspacePath)
	default:
		return true
	}
}

func sessionMatchesWorkspace(project *parser.Project, session *parser.Session, workspacePath string) bool {
	canonical := parser.CanonicalizeWorkspacePath(workspacePath)
	if canonical == "" || session == nil {
		return false
	}
	if strings.TrimSpace(session.CWD) != "" {
		return parser.CanonicalizeWorkspacePath(session.CWD) == canonical
	}
	return ProjectMatchesWorkspace(project, workspacePath)
}

func ProjectMatchesName(project *parser.Project, name string) bool {
	if project == nil {
		return false
	}
	query := strings.ToLower(strings.TrimSpace(name))
	if query == "" {
		return false
	}
	nameLower := strings.ToLower(project.Name)
	encodedLower := strings.ToLower(project.EncodedName)
	pathLower := strings.ToLower(project.Path)
	if nameLower == query || encodedLower == query || pathLower == query {
		return true
	}
	canonicalQuery := parser.CanonicalizeWorkspacePath(name)
	if canonicalQuery != "" {
		if parser.CanonicalizeWorkspacePath(project.Path) == canonicalQuery {
			return true
		}
		for _, session := range project.Sessions {
			if session != nil && parser.CanonicalizeWorkspacePath(session.CWD) == canonicalQuery {
				return true
			}
		}
	}
	return strings.Contains(nameLower, query) || strings.Contains(pathLower, query)
}

func ProjectMatchesWorkspace(project *parser.Project, workspacePath string) bool {
	if project == nil {
		return false
	}
	canonical := parser.CanonicalizeWorkspacePath(workspacePath)
	if canonical == "" {
		return false
	}
	if parser.CanonicalizeWorkspacePath(project.Path) == canonical {
		return true
	}
	if project.EncodedName != "" && parser.CanonicalizeWorkspacePath(parser.DecodePath(project.EncodedName)) == canonical {
		return true
	}
	for _, session := range project.Sessions {
		if session == nil {
			continue
		}
		if parser.CanonicalizeWorkspacePath(session.CWD) == canonical {
			return true
		}
	}
	return false
}

type AmbiguousSessionError struct {
	Query   string
	Matches []*parser.Session
}

func (e *AmbiguousSessionError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("ambiguous session %q matched %d sessions", e.Query, len(e.Matches))
}

func FindSessionByID(sessions []*parser.Session, query string) (*parser.Session, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	var exact []*parser.Session
	for _, session := range sessions {
		if session != nil && session.ID == query {
			exact = append(exact, session)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(exact) > 1 {
		return nil, &AmbiguousSessionError{Query: query, Matches: exact}
	}

	var prefixed []*parser.Session
	for _, session := range sessions {
		if session != nil && strings.HasPrefix(session.ID, query) {
			prefixed = append(prefixed, session)
		}
	}
	if len(prefixed) == 1 {
		return prefixed[0], nil
	}
	if len(prefixed) > 1 {
		return nil, &AmbiguousSessionError{Query: query, Matches: prefixed}
	}
	return nil, nil
}
