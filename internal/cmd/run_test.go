package cmd

import (
	"strings"
	"testing"

	"github.com/thevibeworks/ccx/skills"
)

// Every runner must produce a headless invocation whose final
// argument is the prompt, and map to a real ccx provider ID: the
// receipt's session link depends on both.
func TestRunnersHeadlessArgv(t *testing.T) {
	wantProviders := map[string]string{
		"claude": "claude-code",
		"codex":  "codex",
		"grok":   "grok",
	}
	for agent, wantProvider := range wantProviders {
		spec, ok := runners[agent]
		if !ok {
			t.Fatalf("runner %q missing", agent)
		}
		if spec.providerID != wantProvider {
			t.Errorf("%s provider: got %q, want %q", agent, spec.providerID, wantProvider)
		}
		argv := spec.argv("PROMPT-SENTINEL")
		if argv[0] != spec.binary {
			t.Errorf("%s argv[0]: got %q, want %q", agent, argv[0], spec.binary)
		}
		if argv[len(argv)-1] != "PROMPT-SENTINEL" {
			t.Errorf("%s: prompt must be the final argument, got %v", agent, argv)
		}
		// Headless flags, not interactive defaults.
		joined := strings.Join(argv, " ")
		switch agent {
		case "claude":
			if !strings.Contains(joined, "-p") {
				t.Errorf("claude must run with -p (print mode): %v", argv)
			}
		case "codex":
			if !strings.Contains(joined, "exec") {
				t.Errorf("codex must run with exec: %v", argv)
			}
		case "grok":
			if !strings.Contains(joined, "--single") {
				t.Errorf("grok must run with --single: %v", argv)
			}
		}
	}
}

func TestBuildRunPromptCarriesSkillAndTask(t *testing.T) {
	p := buildRunPrompt("ccx-recap", "SKILL BODY HERE", "recap yesterday")
	for _, want := range []string{"ccx-recap", "SKILL BODY HERE", "TASK: recap yesterday"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	// No task: the skill applies to the current workspace, explicitly.
	p = buildRunPrompt("ccx", "BODY", "")
	if !strings.Contains(p, "current workspace") {
		t.Error("empty task must still give the agent a concrete instruction")
	}
}

// Every bundled skill must resolve — `ccx run` is only as trustworthy
// as its claim that the payload is the bundled skill text.
func TestBundledSkillsResolve(t *testing.T) {
	entries, err := skills.FS.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		body, err := skills.FS.ReadFile(e.Name() + "/SKILL.md")
		if err != nil || len(body) == 0 {
			t.Errorf("bundled skill %s unreadable: %v", e.Name(), err)
		}
		found++
	}
	if found < 3 {
		t.Errorf("expected at least 3 bundled skills, found %d", found)
	}
}

func TestQuoteArgvElidesPayload(t *testing.T) {
	long := strings.Repeat("x", 500)
	out := quoteArgv([]string{"claude", "-p", long})
	if out[2] == long {
		t.Error("large payload must be elided in the human-facing command line")
	}
	if !strings.Contains(out[2], "500 chars") {
		t.Errorf("elision should state the payload size: %q", out[2])
	}
}
