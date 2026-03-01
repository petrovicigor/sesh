package seshcli

import (
	"os/exec"

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
			selected, restoreRequested, err := tuiInstance.Run()
			if err != nil {
				return err
			}
			if selected == "" {
				// User cancelled, exit cleanly
				return nil
			}

			// Connect to selected session
			trimmedName := i.RemoveIcon(selected)

			// Schedule restore BEFORE connect — Connect triggers tmux switch-client
			// which closes the popup and kills this process. tmux run-shell -b
			// runs in tmux's server process and survives popup closure.
			// The delay ensures Connect has created the session first.
			connectOpts := model.ConnectOpts{}
			if restoreRequested {
				// Skip startup command — restore will recreate all windows
				connectOpts.Command = "true"
				// Restore saved state, then delete the save file on success
				sanitized := tui.SanitizeSessionName(trimmedName)
				exec.Command("tmux", "run-shell", "-b",
					"sleep 0.5 && tmux-session-saver restore '"+trimmedName+"'"+
						" && rm -f \"$HOME/.local/share/tmux-session-saver/"+sanitized+".json\"").Run()
			}

			_, err = c.Connect(trimmedName, connectOpts)
			return err
		},
	}

	return cmd
}
