package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/claude-code/ccx/internal/config"
)

var (
	cfgFile    string
	claudeHome string
	verbose    bool
	quiet      bool
	version    string
	buildTime  string
)

var rootCmd = &cobra.Command{
	Use:   "ccx",
	Short: "Claude Code Explorer - inspect and export Claude Code sessions",
	Long: `ccx is a CLI tool for exploring Claude Code conversation sessions.

It provides tree-aware rendering of sessions, multiple export formats,
and an extensible skills system.

Examples:
  ccx projects              List all projects
  ccx sessions              List sessions (interactive picker)
  ccx view                  View session in terminal
  ccx export -f html        Export session to HTML
  ccx web                   Start web UI`,
	SilenceUsage: true,
}

func SetVersionInfo(v, bt string) {
	version = v
	buildTime = bt
	rootCmd.Version = fmt.Sprintf("%s (built %s)", version, buildTime)
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: $XDG_CONFIG_HOME/ccx/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&claudeHome, "claude-home", "", "override CLAUDE_CODE_HOME")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "minimal output")

	_ = viper.BindPFlag("claude_code_home", rootCmd.PersistentFlags().Lookup("claude-home"))

	rootCmd.AddCommand(projectsCmd)
	rootCmd.AddCommand(sessionsCmd)
	rootCmd.AddCommand(viewCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(doctorCmd)
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		configDir := os.Getenv("XDG_CONFIG_HOME")
		if configDir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return
			}
			configDir = home + "/.config"
		}

		ccxConfigDir := configDir + "/ccx"
		viper.AddConfigPath(ccxConfigDir)
		viper.AddConfigPath(".")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	viper.SetEnvPrefix("CCX")
	viper.AutomaticEnv()

	viper.SetDefault("claude_code_home", config.DefaultClaudeHome())
	viper.SetDefault("theme", "dark")
	viper.SetDefault("rendering.syntax_highlight", true)
	viper.SetDefault("rendering.show_thinking", "collapsed")
	viper.SetDefault("rendering.code_theme", "monokai")
	viper.SetDefault("export.default_format", "html")

	_ = viper.ReadInConfig()
}
