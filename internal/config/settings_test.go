package config

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestSettingsProviderAccent(t *testing.T) {
	s := &Settings{
		Providers: map[string]ProviderConfig{
			"claude-code": {AccentLight: "#da7756", AccentDark: "#e09070"},
			"codex":       {AccentLight: "#3b82f6", AccentDark: "#60a5fa"},
		},
	}

	tests := []struct {
		provider string
		theme    string
		want     string
	}{
		{"claude-code", "light", "#da7756"},
		{"claude-code", "dark", "#e09070"},
		{"codex", "light", "#3b82f6"},
		{"codex", "dark", "#60a5fa"},
		{"unknown", "dark", ""},
	}

	for _, tt := range tests {
		got := s.ProviderAccent(tt.provider, tt.theme)
		if got != tt.want {
			t.Errorf("ProviderAccent(%q, %q) = %q, want %q", tt.provider, tt.theme, got, tt.want)
		}
	}
}

func TestSettingsProviderDisplayName(t *testing.T) {
	s := &Settings{
		Providers: map[string]ProviderConfig{
			"claude-code": {DisplayName: "Claude Code"},
		},
	}

	if got := s.ProviderDisplayName("claude-code"); got != "Claude Code" {
		t.Errorf("got %q, want Claude Code", got)
	}
	if got := s.ProviderDisplayName("unknown"); got != "unknown" {
		t.Errorf("got %q, want unknown (fallback)", got)
	}
}

func TestSettingsProviderEnabled(t *testing.T) {
	s := &Settings{
		Providers: map[string]ProviderConfig{
			"claude-code": {Enabled: true},
			"codex":       {Enabled: false},
		},
	}

	if !s.ProviderEnabled("claude-code") {
		t.Error("claude-code should be enabled")
	}
	if s.ProviderEnabled("codex") {
		t.Error("codex should be disabled")
	}
	if !s.ProviderEnabled("unknown") {
		t.Error("unknown provider should default to enabled")
	}
}

