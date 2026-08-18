package trace

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/thevibeworks/ccx/internal/catalog"
	"github.com/thevibeworks/ccx/internal/parser"
)

// SessionSource is the slice of provider.Backend that workspace
// relation needs; narrowed so the CLI, the web API, and tests share
// one implementation.
type SessionSource interface {
	ListSessions(query catalog.SessionQuery) ([]*parser.Session, error)
	ParseSession(filePath string) (*parser.Session, error)
}

// RelateWorkspace profiles every session of the anchor's workspace
// (all providers) on `workers` goroutines and returns the anchor's
// connections. tick, when non-nil, is called once per session
// profiled (progress). Sessions that fail to parse become warnings,
// never silent gaps.
func RelateWorkspace(src SessionSource, anchor *parser.Session, workers int, tick func()) ([]RelatedSession, []TraceWarning, error) {
	if src == nil || anchor == nil {
		return nil, nil, fmt.Errorf("no session")
	}
	query := catalog.SessionQuery{Scope: catalog.ScopeProject, ProjectName: anchor.ProjectName}
	if strings.TrimSpace(anchor.CWD) != "" {
		query = catalog.SessionQuery{Scope: catalog.ScopeWorkspace, WorkspacePath: anchor.CWD}
	}
	sessions, err := src.ListSessions(query.WithoutLimit().WithoutProviderFilter())
	if err != nil {
		return nil, nil, fmt.Errorf("list workspace sessions: %w", err)
	}
	// The anchor may be missing from the workspace listing when its
	// cwd differs from the project path (a fork into another dir);
	// it always takes part.
	found := false
	for _, s := range sessions {
		if s.FilePath == anchor.FilePath {
			found = true
			break
		}
	}
	if !found {
		sessions = append(sessions, anchor)
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(sessions) {
		workers = len(sessions)
	}

	profiles := make([]*SessionProfile, len(sessions))
	var warnings []TraceWarning
	var warnMu sync.Mutex
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				full, err := src.ParseSession(sessions[i].FilePath)
				if err != nil {
					warnMu.Lock()
					warnings = append(warnings, TraceWarning{Kind: "related_parse_failed", Message: fmt.Sprintf("skipping %s: %v", filepath.Base(sessions[i].FilePath), err)})
					warnMu.Unlock()
				} else {
					profiles[i] = ProfileSession(full)
				}
				if tick != nil {
					tick()
				}
			}
		}()
	}
	for i := range sessions {
		next <- i
	}
	close(next)
	wg.Wait()

	var anchorProfile *SessionProfile
	others := make([]*SessionProfile, 0, len(profiles))
	for i, p := range profiles {
		if p == nil {
			continue
		}
		if sessions[i].FilePath == anchor.FilePath {
			anchorProfile = p
			continue
		}
		others = append(others, p)
	}
	if anchorProfile == nil {
		return nil, warnings, fmt.Errorf("could not parse anchor session %s", anchor.ID)
	}
	return RelateSessions(anchorProfile, others), warnings, nil
}
