package me

import (
	"regexp"
	"testing"
	"time"
)

var utc8 = time.FixedZone("+08", 8*3600)

func at(day, clock string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", day+" "+clock, utc8)
	if err != nil {
		panic(err)
	}
	return t
}

func prompt(day, clock, workspace, text string) Prompt {
	return Prompt{Time: at(day, clock), Provider: "claude-code", Workspace: workspace, SessionID: "s1", Text: text}
}

func build(prompts ...Prompt) Profile {
	return Build(prompts, Options{Location: utc8})
}

func TestNightPromptStaysWithTheDayItBegan(t *testing.T) {
	p := build(
		prompt("2026-03-02", "10:00:00", "/w/a", "start the day with a real request here"),
		prompt("2026-03-03", "00:40:00", "/w/a", "still the same working day after midnight"),
	)
	if p.Span.ActiveDays != 1 {
		t.Fatalf("active days = %d, want 1 (00:40 belongs to the day before)", p.Span.ActiveDays)
	}
	if p.Tempo.MedianDayStart != "10:00" || p.Tempo.MedianDayEnd != "24:40" {
		t.Fatalf("day = %s..%s, want 10:00..24:40", p.Tempo.MedianDayStart, p.Tempo.MedianDayEnd)
	}
	if p.Tempo.Hours[0] != 1 || p.Tempo.Hours[10] != 1 {
		t.Fatalf("hours are by the clock: %v", p.Tempo.Hours)
	}
	if got := p.Months[0].LateShare; got != 0.5 {
		t.Fatalf("late share = %v, want 0.5", got)
	}
}

func TestStreakAndBreaks(t *testing.T) {
	var prompts []Prompt
	for _, day := range []string{"2026-01-05", "2026-01-06", "2026-01-07", "2026-01-09", "2026-01-13", "2026-01-14"} {
		prompts = append(prompts, prompt(day, "11:00:00", "/w/a", "a prompt long enough to be prose on "+day))
	}
	p := build(prompts...)
	if s := p.Tempo.LongestStreak; s.Days != 3 || s.Start != "2026-01-05" || s.End != "2026-01-07" {
		t.Fatalf("streak = %+v, want 3 days from 01-05", s)
	}
	// 01-08 alone is one idle day: not a break. 01-10..12 is three.
	if len(p.Tempo.Breaks) != 1 || p.Tempo.Breaks[0] != (Break{After: "2026-01-09", Back: "2026-01-13", Days: 3}) {
		t.Fatalf("breaks = %+v", p.Tempo.Breaks)
	}
	if p.Span.CalendarDays != 10 || p.Span.ActiveDays != 6 {
		t.Fatalf("days = %d active of %d", p.Span.ActiveDays, p.Span.CalendarDays)
	}
}

