package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/thevibeworks/ccx/internal/catalog"
	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/provider"
	"github.com/thevibeworks/ccx/internal/trace"
)

var relatedCmd = &cobra.Command{
	Use:   "related [session]",
	Short: "Which sessions connect to this one, and how",
	Long: `Which sessions connect to this one, and how.

Sessions are islands in the log; the connections between them are
in the transcripts but nothing joins them. This command joins them,
deterministically, from the anchor session's point of view:

  forked_from / fork_of      the transcripts share message ids
  mentions / mentioned_by    text names the other session's id
  handoff_from / handoff_to  a baton file (HANDOFF.md, handoffs/,
                             devlog, PLAN.md) written by one, read
                             by the other later
  builds_on / built_on_by    a workspace file edited by one, then
                             read or edited by the other
  overlaps                   the two ran at the same time
  previous / next            nearest neighbours in the workspace

Every relation carries evidence (message id, time, path, quote) in
--json. Strength is a band — strong, medium, weak — never a score.
Scope is the anchor session's workspace, all providers.

Examples:
  ccx related                 Connections of the latest workspace session
  ccx related 736a7bac        A specific session
  ccx related --json          Full evidence for scripts and skills`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRelated,
}

var (
	relatedProject string
	relatedAll     bool
	relatedJSON    bool
	relatedLimit   int
)

func init() {
	relatedCmd.Flags().StringVarP(&relatedProject, "project", "p", "", "project name")
	relatedCmd.Flags().BoolVar(&relatedAll, "all", false, "resolve the session across all projects")
	relatedCmd.Flags().BoolVar(&relatedJSON, "json", false, "output as JSON")
	relatedCmd.Flags().IntVarP(&relatedLimit, "limit", "n", 20, "max related sessions (0 = no limit)")
	rootCmd.AddCommand(relatedCmd)
}

func runRelated(cmd *cobra.Command, args []string) error {
	backend := provider.Default()

	var session *parser.Session
	var err error
	if len(args) == 0 {
		session, err = latestTraceSession(backend, relatedAll)
		if err != nil {
			return fmt.Errorf("session: %w", err)
		}
	} else {
		projectName, sessionID := parseSessionArg(args[0])
		if relatedProject != "" {
			projectName = relatedProject
		}
		query, err := sessionLookupQuery(projectName, relatedAll)
		if err != nil {
			return err
		}
		session, err = resolveSessionInQuery(backend, query, sessionID)
		if err != nil {
			return err
		}
	}
	if session == nil {
		return fmt.Errorf("no session found")
	}

	related, warnings, err := relateSession(backend, session)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w.Message)
	}

	total := len(related)
	if relatedLimit > 0 && total > relatedLimit {
		fmt.Fprintf(os.Stderr, "showing %d of %d related sessions (raise with -n)\n", relatedLimit, total)
		related = related[:relatedLimit]
	}

	if relatedJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"kind":    "ccx.related.v1",
			"session": map[string]any{"id": session.ID, "provider": session.Provider, "project": session.ProjectName, "path": session.FilePath},
			"related": related,
			"total":   total,
			"shown":   len(related),
		})
	}
	return printRelated(session, related, total)
}

// relateSession profiles every session of the anchor's workspace and
// returns the anchor's connections. Shared by `ccx related` and the
// trace --full bundle.
func relateSession(backend provider.Backend, anchor *parser.Session) ([]trace.RelatedSession, []trace.TraceWarning, error) {
	query := catalog.SessionQuery{Scope: catalog.ScopeProject, ProjectName: anchor.ProjectName}
	if strings.TrimSpace(anchor.CWD) != "" {
		query = catalog.SessionQuery{Scope: catalog.ScopeWorkspace, WorkspacePath: anchor.CWD}
	}
	sessions, err := backend.ListSessions(query.WithoutLimit().WithoutProviderFilter())
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

	profiles := make([]*trace.SessionProfile, len(sessions))
	var warnings []trace.TraceWarning
	var warnMu sync.Mutex
	progress := newScanProgress(len(sessions), true)
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < searchWorkers(true); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				full, err := backend.ParseSession(sessions[i].FilePath)
				if err != nil {
					warnMu.Lock()
					warnings = append(warnings, trace.TraceWarning{Kind: "related_parse_failed", Message: fmt.Sprintf("skipping %s: %v", filepath.Base(sessions[i].FilePath), err)})
					warnMu.Unlock()
				} else {
					profiles[i] = trace.ProfileSession(full)
				}
				progress.tick()
			}
		}()
	}
	for i := range sessions {
		next <- i
	}
	close(next)
	wg.Wait()
	progress.done()

	var anchorProfile *trace.SessionProfile
	others := make([]*trace.SessionProfile, 0, len(profiles))
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
	return trace.RelateSessions(anchorProfile, others), warnings, nil
}