func TestNormalizeProvider(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"cc", "claude-code"},
		{"CC", "claude-code"},
		{"claude", "claude-code"},
		{"claude-code", "claude-code"},
		{"cx", "codex"},
		{"CX", "codex"},
		{"codex", "codex"},
		{"all", ""},
		{"", ""},
		{"  cc  ", "claude-code"},
		{"other", "other"},
	}

	for _, tt := range tests {
		got := NormalizeProvider(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeProvider(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseDate(t *testing.T) {
	t.Run("valid date", func(t *testing.T) {
		got, err := ParseDate("2026-03-01")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local)
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("empty string", func(t *testing.T) {
		got, err := ParseDate("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.IsZero() {
			t.Error("expected zero time")
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		_, err := ParseDate("not-a-date")
		if err == nil {
			t.Error("expected error for invalid date")
		}
	})
}

func TestParseBeforeDate(t *testing.T) {
	t.Run("advances to next day", func(t *testing.T) {
		got, err := ParseBeforeDate("2026-03-01")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := time.Date(2026, 3, 2, 0, 0, 0, 0, time.Local)
		if !got.Equal(want) {
			t.Errorf("got %v, want %v (next day midnight)", got, want)
		}
	})

	t.Run("empty string", func(t *testing.T) {
		got, err := ParseBeforeDate("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.IsZero() {
			t.Error("expected zero time")
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		_, err := ParseBeforeDate("not-a-date")
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("before filter includes the full day", func(t *testing.T) {
		before, _ := ParseBeforeDate("2026-03-01")
		sessionEnd := time.Date(2026, 3, 1, 15, 0, 0, 0, time.UTC)
		if !sessionEnd.Before(before) {
			t.Error("session at 2026-03-01 15:00 should pass --before 2026-03-01 filter")
		}
	})
}

func TestSessionFilterIsEmpty(t *testing.T) {
	if !(SessionFilter{}).IsEmpty() {
		t.Error("zero filter should be empty")
	}
	if (SessionFilter{Provider: "codex"}).IsEmpty() {
		t.Error("filter with provider should not be empty")
	}
	if (SessionFilter{Query: "test"}).IsEmpty() {
		t.Error("filter with query should not be empty")
	}
	if (SessionFilter{MinMessages: 5}).IsEmpty() {
		t.Error("filter with min messages should not be empty")
	}
}

func TestSessionFilterMatch(t *testing.T) {
	baseTime := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)

	item := &mockFilterable{
		provider:     "claude-code",
		startTime:    baseTime,
		endTime:      baseTime.Add(time.Hour),
		summary:      "Fix authentication bug",
		model:        "claude-opus-4-6",
		messageCount: 10,
	}

	tests := []struct {
		name   string
		filter SessionFilter
		want   bool
	}{
		{"empty filter matches all", SessionFilter{}, true},
		{"matching provider", SessionFilter{Provider: "claude-code"}, true},
		{"non-matching provider", SessionFilter{Provider: "codex"}, false},
		{"query in summary", SessionFilter{Query: "auth"}, true},
		{"query not in summary", SessionFilter{Query: "database"}, false},
		{"query case insensitive", SessionFilter{Query: "AUTH"}, true},
		{"model match", SessionFilter{Model: "opus"}, true},
		{"model no match", SessionFilter{Model: "sonnet"}, false},
		{"after before session", SessionFilter{After: baseTime.Add(2 * time.Hour)}, false},
		{"after during session", SessionFilter{After: baseTime.Add(-time.Hour)}, true},
		{"before after session", SessionFilter{Before: baseTime.Add(-time.Hour)}, false},
		{"before at session start still passes by end time", SessionFilter{Before: baseTime}, false},
		{"before after session end passes", SessionFilter{Before: baseTime.Add(2 * time.Hour)}, true},
		{"min messages met", SessionFilter{MinMessages: 5}, true},
		{"min messages not met", SessionFilter{MinMessages: 20}, false},
		{"combined filters pass", SessionFilter{Provider: "claude-code", Query: "auth", MinMessages: 5}, true},
		{"combined filters fail on one", SessionFilter{Provider: "codex", Query: "auth"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filter.Match(item)
			if got != tt.want {
				t.Errorf("filter.Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

type mockFilterable struct {
	provider     string
	startTime    time.Time
	endTime      time.Time
	summary      string
	model        string
	messageCount int
}

func (m *mockFilterable) GetProvider() string     { return m.provider }
func (m *mockFilterable) GetStartTime() time.Time { return m.startTime }
func (m *mockFilterable) GetEndTime() time.Time   { return m.endTime }
func (m *mockFilterable) GetSummary() string      { return m.summary }
func (m *mockFilterable) GetModel() string        { return m.model }
func (m *mockFilterable) GetMessageCount() int    { return m.messageCount }

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		input string
		want  string
	}{
		{"~/foo", home + "/foo"},
		{"/absolute/path", "/absolute/path"},
		{"relative", "relative"},
		{"", ""},
	}
	for _, tt := range tests {
		got := expandPath(tt.input)
		if got != tt.want {
			t.Errorf("expandPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDataDir(t *testing.T) {
	got := DataDir()
	if got == "" {
		t.Fatal("DataDir() returned empty")
	}
	if !strings.HasSuffix(got, "ccx") {
		t.Errorf("DataDir() = %q, should end with ccx", got)
	}
}

func TestDataDirWithXDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg-test")
	got := DataDir()
	if got != "/tmp/xdg-test/ccx" {
		t.Errorf("DataDir() = %q, want /tmp/xdg-test/ccx", got)
	}
}

func TestDefaultClaudeHome(t *testing.T) {
	got := DefaultClaudeHome()
	if got == "" {
		t.Fatal("DefaultClaudeHome() returned empty")
	}
	if !strings.HasSuffix(got, ".claude") {
		t.Errorf("DefaultClaudeHome() = %q, should end with .claude", got)
	}
}

func TestDefaultClaudeHomeEnvOverride(t *testing.T) {
	t.Setenv("CLAUDE_CODE_HOME", "/tmp/test-claude")
	got := DefaultClaudeHome()
	if got != "/tmp/test-claude" {
		t.Errorf("DefaultClaudeHome() = %q, want /tmp/test-claude", got)
	}
}

func TestDefaultCodexHome(t *testing.T) {
	got := DefaultCodexHome()
	if got == "" {
		t.Fatal("DefaultCodexHome() returned empty")
	}
	if !strings.HasSuffix(got, ".codex") {
		t.Errorf("DefaultCodexHome() = %q, should end with .codex", got)
	}
}

func TestDefaultCodexHomeEnvOverride(t *testing.T) {
	t.Setenv("CODEX_HOME", "/tmp/test-codex")
	got := DefaultCodexHome()
	if got != "/tmp/test-codex" {
		t.Errorf("DefaultCodexHome() = %q, want /tmp/test-codex", got)
	}
}

func TestOldConfigGettersWithDefaults(t *testing.T) {
	viper.SetDefault("theme", "dark")
	viper.SetDefault("rendering.syntax_highlight", true)
	viper.SetDefault("rendering.show_thinking", "collapsed")
	viper.SetDefault("rendering.code_theme", "monokai")
	viper.SetDefault("export.default_format", "html")
	defer viper.Reset()

	if Theme() != "dark" {
		t.Errorf("Theme() = %q, want dark", Theme())
	}
	if !SyntaxHighlight() {
		t.Error("SyntaxHighlight() should be true")
	}
	if ShowThinking() != "collapsed" {
		t.Errorf("ShowThinking() = %q, want collapsed", ShowThinking())
	}
	if CodeTheme() != "monokai" {
		t.Errorf("CodeTheme() = %q, want monokai", CodeTheme())
	}
	if DefaultExportFormat() != "html" {
		t.Errorf("DefaultExportFormat() = %q, want html", DefaultExportFormat())
	}
}

func TestOldConfigGettersWithOverride(t *testing.T) {
	viper.Set("theme", "light")
	viper.Set("rendering.show_thinking", "hidden")
	defer viper.Reset()

	if Theme() != "light" {
		t.Errorf("Theme() = %q, want light", Theme())
	}
	if ShowThinking() != "hidden" {
		t.Errorf("ShowThinking() = %q, want hidden", ShowThinking())
	}
}

func TestLoadDefaults(t *testing.T) {
	s := Load()
	if s.Port != 8080 {
		t.Errorf("default Port = %d, want 8080", s.Port)
	}
	if s.Host != "localhost" {
		t.Errorf("default Host = %q, want localhost", s.Host)
	}
	if len(s.Providers) < 2 {
		t.Errorf("expected at least 2 default providers, got %d", len(s.Providers))
	}
	cc := s.Providers["claude-code"]
	if !cc.Enabled {
		t.Error("claude-code should be enabled by default")
	}
	if cc.AccentLight == "" {
		t.Error("claude-code should have default accent color")
	}
}
