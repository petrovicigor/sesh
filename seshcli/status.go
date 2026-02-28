package seshcli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/joshmedeski/sesh/v2/claude"
	"github.com/spf13/cobra"
)

func NewStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show attention indicator for other tmux sessions",
		Long:  "Outputs 🖐️ if any Claude Code session in another tmux session (not the current one) needs user confirmation. Designed for tmux status-right: set -g status-right '#(sesh status)'",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get current tmux session name
			out, err := exec.Command("tmux", "display-message", "-p", "#S").Output()
			if err != nil {
				return nil // Not in tmux — silent exit
			}
			currentSession := strings.TrimSpace(string(out))
			if currentSession == "" {
				return nil
			}

			homeDir, err := os.UserHomeDir()
			if err != nil {
				return nil
			}

			// Check if any OTHER session needs attention (not the current one)
			sessions, _ := claude.SessionsNeedingAttention(homeDir)
			for name := range sessions {
				if name != currentSession {
					fmt.Print("🖐️")
					return nil
				}
			}
			return nil
		},
	}
}
