package config

import (
	"strings"
	"time"
)

type SessionFilter struct {
	Provider    string // "claude-code", "codex", "" for all
	After       time.Time
	Before      time.Time
	Query       string // search in summary
	Model       string // model name substring
	MinMessages int
}

func (f SessionFilter) IsEmpty() bool {
	return f.Provider == "" && f.After.IsZero() && f.Before.IsZero() &&
		f.Query == "" && f.Model == "" && f.MinMessages == 0
}

func NormalizeProvider(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "cc", "claude", "claude-code":
		return "claude-code"
	case "cx", "codex":
		return "codex"
	case "", "all":
		return ""
	default:
		return raw
	}
}

func ParseDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.ParseInLocation("2006-01-02", s, time.Local)
}

// ParseBeforeDate parses a date for --before filtering.
// Returns end-of-day (next day midnight local) so --before 2026-03-01 includes the full day.
func ParseBeforeDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return time.Time{}, err
	}
	return t.AddDate(0, 0, 1), nil
}

type Filterable interface {
	GetProvider() string
	GetStartTime() time.Time
	GetEndTime() time.Time
	GetSummary() string
	GetModel() string
	GetMessageCount() int
}

func (f SessionFilter) Match(item Filterable) bool {
	if f.Provider != "" && item.GetProvider() != f.Provider {
		return false
	}
	if !f.After.IsZero() && item.GetEndTime().Before(f.After) {
		return false
	}
	if !f.Before.IsZero() && item.GetStartTime().After(f.Before) {
		return false
	}
	if f.Query != "" && !strings.Contains(strings.ToLower(item.GetSummary()), strings.ToLower(f.Query)) {
		return false
	}
	if f.Model != "" && !strings.Contains(strings.ToLower(item.GetModel()), strings.ToLower(f.Model)) {
		return false
	}
	if f.MinMessages > 0 && item.GetMessageCount() < f.MinMessages {
		return false
	}
	return true
}
