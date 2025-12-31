package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

func DefaultClaudeHome() string {
	if env := os.Getenv("CLAUDE_CODE_HOME"); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

func ClaudeHome() string {
	if v := viper.GetString("claude_code_home"); v != "" {
		return expandPath(v)
	}
	return DefaultClaudeHome()
}

func ProjectsDir() string {
	return filepath.Join(ClaudeHome(), "projects")
}

func Theme() string {
	return viper.GetString("theme")
}

func SyntaxHighlight() bool {
	return viper.GetBool("rendering.syntax_highlight")
}

func ShowThinking() string {
	return viper.GetString("rendering.show_thinking")
}

func CodeTheme() string {
	return viper.GetString("rendering.code_theme")
}

func DefaultExportFormat() string {
	return viper.GetString("export.default_format")
}

func DataDir() string {
	// XDG_DATA_HOME, or fallback to ~/.local/share
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "ccx")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".ccx", "data")
	}
	return filepath.Join(home, ".local", "share", "ccx")
}

func expandPath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}
