package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/thevibeworks/ccx/internal/catalog"
	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/provider"
	"github.com/thevibeworks/ccx/skills"
)

// ccx run is a bridge, not an agent loop: it launches an INSTALLED
// agent CLI with one of ccx's bundled skills as the prompt, then
// records a receipt pointing at the provider-native session the run
// produced. The provider CLI owns permissions, sandboxing, streaming,
// and the session file; ccx never duplicates that machinery and never
// writes into provider homes (docs/2026-q3-goal.md, Phase 4).

var (
	runAgent  string
	runDryRun bool
)

var runCmd = &cobra.Command{
	Use:   "run <skill> [task...]",
	Short: "Run a bundled ccx skill through an installed agent CLI",
	Long: `Launch an installed agent CLI (claude, codex, or grok) non-interactively
with one of ccx's bundled skills as the prompt, plus an optional task.

The agent CLI owns permissions and sandboxing — ccx passes no
permission flags, so the run inherits exactly the defaults you have
configured for that CLI. The session the run produces is written by
the provider into its own home, readable afterwards with ccx trace.
A receipt linking the run to that session lands in ccx's data dir.

Use --dry-run to see the exact command, the skill payload, and the
permission posture without executing anything.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSkill(args[0], strings.Join(args[1:], " "))
	},
}

func init() {
	runCmd.Flags().StringVar(&runAgent, "agent", "claude", "agent CLI to run: claude, codex, grok")
	runCmd.Flags().BoolVar(&runDryRun, "dry-run", false, "print the command, payload, and permission posture without executing")
	rootCmd.AddCommand(runCmd)
}

// runnerSpec describes how one agent CLI is invoked headlessly and
// which ccx provider its sessions surface under.
type runnerSpec struct {
	binary     string
	providerID string
	// argv builds the full invocation for a prompt.
	argv func(prompt string) []string
}

var runners = map[string]runnerSpec{
	"claude": {
		binary:     "claude",
		providerID: "claude-code",
		argv:       func(p string) []string { return []string{"claude", "-p", p} },
	},
	"codex": {
		binary:     "codex",
		providerID: "codex",
		argv:       func(p string) []string { return []string{"codex", "exec", p} },
	},
	"grok": {
		binary:     "grok",
		providerID: "grok",
		argv:       func(p string) []string { return []string{"grok", "--single", p} },
	},
}

// runReceipt is what ccx retains about a bridged run. It lives in
// ccx's own data dir — never in a provider home — and points at the
// provider-native session so `ccx trace <session>` re-opens the
// evidence.
type runReceipt struct {
	Agent      string    `json:"agent"`
	Provider   string    `json:"provider"`
	Skill      string    `json:"skill"`
	Task       string    `json:"task,omitempty"`
	Argv       []string  `json:"argv"`
	CWD        string    `json:"cwd"`
	Started    time.Time `json:"started"`
	Ended      time.Time `json:"ended"`
	ExitCode   int       `json:"exit_code"`
	SessionID  string    `json:"session_id,omitempty"`
	SessionLog string    `json:"session_log,omitempty"`
}

func runSkill(skillName, task string) error {
	spec, ok := runners[runAgent]
	if !ok {
		return fmt.Errorf("unknown agent %q (use claude, codex, or grok)", runAgent)
	}

	skillBody, err := skills.FS.ReadFile(skillName + "/SKILL.md")
	if err != nil {
		names, _ := skills.FS.ReadDir(".")
		var available []string
		for _, n := range names {
			if n.IsDir() {
				available = append(available, n.Name())
			}
		}
		return fmt.Errorf("no bundled skill %q (bundled: %s)", skillName, strings.Join(available, ", "))
	}

	prompt := buildRunPrompt(skillName, string(skillBody), task)
	argv := spec.argv(prompt)

	if runDryRun {
		printDryRun(spec, skillName, task, prompt, argv)
		return nil
	}

	binPath, err := exec.LookPath(spec.binary)
	if err != nil {
		return fmt.Errorf("%s CLI not found in PATH — install it or pick another --agent", spec.binary)
	}

	cwd, _ := os.Getwd()
	started := time.Now()
	fmt.Printf("ccx run: %s -> %s (skill %s)\n", runAgent, binPath, skillName)
	fmt.Printf("permissions: owned by %s — this run inherits your %s defaults, ccx passes no permission flags\n\n", spec.binary, spec.binary)

	c := exec.Command(binPath, argv[1:]...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	runErr := c.Run()
	ended := time.Now()

	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return fmt.Errorf("launching %s: %w", spec.binary, runErr)
		}
	}

	receipt := runReceipt{
		Agent:    runAgent,
		Provider: spec.providerID,
		Skill:    skillName,
		Task:     task,
		Argv:     argv,
		CWD:      cwd,
		Started:  started,
		Ended:    ended,
		ExitCode: exitCode,
	}

	// The provider wrote its own session during the run; find it so
	// the receipt is a two-way link (run -> session, session -> trace).
	if id, path := findProducedSession(spec.providerID, cwd, started); id != "" {
		receipt.SessionID = id
		receipt.SessionLog = path
	}

	receiptPath, err := writeReceipt(receipt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: receipt not written: %v\n", err)
	} else {
		fmt.Printf("\nreceipt: %s\n", receiptPath)
	}
	if receipt.SessionID != "" {
		fmt.Printf("session: %s — inspect with: ccx trace %s\n", receipt.SessionID, shortID(receipt.SessionID))
	} else {
		fmt.Printf("session: not located (provider may write asynchronously) — try: ccx sessions --provider %s\n", spec.providerID)
	}

	if exitCode != 0 {
		return fmt.Errorf("%s exited with code %d", spec.binary, exitCode)
	}
	return nil
}

func buildRunPrompt(name, body, task string) string {
	var b strings.Builder
	b.WriteString("You are running the ccx skill \"")
	b.WriteString(name)
	b.WriteString("\". Follow it exactly.\n\n")
	b.WriteString(body)
	b.WriteString("\n\n---\nTASK: ")
	if task != "" {
		b.WriteString(task)
	} else {
		b.WriteString("Apply this skill to the current workspace now.")
	}
	return b.String()
}

func printDryRun(spec runnerSpec, skillName, task, prompt string, argv []string) {
	fmt.Println("DRY RUN — nothing will be executed.")
	fmt.Printf("\nagent:    %s (%s sessions)\n", runAgent, spec.providerID)
	fmt.Printf("skill:    %s (bundled with ccx %s)\n", skillName, version)
	if task != "" {
		fmt.Printf("task:     %s\n", task)
	}
	fmt.Printf("command:  %s\n", strings.Join(quoteArgv(argv), " "))
	fmt.Printf("\npermissions: %s owns permissions and sandboxing for this run.\n", spec.binary)
	fmt.Printf("ccx passes NO permission flags — the run inherits your %s defaults\n", spec.binary)
	fmt.Println("(whatever tool approvals, sandbox mode, and allowlists you have configured).")
	fmt.Println("The session is written by the provider into its own home; ccx only reads it.")
	fmt.Printf("\npayload (%d chars):\n", len(prompt))
	preview := prompt
	if len(preview) > 800 {
		preview = preview[:800] + "\n[... truncated in preview; the full skill text is sent]"
	}
	fmt.Println(indent(preview, "  "))
}

// quoteArgv renders the argv for human eyes; the prompt argument is
// large, so it is elided rather than dumped inline.
func quoteArgv(argv []string) []string {
	out := make([]string, len(argv))
	for i, a := range argv {
		if len(a) > 60 {
			out[i] = fmt.Sprintf("'<payload %d chars>'", len(a))
			continue
		}
		out[i] = a
	}
	return out
}

func indent(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n")
}

// findProducedSession locates the provider session this run produced:
// the newest session for this workspace that started after the run
// began (with a small skew allowance for clock drift and provider
// bookkeeping).
func findProducedSession(providerID, cwd string, started time.Time) (id, path string) {
	backend := provider.Default()
	sessions, err := backend.ListSessions(catalog.SessionQuery{
		Scope:         catalog.ScopeWorkspace,
		WorkspacePath: cwd,
	})
	if err != nil {
		return "", ""
	}
	cutoff := started.Add(-1 * time.Minute)
	var best struct {
		id, path string
		start    time.Time
	}
	for _, s := range sessions {
		if s.Provider != providerID || s.StartTime.Before(cutoff) {
			continue
		}
		if s.StartTime.After(best.start) {
			best.id, best.path, best.start = s.ID, s.FilePath, s.StartTime
		}
	}
	return best.id, best.path
}

func writeReceipt(r runReceipt) (string, error) {
	dir := filepath.Join(config.DataDir(), "runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%s-%s.json", r.Started.UTC().Format("20060102T150405Z"), r.Agent, r.Skill)
	path := filepath.Join(dir, name)
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, data, 0o644)
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
