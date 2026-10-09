// Package trail lets `sesh last` return to the Claude Code session you were
// in, not just the tmux session: every switch notes the Claude session in the
// pane being left (Depart), and `sesh last` brings you back to it — its pane
// when it still runs, the claude-sessions picker's resume offer when it
// ended, a status-bar message when it was deleted (Return).
package trail

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/joshmedeski/sesh/v2/claude"
	"github.com/joshmedeski/sesh/v2/connector"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/recent"
)

// Trail holds the seams; New wires the real ones.
type Trail struct {
	Recent recent.Recent
	Home   string
	// Tmux runs one tmux invocation (args after "tmux"); a bare ";" separates
	// commands.
	Tmux        func(args ...string) (string, error)
	PaneSession func(home, pane string) (id, title string, ok bool)
	Lookup      func(home, id string) (claude.Session, bool, error)
	// Offer opens the claude-sessions picker in session with id selected and
	// its resume prompt up.
	Offer      func(id, session string) error
	FileExists func(path string) bool
}

func New(r recent.Recent, home string) *Trail {
	return &Trail{
		Recent: r,
		Home:   home,
		Tmux: func(args ...string) (string, error) {
			out, err := exec.Command("tmux", args...).Output()
			return strings.TrimSpace(string(out)), err
		},
		PaneSession: claude.PaneSession,
		Lookup:      claude.Lookup,
		Offer: func(id, session string) error {
			return exec.Command("claude-sessions", "offer-resume", id, session).Run()
		},
		FileExists: func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		},
	}
}

// spot is the session the client is in and the pane it is looking at. A
// float (sesh's own, the claude-sessions picker) is not where you were: its
// window's last tiled pane is — a modal float is always the active pane, so
// the pane it covers carries pane_last.
func (t *Trail) spot() (session, pane string) {
	out, err := t.Tmux("display-message", "-p", "#{session_name}\t#{pane_id}\t#{pane_floating_flag}")
	if err != nil {
		return "", ""
	}
	parts := strings.Split(out, "\t")
	if len(parts) != 3 {
		return "", ""
	}
	session, pane = parts[0], parts[1]
	if parts[2] != "1" {
		return session, pane
	}
	out, err = t.Tmux("list-panes", "-F", "#{pane_id}\t#{pane_last}\t#{pane_floating_flag}")
	if err != nil {
		return session, ""
	}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) == 3 && f[1] == "1" && f[2] == "0" {
			return session, f[0]
		}
	}
	return session, ""
}

// Depart notes where the client is leaving from before a switch to target:
// the current session moves to now in the recent list and remembers the live
// Claude session in the pane you were looking at (none clears the note). A
// switch within the session records nothing.
func (t *Trail) Depart(target string) {
	session, pane := t.spot()
	if session == "" || session == target {
		return
	}
	id, title, _ := t.PaneSession(t.Home, pane)
	_ = t.Recent.RecordDeparture(session, id, title)
}

// SelectPane makes pane the active one in its window and the window the
// current one in its session — before the client switches there, so the
// switch lands on it with nothing in between.
func (t *Trail) SelectPane(pane string) error {
	_, err := t.Tmux("select-window", "-t", pane, ";", "select-pane", "-t", pane)
	return err
}

// Return is `sesh last`'s way back into the Claude session noted for target.
// Call it before switching to target and run the returned func after the
// switch. A live session's pane is selected now; an ended one gets the
// picker's resume offer and a deleted one a message, both once the client is
// there. Anything unknown (no note, unreadable db) does nothing — the switch
// alone is today's behavior.
func (t *Trail) Return(target string) (after func()) {
	noop := func() {}
	entry, ok := t.Recent.GetEntry(target)
	if !ok || entry.Claude == "" {
		return noop
	}
	s, found, err := t.Lookup(t.Home, entry.Claude)
	if err != nil {
		return noop
	}
	title := entry.ClaudeTitle
	if found && s.Title != "" {
		title = s.Title
	}
	if title == "" {
		title = "untitled"
	}

	switch {
	case !found:
		return func() { t.message(fmt.Sprintf("Claude session “%s” was deleted", title)) }
	case s.Live:
		if s.Pane != "" && t.paneSession(s.Pane) == target {
			_ = t.SelectPane(s.Pane)
		}
		return noop
	case s.Transcript != "" && !t.FileExists(s.Transcript):
		return func() { t.message(fmt.Sprintf("Claude session “%s” is gone (no transcript)", title)) }
	default:
		return func() {
			if err := t.Offer(s.ID, target); err != nil {
				t.message(fmt.Sprintf("Claude session “%s” ended — resume it from the picker", title))
			}
		}
	}
}

// paneSession is the session pane lives in now ("" when it is gone).
func (t *Trail) paneSession(pane string) string {
	out, err := t.Tmux("display-message", "-p", "-t", pane, "#{session_name}")
	if err != nil {
		return ""
	}
	return out
}

// message shows msg on the status line. display-message expands formats, so
// a title's # is escaped.
func (t *Trail) message(msg string) {
	_, _ = t.Tmux("display-message", "-d", "4000", strings.ReplaceAll(msg, "#", "##"))
}

// Connector wraps a connector so every sesh switch inside tmux — connect,
// the tui, last's fallback — notes its departure first, and opts.Pane lands
// on that pane.
type Connector struct {
	connector.Connector
	T *Trail
}

func (c Connector) Connect(name string, opts model.ConnectOpts) (string, error) {
	if !opts.SkipRecent {
		c.T.Depart(name)
	}
	if opts.Pane != "" {
		if err := c.T.SelectPane(opts.Pane); err != nil {
			return "", fmt.Errorf("pane %s: %w", opts.Pane, err)
		}
	}
	return c.Connector.Connect(name, opts)
}
