package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion script",
	Long: `Generate shell completion script for ccx.

To load completions:

Bash:
  $ source <(ccx completion bash)
  # Or add to ~/.bashrc:
  $ ccx completion bash > ~/.local/share/bash-completion/completions/ccx

Zsh:
  $ source <(ccx completion zsh)
  # Or add to ~/.zshrc:
  $ ccx completion zsh > "${fpath[1]}/_ccx"

Fish:
  $ ccx completion fish | source
  # Or persist:
  $ ccx completion fish > ~/.config/fish/completions/ccx.fish

PowerShell:
  PS> ccx completion powershell | Out-String | Invoke-Expression
  # Or add to $PROFILE
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
