package connector

import (
	"github.com/joshmedeski/sesh/v2/dir"
	"github.com/joshmedeski/sesh/v2/home"
	"github.com/joshmedeski/sesh/v2/lister"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/namer"
	"github.com/joshmedeski/sesh/v2/recent"
	"github.com/joshmedeski/sesh/v2/startup"
	"github.com/joshmedeski/sesh/v2/tmux"
	"github.com/joshmedeski/sesh/v2/tmuxinator"
	"github.com/joshmedeski/sesh/v2/zoxide"
)

type Connector interface {
	Connect(name string, opts model.ConnectOpts) (string, error)
}

type RealConnector struct {
	config     model.Config
	dir        dir.Dir
	home       home.Home
	lister     lister.Lister
	namer      namer.Namer
	startup    startup.Startup
	tmux       tmux.Tmux
	zoxide     zoxide.Zoxide
	tmuxinator tmuxinator.Tmuxinator
	recent     recent.Recent
}

func NewConnector(
	config model.Config,
	dir dir.Dir,
	home home.Home,
	lister lister.Lister,
	namer namer.Namer,
	startup startup.Startup,
	tmux tmux.Tmux,
	zoxide zoxide.Zoxide,
	tmuxinator tmuxinator.Tmuxinator,
	recent recent.Recent,
) Connector {
	return &RealConnector{
		config,
		dir,
		home,
		lister,
		namer,
		startup,
		tmux,
		zoxide,
		tmuxinator,
		recent,
	}
}