func printRelated(anchor *parser.Session, related []trace.RelatedSession, total int) error {
	// Mirror the trace header: workspace basename beats the encoded
	// project dir name.
	where := anchor.ProjectName
	if strings.HasPrefix(where, "-") {
		where = parser.GetProjectDisplayName(where)
	}
	if strings.TrimSpace(anchor.CWD) != "" {
		where = filepath.Base(anchor.CWD)
	}
	fmt.Printf("related to %s (%s): %d session(s)\n", truncateID(anchor.ID, 8), cleanDisplayText(where), total)
	if len(related) == 0 {
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "STRENGTH\tSESSION\tPROVIDER\tRELATIONS\tSTART\tEVIDENCE")
	for _, r := range related {
		kinds := make([]string, 0, len(r.Relations))
		for _, rel := range r.Relations {
			k := rel.Kind
			if rel.Count > 1 && (rel.Kind == trace.RelBuildsOn || rel.Kind == trace.RelBuiltOnBy || rel.Kind == trace.RelHandoffFrom || rel.Kind == trace.RelHandoffTo) {
				k = fmt.Sprintf("%s(%d)", k, rel.Count)
			}
			kinds = append(kinds, k)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Strength, truncateID(r.SessionID, 8), providerTag(r.Provider), strings.Join(kinds, ","),
			formatRelatedTime(r.Start), truncateDisplay(cleanDisplayText(relationEvidenceLine(r)), 96))
	}
	return w.Flush()
}

// relationEvidenceLine renders the strongest relation's evidence as one
// bounded line; the rest is in --json.
func relationEvidenceLine(r trace.RelatedSession) string {
	if len(r.Relations) == 0 {
		return ""
	}
	rel := r.Relations[0]
	for _, cand := range r.Relations[1:] {
		if strengthRankOf(cand.Kind) < strengthRankOf(rel.Kind) {
			rel = cand
		}
	}
	switch rel.Kind {
	case trace.RelForkedFrom, trace.RelForkOf:
		id := ""
		if len(rel.Evidence) > 0 {
			id = truncateID(rel.Evidence[0].MessageID, 8)
		}
		return fmt.Sprintf("%d shared message ids (e.g. %s)", rel.Count, id)
	case trace.RelMentions, trace.RelMentionedBy:
		if len(rel.Evidence) > 0 {
			return fmt.Sprintf("%s@%s: %s", truncateID(rel.Evidence[0].SessionID, 8), formatRelatedTime(rel.Evidence[0].Time), rel.Evidence[0].Quote)
		}
	case trace.RelHandoffFrom, trace.RelHandoffTo, trace.RelBuildsOn, trace.RelBuiltOnBy:
		if len(rel.Evidence) >= 2 {
			more := ""
			if rel.Count > 1 {
				more = fmt.Sprintf(" (+%d more)", rel.Count-1)
			}
			return fmt.Sprintf("%s%s: written %s@%s, touched %s@%s",
				shortPath(rel.Evidence[0].Path), more,
				truncateID(rel.Evidence[0].SessionID, 8), formatRelatedTime(rel.Evidence[0].Time),
				truncateID(rel.Evidence[1].SessionID, 8), formatRelatedTime(rel.Evidence[1].Time))
		}
	case trace.RelOverlaps:
		if len(rel.Evidence) >= 2 {
			return fmt.Sprintf("both active %s - %s", formatRelatedTime(rel.Evidence[0].Time), formatRelatedTime(rel.Evidence[1].Time))
		}
	}
	return "-"
}

func strengthRankOf(kind string) int {
	switch kind {
	case trace.RelForkedFrom, trace.RelForkOf, trace.RelMentions, trace.RelMentionedBy, trace.RelHandoffFrom, trace.RelHandoffTo:
		return 0
	case trace.RelBuildsOn, trace.RelBuiltOnBy, trace.RelOverlaps:
		return 1
	}
	return 2
}

func shortPath(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) <= 3 {
		return path
	}
	return ".../" + strings.Join(parts[len(parts)-3:], "/")
}

func formatRelatedTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04")
}
