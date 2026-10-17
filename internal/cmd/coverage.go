package cmd

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/thevibeworks/ccx/internal/me"
)

// Transcript coverage: does the session store still hold what the
// prompt history says happened?
//
// The two records have different lifetimes. Claude Code deletes
// transcripts older than cleanupPeriodDays (default 30, by file mtime)
// and never touches history.jsonl, so a store can lose months without a
// single error: every command simply returns fewer sessions. History
// names the session each prompt was typed into, which makes the loss
// countable without reading a transcript.

const (
	coverageMinPrompts = 20  // smaller months are noise, not a gap
	coverageFloor      = 0.9 // below this share on disk, a month is a gap
	defaultCleanupDays = 30
)

// coverageRow is one provider-month of prompt history, split by
// whether the transcript of the prompt's session is on disk.
type coverageRow struct {
	Provider       string
	Month          string
	Prompts        int // prompts that name a session
	PromptsOnDisk  int
	Sessions       int
	SessionsOnDisk int
	Unattributed   int // prompts with no session id: cannot be checked
}

func (r coverageRow) gap() bool {
	return r.Prompts >= coverageMinPrompts && float64(r.PromptsOnDisk) < coverageFloor*float64(r.Prompts)
}

// transcriptCoverage buckets prompts by provider and month and checks
// each named session against the ids of the transcripts on disk.
func transcriptCoverage(prompts []me.Prompt, onDisk map[string]bool, loc *time.Location) []coverageRow {
	type acc struct {
		row      coverageRow
		sessions map[string]bool
	}
	byKey := map[string]*acc{}
	for _, p := range prompts {
		if p.Time.IsZero() {
			continue
		}
		month := p.Time.In(loc).Format("2006-01")
		key := p.Provider + "\x00" + month
		a := byKey[key]
		if a == nil {
			a = &acc{row: coverageRow{Provider: p.Provider, Month: month}, sessions: map[string]bool{}}
			byKey[key] = a
		}
		id := strings.ToLower(p.SessionID)
		if id == "" {
			a.row.Unattributed++
			continue
		}
		a.row.Prompts++
		if onDisk[id] {
			a.row.PromptsOnDisk++
		}
		if !a.sessions[id] {
			a.sessions[id] = true
			a.row.Sessions++
			if onDisk[id] {
				a.row.SessionsOnDisk++
			}
		}
	}
	rows := make([]coverageRow, 0, len(byKey))
	for _, a := range byKey {
		rows = append(rows, a.row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Provider != rows[j].Provider {
			return rows[i].Provider < rows[j].Provider
		}
		return rows[i].Month < rows[j].Month
	})
	return rows
}

// transcriptIDs lists the session ids that have a transcript on disk.
// Claude Code names the file after the session; Codex ends the rollout
// file name with it, in sessions/ and in archived_sessions/.
func transcriptIDs(claudeHome, codexHome string) map[string]bool {
	ids := map[string]bool{}
	if claudeHome != "" {
		matches, _ := filepath.Glob(filepath.Join(claudeHome, "projects", "*", "*.jsonl"))
		for _, m := range matches {
			ids[strings.ToLower(strings.TrimSuffix(filepath.Base(m), ".jsonl"))] = true
		}
	}
	if codexHome != "" {
		for _, dir := range []string{"sessions", "archived_sessions"} {
			_ = filepath.WalkDir(filepath.Join(codexHome, dir), func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil // a missing or unreadable dir holds no transcripts
				}
				name := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
				if len(name) >= 36 {
					ids[strings.ToLower(name[len(name)-36:])] = true
				}
				return nil
			})
		}
	}
	return ids
}

// claudeRetention reads cleanupPeriodDays from Claude Code's user
// settings. set is false when the file or the key is absent, which
// means the 30-day default is in force.
func claudeRetention(claudeHome string) (days int, set bool, err error) {
	data, err := os.ReadFile(filepath.Join(claudeHome, "settings.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return defaultCleanupDays, false, nil
		}
		return 0, false, err
	}
	var settings struct {
		CleanupPeriodDays *int `json:"cleanupPeriodDays"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return 0, false, err
	}
	if settings.CleanupPeriodDays == nil {
		return defaultCleanupDays, false, nil
	}
	return *settings.CleanupPeriodDays, true, nil
}

// reportCoverage prints the gaps and returns one warning per provider
// that has any. Nothing is printed when the store matches history.
func reportCoverage(rows []coverageRow) []string {
	var warnings []string
	for _, provider := range []string{"claude-code", "codex"} {
		var gaps []coverageRow
		var unattributed, missingPrompts, missingSessions int
		var first, last string
		for _, r := range rows {
			if r.Provider != provider {
				continue
			}
			unattributed += r.Unattributed
			if !r.gap() {
				continue
			}
			gaps = append(gaps, r)
			missingPrompts += r.Prompts - r.PromptsOnDisk
			missingSessions += r.Sessions - r.SessionsOnDisk
			if first == "" {
				first = r.Month
			}
			last = r.Month
		}
		if len(gaps) == 0 {
			fmt.Printf("[OK] %s transcripts: every month of prompt history is on disk\n", provider)
		} else {
			fmt.Printf("[!!] %s transcripts: prompt history names sessions that are not on disk\n", provider)
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
			fmt.Fprintln(w, "     MONTH\tPROMPTS\tON DISK\tSESSIONS\tON DISK\t")
			for _, r := range gaps {
				fmt.Fprintf(w, "     %s\t%d\t%d\t%d\t%d\t\n", r.Month, r.Prompts, r.PromptsOnDisk, r.Sessions, r.SessionsOnDisk)
			}
			w.Flush()
			// Claude Code keeps transcripts under projects/, Codex under sessions/.
			dir := "projects"
			if provider == "codex" {
				dir = "sessions"
			}
			warnings = append(warnings, fmt.Sprintf(
				"%s: %d prompts in %d sessions (%s to %s) have no transcript on disk. "+
					"What was typed survives in prompt history (ccx me, ccx search --content); what the agent did does not. "+
					"A backup restores it: rsync -a --ignore-existing <backup>/%s/ <home>/%s/",
				provider, missingPrompts, missingSessions, first, last, dir, dir))
		}
		if unattributed > 0 {
			fmt.Printf("     %d %s prompts predate session ids in history and cannot be checked\n", unattributed, provider)
		}
	}
	return warnings
}
