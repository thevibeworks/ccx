package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/thevibeworks/ccx/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Long:  `View and manage ccx configuration.`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("claude_code_home: %s\n", config.ClaudeHome())
		fmt.Printf("theme: %s\n", config.Theme())
		fmt.Printf("rendering.syntax_highlight: %v\n", config.SyntaxHighlight())
		fmt.Printf("rendering.show_thinking: %s\n", config.ShowThinking())
		fmt.Printf("rendering.code_theme: %s\n", config.CodeTheme())
		fmt.Printf("export.default_format: %s\n", config.DefaultExportFormat())
		return nil
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show config file location",
	RunE: func(cmd *cobra.Command, args []string) error {
		if f := viper.ConfigFileUsed(); f != "" {
			fmt.Println(f)
		} else {
			home, _ := os.UserHomeDir()
			configDir := os.Getenv("XDG_CONFIG_HOME")
			if configDir == "" {
				configDir = filepath.Join(home, ".config")
			}
			fmt.Printf("%s/ccx/config.yaml (not found)\n", configDir)
		}
		return nil
	},
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create default config file",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}

		configDir := os.Getenv("XDG_CONFIG_HOME")
		if configDir == "" {
			configDir = filepath.Join(home, ".config")
		}

		ccxDir := filepath.Join(configDir, "ccx")
		if err := os.MkdirAll(ccxDir, 0755); err != nil {
			return err
		}

		configPath := filepath.Join(ccxDir, "config.yaml")
		if _, err := os.Stat(configPath); err == nil {
			return fmt.Errorf("config already exists: %s", configPath)
		}

		content := `# ccx configuration
# claude_code_home: ~/.claude

theme: dark              # dark | light | auto

rendering:
  syntax_highlight: true
  show_thinking: collapsed    # collapsed | expanded | hidden
  code_theme: monokai

export:
  default_format: html
  include_thinking: false
  include_images: true

filters:
  exclude_tools: []
  exclude_agents: false
  min_messages: 1

# skills_dir: ~/.config/ccx/skills
`

		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			return err
		}

		fmt.Printf("Created: %s\n", configPath)
		return nil
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get KEY",
	Short: "Get a config value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		value := viper.Get(key)
		if value == nil {
			return fmt.Errorf("key not found: %s", key)
		}
		fmt.Println(value)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configGetCmd)
}
