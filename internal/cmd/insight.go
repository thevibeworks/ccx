package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/thevibeworks/ccx/internal/catalog"
	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/insight"
	"github.com/thevibeworks/ccx/internal/provider"
)

var insightCmd = &cobra.Command{
	Use:   "insight [today|week|month|quarter|year]",
	Short: "Summarize session intelligence for a time scope",
	Long: `Summarize coding-agent session intelligence for a chosen time scope.

Insight answers what is active, what needs closure, what completed, and which
patterns or blockers are emerging. The CLI output is deterministic from
session metadata and summaries. Use the ccx-insight skill for deeper
agent-authored synthesis over the JSON output.

Examples:
  ccx insight
  ccx insight week --tz America/Los_Angeles
  ccx insight month --all --json
  ccx insight quarter --provider cx --limit 12`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInsight,
}

var (
	insightScope    string
	insightTZ       string
	insightJSON     bool
	insightAll      bool
	insightProject  string
	insightProvider string
	insightLimit    int
)

func init() {
	insightCmd.Flags().StringVar(&insightScope, "scope", "", "scope: today, week, month, quarter, year")
	insightCmd.Flags().StringVar(&insightTZ, "tz", "local", "IANA timezone, UTC, or local")
	insightCmd.Flags().BoolVar(&insightJSON, "json", false, "output as JSON")
	insightCmd.Flags().BoolVar(&insightAll, "all", false, "summarize sessions across all projects")
	insightCmd.Flags().StringVarP(&insightProject, "project", "p", "", "project name or path")
	insightCmd.Flags().StringVar(&insightProvider, "provider", "", "filter by provider: cc, cx, all")
	insightCmd.Flags().IntVar(&insightLimit, "limit", 8, "max rows per section")
}

func runInsight(cmd *cobra.Command, args []string) error {
	rawScope := insightScope
	if len(args) > 0 {
		rawScope = args[0]
	}
	scope, err := insight.ParseScope(rawScope)
	if err != nil {
		return err
	}
	loc, err := insight.LoadLocation(insightTZ)
	if err != nil {
		return fmt.Errorf("invalid --tz %q: %w", insightTZ, err)
	}

	backend := provider.Default()
	now := time.Now()
	start, end, _ := insight.ScopeWindow(scope, now, loc)
	query := catalog.SessionQuery{
		Filter: config.SessionFilter{
			Provider: config.NormalizeProvider(insightProvider),
			After:    start,
			Before:   end,
		},
		Sort: catalog.SortTime,
	}
	if insightProject != "" {
		query.Scope = catalog.ScopeProject
		query.ProjectName = insightProject
	} else if insightAll {
		query.Scope = catalog.ScopeAll
	} else {
		current, err := currentWorkspaceQuery()
		if err != nil {
			return err
		}
		query.Scope = current.Scope
		query.WorkspacePath = current.WorkspacePath
	}

	sessions, err := backend.ListSessions(query)
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}
	result := insight.Analyze(sessions, insight.Options{
		Scope:    scope,
		Location: loc,
		Now:      now,
		Limit:    insightLimit,
		Provider: config.NormalizeProvider(insightProvider),
		Project:  insightProject,
	})

	if insightJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	return printInsight(result)
}

func printInsight(result *insight.Summary) error {
	if result == nil {
		return nil
	}
	fmt.Printf("ccx insight · %s · %s\n", result.Scope.Label, result.Scope.TimeZone)
	fmt.Printf("%s to %s\n\n",
		result.Scope.Start.Format("2006-01-02 15:04"),
		result.Scope.End.Format("2006-01-02 15:04"))

	fmt.Printf("Sessions %d · Projects %d · Messages %d · Tools %d · Tokens %s · Cost %s\n\n",
		result.Metrics.Sessions,
		result.Metrics.Projects,
		result.Metrics.Messages,
		result.Metrics.ToolCalls,
		formatInsightTokens(result.Metrics.TotalTokens),
		formatInsightCost(result.Metrics.CostUSD))

	if result.Metrics.Sessions == 0 {
		fmt.Println("No sessions in this scope.")
		return nil
	}

	printInsightSessions("Currently being worked on", result.Current)
	printInsightSignals("Needs closure", result.OpenLoops)
	printInsightSessions("Completed", result.Completed)
	printInsightSignals("Achievements", result.Achievements)
	printInsightSignals("Emerging signals", result.Patterns)
	printInsightProjects(result.Projects)
	return nil
}

func printInsightSessions(title string, sessions []insight.SessionRef) {
	if len(sessions) == 0 {
		return
	}
	fmt.Println(title)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "WHEN\tSRC\tPROJECT\tSESSION\tSUMMARY")
	for _, s := range sessions {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			s.End.Format("Jan 02 15:04"),
			providerTag(s.Provider),
			truncateDisplay(s.Project, 24),
			truncateID(s.ID, 8),
			sessionSummaryPreview(s.Summary, 88))
	}
	_ = w.Flush()
	fmt.Println()
}

func printInsightSignals(title string, signals []insight.Signal) {
	if len(signals) == 0 {
		return
	}
	fmt.Println(title)
	for _, signal := range signals {
		count := ""
		if signal.Count > 0 {
			count = fmt.Sprintf(" (%d)", signal.Count)
		}
		evidence := ""
		if len(signal.EvidenceIDs) > 0 {
			evidence = " · " + strings.Join(signal.EvidenceIDs, ", ")
		}
		fmt.Printf("- %s%s: %s [%s]%s\n", signal.Label, count,
			sessionSummaryPreview(signal.Summary, 100), signal.Confidence, evidence)
	}
	fmt.Println()
}

func printInsightProjects(projects []insight.ProjectRow) {
	if len(projects) == 0 {
		return
	}
	fmt.Println("Project focus")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROJECT\tSESSIONS\tMESSAGES\tTOOLS\tTOKENS\tCOST")
	for _, project := range projects {
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%s\t%s\n",
			truncateDisplay(project.Name, 28),
			project.Sessions,
			project.Messages,
			project.Tools,
			formatInsightTokens(project.Tokens),
			formatInsightCost(project.CostUSD))
	}
	_ = w.Flush()
	fmt.Println()
}

func formatInsightTokens(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func formatInsightCost(usd float64) string {
	if usd <= 0 {
		return "$0.00"
	}
	if usd < 0.01 {
		return "<$0.01"
	}
	if usd < 1 {
		return fmt.Sprintf("$%.4f", usd)
	}
	if usd < 100 {
		return fmt.Sprintf("$%.2f", usd)
	}
	return fmt.Sprintf("$%.0f", usd)
}
