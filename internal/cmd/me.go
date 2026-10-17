package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/insight"
	"github.com/thevibeworks/ccx/internal/me"
)

var meCmd = &cobra.Command{
	Use:   "me",
	Short: "What the prompt record says about the human at the keyboard",
	Long: `What the prompt record says about the human at the keyboard.

Every other command describes what the agent did. This one reads only
what you typed: when you work, how you steer, how often you take a
prompt back, how many workspaces you hold at once. It counts; it does
not judge, and no model is involved.

The source is the prompt history (~/.claude/history.jsonl,
~/.codex/history.jsonl), not the session files: history outlives
session cleanup, so it is the only record that reaches back past the
session horizon. Codex history records no workspace, so workspace
figures cover Claude Code prompts only.

A day runs 05:00 to 05:00, so a night session stays with the day it
began; a day end past 24:00 is after midnight.

--term counts prompts matching a regex (case-insensitive), in total
and per month, so any habit of speech can be trended without ccx
shipping a word list.

Examples:
  ccx me --tz +8                          # everything on record
  ccx me --since 2026-06-01 --tz +8       # from a date until now
  ccx me --term think=ultrathink --term again='\bagain\b|\bstill\b'
  ccx me --json                           # ccx.me.v1, for skills`,
	Args: cobra.NoArgs,
	RunE: runMe,
}

var (
	meTZ       string
	meSince    string
	meUntil    string
	meJSON     bool
	meProvider string
	meTop      int
	meTerms    []string
)

func init() {
	meCmd.Flags().StringVar(&meTZ, "tz", "local", "timezone: IANA name, UTC, local, or offset like +8")
	meCmd.Flags().StringVar(&meSince, "since", "", "start timestamp or date (RFC3339 or YYYY-MM-DD in --tz); default: first prompt")
	meCmd.Flags().StringVar(&meUntil, "until", "", "exclusive end timestamp or date; default: now")
	meCmd.Flags().BoolVar(&meJSON, "json", false, "output as JSON (ccx.me.v1)")
	meCmd.Flags().StringVarP(&meProvider, "provider", "p", "", "filter by provider: cc, cx, all")
	meCmd.Flags().IntVarP(&meTop, "limit", "n", 10, "rows in each ranked list")
	meCmd.Flags().StringArrayVar(&meTerms, "term", nil, "count prompts matching a regex, as name=regex (repeatable)")
	rootCmd.AddCommand(meCmd)
}

func runMe(cmd *cobra.Command, args []string) error {
	loc, err := insight.LoadLocation(meTZ)
	if err != nil {
		return fmt.Errorf("invalid --tz %q: %w", meTZ, err)
	}
	since, err := parseLogTime(meSince, loc)
	if err != nil {
		return fmt.Errorf("invalid --since: %w", err)
	}
	until, err := parseLogTime(meUntil, loc)
	if err != nil {
		return fmt.Errorf("invalid --until: %w", err)
	}
	if !since.IsZero() && !until.IsZero() && !since.Before(until) {
		return fmt.Errorf("--since must be before --until")
	}
	terms, err := parseMeTerms(meTerms)
	if err != nil {
		return err
	}
	providerFilter := config.NormalizeProvider(meProvider)
	if !validLogProvider(providerFilter) {
		return fmt.Errorf("invalid --provider %q (want cc, cx, claude-code, codex, or all)", meProvider)
	}

	settings := config.Load()
	prompts, sources := readPromptHistory(promptHistoryFiles(settings.ClaudeHome, settings.CodexHome), providerFilter, loc)
	profile := me.Build(prompts, me.Options{Location: loc, Since: since, Until: until, Terms: terms, Top: meTop})
	profile.Sources = sources
	if profile.Span.Prompts == 0 {
		return fmt.Errorf("no prompts on record in that window (history files: %s)", strings.Join(sourcePaths(sources), ", "))
	}

	if meJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(profile)
	}
	return printMe(profile)
}

// parseMeTerms turns name=regex flags into case-insensitive patterns.
func parseMeTerms(raw []string) ([]me.Term, error) {
	var terms []me.Term
	seen := map[string]bool{}
	for _, r := range raw {
		name, pattern, ok := strings.Cut(r, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" || pattern == "" {
			return nil, fmt.Errorf("invalid --term %q (want name=regex)", r)
		}
		if seen[name] {
			return nil, fmt.Errorf("--term %q given twice", name)
		}
		seen[name] = true
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid --term %q: %w", r, err)
		}
		terms = append(terms, me.Term{Name: name, Pattern: re})
	}
	return terms, nil
}

