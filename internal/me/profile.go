// Package me profiles the human at the keyboard from the prompt record.
//
// Sessions describe what the agent did; the prompts describe the person
// driving it. Everything here is counted from the prompts alone, with no
// model in the loop and no judgment: when they work, how they steer,
// how often they take a prompt back, how many workspaces they hold at
// once. Interpretation belongs to the reader.
package me

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Kind is the JSON schema tag of a Profile.
const Kind = "ccx.me.v1"

// DayStartHour is where a working day rolls over. A prompt at 00:40
// belongs to the day that began the previous morning, so a night
// session is not split in two and does not fake an extra active day.
const DayStartHour = 5

const (
	repeatMaxRunes    = 60               // a "repeat" is a short prompt typed again
	substantiveRunes  = 30               // shorter prompts are acks, not resends
	similarityRunes   = 400              // compare prompts on their opening
	rewindLookBack    = time.Hour        // the prompt a /rewind takes back
	rewindLookAhead   = 30 * time.Minute // the prompt typed in its place
	resendWindow      = time.Hour
	switchWindow      = 10 * time.Minute
	resentThreshold   = 0.6
	resendThreshold   = 0.85
	minTrigramsToJoin = 8
)

// Prompt is one thing the human typed.
type Prompt struct {
	Time      time.Time
	Provider  string
	Workspace string // absolute path; empty when the source records none
	SessionID string
	Text      string
}

// Term is a named pattern to count across prompts (--term name=regex).
type Term struct {
	Name    string
	Pattern *regexp.Regexp
}

// Options bound and shape a profile.
type Options struct {
	Location *time.Location
	Since    time.Time // zero = from the first prompt
	Until    time.Time // zero = to the last prompt (exclusive otherwise)
	Terms    []Term
	Top      int // rows in each ranked list (0 = 10)
}

type Profile struct {
	Kind       string      `json:"kind"`
	TimeZone   string      `json:"time_zone"`
	Sources    []Source    `json:"sources,omitempty"`
	Span       Span        `json:"span"`
	Tempo      Tempo       `json:"tempo"`
	Months     []Month     `json:"months"`
	Commands   []Count     `json:"commands"`
	Repeats    []Count     `json:"repeats"`
	Openers    []Count     `json:"openers"`
	Rewinds    Rewinds     `json:"rewinds"`
	Resends    Resends     `json:"resends"`
	Workspaces Workspaces  `json:"workspaces"`
	Terms      []TermTotal `json:"terms,omitempty"`
}

// Source is one file the prompts were read from.
type Source struct {
	Provider string `json:"provider"`
	Path     string `json:"path"`
	Prompts  int    `json:"prompts"`
	First    string `json:"first,omitempty"`
	Last     string `json:"last,omitempty"`
}

type Span struct {
	First        string  `json:"first"`
	Last         string  `json:"last"`
	Prompts      int     `json:"prompts"`
	Undated      int     `json:"undated,omitempty"` // dropped: no timestamp
	CalendarDays int     `json:"calendar_days"`
	ActiveDays   int     `json:"active_days"`
	Providers    []Count `json:"providers"`
}

type Count struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

type Tempo struct {
	DayStartHour      int     `json:"day_start_hour"`
	Hours             [24]int `json:"hours"` // by clock hour
	WeekdayActiveDays int     `json:"weekday_active_days"`
	WeekendActiveDays int     `json:"weekend_active_days"`
	WeekdayPerDay     float64 `json:"weekday_prompts_per_day"`
	WeekendPerDay     float64 `json:"weekend_prompts_per_day"`
	MedianDayStart    string  `json:"median_day_start"` // first prompt of a day
	MedianDayEnd      string  `json:"median_day_end"`   // last; past 24:00 is after midnight
	LongestStreak     Streak  `json:"longest_streak"`
	Breaks            []Break `json:"breaks"` // runs of 2+ idle days, longest first
}

type Streak struct {
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
	Days  int    `json:"days"`
}

