package connector

import (
	"github.com/joshmedeski/sesh/v2/model"
)

func projectStrategy(c *RealConnector, name string) (model.Connection, error) {
	project, exists := c.lister.FindProjectSession(name)
	if !exists {
		return model.Connection{Found: false}, nil
	}

	return model.Connection{
		Found:       true,
		Session:     project,
		New:         true,
		AddToZoxide: true,
	}, nil
}
