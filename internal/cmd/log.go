package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/insight"
	"github.com/thevibeworks/ccx/internal/sessionlog"
)

var logCmd = &cobra.Command{
	Use:   "log [project]",
	Short: "Slice session logs by time scope",
	Long: `Slice raw coding-agent session logs by timestamp.

Sessions can run for days or months, so time-scoped review must use log
records, not just session end times. This command emits the evidence layer
for scoped insight: records inside the window plus session overlap metadata.

--kind narrows to record kinds (comma-separated: user_prompt,
assistant_message, tool_call, tool_result, reasoning, ...); --match
keeps records whose raw transcript line contains a phrase (-w for
whole words). Together they turn the firehose into a timeline:
every human prompt today, across every session, in order.

Examples:
  ccx log --scope yesterday --tz +8 --all --json
  ccx log --since 2026-05-21 --until 2026-05-22 --tz +8 --all --json
  ccx log --scope week --provider cx --json
  ccx log /path/to/repo --scope yesterday --json --raw
  ccx log --scope today --all --kind user_prompt        # the humans in the loop, today
  ccx log --scope month --all --match semantica -w      # when a term came up, in a window`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLog,
}

var (
	logScope    string
	logTZ       string
	logSince    string
	logUntil    string
	logJSON     bool
	logRaw      bool
	logAll      bool
	logProvider string
	logLimit    int
	logKinds    string
	logMatch    string
	logWord     bool
	logLocation *time.Location
)

func init() {
	logCmd.Flags().StringVar(&logScope, "scope", "", "scope: today, yesterday, week, month, quarter, year")
	logCmd.Flags().StringVar(&logTZ, "tz", "local", "timezone for scope/date parsing: IANA name, UTC, local, or offset like +8")
	logCmd.Flags().StringVar(&logSince, "since", "", "start timestamp or date (RFC3339 or YYYY-MM-DD in --tz)")
	logCmd.Flags().StringVar(&logUntil, "until", "", "exclusive end timestamp or date (RFC3339 or YYYY-MM-DD in --tz)")
	logCmd.Flags().BoolVar(&logJSON, "json", false, "output as JSON")
	logCmd.Flags().BoolVar(&logRaw, "raw", false, "include raw JSONL lines in JSON output")
	logCmd.Flags().BoolVar(&logAll, "all", false, "slice logs across all projects")
	logCmd.Flags().StringVarP(&logProvider, "provider", "p", "", "filter by provider: cc, cx, all")
	logCmd.Flags().IntVarP(&logLimit, "limit", "n", 0, "limit records in JSON output (0 = no limit)")
	logCmd.Flags().StringVar(&logKinds, "kind", "", "keep only these record kinds (comma-separated, e.g. user_prompt,assistant_message)")
	logCmd.Flags().StringVar(&logMatch, "match", "", "keep only records whose raw transcript line contains this phrase")
	logCmd.Flags().BoolVarP(&logWord, "word", "w", false, "--match whole words only")
}

func runLog(cmd *cobra.Command, args []string) error {
	loc, err := insight.LoadLocation(logTZ)
	if err != nil {
		return fmt.Errorf("invalid --tz %q: %w", logTZ, err)
	}
	logLocation = loc
	start, end, scopeName, scopeLabel, err := logWindow(loc)
	if err != nil {
		return err
	}

	workspacePath := ""
	projectName := ""
	if len(args) > 0 {
		projectName = args[0]
	} else if !logAll {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		workspacePath = cwd
	}

	providerFilter := config.NormalizeProvider(logProvider)
	if !validLogProvider(providerFilter) {
		return fmt.Errorf("invalid --provider %q (want cc, cx, claude-code, codex, or all)", logProvider)
	}

	var kinds []string
	if strings.TrimSpace(logKinds) != "" {
		kinds = strings.Split(logKinds, ",")
	}
	var match func(string) bool
	if strings.TrimSpace(logMatch) != "" {
		m := newTextMatcher(logMatch, logWord)
		match = m.matches
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
		Limit:         logLimit,
		IncludeRaw:    logRaw,
		Now:           time.Now().In(loc),
		Kinds:         kinds,
		Match:         match,
	})
	if err != nil {
		return err
	}
	if logJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(bundle)
	}
	return printLogTable(bundle)
}

