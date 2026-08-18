package trace

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/thevibeworks/ccx/internal/parser"
)

// Session connections (docs/design/0006-session-connections.md): the
// deterministic relations between one anchor session and the other
// sessions of its workspace, each backed by evidence a reader can walk
// to. Stated from the anchor's point of view: "anchor forked_from X",
// "anchor handoff_from X" (anchor read what X wrote).

// RelatedSession is one session connected to the anchor.
type RelatedSession struct {
	SessionID string     `json:"session_id"`
	Provider  string     `json:"provider,omitempty"`
	Summary   string     `json:"summary,omitempty"`
	Start     time.Time  `json:"start"`
	End       time.Time  `json:"end"`
	Strength  string     `json:"strength"` // strong | medium | weak
	Relations []Relation `json:"relations"`
}

// Relation is one kind of link plus its evidence.
type Relation struct {
	Kind string `json:"kind"`
	// Count is the total behind a sampled list: shared message uuids
	// for forks, files for builds_on. Paths/Evidence are capped;
	// Truncated says so.
	Count     int                `json:"count,omitempty"`
	Paths     []string           `json:"paths,omitempty"`
	Evidence  []RelationEvidence `json:"evidence,omitempty"`
	Truncated bool               `json:"truncated,omitempty"`
}

// RelationEvidence is one anchor into a transcript: which session,
// which message, when, and (for file relations) which path.
type RelationEvidence struct {
	SessionID string    `json:"session_id"`
	MessageID string    `json:"message_id,omitempty"`
	Time      time.Time `json:"time,omitempty"`
	Path      string    `json:"path,omitempty"`
	Quote     string    `json:"quote,omitempty"`
}

const (
	RelForkedFrom  = "forked_from"
	RelForkOf      = "fork_of"
	RelMentions    = "mentions"
	RelMentionedBy = "mentioned_by"
	RelHandoffFrom = "handoff_from"
	RelHandoffTo   = "handoff_to"
	RelBuildsOn    = "builds_on"
	RelBuiltOnBy   = "built_on_by"
	RelOverlaps    = "overlaps"
	RelPrevious    = "previous"
	RelNext        = "next"

	StrengthStrong = "strong"
	StrengthMedium = "medium"
	StrengthWeak   = "weak"

	// maxRelationPaths bounds the file list carried per relation; the
	// count says how many there were.
	maxRelationPaths = 5
	// idRefLen is how much of a session id must appear in text to
	// count as a mention: 8 hex chars, the prefix ccx prints everywhere.
	idRefLen = 8
)

// relationStrength is the one shared band table (0005 principle 7).
var relationStrength = map[string]string{
	RelForkedFrom:  StrengthStrong,
	RelForkOf:      StrengthStrong,
	RelMentions:    StrengthStrong,
	RelMentionedBy: StrengthStrong,
	RelHandoffFrom: StrengthStrong,
	RelHandoffTo:   StrengthStrong,
	RelBuildsOn:    StrengthMedium,
	RelBuiltOnBy:   StrengthMedium,
	RelOverlaps:    StrengthMedium,
	RelPrevious:    StrengthWeak,
	RelNext:        StrengthWeak,
}

var strengthRank = map[string]int{StrengthStrong: 0, StrengthMedium: 1, StrengthWeak: 2}

// SessionProfile is what relation detection needs from one parsed
// session; building it is the only per-session cost, so callers can
// profile every session of a workspace once and relate any pair.
type SessionProfile struct {
	ID       string
	Provider string
	Summary  string
	Start    time.Time
	End      time.Time

	uuids   map[string]struct{}
	edits   map[string][]touch // path -> edits, chronological
	touches map[string][]touch // path -> reads and edits, chronological
	idRefs  map[string]refHit  // lowercase 8-hex prefix -> first message quoting it
}

type touch struct {
	msgID string
	t     time.Time
}

type refHit struct {
	msgID string
	t     time.Time
	quote string
}

var hexRunRe = regexp.MustCompile(`[0-9a-fA-F]{8,}`)

