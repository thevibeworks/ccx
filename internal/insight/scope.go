package insight

import (
	"fmt"
	"strings"
	"time"
)

const Kind = "ccx.insight.v1"

func ParseScope(raw string) (Scope, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "today", "day", "daily":
		return ScopeToday, nil
	case "week", "this-week", "weekly":
		return ScopeWeek, nil
	case "month", "this-month", "monthly":
		return ScopeMonth, nil
	case "quarter", "this-quarter", "q":
		return ScopeQuarter, nil
	case "year", "this-year", "yearly":
		return ScopeYear, nil
	default:
		return "", fmt.Errorf("invalid scope %q (want today, week, month, quarter, year)", raw)
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
	return time.LoadLocation(raw)
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
	return []Scope{ScopeToday, ScopeWeek, ScopeMonth, ScopeQuarter, ScopeYear}
}
