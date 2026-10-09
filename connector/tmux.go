package connector

import (
	"fmt"
	"strings"

	"github.com/joshmedeski/sesh/v2/model"
)

func tmuxStrategy(c *RealConnector, name string) (model.Connection, error) {
	session, exists := c.lister.FindTmuxSession(name)
	if !exists {
		return model.Connection{Found: false}, nil
	}
	return model.Connection{
		Found:       true,
		Session:     session,
		New:         false,
		AddToZoxide: true,
	}, nil
}

// samePath compares two session paths, ignoring trailing slashes.
func samePath(a, b string) bool {
	return strings.TrimRight(a, "/") == strings.TrimRight(b, "/")
}

func connectToTmux(c *RealConnector, connection model.Connection, opts model.ConnectOpts) (string, error) {
	if connection.New {
		name := connection.Session.Name

		existingSession, exists := c.lister.FindTmuxSession(name)
		if exists && samePath(existingSession.Path, connection.Session.Path) {
			// A session for this exact name+path already exists (the
			// connect target was a path, so tmuxStrategy never matched it
			// by name and the strategy flagged the connection New). Attach
			// instead of re-creating: new-session -d would fail as a
			// duplicate, and the startup command would then be send-keys'd
			// into the existing session's active pane — typed into
			// whatever interactive program is running there.
			connection.Session = existingSession
		} else {
			if exists {
				// Name collision with a different path.
				// Disambiguate: increment dir_length until unique
				name = c.disambiguateName(connection.Session.Path, name)
				connection.Session.Name = name
			}

			if out, err := c.tmux.NewSession(connection.Session.Name, connection.Session.Path); err != nil {
				return out, fmt.Errorf("failed to create session %q: %w", connection.Session.Name, err)
			}
			if opts.Command != "" {
				c.tmux.SendKeys(connection.Session.Name, opts.Command)
			} else {
				c.startup.Exec(connection.Session)
			}
		}
	}
	// Ensure server is running before switch/attach
	// This handles edge case where new-session succeeded but server
	// state is inconsistent
	_ = c.tmux.StartServer()
	return c.tmux.SwitchOrAttach(connection.Session.Name, opts)
}
