package seshcli

import (
	"fmt"

	"github.com/joshmedeski/sesh/v2/icon"
	"github.com/joshmedeski/sesh/v2/lister"
	"github.com/spf13/cobra"
)

func NewPathCommand(i icon.Icon, list lister.Lister) *cobra.Command {
	return &cobra.Command{
		Use:   "path <name>",
		Short: "Get the filesystem path for a session name",
		Long:  "Resolves a session name to its filesystem path. Useful for scripts that need to determine the working directory for a session.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			// Strip icons if present (handles formats like " geoip ⎇ feature/cdk")
			name = i.RemoveIcon(name)

			// Try to find the session in various sources (same order as connector)
			// Workspace (before projects — more specific)
			if session, exists := list.FindWorkspaceSession(name); exists {
				fmt.Println(session.Path)
				return nil
			}

			// Projects
			if session, exists := list.FindProjectSession(name); exists {
				fmt.Println(session.Path)
				return nil
			}

			// Config
			if session, exists := list.FindConfigSession(name); exists {
				fmt.Println(session.Path)
				return nil
			}

			// Tmux (get current path from running session)
			if session, exists := list.FindTmuxSession(name); exists {
				fmt.Println(session.Path)
				return nil
			}

			// Zoxide
			if session, exists := list.FindZoxideSession(name); exists {
				fmt.Println(session.Path)
				return nil
			}

			// Tmuxinator
			if session, exists := list.FindTmuxinatorConfig(name); exists {
				fmt.Println(session.Path)
				return nil
			}

			return fmt.Errorf("session not found: %s", name)
		},
	}
}
