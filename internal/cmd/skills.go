package cmd

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/skills"
)

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Manage the agent skills bundled with this binary",
	Long: `Manage the agent skills (ccx, ccx-recap, ccx-retro) bundled
with this binary.

Skills instruct agents which ccx flags to drive. Installing them from
the binary — instead of copying from a repo checkout — guarantees the
skill text matches the CLI surface this exact build actually has.`,
}

var skillsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List bundled skills and their installed state",
	RunE:  runSkillsList,
}

var skillsInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install bundled skills for Claude Code",
	Long: `Install the bundled skills into the Claude Code skills
directory, overwriting older copies so the skill text always matches
this binary's CLI surface.

Scopes:
  user      ` + "`" + `<claude-home>/skills/` + "`" + ` (default; ~/.claude/skills)
  project   ` + "`" + `./.claude/skills/` + "`" + ` in the current directory

This is the one place ccx writes inside the Claude Code home, and it
touches only the skill directories it owns. Session data stays
read-only.`,
	RunE: runSkillsInstall,
}

var skillsScope string

func init() {
	skillsInstallCmd.Flags().StringVar(&skillsScope, "scope", "user", "install scope: user or project")
	skillsCmd.AddCommand(skillsListCmd)
	skillsCmd.AddCommand(skillsInstallCmd)
}

type bundledSkill struct {
	Name    string
	Content []byte
}

func bundledSkills() ([]bundledSkill, error) {
	matches, err := fs.Glob(skills.FS, "*/SKILL.md")
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	out := make([]bundledSkill, 0, len(matches))
	for _, path := range matches {
		content, err := fs.ReadFile(skills.FS, path)
		if err != nil {
			return nil, fmt.Errorf("read embedded %s: %w", path, err)
		}
		out = append(out, bundledSkill{
			Name:    filepath.Dir(path),
			Content: content,
		})
	}
	return out, nil
}

func skillsDirForScope(scope string) (string, error) {
	switch scope {
	case "user":
		return filepath.Join(config.ClaudeHome(), "skills"), nil
	case "project":
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Join(cwd, ".claude", "skills"), nil
	default:
		return "", fmt.Errorf("invalid scope %q: use user or project", scope)
	}
}

func runSkillsList(cmd *cobra.Command, args []string) error {
	bundled, err := bundledSkills()
	if err != nil {
		return err
	}
	dir, err := skillsDirForScope("user")
	if err != nil {
		return err
	}
	for _, skill := range bundled {
		state := "not installed"
		installed, err := os.ReadFile(filepath.Join(dir, skill.Name, "SKILL.md"))
		switch {
		case err == nil && bytes.Equal(installed, skill.Content):
			state = "installed, current"
		case err == nil:
			state = "installed, differs from this binary — run `ccx skills install`"
		}
		fmt.Printf("%-12s %s\n", skill.Name, state)
	}
	fmt.Printf("\nbinary %s | user scope: %s\n", version, dir)
	return nil
}

func runSkillsInstall(cmd *cobra.Command, args []string) error {
	bundled, err := bundledSkills()
	if err != nil {
		return err
	}
	dir, err := skillsDirForScope(skillsScope)
	if err != nil {
		return err
	}
	for _, skill := range bundled {
		target := filepath.Join(dir, skill.Name)
		if err := os.MkdirAll(target, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", target, err)
		}
		if err := os.WriteFile(filepath.Join(target, "SKILL.md"), skill.Content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", skill.Name, err)
		}
		fmt.Printf("installed %s -> %s\n", skill.Name, target)
	}
	fmt.Printf("%d skills now match this binary (ccx %s)\n", len(bundled), version)
	return nil
}