// readPromptHistory loads every prompt from the history files. A
// missing file is normal (the provider was never used) and still
// reported as a source with zero prompts.
func readPromptHistory(files map[string]string, providerFilter string, loc *time.Location) ([]me.Prompt, []me.Source) {
	var prompts []me.Prompt
	var sources []me.Source
	for _, provider := range []string{"claude-code", "codex"} {
		path, ok := files[provider]
		if !ok || (providerFilter != "" && providerFilter != provider) {
			continue
		}
		src := me.Source{Provider: provider, Path: path}
		var first, last time.Time
		file, err := os.Open(path)
		if err != nil {
			sources = append(sources, src)
			continue
		}
		reader := bufio.NewReaderSize(file, 64*1024)
		for {
			line, err := reader.ReadString('\n')
			if rec, ok := decodePromptLine(provider, line); ok {
				prompts = append(prompts, me.Prompt{Time: rec.time, Provider: provider, Workspace: rec.workspace, SessionID: rec.SessionID, Text: rec.text})
				src.Prompts++
				if !rec.time.IsZero() {
					if first.IsZero() || rec.time.Before(first) {
						first = rec.time
					}
					if rec.time.After(last) {
						last = rec.time
					}
				}
			}
			if err != nil {
				if err != io.EOF {
					fmt.Fprintf(os.Stderr, "warning: read error in %s: %v\n", filepath.Base(path), err)
				}
				break
			}
		}
		file.Close()
		if !first.IsZero() {
			src.First, src.Last = first.In(loc).Format(time.RFC3339), last.In(loc).Format(time.RFC3339)
		}
		sources = append(sources, src)
	}
	return prompts, sources
}

func sourcePaths(sources []me.Source) []string {
	paths := make([]string, 0, len(sources))
	for _, s := range sources {
		paths = append(paths, s.Path)
	}
	return paths
}