func validLogProvider(provider string) bool {
	return provider == "" || provider == "claude-code" || provider == "codex"
}

func logSources(settings *config.Settings, providerFilter string) []sessionlog.Source {
	var sources []sessionlog.Source
	if (providerFilter == "" || providerFilter == "claude-code") && settings.ProviderEnabled("claude-code") {
		sources = append(sources, sessionlog.Source{Provider: "claude-code", Home: settings.ClaudeHome})
	}
	if (providerFilter == "" || providerFilter == "codex") && settings.ProviderEnabled("codex") {
		sources = append(sources, sessionlog.Source{Provider: "codex", Home: settings.CodexHome})
	}
	return sources
}

func logWindow(loc *time.Location) (time.Time, time.Time, string, string, error) {
	if logScope != "" && (logSince != "" || logUntil != "") {
		return time.Time{}, time.Time{}, "", "", fmt.Errorf("--scope cannot be combined with --since or --until")
	}
	if logScope != "" || (logSince == "" && logUntil == "") {
		scope, err := insight.ParseScope(logScope)
		if err != nil {
			return time.Time{}, time.Time{}, "", "", err
		}
		start, end, label := insight.ScopeWindow(scope, time.Now(), loc)
		return start, end, string(scope), label, nil
	}
	start, err := parseLogTime(logSince, loc)
	if err != nil {
		return time.Time{}, time.Time{}, "", "", fmt.Errorf("invalid --since: %w", err)
	}
	end, err := parseLogTime(logUntil, loc)
	if err != nil {
		return time.Time{}, time.Time{}, "", "", fmt.Errorf("invalid --until: %w", err)
	}
	if start.IsZero() || end.IsZero() {
		return time.Time{}, time.Time{}, "", "", fmt.Errorf("--since and --until are both required when --scope is not used")
	}
	if !start.Before(end) {
		return time.Time{}, time.Time{}, "", "", fmt.Errorf("--since must be before --until")
	}
	label := fmt.Sprintf("%s to %s", start.In(loc).Format("Jan 2 15:04"), end.In(loc).Format("Jan 2 15:04"))
	return start, end, "custom", label, nil
}

func parseLogTime(raw string, loc *time.Location) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed, nil
		}
	}
	if parsed, err := time.ParseInLocation("2006-01-02", raw, loc); err == nil {
		return parsed, nil
	}
	return time.Time{}, fmt.Errorf("expected RFC3339 timestamp or YYYY-MM-DD date")
}

func printLogTable(bundle *sessionlog.Bundle) error {
	if bundle == nil {
		return nil
	}
	fmt.Printf("ccx log · %s · %s\n", bundle.Scope.Label, bundle.Scope.TimeZone)
	recordsLabel := fmt.Sprintf("records %d", bundle.Metrics.Records)
	if bundle.Metrics.RecordsMatched > 0 || len(bundle.Records) == 0 && bundle.Metrics.Records > 0 && bundle.Metrics.RecordsReturned == 0 {
		recordsLabel = fmt.Sprintf("records %d · matched %d", bundle.Metrics.Records, bundle.Metrics.RecordsMatched)
	}
	if bundle.Metrics.RecordsReturned != bundle.Metrics.Records && bundle.Metrics.RecordsReturned != bundle.Metrics.RecordsMatched {
		recordsLabel += fmt.Sprintf(" · showing %d", bundle.Metrics.RecordsReturned)
	}
	fmt.Printf("%s to %s · source log files %d · %s\n\n",
		bundle.Scope.Start.Format("2006-01-02 15:04"),
		bundle.Scope.End.Format("2006-01-02 15:04"),
		bundle.Metrics.Sessions,
		recordsLabel)
	if len(bundle.Records) == 0 {
		fmt.Println("No log records in this scope.")
		return nil
	}
	loc := logLocation
	if loc == nil {
		loc = time.Local
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "WHEN\tSRC\tKIND\tSESSION\tPROJECT\tTEXT")
	for _, record := range bundle.Records {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			record.Timestamp.In(loc).Format("Jan 02 15:04"),
			providerTag(record.Provider),
			record.Kind,
			truncateID(record.SessionID, 8),
			truncateDisplay(record.Project, 20),
			truncateDisplay(record.Text, 96))
	}
	return w.Flush()
}
