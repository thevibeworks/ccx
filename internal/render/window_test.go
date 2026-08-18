package render

import (
	"strings"
	"testing"

	"github.com/thevibeworks/ccx/internal/parser"
)

func winMsg(uuid string, children ...*parser.Message) *parser.Message {
	return &parser.Message{UUID: uuid, Kind: parser.KindAssistant, Type: "assistant", Children: children}
}

// The window is a wire-order slice around the target, flattened
// (children detached), clamped at both ends; the target is found by
// exact id or unique prefix; ambiguity and misses are errors.
func TestWindowSession(t *testing.T) {
	// Tree: a -> b -> c -> d -> e (linear chain), plus f as a second root.
	e := winMsg("eeee-5")
	d := winMsg("dddd-4", e)
	c := winMsg("cccc-3", d)
	b := winMsg("bbbb-2", c)
	a := winMsg("aaaa-1", b)
	f := winMsg("aaab-6")
	session := &parser.Session{ID: "s", RootMessages: []*parser.Message{a, f}}

	win, idx, total, err := WindowSession(session, "cccc", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if idx != 3 || total != 6 {
		t.Fatalf("index/total: %d/%d", idx, total)
	}
	got := []string{}
	for _, m := range win.RootMessages {
		got = append(got, m.UUID)
		if len(m.Children) != 0 {
			t.Fatalf("children must be detached in the window: %s", m.UUID)
		}
	}
	if strings.Join(got, ",") != "bbbb-2,cccc-3,dddd-4" {
		t.Fatalf("window: %v", got)
	}
	// Original tree untouched.
	if len(session.RootMessages[0].Children) != 1 {
		t.Fatal("window must not mutate the parsed session")
	}

	// Clamp at the start; big context.
	win, _, _, err = WindowSession(session, "aaaa-1", 5, 1)
	if err != nil || len(win.RootMessages) != 2 || win.RootMessages[0].UUID != "aaaa-1" {
		t.Fatalf("clamped window: %+v %v", win.RootMessages, err)
	}
	// Ambiguous prefix "aaa" matches aaaa-1 and aaab-6.
	if _, _, _, err := WindowSession(session, "aaa", 1, 1); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous prefix must error, got %v", err)
	}
	// Exact id wins even when it is also a prefix of another id.
	if _, idx, _, err := WindowSession(session, "aaaa-1", 0, 0); err != nil || idx != 1 {
		t.Fatalf("exact id: %d %v", idx, err)
	}
	if _, _, _, err := WindowSession(session, "zzzz", 1, 1); err == nil {
		t.Fatal("missing id must error")
	}
	if _, _, _, err := WindowSession(session, "", 1, 1); err == nil {
		t.Fatal("empty target must error")
	}
}
