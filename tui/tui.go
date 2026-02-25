package tui

import (
	"os"

	"github.com/charmbracelet/bubbletea"
	"github.com/joshmedeski/sesh/v2/connector"
	"github.com/joshmedeski/sesh/v2/icon"
	"github.com/joshmedeski/sesh/v2/lister"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/previewer"
	"github.com/joshmedeski/sesh/v2/recent"
	"github.com/joshmedeski/sesh/v2/state"
	"github.com/joshmedeski/sesh/v2/tmux"
)

type TUI struct {
	lister    lister.Lister
	connector connector.Connector
	icon      icon.Icon
	tmux      tmux.Tmux
	config    model.Config
	previewer previewer.Previewer
	recent    recent.Recent
}

func NewTUI(
	lister lister.Lister,
	connector connector.Connector,
	icon icon.Icon,
	tmux tmux.Tmux,
	config model.Config,
	previewer previewer.Previewer,
	recent recent.Recent,
) *TUI {
	return &TUI{
		lister:    lister,
		connector: connector,
		icon:      icon,
		tmux:      tmux,
		config:    config,
		previewer: previewer,
		recent:    recent,
	}
}

func (t *TUI) Run() (string, error) {
	resetStartTime()
	logDebug("Run() entered")

	// Load sessions synchronously (fast: ~10-15ms parallel)
	sessions, err := t.lister.List(lister.ListOptions{
		HideDuplicates: true,
	})
	if err != nil {
		return "", err
	}
	logDebug("lister.List() done (%d sessions)", len(sessions.OrderedIndex))

	// Load worktree defaults (sub-millisecond, never fails)
	defaultsPath := state.DefaultsPath(os.Getenv("XDG_STATE_HOME"))
	worktreeDefaults, _ := state.LoadDefaults(defaultsPath)
	logDebug("state.LoadDefaults() done")

	// Load frecency scores for filter tiebreaking
	frecencyScores := t.recent.GetFrecencyScores()
	logDebug("GetFrecencyScores() done (%d entries)", len(frecencyScores))

	// Compute excludes path for workspace manager
	excludesPath := state.ExcludesPath(os.Getenv("XDG_STATE_HOME"))

	m := newModel(t.lister, t.connector, t.icon, t.tmux, t.config, t.previewer, sessions, worktreeDefaults, defaultsPath, frecencyScores, excludesPath)
	logDebug("newModel() done")

	p := tea.NewProgram(m, tea.WithAltScreen())
	logDebug("tea.NewProgram() created, calling p.Run()")

	result, err := p.Run()
	logDebug("p.Run() returned")
	flushDebugLog()

	if err != nil {
		return "", err
	}
	finalModel, ok := result.(Model)
	if !ok {
		return "", nil
	}
	return finalModel.selected, nil
}
