package tui

import (
	"github.com/charmbracelet/bubbletea"
	"github.com/joshmedeski/sesh/v2/connector"
	"github.com/joshmedeski/sesh/v2/icon"
	"github.com/joshmedeski/sesh/v2/lister"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/previewer"
	"github.com/joshmedeski/sesh/v2/tmux"
)

type TUI struct {
	lister    lister.Lister
	connector connector.Connector
	icon      icon.Icon
	tmux      tmux.Tmux
	config    model.Config
	previewer previewer.Previewer
}

func NewTUI(
	lister lister.Lister,
	connector connector.Connector,
	icon icon.Icon,
	tmux tmux.Tmux,
	config model.Config,
	previewer previewer.Previewer,
) *TUI {
	return &TUI{
		lister:    lister,
		connector: connector,
		icon:      icon,
		tmux:      tmux,
		config:    config,
		previewer: previewer,
	}
}

func (t *TUI) Run() (string, error) {
	// Pre-load sessions synchronously for instant display
	sessions, err := t.lister.List(lister.ListOptions{
		HideDuplicates: true, // Hide duplicate sessions (e.g., tmux + projects for same dir)
	})
	if err != nil {
		return "", err
	}

	m := newModel(t.lister, t.connector, t.icon, t.tmux, t.config, t.previewer, sessions)
	p := tea.NewProgram(m) // Try without alt screen

	result, err := p.Run()
	if err != nil {
		return "", err
	}
	finalModel, ok := result.(Model)
	if !ok {
		return "", nil
	}
	return finalModel.selected, nil
}
