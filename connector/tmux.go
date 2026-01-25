package connector

import "github.com/joshmedeski/sesh/v2/model"

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

func connectToTmux(c *RealConnector, connection model.Connection, opts model.ConnectOpts) (string, error) {
	if connection.New {
		name := connection.Session.Name

		// Check for name collision with different path
		existingSession, exists := c.lister.FindTmuxSession(name)
		if exists && existingSession.Path != connection.Session.Path {
			// Disambiguate: increment dir_length until unique
			name = c.disambiguateName(connection.Session.Path, name)
			connection.Session.Name = name
		}

		c.tmux.NewSession(connection.Session.Name, connection.Session.Path)
		if opts.Command != "" {
			c.tmux.SendKeys(connection.Session.Name, opts.Command)
		} else {
			c.startup.Exec(connection.Session)
		}
	}
	// Ensure server is running before switch/attach
	// This handles edge case where new-session succeeded but server
	// state is inconsistent
	_ = c.tmux.StartServer()
	return c.tmux.SwitchOrAttach(connection.Session.Name, opts)
}
