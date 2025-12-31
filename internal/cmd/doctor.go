package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/parser"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Validate environment and configuration",
	Long:  `Check that ccx is properly configured and can access Claude Code sessions.`,
	RunE:  runDoctor,
}

func runDoctor(cmd *cobra.Command, args []string) error {
	var warnings []string
	var errors []string

	claudeHome := config.ClaudeHome()
	if _, err := os.Stat(claudeHome); os.IsNotExist(err) {
		errors = append(errors, fmt.Sprintf("CLAUDE_CODE_HOME not found: %s", claudeHome))
	} else {
		fmt.Printf("[OK] CLAUDE_CODE_HOME: %s\n", claudeHome)
	}

	if f := viper.ConfigFileUsed(); f != "" {
		fmt.Printf("[OK] Config: %s\n", f)
	} else {
		warnings = append(warnings, "No config file found (using defaults)")
	}

	projectsDir := config.ProjectsDir()
	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		errors = append(errors, fmt.Sprintf("Projects directory not found: %s", projectsDir))
	} else {
		projects, err := parser.DiscoverProjects(projectsDir)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Failed to scan projects: %v", err))
		} else {
			totalSessions := 0
			emptyProjects := 0
			for _, p := range projects {
				totalSessions += len(p.Sessions)
				if len(p.Sessions) == 0 {
					emptyProjects++
				}
			}
			fmt.Printf("[OK] Projects: %d found (%d sessions)\n", len(projects), totalSessions)
			if emptyProjects > 0 {
				warnings = append(warnings, fmt.Sprintf("%d projects have no sessions", emptyProjects))
			}
		}
	}

	if _, err := os.Stat(claudeHome + "/settings.json"); err == nil {
		fmt.Println("[OK] Claude Code settings.json: found")
	}

	homeDir, _ := os.UserHomeDir()
	if _, err := os.Stat(homeDir + "/.claude.json"); err == nil {
		fmt.Println("[OK] Claude Code .claude.json: found")
	}

	fmt.Println()

	for _, w := range warnings {
		fmt.Printf("! Warning: %s\n", w)
	}

	for _, e := range errors {
		fmt.Printf("✗ Error: %s\n", e)
	}

	if len(errors) > 0 {
		return fmt.Errorf("doctor found %d error(s)", len(errors))
	}

	if len(warnings) == 0 && len(errors) == 0 {
		fmt.Println("All checks passed!")
	}

	return nil
}
