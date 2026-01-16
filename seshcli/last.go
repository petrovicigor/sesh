package seshcli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/joshmedeski/sesh/v2/lister"
	"github.com/joshmedeski/sesh/v2/tmux"
)

func NewLastCommand(l lister.Lister, t tmux.Tmux) *cobra.Command {
	return &cobra.Command{
		Use:     "last",
		Aliases: []string{"L"},
		Short:   "Connect to the last tmux session",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get current session
			currentSession, currentExists := l.GetAttachedTmuxSession()
			if !currentExists {
				return fmt.Errorf("not attached to any session")
			}

			// Get the "last" session (second in sorted order)
			lastSession, lastExists := l.GetLastTmuxSession()
			if !lastExists {
				return fmt.Errorf("no last session found")
			}

			// If "last" is actually current (race condition), find first different session
			if lastSession.Name == currentSession.Name {
				sessions := l.ListTmuxSessions()
				found := false
				for _, key := range sessions.OrderedIndex {
					session := sessions.Directory[key]
					if session.Name != currentSession.Name {
						lastSession = session
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("only one session exists")
				}
			}

			// Switch with proper error handling
			_, err := t.SwitchClient(lastSession.Name)
			if err != nil {
				return fmt.Errorf("failed to switch: %w", err)
			}
			return nil
		},
	}
}
