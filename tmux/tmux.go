package tmux

import (
	"strings"

	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/oswrap"
	"github.com/joshmedeski/sesh/v2/shell"
)

type Tmux interface {
	ListSessions() ([]*model.TmuxSession, error)
	NewSession(sessionName string, startDir string) (string, error)
	NewWindow(startDir string, name string) (string, error)
	IsAttached() bool
	GetCurrentSessionName() (string, error)
	AttachSession(targetSession string) (string, error)
	SendKeys(name string, command string) (string, error)
	SwitchClient(targetSession string) (string, error)
	CapturePane(targetSession string) (string, error)
	NextWindow() (string, error)
	NextWindowInSession(sessionName string) (string, error)
	SwitchOrAttach(name string, opts model.ConnectOpts) (string, error)
	KillSession(name string) (string, error)
}

type RealTmux struct {
	os    oswrap.Os
	shell shell.Shell
}

func NewTmux(os oswrap.Os, shell shell.Shell) Tmux {
	return &RealTmux{os, shell}
}

func (t *RealTmux) AttachSession(targetSession string) (string, error) {
	return t.shell.Cmd("tmux", "attach-session", "-t", targetSession)
}

func (t *RealTmux) SwitchClient(targetSession string) (string, error) {
	return t.shell.Cmd("tmux", "switch-client", "-t", targetSession)
}

func (t *RealTmux) SendKeys(targetPane string, keys string) (string, error) {
	return t.shell.Cmd("tmux", "send-keys", "-t", targetPane, keys, "Enter")
}

func (t *RealTmux) NewSession(sessionName string, startDir string) (string, error) {
	return t.shell.Cmd("tmux", "new-session", "-d", "-s", sessionName, "-c", startDir)
}

func (t *RealTmux) NewWindow(startDir string, name string) (string, error) {
	return t.shell.Cmd("tmux", "new-window", "-n", name, "-c", startDir)
}

func (t *RealTmux) CapturePane(targetSession string) (string, error) {
	return t.shell.Cmd("tmux", "capture-pane", "-e", "-p", "-t", targetSession)
}

func (t *RealTmux) NextWindow() (string, error) {
	return t.shell.Cmd("tmux", "next-window")
}

func (t *RealTmux) NextWindowInSession(sessionName string) (string, error) {
	return t.shell.Cmd("tmux", "next-window", "-t", sessionName)
}

func (t *RealTmux) KillSession(name string) (string, error) {
	return t.shell.Cmd("tmux", "kill-session", "-t", name)
}

func (t *RealTmux) IsAttached() bool {
	return len(t.os.Getenv("TMUX")) > 0
}

func (t *RealTmux) GetCurrentSessionName() (string, error) {
	name, err := t.shell.Cmd("tmux", "display-message", "-p", "#S")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(name), nil
}
