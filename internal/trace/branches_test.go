package trace

import (
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

// TestAnalyzeMarksBranchSiblings is the phantom-turn contract: a user
// edit/resend appears as two user records sharing one parentUuid. The
// abandoned sibling must be marked superseded — not counted as a turn,
// not silently dropped.
func TestAnalyzeMarksBranchSiblings(t *testing.T) {
	now := time.Now()
	aA := &parser.Message{UUID: "aA", ParentUUID: "uA", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(2 * time.Minute),
		Content: []parser.ContentBlock{{Type: "text", Text: "answering the first phrasing"}}}
	uA := &parser.Message{UUID: "uA", ParentUUID: "a0", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now.Add(time.Minute),
		Content:  []parser.ContentBlock{{Type: "text", Text: "next, we want to learn"}},
		Children: []*parser.Message{aA}}
	aB := &parser.Message{UUID: "aB", ParentUUID: "uB", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(4 * time.Minute),
		Content: []parser.ContentBlock{{Type: "text", Text: "answering the resend"}}}
	uB := &parser.Message{UUID: "uB", ParentUUID: "a0", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now.Add(3 * time.Minute),
		Content:  []parser.ContentBlock{{Type: "text", Text: "next, we want to learn"}},
		Children: []*parser.Message{aB}}
	a0 := &parser.Message{UUID: "a0", ParentUUID: "u1", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(30 * time.Second),
		Content:  []parser.ContentBlock{{Type: "text", Text: "first answer"}},
		Children: []*parser.Message{uA, uB}}
	u1 := &parser.Message{UUID: "u1", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now,
		Content:  []parser.ContentBlock{{Type: "text", Text: "first ask"}},
		Children: []*parser.Message{a0}}

	session := &parser.Session{
		ID:           "branch-siblings",
		StartTime:    now,
		EndTime:      now.Add(5 * time.Minute),
		RootMessages: []*parser.Message{u1},
	}

	result := Analyze(session)
	if len(result.Turns) != 3 {
		t.Fatalf("turns kept: got %d, want 3 (superseded stays as evidence)", len(result.Turns))
	}
	if result.Stats.TurnCount != 2 {
		t.Fatalf("turn count: got %d, want 2 active", result.Stats.TurnCount)
	}
	if result.Stats.SupersededTurns != 1 {
		t.Fatalf("superseded turns: got %d, want 1", result.Stats.SupersededTurns)
	}

	branch := result.Turns[1]
	if branch.AnchorID != "uA" || !branch.Superseded {
		t.Fatalf("turn 2 must be the superseded sibling: %+v", branch)
	}
	if branch.SupersededByTurn != result.Turns[2].Index {
		t.Fatalf("superseded_by_turn: got %d, want %d", branch.SupersededByTurn, result.Turns[2].Index)
	}
	if result.Turns[0].Superseded || result.Turns[2].Superseded {
		t.Fatal("active turns must not be marked superseded")
	}

	text := RenderOutlineText(BuildOutline(result))
	if !strings.Contains(text, "2 turns (+1 superseded)") {
		t.Fatalf("header must count active turns and disclose the branch:\n%s", text)
	}
	if !strings.Contains(text, "superseded by #3") {
		t.Fatalf("outline must mark the abandoned sibling:\n%s", text)
	}
}

// TestAnalyzeMarksAbandonedSubtreePrompts covers branching from an
// earlier point: prompts that continued a branch later abandoned are
// superseded by the same replacement as the branch root.
func TestAnalyzeMarksAbandonedSubtreePrompts(t *testing.T) {
	now := time.Now()
	aA2 := &parser.Message{UUID: "aA2", ParentUUID: "uA2", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(4 * time.Minute),
		Content: []parser.ContentBlock{{Type: "text", Text: "follow-up answer"}}}
	uA2 := &parser.Message{UUID: "uA2", ParentUUID: "aA", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now.Add(3 * time.Minute),
		Content:  []parser.ContentBlock{{Type: "text", Text: "follow-up on the abandoned branch"}},
		Children: []*parser.Message{aA2}}
	aA := &parser.Message{UUID: "aA", ParentUUID: "uA", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(2 * time.Minute),
		Content:  []parser.ContentBlock{{Type: "text", Text: "first branch answer"}},
		Children: []*parser.Message{uA2}}
	uA := &parser.Message{UUID: "uA", ParentUUID: "a0", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now.Add(time.Minute),
		Content:  []parser.ContentBlock{{Type: "text", Text: "take path A"}},
		Children: []*parser.Message{aA}}
	aB := &parser.Message{UUID: "aB", ParentUUID: "uB", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(6 * time.Minute),
		Content: []parser.ContentBlock{{Type: "text", Text: "path B answer"}}}
	uB := &parser.Message{UUID: "uB", ParentUUID: "a0", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now.Add(5 * time.Minute),
		Content:  []parser.ContentBlock{{Type: "text", Text: "actually, take path B"}},
		Children: []*parser.Message{aB}}
	a0 := &parser.Message{UUID: "a0", ParentUUID: "u1", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(30 * time.Second),
		Content:  []parser.ContentBlock{{Type: "text", Text: "which path?"}},
		Children: []*parser.Message{uA, uB}}
	u1 := &parser.Message{UUID: "u1", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now,
		Content:  []parser.ContentBlock{{Type: "text", Text: "start"}},
		Children: []*parser.Message{a0}}

	session := &parser.Session{
		ID:           "branch-subtree",
		StartTime:    now,
		EndTime:      now.Add(7 * time.Minute),
		RootMessages: []*parser.Message{u1},
	}

	result := Analyze(session)
	if len(result.Turns) != 4 {
		t.Fatalf("turns kept: got %d, want 4", len(result.Turns))
	}
	if result.Stats.TurnCount != 2 || result.Stats.SupersededTurns != 2 {
		t.Fatalf("counts: active=%d superseded=%d, want 2/2",
			result.Stats.TurnCount, result.Stats.SupersededTurns)
	}
	winner := result.Turns[3]
	if winner.AnchorID != "uB" || winner.Superseded {
		t.Fatalf("winner turn wrong: %+v", winner)
	}
	for _, i := range []int{1, 2} {
		turn := result.Turns[i]
		if !turn.Superseded || turn.SupersededByTurn != winner.Index {
			t.Fatalf("turn %d (%s): superseded=%v by=%d, want by #%d",
				i+1, turn.AnchorID, turn.Superseded, turn.SupersededByTurn, winner.Index)
		}
	}
}

// TestAnalyzeQueuedPromptsAreNotBranches guards the other direction:
// consecutive prompts chained parent->child (a user sending twice) are
// two real turns, not a branch.
func TestAnalyzeQueuedPromptsAreNotBranches(t *testing.T) {
	now := time.Now()
	u2 := &parser.Message{UUID: "u2", ParentUUID: "a1", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now.Add(2 * time.Minute),
		Content: []parser.ContentBlock{{Type: "text", Text: "and one more thing"}}}
	a1 := &parser.Message{UUID: "a1", ParentUUID: "u1", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(time.Minute),
		Content:  []parser.ContentBlock{{Type: "text", Text: "done"}},
		Children: []*parser.Message{u2}}
	u1 := &parser.Message{UUID: "u1", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now,
		Content:  []parser.ContentBlock{{Type: "text", Text: "do a thing"}},
		Children: []*parser.Message{a1}}

	result := Analyze(&parser.Session{
		ID: "queued", StartTime: now, EndTime: now.Add(3 * time.Minute),
		RootMessages: []*parser.Message{u1},
	})
	if result.Stats.TurnCount != 2 || result.Stats.SupersededTurns != 0 {
		t.Fatalf("counts: active=%d superseded=%d, want 2/0",
			result.Stats.TurnCount, result.Stats.SupersededTurns)
	}
}