type Break struct {
	After string `json:"after"` // last active day before the gap
	Back  string `json:"back"`  // first active day after it
	Days  int    `json:"days"`  // idle days in between
}

type Month struct {
	Key                    string         `json:"key"`
	Prompts                int            `json:"prompts"`
	ActiveDays             int            `json:"active_days"`
	Workspaces             int            `json:"workspaces"`
	NewWorkspaces          int            `json:"new_workspaces"`
	MedianWorkspacesPerDay float64        `json:"median_workspaces_per_day"`
	MedianChars            int            `json:"median_chars"`
	CJKShare               float64        `json:"cjk_share"`
	CommandShare           float64        `json:"command_share"`
	QuestionShare          float64        `json:"question_share"`
	LateShare              float64        `json:"late_share"` // 23:00 up to the day roll
	MedianDayEnd           string         `json:"median_day_end"`
	Terms                  map[string]int `json:"terms,omitempty"`
}

// Rewinds reads /rewind as "I take that prompt back". Resent counts the
// rewinds followed by the same prompt again, edited.
type Rewinds struct {
	Count               int `json:"count"`
	Paired              int `json:"paired"` // a prompt before and one after
	Resent              int `json:"resent"`
	ResentLonger        int `json:"resent_longer"`
	ResentShorter       int `json:"resent_shorter"`
	MedianSecondsBefore int `json:"median_seconds_before"` // prompt -> rewind
}

// Resends are prompts sent again, nearly verbatim, within the hour.
type Resends struct {
	Substantive int     `json:"substantive"`
	Resent      int     `json:"resent"`
	Share       float64 `json:"share"`
}

type Workspaces struct {
	Total     int            `json:"total"`
	Lifespans Lifespans      `json:"lifespans"`
	Switches  Switches       `json:"switches"`
	Top       []WorkspaceRow `json:"top"`
}

// Lifespans buckets workspaces by how many days they were prompted on.
type Lifespans struct {
	OneDay        int `json:"one_day"`
	TwoToThree    int `json:"two_to_three"`
	FourToSeven   int `json:"four_to_seven"`
	EightToThirty int `json:"eight_to_thirty"`
	OverThirty    int `json:"over_thirty"`
}

// Switches counts prompt pairs less than ten minutes apart, and how
// many of them land in a different workspace than the one before.
type Switches struct {
	Pairs    int     `json:"pairs"`
	Switched int     `json:"switched"`
	Share    float64 `json:"share"`
}

type WorkspaceRow struct {
	Path       string `json:"path"`
	Prompts    int    `json:"prompts"`
	First      string `json:"first"`
	Last       string `json:"last"`
	ActiveDays int    `json:"active_days"`
}

type TermTotal struct {
	Name    string  `json:"name"`
	Pattern string  `json:"pattern"`
	Prompts int     `json:"prompts"`
	Share   float64 `json:"share"`
}

const dayLayout = "2006-01-02"

// item is a prompt with its derived fields, computed once.
type item struct {
	Prompt
	local   time.Time
	day     time.Time // working day (midnight of the day it began)
	command string    // "/rewind"; empty for prose
	stream  string    // workspace, else the session
}