// ProfileSession extracts the relation signals from one parsed session.
func ProfileSession(s *parser.Session) *SessionProfile {
	p := &SessionProfile{
		uuids:   make(map[string]struct{}),
		edits:   make(map[string][]touch),
		touches: make(map[string][]touch),
		idRefs:  make(map[string]refHit),
	}
	if s == nil {
		return p
	}
	p.ID = s.ID
	p.Provider = s.Provider
	p.Summary = s.Summary
	p.Start = s.StartTime
	p.End = s.EndTime

	for _, msg := range parser.FlattenSessionMessages(s) {
		if msg.UUID != "" {
			p.uuids[msg.UUID] = struct{}{}
		}
		if msg.Kind == parser.KindUserPrompt || msg.Kind == parser.KindAssistant {
			for _, b := range msg.Content {
				if b.Type != "text" && b.Type != "thinking" {
					continue
				}
				p.collectIDRefs(msg, b.Text)
			}
		}
		for _, cb := range msg.Content {
			if cb.Type != "tool_use" || cb.ToolName == "" {
				continue
			}
			paths := extractPaths(cb.ToolInput)
			if len(paths) == 0 {
				continue
			}
			isEdit := mutatingTools[cb.ToolName]
			isRead := readTools[cb.ToolName]
			if !isEdit && !isRead {
				continue
			}
			for _, path := range paths {
				path = absolutePath(path, s.CWD)
				if path == "" {
					continue
				}
				t := touch{msgID: msg.UUID, t: msg.Timestamp}
				p.touches[path] = append(p.touches[path], t)
				if isEdit {
					p.edits[path] = append(p.edits[path], t)
				}
			}
		}
	}
	for _, m := range []map[string][]touch{p.edits, p.touches} {
		for path := range m {
			list := m[path]
			sort.SliceStable(list, func(i, j int) bool { return list[i].t.Before(list[j].t) })
			m[path] = list
		}
	}
	return p
}

func (p *SessionProfile) collectIDRefs(msg *parser.Message, text string) {
	for _, loc := range hexRunRe.FindAllStringIndex(text, -1) {
		prefix := strings.ToLower(text[loc[0] : loc[0]+idRefLen])
		if _, seen := p.idRefs[prefix]; seen {
			continue
		}
		p.idRefs[prefix] = refHit{
			msgID: msg.UUID,
			t:     msg.Timestamp,
			quote: quoteAround(text, loc[0], loc[1]-loc[0]),
		}
	}
}

// absolutePath makes tool paths comparable across sessions of one
// workspace: relative paths (Codex, shell redirects) are joined onto
// the session cwd.
func absolutePath(path, cwd string) string {
	path = cleanEvidencePath(path)
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) && cwd != "" {
		path = filepath.Join(cwd, path)
	}
	return filepath.Clean(path)
}

// isBatonPath reports whether a path is a session baton — the files
// one session writes so the next can pick the work up.
func isBatonPath(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(lower)
	switch {
	case strings.HasPrefix(base, "handoff") && (strings.HasSuffix(base, ".md") || strings.HasSuffix(base, ".org")):
		return true
	case strings.Contains(lower, "/handoffs/"):
		return true
	case strings.Contains(lower, "devlog"):
		return true
	case base == "plan.md", base == "todo.md", base == "notes.md":
		return true
	}
	return false
}

// idPrefix is the lowercase 8-hex prefix ccx prints for a session id;
// empty when the id does not start with one.
func idPrefix(id string) string {
	id = strings.ToLower(id)
	if len(id) < idRefLen {
		return ""
	}
	for i := 0; i < idRefLen; i++ {
		c := id[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return ""
		}
	}
	return id[:idRefLen]
}

