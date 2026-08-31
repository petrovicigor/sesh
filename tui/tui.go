package tui

import (
	"os"

	tea "charm.land/bubbletea/v2"
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

	// The bind-o float marks itself with SESH_FLOAT=1 (new-pane -e): the
	// floating flag alone is not enough — `sesh tui` inside a float the user
	// placed by hand must not have its geometry clobbered by the preview
	// toggle. The flag is still verified so a leaked env var can't make the
	// toggle resize a tiled pane. Outside tmux ($TMUX_PANE unset) this stays
	// false and ctrl+p just re-flows.
	isFloating := false
	pane := os.Getenv("TMUX_PANE")
	if pane != "" && os.Getenv("SESH_FLOAT") != "" {
		var ferr error
		isFloating, ferr = t.tmux.IsPaneFloating(pane)
		if ferr != nil {
			logDebug("Run: IsPaneFloating: %v (treating as not floating)", ferr)
		}
	}

	// The dim backdrop is not handled here: the bind sets window-style
	// dim=60% on the way in and its shell tail unsets it when sesh exits
	// (see bind o in .tmux.conf, including the accepted kill-pane wedge —
	// kill-pane SIGKILLs this whole process group, so nothing in-process
	// could clean up anyway).

	m := newModel(t.lister, t.connector, t.icon, t.tmux, t.config, t.previewer, sessions, worktreeDefaults, defaultsPath, frecencyScores, excludesPath, isFloating)
	logDebug("newModel() done")

	p := tea.NewProgram(m)
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