// Build profiles the prompts. Order of the input does not matter.
func Build(prompts []Prompt, opts Options) Profile {
	loc := opts.Location
	if loc == nil {
		loc = time.Local
	}
	top := opts.Top
	if top <= 0 {
		top = 10
	}

	p := Profile{Kind: Kind, TimeZone: loc.String()}
	p.Tempo.DayStartHour = DayStartHour

	items := make([]item, 0, len(prompts))
	for _, pr := range prompts {
		if strings.TrimSpace(pr.Text) == "" {
			continue
		}
		if pr.Time.IsZero() {
			p.Span.Undated++
			continue
		}
		if !opts.Since.IsZero() && pr.Time.Before(opts.Since) {
			continue
		}
		if !opts.Until.IsZero() && !pr.Time.Before(opts.Until) {
			continue
		}
		local := pr.Time.In(loc)
		it := item{Prompt: pr, local: local, day: workingDay(local), command: slashCommand(pr.Text)}
		it.stream = pr.Workspace
		if it.stream == "" {
			it.stream = "session:" + pr.SessionID
		}
		items = append(items, it)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Time.Before(items[j].Time) })
	if len(items) == 0 {
		p.Months, p.Commands, p.Repeats, p.Openers = []Month{}, []Count{}, []Count{}, []Count{}
		p.Tempo.Breaks, p.Span.Providers, p.Workspaces.Top = []Break{}, []Count{}, []WorkspaceRow{}
		return p
	}

	p.Span = span(items, p.Span.Undated)
	p.Tempo = tempo(items, top)
	p.Months = months(items, opts.Terms)
	p.Commands = topCounts(countBy(items, func(it item) string { return it.command }), top)
	p.Repeats = repeats(items, top)
	p.Openers = topCounts(countBy(items, opener), top)
	p.Rewinds = rewinds(items)
	p.Resends = resends(items)
	p.Workspaces = workspaces(items, top)
	for _, t := range opts.Terms {
		n := 0
		for _, it := range items {
			if t.Pattern.MatchString(it.Text) {
				n++
			}
		}
		p.Terms = append(p.Terms, TermTotal{Name: t.Name, Pattern: t.Pattern.String(), Prompts: n, Share: share(n, len(items))})
	}
	return p
}

func workingDay(local time.Time) time.Time {
	d := local.Add(-DayStartHour * time.Hour)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, local.Location())
}

// slashCommand returns the command of a prompt that is one ("/model
// opus" -> "/model"). A pasted absolute path is not a command.
func slashCommand(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return ""
	}
	token := text
	if i := strings.IndexFunc(text, unicode.IsSpace); i >= 0 {
		token = text[:i]
	}
	if len(token) < 2 || len(token) > 40 || strings.Contains(token[1:], "/") {
		return ""
	}
	for _, r := range token[1:] {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' && r != ':' {
			return ""
		}
	}
	return strings.ToLower(token)
}

func span(items []item, undated int) Span {
	first, last := items[0], items[len(items)-1]
	days := map[time.Time]bool{}
	for _, it := range items {
		days[it.day] = true
	}
	return Span{
		First:        first.local.Format(time.RFC3339),
		Last:         last.local.Format(time.RFC3339),
		Prompts:      len(items),
		Undated:      undated,
		CalendarDays: daysBetween(first.day, last.day) + 1,
		ActiveDays:   len(days),
		Providers:    topCounts(countBy(items, func(it item) string { return it.Provider }), 0),
	}
}

func tempo(items []item, top int) Tempo {
	t := Tempo{DayStartHour: DayStartHour, Breaks: []Break{}}
	firstOf, lastOf := map[time.Time]time.Time{}, map[time.Time]time.Time{}
	var weekdayPrompts, weekendPrompts int
	for _, it := range items {
		t.Hours[it.local.Hour()]++
		if _, ok := firstOf[it.day]; !ok {
			firstOf[it.day] = it.local
		}
		lastOf[it.day] = it.local
		if isWeekend(it.day) {
			weekendPrompts++
		} else {
			weekdayPrompts++
		}
	}

	days := make([]time.Time, 0, len(firstOf))
	var starts, ends []int
	for d := range firstOf {
		days = append(days, d)
		starts = append(starts, minutesInto(d, firstOf[d]))
		ends = append(ends, minutesInto(d, lastOf[d]))
		if isWeekend(d) {
			t.WeekendActiveDays++
		} else {
			t.WeekdayActiveDays++
		}
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })
	t.WeekdayPerDay = ratio(weekdayPrompts, t.WeekdayActiveDays)
	t.WeekendPerDay = ratio(weekendPrompts, t.WeekendActiveDays)
	t.MedianDayStart = clock(medianInt(starts))
	t.MedianDayEnd = clock(medianInt(ends))

	runStart, run := days[0], 1
	best := Streak{Start: days[0].Format(dayLayout), End: days[0].Format(dayLayout), Days: 1}
	for i := 1; i < len(days); i++ {
		gap := daysBetween(days[i-1], days[i])
		if gap == 1 {
			run++
		} else {
			runStart, run = days[i], 1
			if gap > 2 {
				t.Breaks = append(t.Breaks, Break{After: days[i-1].Format(dayLayout), Back: days[i].Format(dayLayout), Days: gap - 1})
			}
		}
		if run > best.Days {
			best = Streak{Start: runStart.Format(dayLayout), End: days[i].Format(dayLayout), Days: run}
		}
	}
	t.LongestStreak = best
	sort.SliceStable(t.Breaks, func(i, j int) bool { return t.Breaks[i].Days > t.Breaks[j].Days })
	if len(t.Breaks) > top {
		t.Breaks = t.Breaks[:top]
	}
	return t
}