func printMe(p me.Profile) error {
	providers := make([]string, 0, len(p.Span.Providers))
	for _, c := range p.Span.Providers {
		providers = append(providers, fmt.Sprintf("%s %d", c.Key, c.Count))
	}
	fmt.Printf("ccx me · %s to %s · %s\n", dayOf(p.Span.First), dayOf(p.Span.Last), p.TimeZone)
	fmt.Printf("prompts %d (%s) · active %d of %d days · longest streak %d days from %s\n",
		p.Span.Prompts, strings.Join(providers, ", "), p.Span.ActiveDays, p.Span.CalendarDays,
		p.Tempo.LongestStreak.Days, p.Tempo.LongestStreak.Start)

	fmt.Printf("\nTEMPO  (a day runs %02d:00 to %02d:00)\n", p.Tempo.DayStartHour, p.Tempo.DayStartHour)
	fmt.Printf("  weekdays %.1f prompts/day over %d days · weekends %.1f over %d\n",
		p.Tempo.WeekdayPerDay, p.Tempo.WeekdayActiveDays, p.Tempo.WeekendPerDay, p.Tempo.WeekendActiveDays)
	fmt.Printf("  median day starts %s · ends %s\n", p.Tempo.MedianDayStart, p.Tempo.MedianDayEnd)
	if len(p.Tempo.Breaks) > 0 {
		parts := make([]string, 0, len(p.Tempo.Breaks))
		for _, b := range p.Tempo.Breaks {
			parts = append(parts, fmt.Sprintf("%d after %s", b.Days, b.After))
		}
		fmt.Printf("  breaks of 2+ idle days: %s\n", strings.Join(parts, " · "))
	} else {
		fmt.Println("  breaks of 2+ idle days: none")
	}
	peak := 0
	for _, n := range p.Tempo.Hours {
		if n > peak {
			peak = n
		}
	}
	for h, n := range p.Tempo.Hours {
		fmt.Printf("  %02d %6d %s\n", h, n, strings.Repeat("#", bar(n, peak, 48)))
	}

	termNames := make([]string, 0, len(p.Terms))
	for _, t := range p.Terms {
		termNames = append(termNames, t.Name)
	}
	fmt.Println("\nMONTHS  (CHARS = median prose length; CJK/ASK = share of prose; CMD/LATE = share of prompts; LATE = 23:00 to day roll)")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	header := "MONTH\tPROMPTS\tDAYS\tSPACES\tNEW\tPER DAY\tCHARS\tCJK\tASK\tCMD\tLATE\tENDS"
	for _, name := range termNames {
		header += "\t" + strings.ToUpper(name)
	}
	fmt.Fprintln(w, header)
	for _, m := range p.Months {
		row := fmt.Sprintf("%s\t%d\t%d\t%d\t%d\t%.1f\t%d\t%s\t%s\t%s\t%s\t%s",
			m.Key, m.Prompts, m.ActiveDays, m.Workspaces, m.NewWorkspaces, m.MedianWorkspacesPerDay, m.MedianChars,
			pct(m.CJKShare), pct(m.QuestionShare), pct(m.CommandShare), pct(m.LateShare), m.MedianDayEnd)
		for _, name := range termNames {
			row += fmt.Sprintf("\t%s", pct(termShare(m.Terms[name], m.Prompts)))
		}
		fmt.Fprintln(w, row)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	fmt.Println("\nSTEERING")
	fmt.Printf("  commands  %s\n", countLine(p.Commands))
	fmt.Printf("  rewinds   %d · median %ds after the prompt · %d of %d are followed by the same prompt again (%d longer, %d shorter)\n",
		p.Rewinds.Count, p.Rewinds.MedianSecondsBefore, p.Rewinds.Resent, p.Rewinds.Paired, p.Rewinds.ResentLonger, p.Rewinds.ResentShorter)
	fmt.Printf("  resends   %d of %d substantive prompts repeat one from the last hour (%s)\n",
		p.Resends.Resent, p.Resends.Substantive, pct(p.Resends.Share))
	fmt.Printf("  openers   %s\n", countLine(p.Openers))
	if len(p.Repeats) > 0 {
		fmt.Println("  typed again and again")
		for _, c := range p.Repeats {
			fmt.Printf("    %5d  %s\n", c.Count, truncateDisplay(cleanDisplayText(c.Key), 72))
		}
	}

	ws := p.Workspaces
	fmt.Println("\nWORKSPACES  (Claude Code only; Codex history records none)")
	fmt.Printf("  %d total · prompted on 1 day: %d · 2-3: %d · 4-7: %d · 8-30: %d · 30+: %d\n",
		ws.Total, ws.Lifespans.OneDay, ws.Lifespans.TwoToThree, ws.Lifespans.FourToSeven, ws.Lifespans.EightToThirty, ws.Lifespans.OverThirty)
	fmt.Printf("  %s of prompts sent within 10 min of the previous one land in a different workspace (%d of %d)\n",
		pct(ws.Switches.Share), ws.Switches.Switched, ws.Switches.Pairs)
	w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  PROMPTS\tDAYS\tFIRST\tLAST\tWORKSPACE")
	for _, r := range ws.Top {
		fmt.Fprintf(w, "  %d\t%d\t%s\t%s\t%s\n", r.Prompts, r.ActiveDays, r.First, r.Last, shortPath(r.Path))
	}
	if err := w.Flush(); err != nil {
		return err
	}

	if len(p.Terms) > 0 {
		fmt.Println("\nTERMS")
		terms := append([]me.TermTotal(nil), p.Terms...)
		sort.SliceStable(terms, func(i, j int) bool { return terms[i].Prompts > terms[j].Prompts })
		for _, t := range terms {
			fmt.Printf("  %-14s %6d prompts (%s)  /%s/\n", t.Name, t.Prompts, pct(t.Share), strings.TrimPrefix(t.Pattern, "(?i)"))
		}
	}
	return nil
}

func dayOf(rfc3339 string) string {
	if len(rfc3339) >= 10 {
		return rfc3339[:10]
	}
	return rfc3339
}

func pct(share float64) string {
	return fmt.Sprintf("%.1f%%", share*100)
}

func termShare(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total)
}

func bar(n, peak, width int) int {
	if peak == 0 {
		return 0
	}
	return n * width / peak
}

func countLine(counts []me.Count) string {
	if len(counts) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(counts))
	for _, c := range counts {
		parts = append(parts, fmt.Sprintf("%s %d", c.Key, c.Count))
	}
	return strings.Join(parts, " · ")
}
