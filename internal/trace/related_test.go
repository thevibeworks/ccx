package trace

import (
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

func relT(min int) time.Time { return time.Date(2026, 8, 18, 10, min, 0, 0, time.UTC) }

func relSession(id string, start, end time.Time, msgs ...*parser.Message) *parser.Session {
	return &parser.Session{ID: id, Provider: "claude-code", CWD: "/w", StartTime: start, EndTime: end, RootMessages: msgs}
}

func relUser(uuid string, t time.Time, text string) *parser.Message {
	return &parser.Message{UUID: uuid, Type: "user", Kind: parser.KindUserPrompt, Timestamp: t,
		Content: []parser.ContentBlock{{Type: "text", Text: text}}}
}

func relTool(uuid string, t time.Time, tool string, input map[string]any) *parser.Message {
	return &parser.Message{UUID: uuid, Type: "assistant", Kind: parser.KindAssistant, Timestamp: t,
		Content: []parser.ContentBlock{{Type: "tool_use", ToolName: tool, ToolID: "t-" + uuid, ToolInput: input}}}
}

func kinds(rels []Relation) map[string]Relation {
	m := make(map[string]Relation)
	for _, r := range rels {
		m[r.Kind] = r
	}
	return m
}

func findRelated(list []RelatedSession, id string) *RelatedSession {
	for i := range list {
		if list[i].SessionID == id {
			return &list[i]
		}
	}
	return nil
}

// A handoff is a baton file written by one session and read by a
// later one; an ordinary file edited then read is builds_on; both
// carry writer and reader anchors. Direction flips with the point of
// view, and a read that happened BEFORE the write is not a link.
func TestRelateSessionsHandoffAndBuildsOn(t *testing.T) {
	writer := relSession("aaaaaaaa-1", relT(0), relT(10),
		relTool("w1", relT(1), "Write", map[string]any{"file_path": "/w/HANDOFF.md"}),
		relTool("w2", relT(2), "Edit", map[string]any{"file_path": "/w/src/main.go"}),
		relTool("w3", relT(3), "Read", map[string]any{"file_path": "/w/README.md"}),
	)
	reader := relSession("bbbbbbbb-2", relT(20), relT(30),
		relTool("r1", relT(21), "Read", map[string]any{"file_path": "/w/HANDOFF.md"}),
		relTool("r2", relT(22), "Read", map[string]any{"file_path": "src/main.go"}), // relative: joined onto cwd
		relTool("r3", relT(23), "Edit", map[string]any{"file_path": "/w/README.md"}),
	)
	pw, pr := ProfileSession(writer), ProfileSession(reader)

	got := RelateSessions(pr, []*SessionProfile{pw})
	if len(got) != 1 || got[0].SessionID != "aaaaaaaa-1" || got[0].Strength != StrengthStrong {
		t.Fatalf("reader view: %+v", got)
	}
	k := kinds(got[0].Relations)
	h, ok := k[RelHandoffFrom]
	if !ok || h.Count != 1 || h.Paths[0] != "/w/HANDOFF.md" || len(h.Evidence) != 2 {
		t.Fatalf("handoff_from: %+v", h)
	}
	if h.Evidence[0].SessionID != "aaaaaaaa-1" || h.Evidence[0].MessageID != "w1" || h.Evidence[1].MessageID != "r1" {
		t.Fatalf("handoff evidence must pair writer then reader: %+v", h.Evidence)
	}
	b, ok := k[RelBuildsOn]
	if !ok || b.Count != 1 || b.Paths[0] != "/w/src/main.go" {
		t.Fatalf("builds_on: %+v (README was read by writer, not edited — must not count)", b)
	}
	if _, ok := k[RelBuiltOnBy]; ok {
		t.Fatal("reader edited README after writer READ it; that is not built_on_by")
	}
	if _, ok := k[RelPrevious]; !ok {
		t.Fatal("writer is the reader's nearest earlier session")
	}

	got = RelateSessions(pw, []*SessionProfile{pr})
	k = kinds(got[0].Relations)
	if _, ok := k[RelHandoffTo]; !ok {
		t.Fatalf("writer view must say handoff_to: %+v", k)
	}
	if _, ok := k[RelBuiltOnBy]; !ok {
		t.Fatalf("writer view must say built_on_by: %+v", k)
	}
	if _, ok := k[RelNext]; !ok {
		t.Fatal("reader is the writer's next session")
	}
}

// Shared message uuids mean a fork; the earlier session is the origin.
// A mention is the other session's 8-hex prefix in conversation text
// (not in tool results), quoted as evidence.
func TestRelateSessionsForkAndMentions(t *testing.T) {
	origin := relSession("11111111-aaaa", relT(0), relT(5),
		relUser("shared-1", relT(0), "start"),
		relUser("shared-2", relT(1), "more"),
	)
	fork := relSession("22222222-bbbb", relT(10), relT(15),
		relUser("shared-1", relT(0), "start"),
		relUser("shared-2", relT(1), "more"),
		relUser("own-3", relT(11), "continue from session 11111111 please"),
	)
	// A tool result naming the id is not a mention.
	bystander := relSession("33333333-cccc", relT(20), relT(25),
		&parser.Message{UUID: "tr", Type: "user", Kind: parser.KindToolResult, Timestamp: relT(21),
			Content: []parser.ContentBlock{{Type: "tool_result", ToolResult: "22222222-bbbb"}, {Type: "text", Text: "session 22222222 listed"}}},
	)
	po, pf, pb := ProfileSession(origin), ProfileSession(fork), ProfileSession(bystander)

	got := RelateSessions(pf, []*SessionProfile{po, pb})
	r := findRelated(got, "11111111-aaaa")
	if r == nil || r.Strength != StrengthStrong {
		t.Fatalf("fork view of origin: %+v", got)
	}
	k := kinds(r.Relations)
	f, ok := k[RelForkedFrom]
	if !ok || f.Count != 2 || f.Evidence[0].MessageID != "shared-1" {
		t.Fatalf("forked_from: %+v", f)
	}
	m, ok := k[RelMentions]
	if !ok || m.Evidence[0].MessageID != "own-3" || m.Evidence[0].SessionID != "22222222-bbbb" || m.Evidence[0].Quote == "" {
		t.Fatalf("mentions: %+v", m)
	}
	if b := findRelated(got, "33333333-cccc"); b != nil {
		for _, rel := range b.Relations {
			if rel.Kind == RelMentionedBy {
				t.Fatalf("tool-result text must not count as a mention: %+v", rel)
			}
		}
	}

	got = RelateSessions(po, []*SessionProfile{pf})
	k = kinds(got[0].Relations)
	if _, ok := k[RelForkOf]; !ok {
		t.Fatalf("origin view must say fork_of: %+v", k)
	}
	if _, ok := k[RelMentionedBy]; !ok {
		t.Fatalf("origin view must say mentioned_by: %+v", k)
	}
}

// Overlap needs intersecting windows; ordering is strongest first,
// then by start; unrelated sessions are absent; the anchor is skipped.
func TestRelateSessionsOverlapOrderingAndSelf(t *testing.T) {
	anchor := relSession("aaaaaaaa-0", relT(10), relT(20),
		relTool("a1", relT(12), "Edit", map[string]any{"file_path": "/w/x.go"}))
	concurrent := relSession("bbbbbbbb-1", relT(15), relT(25))
	later := relSession("cccccccc-2", relT(30), relT(35),
		relTool("c1", relT(31), "Read", map[string]any{"file_path": "/w/x.go"}))
	unrelated := relSession("dddddddd-3", relT(40), relT(45),
		relTool("d1", relT(41), "Read", map[string]any{"file_path": "/w/other.go"}))
	pa := ProfileSession(anchor)
	got := RelateSessions(pa, []*SessionProfile{ProfileSession(unrelated), ProfileSession(later), ProfileSession(concurrent), pa})

	if len(got) != 2 {
		t.Fatalf("want concurrent + later only, got %+v", got)
	}
	// Both medium: order by start.
	if got[0].SessionID != "bbbbbbbb-1" || got[1].SessionID != "cccccccc-2" {
		t.Fatalf("order: %s, %s", got[0].SessionID, got[1].SessionID)
	}
	k := kinds(got[0].Relations)
	o, ok := k[RelOverlaps]
	if !ok || !o.Evidence[0].Time.Equal(relT(15)) || !o.Evidence[1].Time.Equal(relT(20)) {
		t.Fatalf("overlap window: %+v", o)
	}
	if _, ok := kinds(got[1].Relations)[RelBuiltOnBy]; !ok {
		t.Fatalf("later read of anchor's edit: %+v", got[1].Relations)
	}
	// Timeless sessions never become previous/next.
	if got := RelateSessions(pa, []*SessionProfile{ProfileSession(relSession("eeeeeeee-4", time.Time{}, time.Time{}))}); len(got) != 0 {
		t.Fatalf("timeless session related: %+v", got)
	}
}

// Path lists are capped at maxRelationPaths with the count intact and
// Truncated set — never a silent cut.
func TestRelateSessionsCapsPaths(t *testing.T) {
	var wmsgs, rmsgs []*parser.Message
	for i := 0; i < maxRelationPaths+3; i++ {
		p := "/w/f" + string(rune('a'+i)) + ".go"
		wmsgs = append(wmsgs, relTool("w"+string(rune('a'+i)), relT(1), "Edit", map[string]any{"file_path": p}))
		rmsgs = append(rmsgs, relTool("r"+string(rune('a'+i)), relT(30), "Read", map[string]any{"file_path": p}))
	}
	pw := ProfileSession(relSession("aaaaaaaa-w", relT(0), relT(5), wmsgs...))
	pr := ProfileSession(relSession("bbbbbbbb-r", relT(20), relT(40), rmsgs...))
	got := RelateSessions(pr, []*SessionProfile{pw})
	b := kinds(got[0].Relations)[RelBuildsOn]
	if b.Count != maxRelationPaths+3 || len(b.Paths) != maxRelationPaths || !b.Truncated {
		t.Fatalf("cap: count=%d paths=%d truncated=%v", b.Count, len(b.Paths), b.Truncated)
	}
}

func TestIsBatonPath(t *testing.T) {
	yes := []string{"/w/HANDOFF.md", "/w/handoff-notes.org", "/w/.claude/handoffs/latest.md", "/w/docs/devlog/2026-08-18-x.org", "/w/DEVLOG.org", "/w/PLAN.md", "/w/todo.md"}
	no := []string{"/w/README.md", "/w/internal/cmd/search.go", "/w/handoff.go", "/w/docs/design/0006.md"}
	for _, p := range yes {
		if !isBatonPath(p) {
			t.Errorf("%s should be a baton path", p)
		}
	}
	for _, p := range no {
		if isBatonPath(p) {
			t.Errorf("%s should not be a baton path", p)
		}
	}
}

func TestIDPrefix(t *testing.T) {
	cases := map[string]string{
		"736a7bac-0a5d-4e3f-9036-c8a94111a347": "736a7bac",
		"019F6528-ABCD":                        "019f6528",
		"short":                                "",
		"zzzzzzzz-not-hex":                     "",
	}
	for in, want := range cases {
		if got := idPrefix(in); got != want {
			t.Errorf("idPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}
