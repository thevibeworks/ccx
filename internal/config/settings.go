package config

import "github.com/spf13/viper"

type Settings struct {
	ClaudeHome string
	CodexHome  string
	GrokHome   string

	Theme           string
	SyntaxHighlight bool
	ShowThinking    string
	CodeTheme       string

	DefaultFormat string

	Port int
	Host string

	Providers map[string]ProviderConfig
}

type ProviderConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	DisplayName string `mapstructure:"display_name"`
	AccentLight string `mapstructure:"accent_light"`
	AccentDark  string `mapstructure:"accent_dark"`
}

var defaultProviders = map[string]ProviderConfig{
	"claude-code": {
		Enabled:     true,
		DisplayName: "Claude Code",
		AccentLight: "#da7756",
		AccentDark:  "#e09070",
	},
	"codex": {
		Enabled:     true,
		DisplayName: "Codex",
		AccentLight: "#3b82f6",
		AccentDark:  "#60a5fa",
	},
	"grok": {
		Enabled:     true,
		DisplayName: "Grok",
		AccentLight: "#8b5cf6",
		AccentDark:  "#a78bfa",
	},
}

func Load() *Settings {
	s := &Settings{
		ClaudeHome:      ClaudeHome(),
		CodexHome:       CodexHome(),
		GrokHome:        GrokHome(),
		Theme:           viper.GetString("theme"),
		SyntaxHighlight: viper.GetBool("rendering.syntax_highlight"),
		ShowThinking:    viper.GetString("rendering.show_thinking"),
		CodeTheme:       viper.GetString("rendering.code_theme"),
		DefaultFormat:   viper.GetString("export.default_format"),
		Port:            viper.GetInt("port"),
		Host:            viper.GetString("host"),
	}

	if s.Port == 0 {
		s.Port = 8080
	}
	if s.Host == "" {
		s.Host = "localhost"
	}

	s.Providers = make(map[string]ProviderConfig)
	for id, def := range defaultProviders {
		s.Providers[id] = def
	}

	configured := viper.GetStringMap("providers")
	for id := range configured {
		pc := s.Providers[id]
		sub := viper.Sub("providers." + id)
		if sub == nil {
			continue
		}
		if sub.IsSet("enabled") {
			pc.Enabled = sub.GetBool("enabled")
		}
		if v := sub.GetString("display_name"); v != "" {
			pc.DisplayName = v
		}
		if v := sub.GetString("accent_light"); v != "" {
			pc.AccentLight = v
		}
		if v := sub.GetString("accent_dark"); v != "" {
			pc.AccentDark = v
		}
		s.Providers[id] = pc
	}

	return s
}

func (s *Settings) ProviderAccent(provider, theme string) string {
	pc, ok := s.Providers[provider]
	if !ok {
		return ""
	}
	if theme == "dark" {
		return pc.AccentDark
	}
	return pc.AccentLight
}

func (s *Settings) ProviderDisplayName(provider string) string {
	if pc, ok := s.Providers[provider]; ok && pc.DisplayName != "" {
		return pc.DisplayName
	}
	return provider
}

func (s *Settings) ProviderEnabled(provider string) bool {
	if pc, ok := s.Providers[provider]; ok {
		return pc.Enabled
	}
	return true
}
