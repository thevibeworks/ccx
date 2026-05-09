package web

import (
	"strings"
	"testing"
)

// TestRenderToolInput_BashClaudeShape verifies the Claude Code
// {command, description} shape renders the command as a shell line
// and shows the description.
func TestRenderToolInput_BashClaudeShape(t *testing.T) {
	var b strings.Builder
	renderToolInput(&b, "Bash", map[string]any{
		"command":     "go test ./...",
		"description": "run the full test suite",
	})
	out := b.String()
	if !strings.Contains(out, `class="bash-cmd"`) {
		t.Errorf("expected bash-cmd wrapper, got: %s", out)
	}
	if !strings.Contains(out, "$ go test ./...") {
		t.Errorf("expected shell-prefixed command, got: %s", out)
	}
	if !strings.Contains(out, "run the full test suite") {
		t.Errorf("description missing, got: %s", out)
	}
}

// TestRenderToolInput_BashCodexShape verifies the Codex
// {cmd, workdir, max_output_tokens} shape is handled — this is what
// broke rendering for the 019d8f43 session before the fix.
func TestRenderToolInput_BashCodexShape(t *testing.T) {
	var b strings.Builder
	renderToolInput(&b, "Bash", map[string]any{
		"cmd":               "rg --files internal/web",
		"workdir":           "/Users/eric/ccx",
		"max_output_tokens": 4000,
	})
	out := b.String()
	if !strings.Contains(out, "$ rg --files internal/web") {
		t.Errorf("Codex 'cmd' shape should render as shell, got: %s", out)
	}
	if !strings.Contains(out, "/Users/eric/ccx") {
		t.Errorf("workdir should appear, got: %s", out)
	}
	// Must NOT fall through to raw JSON dump — that's the old broken
	// behavior that showed `{"cmd":"...","workdir":"..."}` as literal text.
	if strings.Contains(out, `"cmd":`) || strings.Contains(out, `"workdir":`) {
		t.Errorf("should not leak raw JSON object syntax, got: %s", out)
	}
}

// TestRenderToolInput_BashArgvShape verifies argv-as-array shape
// (legacy event_msg.exec_command_end path).
func TestRenderToolInput_BashArgvShape(t *testing.T) {
	var b strings.Builder
	renderToolInput(&b, "Bash", map[string]any{
		"argv": []any{"/bin/sh", "-lc", "echo hello"},
		"cwd":  "/tmp",
	})
	out := b.String()
	if !strings.Contains(out, "$ /bin/sh -lc echo hello") {
		t.Errorf("argv should be joined and rendered as shell, got: %s", out)
	}
}

// TestRenderToolInput_ApplyPatchStringInput verifies the raw-patch
// string shape (Codex custom_tool_call with input = patch text).
// Before the fix this rendered as a JSON-quoted scalar which was
// unreadable.
func TestRenderToolInput_ApplyPatchStringInput(t *testing.T) {
	var b strings.Builder
	patch := "*** Begin Patch\n*** Update File: foo.go\n@@\n-old\n+new\n*** End Patch"
	renderToolInput(&b, "ApplyPatch", patch)
	out := b.String()
	if !strings.Contains(out, "apply-patch") {
		t.Errorf("expected .apply-patch class, got: %s", out)
	}
	if !strings.Contains(out, "*** Begin Patch") {
		t.Errorf("patch body should render verbatim, got: %s", out)
	}
	// Must NOT wrap in JSON quotes like `"*** Begin Patch\n..."`
	if strings.HasPrefix(strings.TrimSpace(stripTags(out)), `"`) {
		t.Errorf("patch should not be JSON-quoted, got: %s", out)
	}
}

// TestRenderToolInput_UpdatePlan verifies the Codex plan tool renders
// as a checklist, not raw JSON.
func TestRenderToolInput_UpdatePlan(t *testing.T) {
	var b strings.Builder
	renderToolInput(&b, "UpdatePlan", map[string]any{
		"explanation": "Mapping out the review items.",
		"plan": []any{
			map[string]any{"step": "Read the review", "status": "completed"},
			map[string]any{"step": "Apply the fixes", "status": "in_progress"},
			map[string]any{"step": "Run tests", "status": "pending"},
		},
	})
	out := b.String()
	if !strings.Contains(out, "update-plan") {
		t.Errorf("expected .update-plan wrapper, got: %s", out)
	}
	if !strings.Contains(out, "Mapping out the review items") {
		t.Errorf("explanation missing, got: %s", out)
	}
	if !strings.Contains(out, "plan-done") {
		t.Errorf("completed step should get .plan-done class, got: %s", out)
	}
	if !strings.Contains(out, "plan-active") {
		t.Errorf("in_progress step should get .plan-active class, got: %s", out)
	}
	if !strings.Contains(out, "plan-pending") {
		t.Errorf("pending step should get .plan-pending class, got: %s", out)
	}
	if !strings.Contains(out, "Read the review") {
		t.Errorf("step text missing, got: %s", out)
	}
}

// stripTags is a minimal HTML tag stripper for test assertions that
// want to look at the inner text of a rendered block.
func stripTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return b.String()
}
