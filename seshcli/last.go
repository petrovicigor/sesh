package seshcli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/joshmedeski/sesh/v2/connector"
	"github.com/joshmedeski/sesh/v2/lister"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/recent"
	"github.com/joshmedeski/sesh/v2/tmux"
)

func NewLastCommand(l lister.Lister, t tmux.Tmux, r recent.Recent, c connector.Connector) *cobra.Command {
	return &cobra.Command{
		Use:     "last",
		Aliases: []string{"L"},
		Short:   "Connect to the last tmux session",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get current session (if attached)
			currentSession, currentExists := l.GetAttachedTmuxSession()

			// Get recent sessions sorted by timestamp (most recent first)
			recentSessions := r.GetAll()

			// Build sorted list of recent sessions
			type sessionWithTime struct {
				name string
				time time.Time
			}
			var sortedRecent []sessionWithTime

			for name, timestamp := range recentSessions {
				sortedRecent = append(sortedRecent, sessionWithTime{name, timestamp})
			}

			// Sort by timestamp (most recent first)
			sort.Slice(sortedRecent, func(i, j int) bool {
				return sortedRecent[i].time.After(sortedRecent[j].time)
			})

			// Find the most recent session that's NOT the current one
			// First pass: prefer sessions that still exist in tmux
			var targetSession string
			var fallbackSession string // First non-current session (may not exist in tmux)

			for _, entry := range sortedRecent {
				if !currentExists || entry.name != currentSession.Name {
					// Track the first candidate as fallback (for recreating killed sessions)
					if fallbackSession == "" {
						fallbackSession = entry.name
					}
					// Prefer sessions that still exist in tmux
					if _, exists := l.FindTmuxSession(entry.name); exists {
						targetSession = entry.name
						break
					}
				}
			}

			// If no existing session found, use fallback (recreates the killed session)
			if targetSession == "" {
				targetSession = fallbackSession
			}

			if targetSession == "" {
				return fmt.Errorf("no recent sessions found")
			}

			// Use connector - it handles switch/attach and session creation
			// Only skip recording when NOT in tmux (to avoid feedback loop)
			// When in tmux, record normally (legitimate session switching)
			_, err := c.Connect(targetSession, model.ConnectOpts{SkipRecent: !currentExists})
			if err != nil {
				return fmt.Errorf("failed to connect to last session '%s': %w", targetSession, err)
			}
			return nil
		},
	}
}
