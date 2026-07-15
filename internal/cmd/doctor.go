package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/provider"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Validate environment and configuration",
	Long:  `Check that ccx is properly configured and can access session sources.`,
	RunE:  runDoctor,
}

func runDoctor(cmd *cobra.Command, args []string) error {
	var warnings []string
	var errors []string

	backend := provider.Default()

	fmt.Printf("[OK] Backend: %s\n", backend.ID())
	for _, home := range backend.Homes() {
		fmt.Printf("[OK] Source: %s\n", home)
	}

	if f := viper.ConfigFileUsed(); f != "" {
		fmt.Printf("[OK] Config: %s\n", f)
	} else {
		warnings = append(warnings, "No config file found (using defaults)")
	}

	projects, err := backend.DiscoverProjects()
	if err != nil {
		errors = append(errors, fmt.Sprintf("Failed to scan projects: %v", err))
	} else {
		totalSessions := 0
		for _, p := range projects {
			totalSessions += len(p.Sessions)
		}
		fmt.Printf("[OK] Projects: %d found (%d sessions)\n", len(projects), totalSessions)
	}

	claudeHome := config.ClaudeHome()
	if _, err := os.Stat(claudeHome); err == nil {
		fmt.Printf("[OK] Claude Code home: %s\n", claudeHome)
		if _, err := os.Stat(claudeHome + "/settings.json"); err == nil {
			fmt.Println("[OK] Claude Code settings.json: found")
		}
	} else {
		warnings = append(warnings, fmt.Sprintf("Claude Code home not found: %s", claudeHome))
	}

	codexHome := config.CodexHome()
	if _, err := os.Stat(codexHome); err == nil {
		fmt.Printf("[OK] Codex home: %s\n", codexHome)
		if _, err := os.Stat(filepath.Join(codexHome, "config.toml")); err == nil {
			fmt.Println("[OK] Codex config.toml: found")
		}
		if _, err := os.Stat(filepath.Join(codexHome, "config.json")); err == nil {
			fmt.Println("[OK] Codex config.json: found")
		}
		if _, err := os.Stat(filepath.Join(codexHome, "history.jsonl")); err == nil {
			fmt.Println("[OK] Codex history.jsonl: found")
		}
		if info, err := os.Stat(filepath.Join(codexHome, "sessions")); err == nil && info.IsDir() {
			fmt.Println("[OK] Codex sessions/: found")
		}
		if _, err := os.Stat(filepath.Join(codexHome, "session_index.jsonl")); err == nil {
			fmt.Println("[OK] Codex session_index.jsonl: found")
		}
		if matches, err := filepath.Glob(filepath.Join(codexHome, "state_*.sqlite")); err == nil && len(matches) > 0 {
			fmt.Printf("[OK] Codex state DBs: %d found\n", len(matches))
		}
	} else {
		warnings = append(warnings, fmt.Sprintf("Codex home not found: %s", codexHome))
	}

	grokHome := config.GrokHome()
	if _, err := os.Stat(grokHome); err == nil {
		fmt.Printf("[OK] Grok home: %s\n", grokHome)
		if info, err := os.Stat(filepath.Join(grokHome, "sessions")); err == nil && info.IsDir() {
			fmt.Println("[OK] Grok sessions/: found")
		}
		if _, err := os.Stat(filepath.Join(grokHome, "config.toml")); err == nil {
			fmt.Println("[OK] Grok config.toml: found")
		}
	} else {
		warnings = append(warnings, fmt.Sprintf("Grok home not found: %s", grokHome))
	}

	fmt.Println()

	for _, w := range warnings {
		fmt.Printf("! Warning: %s\n", w)
	}

	for _, e := range errors {
		fmt.Printf("x Error: %s\n", e)
	}

	if len(errors) > 0 {
		return fmt.Errorf("doctor found %d error(s)", len(errors))
	}

	if len(warnings) == 0 && len(errors) == 0 {
		fmt.Println("All checks passed!")
	}

	return nil
}
