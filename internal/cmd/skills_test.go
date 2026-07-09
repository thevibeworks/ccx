package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundledSkillsShipAllThree(t *testing.T) {
	bundled, err := bundledSkills()
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool)
	for _, s := range bundled {
		got[s.Name] = true
		if len(s.Content) == 0 {
			t.Fatalf("skill %s embedded empty", s.Name)
		}
		if !bytes.Contains(s.Content, []byte("---")) {
			t.Fatalf("skill %s missing frontmatter", s.Name)
		}
	}
	for _, want := range []string{"ccx", "ccx-recap", "ccx-retro"} {
		if !got[want] {
			t.Fatalf("bundled skills missing %s (got %v)", want, got)
		}
	}
}

// TestBundledSkillsMatchRepoFiles is the drift guard from issue #19:
// the embedded skill text must be byte-identical to the repo files,
// so a release always ships skills describing its own CLI surface.
func TestBundledSkillsMatchRepoFiles(t *testing.T) {
	bundled, err := bundledSkills()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range bundled {
		repo, err := os.ReadFile(filepath.Join("..", "..", "skills", s.Name, "SKILL.md"))
		if err != nil {
			t.Fatalf("read repo skill %s: %v", s.Name, err)
		}
		if !bytes.Equal(repo, s.Content) {
			t.Fatalf("embedded %s differs from repo file — rebuild", s.Name)
		}
	}
}

func TestSkillsInstallWritesProjectScope(t *testing.T) {
	dir := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	skillsScope = "project"
	t.Cleanup(func() { skillsScope = "user" })

	if err := runSkillsInstall(skillsInstallCmd, nil); err != nil {
		t.Fatalf("install: %v", err)
	}

	for _, name := range []string{"ccx", "ccx-recap", "ccx-retro"} {
		path := filepath.Join(dir, ".claude", "skills", name, "SKILL.md")
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("expected %s installed: %v", path, err)
		}
		if !strings.Contains(string(content), "name: "+name) {
			t.Fatalf("%s content wrong:\n%s", path, content[:min(200, len(content))])
		}
	}
}

func TestSkillsDirForScopeRejectsUnknown(t *testing.T) {
	if _, err := skillsDirForScope("global"); err == nil {
		t.Fatal("expected error for unknown scope")
	}
}