func months(items []item, terms []Term) []Month {
	type acc struct {
		m            Month
		days         map[time.Time]map[string]bool // day -> workspaces touched
		lastOf       map[time.Time]time.Time
		workspaces   map[string]bool
		chars        []int
		prose        int
		cjk, ask     int
		cmds, late   int
		newWorkspace int
	}
	seen := map[string]bool{}
	byKey := map[string]*acc{}
	var keys []string
	for _, it := range items {
		key := it.day.Format("2006-01")
		a := byKey[key]
		if a == nil {
			a = &acc{m: Month{Key: key}, days: map[time.Time]map[string]bool{}, lastOf: map[time.Time]time.Time{}, workspaces: map[string]bool{}}
			if len(terms) > 0 {
				a.m.Terms = map[string]int{}
			}
			byKey[key] = a
			keys = append(keys, key)
		}
		a.m.Prompts++
		if a.days[it.day] == nil {
			a.days[it.day] = map[string]bool{}
		}
		a.lastOf[it.day] = it.local
		if it.Workspace != "" {
			a.days[it.day][it.Workspace] = true
			a.workspaces[it.Workspace] = true
			if !seen[it.Workspace] {
				seen[it.Workspace] = true
				a.newWorkspace++
			}
		}
		if h := it.local.Hour(); h >= 23 || h < DayStartHour {
			a.late++
		}
		if it.command != "" {
			a.cmds++
		} else {
			a.prose++
			a.chars = append(a.chars, len([]rune(it.Text)))
			if hasHan(it.Text) {
				a.cjk++
			}
			if strings.ContainsAny(it.Text, "?？") {
				a.ask++
			}
		}
		for _, t := range terms {
			if t.Pattern.MatchString(it.Text) {
				a.m.Terms[t.Name]++
			}
		}
	}
	sort.Strings(keys)
	out := make([]Month, 0, len(keys))
	for _, key := range keys {
		a := byKey[key]
		var perDay, ends []int
		for d, ws := range a.days {
			if len(ws) > 0 {
				perDay = append(perDay, len(ws))
			}
			ends = append(ends, minutesInto(d, a.lastOf[d]))
		}
		a.m.ActiveDays = len(a.days)
		a.m.Workspaces = len(a.workspaces)
		a.m.NewWorkspaces = a.newWorkspace
		a.m.MedianWorkspacesPerDay = medianFloat(perDay)
		a.m.MedianChars = medianInt(a.chars)
		a.m.CJKShare = share(a.cjk, a.prose)
		a.m.QuestionShare = share(a.ask, a.prose)
		a.m.CommandShare = share(a.cmds, a.m.Prompts)
		a.m.LateShare = share(a.late, a.m.Prompts)
		a.m.MedianDayEnd = clock(medianInt(ends))
		out = append(out, a.m)
	}
	return out
}

