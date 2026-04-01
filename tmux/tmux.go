package tmux

import (
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

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
	StartServer() error
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

// GracefulPaneCleanup performs pre-kill cleanup for all panes in a session:
// - Notifies claude-sessions about pane exits (fire-and-forget)
// - Sends SIGTERM to neovim/vim processes for clean VimLeave autocmd shutdown
// Must be called before KillSession while tmux metadata still exists.
func GracefulPaneCleanup(sessionName string) {
	out, err := exec.Command("tmux", "list-panes", "-s", "-t", sessionName,
		"-F", "#{pane_id}\t#{pane_current_command}\t#{pane_pid}").Output()
	if err != nil {
		return
	}

	var wg sync.WaitGroup
	var hasNvim bool
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		paneID := parts[0]
		paneCmd := strings.TrimSpace(parts[1])
		panePid := strings.TrimSpace(parts[2])

		// Notify claude-sessions (parallel, but wait before session kill)
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			exec.Command("claude-sessions", "pane-exited", id).Run()
		}(paneID)

		// Send SIGTERM to neovim for graceful shutdown (triggers VimLeave
		// autocmds without flashing :qa! in the command line)
		if paneCmd == "nvim" || paneCmd == "vim" {
			if nvimPidOut, err := exec.Command("pgrep", "-P", panePid, "-x", paneCmd).Output(); err == nil {
				if pid, err := strconv.Atoi(strings.TrimSpace(string(nvimPidOut))); err == nil {
					syscall.Kill(pid, syscall.SIGTERM)
				}
			}
			hasNvim = true
		}
	}

	// Wait for claude-sessions notifications to finish (with timeout)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	timeout := 50 * time.Millisecond
	if hasNvim {
		timeout = 100 * time.Millisecond
	}
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

// CollectPanePids enumerates all pane PIDs and their descendant process trees
// for a tmux session. Must be called before kill-session destroys the metadata.
func CollectPanePids(sessionName string) []int {
	out, err := exec.Command("tmux", "list-panes", "-s", "-t", sessionName, "-F", "#{pane_pid}").Output()
	if err != nil {
		return nil
	}

	var allPids []int
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pid, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || pid <= 1 {
			continue
		}
		allPids = append(allPids, collectDescendants(pid)...)
	}
	return allPids
}

// KillProcessTrees sends SIGTERM then SIGKILL to a list of PIDs (leaf-first).
func KillProcessTrees(pids []int) {
	for i := len(pids) - 1; i >= 0; i-- {
		syscall.Kill(pids[i], syscall.SIGTERM)
	}
	time.Sleep(50 * time.Millisecond)
	for i := len(pids) - 1; i >= 0; i-- {
		syscall.Kill(pids[i], syscall.SIGKILL)
	}
}

// collectDescendants returns a PID and all its descendants (breadth-first).
func collectDescendants(root int) []int {
	pids := []int{root}
	queue := []int{root}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		out, err := exec.Command("pgrep", "-P", strconv.Itoa(parent)).Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			child, err := strconv.Atoi(strings.TrimSpace(line))
			if err != nil || child <= 1 {
				continue
			}
			pids = append(pids, child)
			queue = append(queue, child)
		}
	}
	return pids
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

func (t *RealTmux) StartServer() error {
	_, err := t.shell.Cmd("tmux", "start-server")
	return err
}
