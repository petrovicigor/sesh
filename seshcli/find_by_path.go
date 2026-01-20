package seshcli

import (
	"fmt"

	"github.com/joshmedeski/sesh/v2/lister"
	"github.com/spf13/cobra"
)

func NewFindByPathCommand(list lister.Lister) *cobra.Command {
	return &cobra.Command{
		Use:   "find-by-path <path>",
		Short: "Find tmux session by filesystem path",
		Long:  "Searches for a tmux session by its working directory path. Returns the session name if found.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]

			if session, exists := list.FindTmuxSessionByPath(path); exists {
				fmt.Println(session.Name)
				return nil
			}

			return fmt.Errorf("no tmux session found for path: %s", path)
		},
	}
}