func repeats(items []item, top int) []Count {
	counts := map[string]int{}
	for _, it := range items {
		if it.command != "" {
			continue
		}
		text := strings.ToLower(strings.Join(strings.Fields(it.Text), " "))
		if n := len([]rune(text)); n == 0 || n > repeatMaxRunes {
			continue
		}
		counts[text]++
	}
	for k, n := range counts {
		if n < 2 {
			delete(counts, k)
		}
	}
	return topCounts(counts, top)
}

// opener is the first word of a prose prompt: how a request begins.
func opener(it item) string {
	if it.command != "" {
		return ""
	}
	fields := strings.Fields(strings.ToLower(it.Text))
	if len(fields) == 0 {
		return ""
	}
	word := strings.TrimFunc(fields[0], func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if n := len([]rune(word)); n == 0 || n > 24 {
		return ""
	}
	return word
}

func rewinds(items []item) Rewinds {
	var r Rewinds
	var before []int
	for _, stream := range streams(items) {
		for i, it := range stream {
			if it.command != "/rewind" {
				continue
			}
			r.Count++
			prev := nearestProse(stream, i, -1, 6, rewindLookBack)
			next := nearestProse(stream, i, +1, 3, rewindLookAhead)
			if prev != nil {
				before = append(before, int(it.Time.Sub(prev.Time).Seconds()))
			}
			if prev == nil || next == nil {
				continue
			}
			r.Paired++
			if similarity(prev.Text, next.Text) < resentThreshold {
				continue
			}
			r.Resent++
			switch a, b := len([]rune(prev.Text)), len([]rune(next.Text)); {
			case b > a:
				r.ResentLonger++
			case b < a:
				r.ResentShorter++
			}
		}
	}
	r.MedianSecondsBefore = medianInt(before)
	return r
}

// nearestProse walks from a rewind to the closest non-command prompt in
// one direction, within a bounded number of steps and a time window.
func nearestProse(stream []item, at, dir, steps int, window time.Duration) *item {
	for n, i := 0, at+dir; n < steps && i >= 0 && i < len(stream); n, i = n+1, i+dir {
		gap := stream[i].Time.Sub(stream[at].Time)
		if gap < 0 {
			gap = -gap
		}
		if gap > window {
			return nil
		}
		if stream[i].command == "" {
			return &stream[i]
		}
	}
	return nil
}

func resends(items []item) Resends {
	var r Resends
	for _, stream := range streams(items) {
		var recent []item // substantive prose only
		for _, it := range stream {
			if it.command != "" || len([]rune(it.Text)) < substantiveRunes {
				continue
			}
			r.Substantive++
			for i := len(recent) - 1; i >= 0 && i >= len(recent)-4; i-- {
				if it.Time.Sub(recent[i].Time) > resendWindow {
					break
				}
				if similarity(recent[i].Text, it.Text) >= resendThreshold && lengthsClose(recent[i].Text, it.Text) {
					r.Resent++
					break
				}
			}
			recent = append(recent, it)
		}
	}
	r.Share = share(r.Resent, r.Substantive)
	return r
}

func workspaces(items []item, top int) Workspaces {
	type acc struct {
		row  WorkspaceRow
		days map[time.Time]bool
	}
	byPath := map[string]*acc{}
	w := Workspaces{Top: []WorkspaceRow{}}
	var prev *item
	for i := range items {
		it := &items[i]
		if it.Workspace == "" {
			continue
		}
		a := byPath[it.Workspace]
		if a == nil {
			a = &acc{row: WorkspaceRow{Path: it.Workspace, First: it.day.Format(dayLayout)}, days: map[time.Time]bool{}}
			byPath[it.Workspace] = a
		}
		a.row.Prompts++
		a.row.Last = it.day.Format(dayLayout)
		a.days[it.day] = true
		if prev != nil && it.Time.Sub(prev.Time) < switchWindow {
			w.Switches.Pairs++
			if prev.Workspace != it.Workspace {
				w.Switches.Switched++
			}
		}
		prev = it
	}
	w.Total = len(byPath)
	w.Switches.Share = share(w.Switches.Switched, w.Switches.Pairs)
	for _, a := range byPath {
		a.row.ActiveDays = len(a.days)
		switch n := a.row.ActiveDays; {
		case n == 1:
			w.Lifespans.OneDay++
		case n <= 3:
			w.Lifespans.TwoToThree++
		case n <= 7:
			w.Lifespans.FourToSeven++
		case n <= 30:
			w.Lifespans.EightToThirty++
		default:
			w.Lifespans.OverThirty++
		}
		w.Top = append(w.Top, a.row)
	}
	sort.Slice(w.Top, func(i, j int) bool {
		if w.Top[i].Prompts != w.Top[j].Prompts {
			return w.Top[i].Prompts > w.Top[j].Prompts
		}
		return w.Top[i].Path < w.Top[j].Path
	})
	if len(w.Top) > top {
		w.Top = w.Top[:top]
	}
	return w
}

// streams splits the time-ordered prompts into one sequence per
// workspace (or per session when the source records no workspace), in
// a stable order.
func streams(items []item) [][]item {
	index := map[string]int{}
	var out [][]item
	for _, it := range items {
		i, ok := index[it.stream]
		if !ok {
			i = len(out)
			index[it.stream] = i
			out = append(out, nil)
		}
		out[i] = append(out[i], it)
	}
	return out
}

// similarity is how much of the shorter prompt's opening survives in
// the other: rune-trigram containment, so it reads the same for an
// appended tail, a prepended line, and unspaced CJK. 1 = contained.
func similarity(a, b string) float64 {
	ta, tb := trigrams(a), trigrams(b)
	if len(ta) > len(tb) {
		ta, tb = tb, ta
	}
	if len(ta) < minTrigramsToJoin {
		if normalize(a) == normalize(b) {
			return 1
		}
		return 0
	}
	shared := 0
	for g := range ta {
		if tb[g] {
			shared++
		}
	}
	return float64(shared) / float64(len(ta))
}

func normalize(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func trigrams(s string) map[string]bool {
	runes := []rune(normalize(s))
	if len(runes) > similarityRunes {
		runes = runes[:similarityRunes]
	}
	out := map[string]bool{}
	for i := 0; i+3 <= len(runes); i++ {
		out[string(runes[i:i+3])] = true
	}
	return out
}

func lengthsClose(a, b string) bool {
	la, lb := len([]rune(a)), len([]rune(b))
	if la > lb {
		la, lb = lb, la
	}
	return lb <= 2*la+40
}

func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func isWeekend(day time.Time) bool {
	wd := day.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

// daysBetween counts calendar days from a to b (both at midnight);
// rounding absorbs DST days that are 23 or 25 hours long.
func daysBetween(a, b time.Time) int {
	return int(b.Sub(a).Hours()/24 + 0.5)
}

// minutesInto is the distance from a working day's midnight to t, so a
// prompt after midnight reads past 24:00 instead of wrapping to 00:xx.
func minutesInto(day, t time.Time) int {
	return int(t.Sub(day).Minutes())
}

func clock(minutes int) string {
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}

func countBy(items []item, key func(item) string) map[string]int {
	counts := map[string]int{}
	for _, it := range items {
		if k := key(it); k != "" {
			counts[k]++
		}
	}
	return counts
}

// topCounts ranks by count, ties by key, so output is deterministic.
func topCounts(counts map[string]int, top int) []Count {
	out := make([]Count, 0, len(counts))
	for k, n := range counts {
		out = append(out, Count{Key: k, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	if top > 0 && len(out) > top {
		out = out[:top]
	}
	return out
}

func medianInt(values []int) int {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

func medianFloat(values []int) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return float64(sorted[mid])
	}
	return float64(sorted[mid-1]+sorted[mid]) / 2
}

func share(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return float64(int(float64(part)/float64(whole)*1000+0.5)) / 1000
}

func ratio(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return float64(int(float64(part)/float64(whole)*10+0.5)) / 10
}
