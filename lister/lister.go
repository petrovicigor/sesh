package lister

import (
	"github.com/joshmedeski/sesh/v2/git"
	"github.com/joshmedeski/sesh/v2/home"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/recent"
	"github.com/joshmedeski/sesh/v2/tmux"
	"github.com/joshmedeski/sesh/v2/tmuxinator"
	"github.com/joshmedeski/sesh/v2/zoxide"
)

type Lister interface {
	List(opts ListOptions) (model.SeshSessions, error)
	FindTmuxSession(name string) (model.SeshSession, bool)
	FindTmuxSessionByPath(path string) (model.SeshSession, bool)
	GetAttachedTmuxSession() (model.SeshSession, bool)
	GetLastTmuxSession() (model.SeshSession, bool)
	ListTmuxSessions() model.SeshSessions
	FindConfigSession(name string) (model.SeshSession, bool)
	FindZoxideSession(name string) (model.SeshSession, bool)
	FindTmuxinatorConfig(name string) (model.SeshSession, bool)
	FindProjectSession(name string) (model.SeshSession, bool)
	InvalidateTmuxCache()
}

type RealLister struct {
	config     model.Config
	home       home.Home
	tmux       tmux.Tmux
	zoxide     zoxide.Zoxide
	tmuxinator tmuxinator.Tmuxinator
	git        git.Git
	recent     recent.Recent

	// Tmux session cache (per-request)
	tmuxCache       model.SeshSessions
	tmuxCacheLoaded bool
}

func NewLister(config model.Config, home home.Home, tmux tmux.Tmux, zoxide zoxide.Zoxide, tmuxinator tmuxinator.Tmuxinator, git git.Git, recent recent.Recent) Lister {
	return &RealLister{
		config:          config,
		home:            home,
		tmux:            tmux,
		zoxide:          zoxide,
		tmuxinator:      tmuxinator,
		git:             git,
		recent:          recent,
		tmuxCache:       model.SeshSessions{},
		tmuxCacheLoaded: false,
	}
}

func (l *RealLister) InvalidateTmuxCache() {
	l.tmuxCacheLoaded = false
	l.tmuxCache = model.SeshSessions{}
}
