package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for Sift (sf).

To load completions:

Bash:
  $ source <(sf completion bash)
  # To load completions for each session on macOS:
  $ sf completion bash > $(brew --prefix)/etc/bash_completion.d/sf

Zsh:
  # If shell completion is not already enabled in your environment:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc
  # To load completions for each session:
  $ sf completion zsh > "${fpath[1]}/_sf"

Fish:
  $ sf completion fish | source
  # To load completions for each session:
  $ sf completion fish > ~/.config/fish/completions/sf.fish

PowerShell:
  PS> sf completion powershell | Out-String | Invoke-Expression
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.RunE(cmd, args)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		shell := ""
		if len(args) > 0 {
			shell = strings.ToLower(args[0])
		} else {
			// Auto-detect from environment
			envShell := os.Getenv("SHELL")
			if strings.Contains(envShell, "zsh") {
				shell = "zsh"
			} else if strings.Contains(envShell, "fish") {
				shell = "fish"
			} else if strings.Contains(envShell, "bash") {
				shell = "bash"
			} else {
				shell = "zsh" // default on macOS
			}
		}

		out := cmd.OutOrStdout()
		switch shell {
		case "bash":
			return cmd.Root().GenBashCompletion(out)
		case "zsh":
			return cmd.Root().GenZshCompletion(out)
		case "fish":
			return cmd.Root().GenFishCompletion(out, true)
		case "powershell":
			return cmd.Root().GenPowerShellCompletionWithDesc(out)
		default:
			return fmt.Errorf("unsupported shell type: %s (supported: bash, zsh, fish, powershell)", shell)
		}
	},
}
