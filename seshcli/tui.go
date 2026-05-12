package seshcli

import (
	"github.com/spf13/cobra"

	"github.com/joshmedeski/sesh/v2/connector"
	"github.com/joshmedeski/sesh/v2/icon"
	"github.com/joshmedeski/sesh/v2/lister"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/previewer"
	"github.com/joshmedeski/sesh/v2/recent"
	"github.com/joshmedeski/sesh/v2/tmux"
	"github.com/joshmedeski/sesh/v2/tui"
)

func NewTuiCommand(
	l lister.Lister,
	c connector.Connector,
	i icon.Icon,
	t tmux.Tmux,
	cfg model.Config,
	p previewer.Previewer,
	r recent.Recent,
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Interactive session picker (Bubble Tea TUI)",
		RunE: func(cmd *cobra.Command, args []string) error {
			tuiInstance := tui.NewTUI(l, c, i, t, cfg, p, r)
			selected, err := tuiInstance.Run()
			if err != nil {
				return err
			}
			if selected == "" {
				// User cancelled, exit cleanly
				return nil
			}

			// Connect to selected session. Auto-restore happens via sesh's
			// startup_command → `tmux-session-saver restore-or` for newly
			// created sessions; no manual restore path here.
			trimmedName := i.RemoveIcon(selected)
			_, err = c.Connect(trimmedName, model.ConnectOpts{})
			return err
		},
	}

	return cmd
}
