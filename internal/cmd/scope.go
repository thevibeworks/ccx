package cmd

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/thevibeworks/ccx/internal/catalog"
	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/provider"
)

func currentWorkspaceQuery() (catalog.SessionQuery, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return catalog.SessionQuery{}, fmt.Errorf("failed to get current directory: %w", err)
	}
	return catalog.SessionQuery{
		Scope:         catalog.ScopeWorkspace,
		WorkspacePath: cwd,
	}, nil
}

func allSessionsQuery() catalog.SessionQuery {
	return catalog.SessionQuery{Scope: catalog.ScopeAll}
}

func resolveSessionInQuery(backend provider.Backend, query catalog.SessionQuery, sessionID string) (*parser.Session, error) {
	sessions, err := backend.ListSessions(query.WithoutLimit())
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(sessionID, "@") {
		return resolveSessionIndex(sessions, sessionID)
	}
	session, err := findSessionByIDDescribed(sessions, sessionID)
	if err != nil {
		return nil, err
	}
	if session != nil {
		return session, nil
	}

	// Session IDs are globally unique, so a miss in the default
	// workspace scope widens to all projects before giving up. An
	// explicit -p scope is respected: report the miss instead.
	switch query.Scope {
	case catalog.ScopeWorkspace:
		allSessions, err := backend.ListSessions(allSessionsQuery().WithoutLimit())
		if err != nil {
			return nil, err
		}
		session, err = findSessionByIDDescribed(allSessions, sessionID)
		if err != nil {
			return nil, err
		}
		if session != nil {
			return session, nil
		}
		return nil, fmt.Errorf("session %q not found in any project", sessionID)
	case catalog.ScopeProject:
		return nil, fmt.Errorf("session %q not found in project %q; try --all", sessionID, query.ProjectName)
	default:
		return nil, fmt.Errorf("session %q not found in any project", sessionID)
	}
}

func findSessionByIDDescribed(sessions []*parser.Session, sessionID string) (*parser.Session, error) {
	session, err := catalog.FindSessionByID(sessions, sessionID)
	if err == nil {
		return session, nil
	}
	var ambiguous *catalog.AmbiguousSessionError
	if errors.As(err, &ambiguous) {
		return nil, describeAmbiguousSession(ambiguous)
	}
	return nil, err
}

func resolveSessionIndex(sessions []*parser.Session, sessionID string) (*parser.Session, error) {
	n, err := strconv.Atoi(strings.TrimPrefix(sessionID, "@"))
	if err != nil || n < 1 {
		return nil, fmt.Errorf("invalid session index %q", sessionID)
	}
	catalog.SortSessions(sessions, catalog.SortTime)
	if n > len(sessions) {
		return nil, fmt.Errorf("session index %s out of range; only %d sessions available", sessionID, len(sessions))
	}
	return sessions[n-1], nil
}

func describeAmbiguousSession(err *catalog.AmbiguousSessionError) error {
	if err == nil {
		return nil
	}
	var details []string
	for _, session := range err.Matches {
		if session == nil {
			continue
		}
		if session.ProjectName != "" {
			details = append(details, fmt.Sprintf("%s:%s", session.ProjectName, session.ID))
			continue
		}
		details = append(details, session.ID)
	}
	if len(details) == 0 {
		return err
	}
	return fmt.Errorf("%w: %s", err, strings.Join(details, ", "))
}
