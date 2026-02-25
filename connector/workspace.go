package connector

import (
	"github.com/joshmedeski/sesh/v2/model"
)

func workspaceStrategy(c *RealConnector, name string) (model.Connection, error) {
	session, exists := c.lister.FindWorkspaceSession(name)
	if !exists {
		return model.Connection{Found: false}, nil
	}

	return model.Connection{
		Found:       true,
		Session:     session,
		New:         true,
		AddToZoxide: true,
	}, nil
}
