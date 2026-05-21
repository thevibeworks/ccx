package provider

import (
	"os"

	"github.com/thevibeworks/ccx/internal/catalog"
	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/provider/claude"
	"github.com/thevibeworks/ccx/internal/provider/codex"
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
	claudeEnabled := settings.ProviderEnabled("claude-code")
	codexEnabled := settings.ProviderEnabled("codex")

	if claudeEnabled && dirExists(settings.ClaudeHome) {
		backends = append(backends, claudeBackend)
	}
	if codexEnabled && dirExists(settings.CodexHome) {
		backends = append(backends, codexBackend)
	}
	if len(backends) == 0 {
		switch {
		case claudeEnabled && codexEnabled:
			return NewMulti(claudeBackend, codexBackend)
		case claudeEnabled:
			return claudeBackend
		case codexEnabled:
			return codexBackend
		default:
			return NewMulti()
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
