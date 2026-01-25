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
			isAttached := t.IsAttached()

			// Ensure tmux server is running before attempting attach operations
			// This handles the case where server was killed (tkill/tmux kill-server)
			if !isAttached {
				if err := t.StartServer(); err != nil {
					// Non-fatal: some operations may still work
				}
			}

			// FAST PATH: Try to get current session name without full list
			currentName := ""
			if isAttached {
				if name, err := t.GetCurrentSessionName(); err == nil {
					currentName = name
				}
			}

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

			// FAST PATH: Try direct attach/switch to first candidate
			// This skips the full session list call in the common case
			for _, entry := range sortedRecent {
				if entry.name != currentName {
					// Try direct attach (outside tmux) or switch (inside tmux)
					var err error
					if isAttached {
						_, err = t.SwitchClient(entry.name)
					} else {
						_, err = t.AttachSession(entry.name)
					}

					if err == nil {
						// Success! Record the session if attached
						if isAttached {
							_ = r.RecordSession(entry.name)
						}
						return nil
					}
					// Attach/switch failed - break to fall back to full approach
					break
				}
			}

			// FALLBACK: Full approach with existence checking
			// This happens if the fast-path switch failed (session doesn't exist)
			currentSession, currentExists := l.GetAttachedTmuxSession()

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