// RelateSessions computes the anchor's connections to others (any
// order; the anchor itself is skipped if present). Result is strongest
// first, then by start time.
func RelateSessions(anchor *SessionProfile, others []*SessionProfile) []RelatedSession {
	if anchor == nil {
		return nil
	}
	var out []RelatedSession
	var prev, next *SessionProfile
	for _, o := range others {
		if o == nil || o.ID == anchor.ID {
			continue
		}
		if !o.Start.IsZero() && !anchor.Start.IsZero() {
			if o.Start.Before(anchor.Start) && (prev == nil || o.Start.After(prev.Start)) {
				prev = o
			}
			if o.Start.After(anchor.Start) && (next == nil || o.Start.Before(next.Start)) {
				next = o
			}
		}
	}
	for _, o := range others {
		if o == nil || o.ID == anchor.ID {
			continue
		}
		rels := relatePair(anchor, o)
		if o == prev {
			rels = append(rels, Relation{Kind: RelPrevious})
		}
		if o == next {
			rels = append(rels, Relation{Kind: RelNext})
		}
		if len(rels) == 0 {
			continue
		}
		out = append(out, RelatedSession{
			SessionID: o.ID,
			Provider:  o.Provider,
			Summary:   o.Summary,
			Start:     o.Start,
			End:       o.End,
			Strength:  strongest(rels),
			Relations: rels,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if strengthRank[a.Strength] != strengthRank[b.Strength] {
			return strengthRank[a.Strength] < strengthRank[b.Strength]
		}
		return a.Start.Before(b.Start)
	})
	return out
}

func strongest(rels []Relation) string {
	best := StrengthWeak
	for _, r := range rels {
		if s := relationStrength[r.Kind]; strengthRank[s] < strengthRank[best] {
			best = s
		}
	}
	return best
}

// relatePair finds every relation between anchor a and other o, from
// a's point of view.
func relatePair(a, o *SessionProfile) []Relation {
	var rels []Relation

	// Fork: shared message uuids; the earlier session is the origin.
	if shared, first := sharedUUIDs(a, o); shared > 0 {
		kind := RelForkOf
		if !a.Start.IsZero() && !o.Start.IsZero() && a.Start.After(o.Start) {
			kind = RelForkedFrom
		}
		rels = append(rels, Relation{
			Kind:     kind,
			Count:    shared,
			Evidence: []RelationEvidence{{SessionID: a.ID, MessageID: first}, {SessionID: o.ID, MessageID: first}},
		})
	}

	// Mentions: one session's text names the other's id.
	if pfx := idPrefix(o.ID); pfx != "" {
		if hit, ok := a.idRefs[pfx]; ok {
			rels = append(rels, Relation{Kind: RelMentions, Evidence: []RelationEvidence{
				{SessionID: a.ID, MessageID: hit.msgID, Time: hit.t, Quote: hit.quote},
			}})
		}
	}
	if pfx := idPrefix(a.ID); pfx != "" {
		if hit, ok := o.idRefs[pfx]; ok {
			rels = append(rels, Relation{Kind: RelMentionedBy, Evidence: []RelationEvidence{
				{SessionID: o.ID, MessageID: hit.msgID, Time: hit.t, Quote: hit.quote},
			}})
		}
	}

	// Files: o wrote, a touched later -> a handoff_from / builds_on o;
	// a wrote, o touched later -> a handoff_to / built_on_by o.
	if h, b := fileLinks(o, a); h != nil || b != nil {
		if h != nil {
			h.Kind = RelHandoffFrom
			rels = append(rels, *h)
		}
		if b != nil {
			b.Kind = RelBuildsOn
			rels = append(rels, *b)
		}
	}
	if h, b := fileLinks(a, o); h != nil || b != nil {
		if h != nil {
			h.Kind = RelHandoffTo
			rels = append(rels, *h)
		}
		if b != nil {
			b.Kind = RelBuiltOnBy
			rels = append(rels, *b)
		}
	}

	// Overlap: concurrent windows.
	if !a.Start.IsZero() && !a.End.IsZero() && !o.Start.IsZero() && !o.End.IsZero() &&
		a.Start.Before(o.End) && o.Start.Before(a.End) {
		start, end := a.Start, a.End
		if o.Start.After(start) {
			start = o.Start
		}
		if o.End.Before(end) {
			end = o.End
		}
		rels = append(rels, Relation{Kind: RelOverlaps, Evidence: []RelationEvidence{
			{SessionID: a.ID, Time: start}, {SessionID: o.ID, Time: end},
		}})
	}
	return rels
}

func sharedUUIDs(a, o *SessionProfile) (int, string) {
	small, large := a.uuids, o.uuids
	if len(large) < len(small) {
		small, large = large, small
	}
	n := 0
	first := ""
	for id := range small {
		if _, ok := large[id]; ok {
			n++
			if first == "" || id < first {
				first = id
			}
		}
	}
	return n, first
}

// fileLinks finds paths writer edited that reader touched afterwards,
// split into baton files (handoff) and everything else (builds_on).
// Evidence pairs the writer's first qualifying edit with the reader's
// first touch after it. Paths are sorted for deterministic output.
func fileLinks(writer, reader *SessionProfile) (handoff, builds *Relation) {
	paths := make([]string, 0, len(writer.edits))
	for path := range writer.edits {
		if _, ok := reader.touches[path]; ok {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)

	add := func(rel **Relation, path string, w, r touch) {
		if *rel == nil {
			*rel = &Relation{}
		}
		(*rel).Count++
		if len((*rel).Paths) >= maxRelationPaths {
			(*rel).Truncated = true
			return
		}
		(*rel).Paths = append((*rel).Paths, path)
		(*rel).Evidence = append((*rel).Evidence,
			RelationEvidence{SessionID: writer.ID, MessageID: w.msgID, Time: w.t, Path: path},
			RelationEvidence{SessionID: reader.ID, MessageID: r.msgID, Time: r.t, Path: path},
		)
	}

	for _, path := range paths {
		w := writer.edits[path][0]
		var r touch
		found := false
		for _, t := range reader.touches[path] {
			if t.t.After(w.t) {
				r, found = t, true
				break
			}
		}
		if !found {
			continue
		}
		if isBatonPath(path) {
			add(&handoff, path, w, r)
		} else {
			add(&builds, path, w, r)
		}
	}
	return handoff, builds
}

// quoteAround cuts a bounded window around a match, clamped to rune
// boundaries, for evidence quotes.
func quoteAround(text string, idx, n int) string {
	start := idx - 40
	if start < 0 {
		start = 0
	}
	end := idx + n + 40
	if end > len(text) {
		end = len(text)
	}
	for start > 0 && !utf8.RuneStart(text[start]) {
		start--
	}
	for end < len(text) && !utf8.RuneStart(text[end]) {
		end++
	}
	out := strings.Join(strings.Fields(text[start:end]), " ")
	if start > 0 {
		out = "..." + out
	}
	if end < len(text) {
		out += "..."
	}
	return out
}