func TestSlashCommandIsNotAPath(t *testing.T) {
	cases := map[string]string{
		"/rewind":                        "/rewind",
		"/model claude-opus-4-6[1m]":     "/model",
		"/ralph-loop:ralph-loop \"go\"":  "/ralph-loop:ralph-loop",
		"/Users/eric/wrk/file.go broken": "",
		"/":                              "",
		"//model":                        "",
		"not a command":                  "",
	}
	for text, want := range cases {
		if got := slashCommand(text); got != want {
			t.Errorf("slashCommand(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestRewindFollowedByTheSamePromptLonger(t *testing.T) {
	first := "review the pull request and merge it if the tests are green"
	p := build(
		prompt("2026-04-01", "10:00:00", "/w/a", first),
		prompt("2026-04-01", "10:01:30", "/w/a", "/rewind"),
		prompt("2026-04-01", "10:02:00", "/w/a", first+"; then cut a patch release"),
		// A rewind into a different request is paired but not a resend.
		prompt("2026-04-01", "11:00:00", "/w/a", "now look at the flaky integration test in the ci run"),
		prompt("2026-04-01", "11:03:00", "/w/a", "/rewind"),
		prompt("2026-04-01", "11:04:00", "/w/a", "let us write the changelog entry for this release instead"),
		// A rewind in another workspace must not pair with /w/a prompts.
		prompt("2026-04-01", "11:05:00", "/w/b", "/rewind"),
	)
	r := p.Rewinds
	if r.Count != 3 || r.Paired != 2 || r.Resent != 1 || r.ResentLonger != 1 || r.ResentShorter != 0 {
		t.Fatalf("rewinds = %+v", r)
	}
	if r.MedianSecondsBefore != 135 { // 90s and 180s
		t.Fatalf("median seconds before rewind = %d, want 135", r.MedianSecondsBefore)
	}
}

func TestResendWithinTheHour(t *testing.T) {
	text := "generate the quarterly usage report and verify every number against the log"
	p := build(
		prompt("2026-04-02", "10:00:00", "/w/a", text),
		prompt("2026-04-02", "10:20:00", "/w/a", text),
		prompt("2026-04-02", "13:00:00", "/w/a", text),     // past the hour: not a resend
		prompt("2026-04-02", "13:01:00", "/w/b", text),     // other workspace: not a resend
		prompt("2026-04-02", "13:02:00", "/w/a", "ok go"),  // too short to count
		prompt("2026-04-02", "13:03:00", "/w/a", "/model"), // commands never count
	)
	if r := p.Resends; r.Substantive != 4 || r.Resent != 1 || r.Share != 0.25 {
		t.Fatalf("resends = %+v", r)
	}
}

func TestSimilarityHandlesPrependAndCJK(t *testing.T) {
	base := "read the devlog spec and include the checklist in a new page"
	if s := similarity(base, "it is Jan 27 today; "+base); s < 0.9 {
		t.Errorf("prepended line: similarity %v, want >= 0.9", s)
	}
	zh := "我觉得我们需要把我们的方案文档重写一下然后转成文档"
	if s := similarity(zh, zh+"文件名不要写客户版"); s < 0.9 {
		t.Errorf("cjk append: similarity %v, want >= 0.9", s)
	}
	if s := similarity(base, "deploy the worker and check the live version id"); s > 0.3 {
		t.Errorf("unrelated: similarity %v, want low", s)
	}
	if similarity("go", "go") != 1 || similarity("go", "no") != 0 {
		t.Errorf("short prompts compare by equality")
	}
}

func TestWorkspacesLifespansAndSwitches(t *testing.T) {
	p := build(
		prompt("2026-05-01", "10:00:00", "/w/a", "first prompt in workspace a for the day"),
		prompt("2026-05-01", "10:05:00", "/w/b", "jump to workspace b within ten minutes"),
		prompt("2026-05-01", "10:08:00", "/w/b", "stay in workspace b within ten minutes"),
		prompt("2026-05-01", "12:00:00", "/w/a", "back to a, but hours later: not a pair"),
		prompt("2026-05-02", "10:00:00", "/w/a", "second day in workspace a"),
		Prompt{Time: at("2026-05-02", "10:01:00"), Provider: "codex", SessionID: "cx1", Text: "codex prompt carries no workspace"},
	)
	ws := p.Workspaces
	if ws.Total != 2 || ws.Lifespans.OneDay != 1 || ws.Lifespans.TwoToThree != 1 {
		t.Fatalf("workspaces = %+v", ws)
	}
	if ws.Switches.Pairs != 2 || ws.Switches.Switched != 1 || ws.Switches.Share != 0.5 {
		t.Fatalf("switches = %+v", ws.Switches)
	}
	if top := ws.Top[0]; top.Path != "/w/a" || top.Prompts != 3 || top.ActiveDays != 2 || top.First != "2026-05-01" || top.Last != "2026-05-02" {
		t.Fatalf("top workspace = %+v", top)
	}
	m := p.Months[0]
	if m.Workspaces != 2 || m.NewWorkspaces != 2 || m.MedianWorkspacesPerDay != 1.5 {
		t.Fatalf("month workspaces = %+v", m)
	}
	if len(p.Span.Providers) != 2 || p.Span.Providers[0] != (Count{Key: "claude-code", Count: 5}) {
		t.Fatalf("providers = %+v", p.Span.Providers)
	}
}

func TestMonthSharesRepeatsOpenersAndTerms(t *testing.T) {
	prompts := []Prompt{
		prompt("2026-06-01", "10:00:00", "/w/a", "let's continue"),
		prompt("2026-06-01", "10:10:00", "/w/a", "Let's  continue"),
		prompt("2026-06-01", "10:20:00", "/w/a", "why is this failing again?"),
		prompt("2026-06-01", "10:30:00", "/w/a", "我觉得这个方案可以"),
		prompt("2026-06-01", "10:40:00", "/w/a", "/compact"),
		prompt("2026-07-01", "10:00:00", "/w/c", "good, again please"),
	}
	p := Build(prompts, Options{Location: utc8, Terms: []Term{{Name: "again", Pattern: regexp.MustCompile(`(?i)\bagain\b`)}}})
	if len(p.Months) != 2 {
		t.Fatalf("months = %d, want 2", len(p.Months))
	}
	june := p.Months[0]
	if june.Key != "2026-06" || june.Prompts != 5 || june.CommandShare != 0.2 || june.CJKShare != 0.25 || june.QuestionShare != 0.25 {
		t.Fatalf("june = %+v", june)
	}
	if june.Terms["again"] != 1 || p.Months[1].Terms["again"] != 1 || p.Terms[0].Prompts != 2 {
		t.Fatalf("terms: june %v july %v total %+v", june.Terms, p.Months[1].Terms, p.Terms)
	}
	if p.Months[1].NewWorkspaces != 1 {
		t.Fatalf("july new workspaces = %d, want 1", p.Months[1].NewWorkspaces)
	}
	if len(p.Repeats) != 1 || p.Repeats[0] != (Count{Key: "let's continue", Count: 2}) {
		t.Fatalf("repeats = %+v", p.Repeats)
	}
	if p.Openers[0] != (Count{Key: "let's", Count: 2}) {
		t.Fatalf("openers = %+v", p.Openers)
	}
	if len(p.Commands) != 1 || p.Commands[0] != (Count{Key: "/compact", Count: 1}) {
		t.Fatalf("commands = %+v", p.Commands)
	}
}

func TestWindowAndUndated(t *testing.T) {
	prompts := []Prompt{
		prompt("2026-01-01", "10:00:00", "/w/a", "before the window"),
		prompt("2026-02-01", "10:00:00", "/w/a", "inside the window"),
		prompt("2026-03-01", "10:00:00", "/w/a", "at the exclusive end"),
		{Provider: "codex", Text: "no timestamp at all"},
		prompt("2026-02-02", "10:00:00", "/w/a", "   "),
	}
	p := Build(prompts, Options{Location: utc8, Since: at("2026-01-15", "00:00:00"), Until: at("2026-03-01", "10:00:00")})
	if p.Span.Prompts != 1 || p.Span.Undated != 1 {
		t.Fatalf("span = %+v", p.Span)
	}
	empty := Build(nil, Options{Location: utc8})
	if empty.Span.Prompts != 0 || empty.Months == nil || empty.Tempo.Breaks == nil {
		t.Fatalf("empty profile must carry empty lists, not nulls: %+v", empty)
	}
}
