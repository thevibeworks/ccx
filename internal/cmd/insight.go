package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/thevibeworks/ccx/internal/catalog"
	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/insight"
	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/provider"
	"github.com/thevibeworks/ccx/internal/sessionlog"
)

var insightCmd = &cobra.Command{
	Use:     "insight [project]",
	Aliases: []string{"report"},
	Short:   "Digest every session in a time window: prompts, answers, edits, cost (alias: report)",
	Long: `Digest the sessions active in a time window, across every workspace.

For each session: the human's prompts, the agent's final answer, file
edits and commits, interrupts and denials, tokens and cost — joined to
the provider's own accounting (model, summary, usage). Plus rollups by
workspace, day, provider and model, with cost coverage (the share of
tokens a pricing row covers) so an unpriced model never reads as free.

--json emits the digest (ccx.insight.v1) for skills; it is bounded
(a month of heavy use is a few MB, not hundreds). --records adds the
raw log records (ccx log's payload), -n limits them. Without --json the
same digest is rendered as a self-contained HTML cockpit, saved under
$XDG_DATA_HOME/ccx/insights/ and browsable at /insights in ccx web.

Examples:
  ccx insight --scope week --all --json          # the digest, for a recap
  ccx insight --scope month --all                # HTML cockpit into the insights dir
  ccx insight --since 2026-05-21 --until 2026-05-22 --tz +8 --all
  ccx insight --scope today --all --json --records -n 2000`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInsight,
}

var (
	insightScope    string
	insightTZ       string
	insightSince    string
	insightUntil    string
	insightJSON     bool
	insightAll      bool
	insightProvider string
	insightOutput   string
	insightRecords  bool
	insightLimit    int
)

func init() {
	insightCmd.Flags().StringVar(&insightScope, "scope", "today", "scope: today, yesterday, week, month, quarter, year")
	insightCmd.Flags().StringVar(&insightTZ, "tz", "local", "timezone: IANA name, UTC, local, or offset like +8")
	insightCmd.Flags().StringVar(&insightSince, "since", "", "start date (YYYY-MM-DD)")
	insightCmd.Flags().StringVar(&insightUntil, "until", "", "exclusive end date (YYYY-MM-DD)")
	insightCmd.Flags().BoolVar(&insightJSON, "json", false, "output the digest as JSON (ccx.insight.v1)")
	insightCmd.Flags().BoolVar(&insightAll, "all", false, "across all projects")
	insightCmd.Flags().StringVarP(&insightProvider, "provider", "p", "", "filter by provider: cc, cx, all")
	insightCmd.Flags().StringVarP(&insightOutput, "output", "o", "", "output file path (default: insights dir)")
	insightCmd.Flags().BoolVar(&insightRecords, "records", false, "include the raw log records in the output")
	insightCmd.Flags().IntVarP(&insightLimit, "limit", "n", 0, "limit records when --records is set (0 = no limit)")
}

func runInsight(cmd *cobra.Command, args []string) error {
	loc, err := insight.LoadLocation(insightTZ)
	if err != nil {
		return fmt.Errorf("invalid --tz %q: %w", insightTZ, err)
	}

	logSince = insightSince
	logUntil = insightUntil
	logTZ = insightTZ

	if (insightSince != "" || insightUntil != "") && !cmd.Flags().Changed("scope") {
		logScope = ""
	} else {
		logScope = insightScope
	}

	start, end, scopeName, scopeLabel, err := logWindow(loc)
	if err != nil {
		return err
	}

	workspacePath := ""
	projectName := ""
	if len(args) > 0 {
		projectName = args[0]
	} else if !insightAll {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		workspacePath = cwd
	}

	providerFilter := config.NormalizeProvider(insightProvider)
	if !validLogProvider(providerFilter) {
		return fmt.Errorf("invalid --provider %q", insightProvider)
	}

	settings := config.Load()
	bundle, err := sessionlog.Collect(logSources(settings, providerFilter), sessionlog.Options{
		Start:         start,
		End:           end,
		ScopeName:     scopeName,
		ScopeLabel:    scopeLabel,
		TimeZone:      loc.String(),
		Provider:      providerFilter,
		WorkspacePath: workspacePath,
		ProjectName:   projectName,
		IncludeRaw:    false,
		Now:           time.Now().In(loc),
	})
	if err != nil {
		return err
	}
	if projectName != "" && len(bundle.Sessions) == 0 {
		warnUnknownLogProject(projectName)
	}

	report := insight.BuildReport(bundle, sessionAccounting(providerFilter), insightRecords, insightLimit)
	report.Command = insightCommand(cmd, args)

	if insightJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	page := insight.RenderHTML(report)
	outputPath := insightOutput
	if outputPath == "" {
		name := fmt.Sprintf("%s-%s.html", time.Now().In(loc).Format("2006-01-02"), scopeName)
		var saveErr error
		outputPath, saveErr = insight.SaveReport(name, page)
		if saveErr != nil {
			return saveErr
		}
	} else if err := os.WriteFile(outputPath, page, 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	m := report.Metrics
	fmt.Fprintf(os.Stderr, "%d sessions · %d workspaces · %d prompts · %d edits · %d commits · %s",
		m.Sessions, m.Workspaces, m.UserPrompts, m.Edits, m.Commits, formatInsightCost(m))
	fmt.Fprintf(os.Stderr, "\nReport saved: %s\n", outputPath)
	return nil
}

// sessionAccounting joins the log slice to the provider catalog: model,
// summary, tokens, cost per session id. The catalog is the same
// quick-parse index `ccx sessions` reads, so this costs one listing.
func sessionAccounting(providerFilter string) insight.SessionLookup {
	backend := provider.Default()
	sessions, err := backend.ListSessions(catalog.SessionQuery{
		Scope:  catalog.ScopeAll,
		Filter: config.SessionFilter{Provider: providerFilter},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: session accounting unavailable: %v\n", err)
		return nil
	}
	byID := make(map[string]*parser.Session, len(sessions))
	for _, s := range sessions {
		byID[s.ID] = s
	}
	return func(id string) *parser.Session { return byID[id] }
}

func formatInsightCost(m insight.Metrics) string {
	switch {
	case m.Tokens == 0:
		return "no usage"
	case m.UnpricedTokens == 0:
		return fmt.Sprintf("$%.0f", m.CostUSD)
	}
	return fmt.Sprintf("$%.0f (covers %.0f%% of tokens; %d sessions unpriced)", m.CostUSD, m.CostCoverage*100, m.UnpricedSessions)
}

// insightCommand reconstructs the evidence command for the report
// header so the reader can re-run it.
func insightCommand(cmd *cobra.Command, args []string) string {
	parts := []string{"ccx insight"}
	parts = append(parts, args...)
	if insightSince != "" || insightUntil != "" {
		parts = append(parts, "--since", insightSince, "--until", insightUntil)
	} else {
		parts = append(parts, "--scope", insightScope)
	}
	if insightTZ != "local" {
		parts = append(parts, "--tz", insightTZ)
	}
	if insightAll {
		parts = append(parts, "--all")
	}
	if insightProvider != "" {
		parts = append(parts, "--provider", insightProvider)
	}
	parts = append(parts, "--json")
	return strings.Join(parts, " ")
}
