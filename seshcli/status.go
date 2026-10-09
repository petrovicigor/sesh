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
		Long:  "Outputs a copper ◆ if any Claude Code session in another tmux session needs user confirmation. Uses tmux @claude_icon (b36d43 marker) for fast detection, validated by DB + process liveness check.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get current tmux session name
			out, err := exec.Command("tmux", "display-message", "-p", "#S").Output()
			if err != nil {
				return nil // Not in tmux
			}
			currentSession := strings.TrimSpace(string(out))
			if currentSession == "" {
				return nil
			}

			// Fast path: read @claude_icon from tmux windows.
			// claude-sessions hooks set this instantly on PermissionRequest.
			// The awaiting marker is "#[fg=#b36d43]◆" — matched by the copper hex.
			out, err = exec.Command("tmux", "list-windows", "-a", "-F", "#{session_name}\t#{@claude_icon}").Output()
			if err != nil {
				return nil
			}

			// Collect sessions with the awaiting marker on any window (excluding current)
			candidates := make(map[string]bool)
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				parts := strings.SplitN(line, "\t", 2)
				if len(parts) != 2 {
					continue
				}
				sessionName, icon := parts[0], parts[1]
				if sessionName != currentSession && strings.Contains(icon, "b36d43") {
					candidates[sessionName] = true
				}
			}
			if len(candidates) == 0 {
				return nil // Common case: no awaiting icons anywhere
			}

			// Validate: cross-check with DB + process liveness.
			// Eliminates stale icons from dead processes or debounce bugs
			// where the icon wasn't cleared but the DB status already changed.
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return nil
			}
			verified, _ := claude.SessionsNeedingAttention(homeDir)
			for session := range candidates {
				if verified[session] {
					fmt.Print("#[fg=#b36d43]◆#[default]")
					return nil
				}
			}
			return nil
		},
	}
}
