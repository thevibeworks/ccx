package insight

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Scope string

const (
	ScopeToday     Scope = "today"
	ScopeYesterday Scope = "yesterday"
	ScopeWeek      Scope = "week"
	ScopeMonth     Scope = "month"
	ScopeQuarter   Scope = "quarter"
	ScopeYear      Scope = "year"
)

var fixedOffsetPattern = regexp.MustCompile(`^([+-])(\d{1,2})(?::?(\d{2}))?$`)

func ParseScope(raw string) (Scope, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "today", "day", "daily":
		return ScopeToday, nil
	case "yesterday":
		return ScopeYesterday, nil
	case "week", "this-week", "weekly":
		return ScopeWeek, nil
	case "month", "this-month", "monthly":
		return ScopeMonth, nil
	case "quarter", "this-quarter", "q":
		return ScopeQuarter, nil
	case "year", "this-year", "yearly":
		return ScopeYear, nil
	default:
		return "", fmt.Errorf("invalid scope %q (want today, yesterday, week, month, quarter, year)", raw)
	}
}

func LoadLocation(raw string) (*time.Location, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "local") {
		return time.Local, nil
	}
	if strings.EqualFold(raw, "utc") {
		return time.UTC, nil
	}
	if loc, ok, err := parseFixedOffsetLocation(raw); ok || err != nil {
		return loc, err
	}
	return time.LoadLocation(raw)
}

func parseFixedOffsetLocation(raw string) (*time.Location, bool, error) {
	m := fixedOffsetPattern.FindStringSubmatch(raw)
	if m == nil {
		return nil, false, nil
	}
	hours, err := strconv.Atoi(m[2])
	if err != nil {
		return nil, true, err
	}
	minutes := 0
	if m[3] != "" {
		minutes, err = strconv.Atoi(m[3])
		if err != nil {
			return nil, true, err
		}
	}
	if hours > 14 || minutes > 59 {
		return nil, true, fmt.Errorf("invalid UTC offset %q", raw)
	}
	offset := hours*3600 + minutes*60
	if m[1] == "-" {
		offset = -offset
	}
	name := fmt.Sprintf("%s%02d:%02d", m[1], hours, minutes)
	return time.FixedZone(name, offset), true, nil
}

func ScopeWindow(scope Scope, now time.Time, loc *time.Location) (time.Time, time.Time, string) {
	if loc == nil {
		loc = time.Local
	}
	if now.IsZero() {
		now = time.Now()
	}
	local := now.In(loc)
	startOfDay := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)

	switch scope {
	case ScopeYesterday:
		start := startOfDay.AddDate(0, 0, -1)
		return start, startOfDay, "Yesterday"
	case ScopeWeek:
		weekday := int(startOfDay.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start := startOfDay.AddDate(0, 0, -(weekday - 1))
		return start, start.AddDate(0, 0, 7), fmt.Sprintf("This week, since %s", start.Format("Jan 2"))
	case ScopeMonth:
		start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, loc)
		return start, start.AddDate(0, 1, 0), local.Format("January 2006")
	case ScopeQuarter:
		month := time.Month(((int(local.Month())-1)/3)*3 + 1)
		start := time.Date(local.Year(), month, 1, 0, 0, 0, 0, loc)
		q := ((int(local.Month()) - 1) / 3) + 1
		return start, start.AddDate(0, 3, 0), fmt.Sprintf("Q%d %d", q, local.Year())
	case ScopeYear:
		start := time.Date(local.Year(), 1, 1, 0, 0, 0, 0, loc)
		return start, start.AddDate(1, 0, 0), fmt.Sprintf("%d", local.Year())
	default:
		return startOfDay, startOfDay.AddDate(0, 0, 1), "Today"
	}
}

func ScopeAliases() []Scope {
	return []Scope{ScopeToday, ScopeYesterday, ScopeWeek, ScopeMonth, ScopeQuarter, ScopeYear}
}
