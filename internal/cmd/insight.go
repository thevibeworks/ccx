package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/insight"
	"github.com/thevibeworks/ccx/internal/sessionlog"
)

var insightCmd = &cobra.Command{
	Use:     "insight [project]",
	Aliases: []string{"report"},
	Short:   "Generate a data report from session logs (alias: report)",
	Long: `Generate an HTML data report from time-scoped session logs.

Reports are saved to $XDG_DATA_HOME/ccx/insights/ and can be browsed
via ccx web at /insights.

Examples:
  ccx insight --scope week --tz +8 --all
  ccx insight --scope today --json
  ccx insight --since 2026-05-21 --until 2026-05-22 --tz +8 --all
  ccx insight --scope month -o monthly.html`,
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
)

func init() {
	insightCmd.Flags().StringVar(&insightScope, "scope", "today", "scope: today, yesterday, week, month, quarter, year")
	insightCmd.Flags().StringVar(&insightTZ, "tz", "local", "timezone: IANA name, UTC, local, or offset like +8")
	insightCmd.Flags().StringVar(&insightSince, "since", "", "start date (YYYY-MM-DD)")
	insightCmd.Flags().StringVar(&insightUntil, "until", "", "end date (YYYY-MM-DD)")
	insightCmd.Flags().BoolVar(&insightJSON, "json", false, "output aggregated data as JSON (for LLM skill)")
	insightCmd.Flags().BoolVar(&insightAll, "all", false, "across all projects")
	insightCmd.Flags().StringVarP(&insightProvider, "provider", "p", "", "filter by provider: cc, cx, gx, all")
	insightCmd.Flags().StringVarP(&insightOutput, "output", "o", "", "output file path (default: insights dir)")
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

	if insightJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(bundle)
	}

	html := insight.GenerateHTMLReport(bundle)

	outputPath := insightOutput
	if outputPath == "" {
		name := fmt.Sprintf("%s-%s.html",
			time.Now().In(loc).Format("2006-01-02"),
			scopeName)
		var saveErr error
		outputPath, saveErr = insight.SaveReport(name, html)
		if saveErr != nil {
			return saveErr
		}
	} else {
		if err := os.WriteFile(outputPath, html, 0o644); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
	}

	fmt.Fprintf(os.Stderr, "Report saved: %s\n", outputPath)
	return nil
}
