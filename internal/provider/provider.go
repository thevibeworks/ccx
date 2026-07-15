package provider

import (
	"os"

	"github.com/thevibeworks/ccx/internal/catalog"
	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/provider/claude"
	"github.com/thevibeworks/ccx/internal/provider/codex"
	"github.com/thevibeworks/ccx/internal/provider/grok"
)

type Backend interface {
	ID() string
	Homes() []string
	DiscoverProjects() ([]*parser.Project, error)
	ListSessions(query catalog.SessionQuery) ([]*parser.Session, error)
	FindProject(name string) (*parser.Project, error)
	FindSession(projectName, sessionID string) (*parser.Session, error)
	ParseSession(filePath string) (*parser.Session, error)
}

func Default() Backend {
	settings := config.Load()
	var backends []Backend

	claudeBackend := claude.New(settings.ClaudeHome)
	codexBackend := codex.New(settings.CodexHome)
	grokBackend := grok.New(settings.GrokHome)
	claudeEnabled := settings.ProviderEnabled("claude-code")
	codexEnabled := settings.ProviderEnabled("codex")
	grokEnabled := settings.ProviderEnabled("grok")

	if claudeEnabled && dirExists(settings.ClaudeHome) {
		backends = append(backends, claudeBackend)
	}
	if codexEnabled && dirExists(settings.CodexHome) {
		backends = append(backends, codexBackend)
	}
	if grokEnabled && dirExists(settings.GrokHome) {
		backends = append(backends, grokBackend)
	}
	if len(backends) == 0 {
		// No provider home exists yet: return every enabled backend so
		// error messages and doctor output stay provider-aware.
		if claudeEnabled {
			backends = append(backends, claudeBackend)
		}
		if codexEnabled {
			backends = append(backends, codexBackend)
		}
		if grokEnabled {
			backends = append(backends, grokBackend)
		}
	}
	if len(backends) == 1 {
		return backends[0]
	}
	return NewMulti(backends...)
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
