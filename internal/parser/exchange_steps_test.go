package parser

import (
	"testing"
	"time"
)

func TestComputeExchanges_ExtractsStepsFromAssistant(t *testing.T) {
	// One user turn whose assistant dispatches a sub-agent, calls a
	// skill, and runs two ordinary tools. The resulting Exchange
	// should carry 4 Steps in wire order, each with its UUID pointing
	// at the parent assistant message.
	start := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)
	assistant := &Message{
		UUID:      "a1",
		Kind:      KindAssistant,
		Timestamp: start.Add(time.Second),
		Content: []ContentBlock{
			{Type: "text", Text: "let me think"},
			{
				Type:     "tool_use",
				ToolName: "Task",
				ToolInput: map[string]any{
					"subagent_type": "Plan",
					"description":   "design auth",
				},
			},
			{
				Type:     "tool_use",
				ToolName: "Skill",
				ToolInput: map[string]any{
					"skill": "commit",
				},
			},
			{Type: "tool_use", ToolName: "Read"},
			{Type: "tool_use", ToolName: "Edit"},
		},
	}
	user := &Message{
		UUID:      "u1",
		Kind:      KindUserPrompt,
		Timestamp: start,
		Children:  []*Message{assistant},
	}
	session := &Session{RootMessages: []*Message{user}}
	messages := FlattenSessionMessages(session)

	exchanges := ComputeExchanges(messages)
	if len(exchanges) != 1 {
		t.Fatalf("want 1 exchange, got %d", len(exchanges))
	}
	ex := exchanges[0]
	if ex.CountSteps(StepSubagent) != 1 {
		t.Errorf("Subagent count = %d, want 1", ex.CountSteps(StepSubagent))
	}
	if ex.CountSteps(StepSkill) != 1 {
		t.Errorf("Skill count = %d, want 1", ex.CountSteps(StepSkill))
	}
	if ex.CountSteps(StepToolUse) != 2 {
		t.Errorf("Tool count = %d, want 2 (Read + Edit)", ex.CountSteps(StepToolUse))
	}
	if len(ex.Steps) != 4 {
		t.Errorf("total Steps = %d, want 4", len(ex.Steps))
	}
	// Every step's UUID should point at the parent assistant, so
	// rail satellites jump to the right anchor.
	for i, s := range ex.Steps {
		if s.UUID != "a1" {
			t.Errorf("Steps[%d].UUID = %q, want a1", i, s.UUID)
		}
	}
}

func TestComputeExchanges_SubagentLabelHasTypeAndDescription(t *testing.T) {
	assistant := &Message{
		UUID: "a1",
		Kind: KindAssistant,
		Content: []ContentBlock{
			{
				Type:     "tool_use",
				ToolName: "Task",
				ToolInput: map[string]any{
					"subagent_type": "Explore",
					"description":   "find hot spots",
				},
			},
		},
	}
	user := &Message{UUID: "u1", Kind: KindUserPrompt, Children: []*Message{assistant}}
	session := &Session{RootMessages: []*Message{user}}
	ex := ComputeExchanges(FlattenSessionMessages(session))
	if len(ex) != 1 || len(ex[0].Steps) != 1 {
		t.Fatalf("expected 1 exchange with 1 step, got %#v", ex)
	}
	step := ex[0].Steps[0]
	if step.Kind != StepSubagent {
		t.Errorf("Kind = %v, want StepSubagent", step.Kind)
	}
	if step.Name != "Explore" {
		t.Errorf("Name = %q, want Explore", step.Name)
	}
	if step.Label != "[Explore] find hot spots" {
		t.Errorf("Label = %q, want [Explore] find hot spots", step.Label)
	}
}

func TestComputeExchanges_SkillLabelHasSlashAndArgs(t *testing.T) {
	assistant := &Message{
		UUID: "a1",
		Kind: KindAssistant,
		Content: []ContentBlock{
			{
				Type:     "tool_use",
				ToolName: "Skill",
				ToolInput: map[string]any{
					"skill": "commit",
					"args":  "-m 'fix'",
				},
			},
		},
	}
	user := &Message{UUID: "u1", Kind: KindUserPrompt, Children: []*Message{assistant}}
	session := &Session{RootMessages: []*Message{user}}
	ex := ComputeExchanges(FlattenSessionMessages(session))
	if len(ex) != 1 || len(ex[0].Steps) != 1 {
		t.Fatalf("expected 1 exchange with 1 step, got %#v", ex)
	}
	step := ex[0].Steps[0]
	if step.Kind != StepSkill {
		t.Errorf("Kind = %v, want StepSkill", step.Kind)
	}
	if step.Label != "/commit -m 'fix'" {
		t.Errorf("Label = %q, want /commit -m 'fix'", step.Label)
	}
}

func TestExchange_Duration(t *testing.T) {
	start := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)
	ex := &Exchange{Start: start, End: start.Add(2 * time.Minute)}
	if ex.Duration() != 2*time.Minute {
		t.Errorf("Duration = %v, want 2m", ex.Duration())
	}
	var nilEx *Exchange
	if nilEx.Duration() != 0 {
		t.Error("nil Exchange.Duration should be 0")
	}
	zero := &Exchange{}
	if zero.Duration() != 0 {
		t.Error("zero-time Exchange.Duration should be 0")
	}
	reversed := &Exchange{Start: start.Add(time.Minute), End: start}
	if reversed.Duration() != 0 {
		t.Error("reversed times should return 0")
	}
}

func TestExchange_CountSteps(t *testing.T) {
	var ex *Exchange
	if ex.CountSteps(StepSubagent) != 0 {
		t.Error("nil Exchange.CountSteps should be 0")
	}
	ex = &Exchange{
		Steps: []Step{
			{Kind: StepSubagent},
			{Kind: StepSubagent},
			{Kind: StepSkill},
		},
	}
	if ex.CountSteps(StepSubagent) != 2 {
		t.Errorf("Subagent count = %d, want 2", ex.CountSteps(StepSubagent))
	}
	if ex.CountSteps(StepSkill) != 1 {
		t.Errorf("Skill count = %d, want 1", ex.CountSteps(StepSkill))
	}
	if ex.CountSteps(StepToolUse) != 0 {
		t.Errorf("Tool count = %d, want 0", ex.CountSteps(StepToolUse))
	}
}

func TestComputeTurnStats_Alias(t *testing.T) {
	// The deprecated alias should forward to ComputeExchanges and
	// return the same Exchange type (which aliases TurnStats).
	assistant := &Message{UUID: "a1", Kind: KindAssistant, Timestamp: time.Now()}
	user := &Message{UUID: "u1", Kind: KindUserPrompt, Timestamp: time.Now(), Children: []*Message{assistant}}
	session := &Session{RootMessages: []*Message{user}}
	msgs := FlattenSessionMessages(session)

	viaAlias := ComputeTurnStats(msgs)
	viaDirect := ComputeExchanges(msgs)
	if len(viaAlias) != len(viaDirect) {
		t.Errorf("alias returned different count: %d vs %d", len(viaAlias), len(viaDirect))
	}
	// TurnStats is a type alias, so this comparison is legal
	var _ *TurnStats = viaAlias[0]
	var _ *Exchange = viaAlias[0]
}
