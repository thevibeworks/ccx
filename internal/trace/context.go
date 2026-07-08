package trace

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultContextMaxDocs  = 80
	defaultContextMaxBytes = 8192
)

var rootContextFiles = []string{
	"AGENTS.md",
	"CLAUDE.md",
	"CONTEXT.md",
	"README.md",
}

var contextDirs = []string{
	"docs/adr",
	"docs/design",
	"docs/devlog",
	".ccx/knowledge",
}

func CollectWorkspaceContext(result *TraceResult, repoDir string) error {
	if result == nil || repoDir == "" {
		return nil
	}

	ctx := WorkspaceContext{
		RepoRoot:     repoDir,
		MaxBytes:     defaultContextMaxBytes,
		MaxDocuments: defaultContextMaxDocs,
	}

	addDocument := func(relPath, kind string) error {
		if len(ctx.Documents)+len(ctx.Knowledge) >= defaultContextMaxDocs {
			ctx.Truncated = true
			return nil
		}
		doc, err := readContextDocument(repoDir, relPath, kind, defaultContextMaxBytes)
		if err != nil {
			return err
		}
		if kind == "knowledge" {
			ctx.Knowledge = append(ctx.Knowledge, doc)
		} else {
			ctx.Documents = append(ctx.Documents, doc)
		}
		return nil
	}

	for _, relPath := range rootContextFiles {
		if _, err := os.Stat(filepath.Join(repoDir, relPath)); err != nil {
			if os.IsNotExist(err) {
				ctx.Missing = append(ctx.Missing, relPath)
				continue
			}
			return err
		}
		if err := addDocument(relPath, classifyContextPath(relPath)); err != nil {
			return err
		}
	}

	for _, relDir := range contextDirs {
		fullDir := filepath.Join(repoDir, relDir)
		if _, err := os.Stat(fullDir); err != nil {
			if os.IsNotExist(err) {
				ctx.Missing = append(ctx.Missing, relDir+"/")
				continue
			}
			return err
		}

		err := filepath.WalkDir(fullDir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				name := entry.Name()
				if strings.HasPrefix(name, ".") && path != fullDir {
					return filepath.SkipDir
				}
				return nil
			}
			if !isContextFile(entry.Name()) {
				return nil
			}
			relPath, err := filepath.Rel(repoDir, path)
			if err != nil {
				return err
			}
			kind := classifyContextPath(filepath.ToSlash(relPath))
			return addDocument(filepath.ToSlash(relPath), kind)
		})
		if err != nil {
			return err
		}
	}

	result.Workspace = ctx
	result.Stats.WorkspaceDocs = len(ctx.Documents)
	result.Stats.KnowledgeEntries = len(ctx.Knowledge)
	return nil
}

func readContextDocument(repoDir, relPath, kind string, maxBytes int) (ContextDocument, error) {
	fullPath := filepath.Join(repoDir, filepath.FromSlash(relPath))
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return ContextDocument{}, err
	}
	sum := sha256.Sum256(data)
	doc := ContextDocument{
		Path:      filepath.ToSlash(relPath),
		Kind:      kind,
		Title:     extractTitle(string(data)),
		Bytes:     len(data),
		SHA256:    fmt.Sprintf("%x", sum),
		Truncated: len(data) > maxBytes,
	}
	return doc, nil
}

func classifyContextPath(path string) string {
	path = filepath.ToSlash(path)
	switch {
	case strings.HasPrefix(path, ".ccx/knowledge/"):
		return "knowledge"
	case strings.HasPrefix(path, "docs/adr/"):
		return "adr"
	case strings.HasPrefix(path, "docs/design/"):
		return "design"
	case strings.HasPrefix(path, "docs/devlog/"):
		return "devlog"
	case strings.HasSuffix(path, "AGENTS.md") || strings.HasSuffix(path, "CLAUDE.md"):
		return "agent-instructions"
	case strings.HasSuffix(path, "CONTEXT.md"):
		return "vocabulary"
	case strings.HasSuffix(path, "MEMORY.md"):
		return "memory"
	default:
		return "doc"
	}
}

func isContextFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".md", ".markdown", ".org", ".txt":
		return true
	default:
		return false
	}
}

func extractTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimPrefix(line, "#")
		line = strings.TrimPrefix(line, "*")
		return strings.TrimSpace(line)
	}
	return ""
}
