package tui

import "github.com/charmbracelet/bubbles/key"

type KeyMap struct {
	Quit             key.Binding
	Select           key.Binding
	FilterAll        key.Binding
	FilterTmux       key.Binding
	FilterConfig     key.Binding
	FilterZoxide     key.Binding
	ToggleZoxide     key.Binding
	Delete           key.Binding
	GoToWorktreeRoot key.Binding
}

var DefaultKeyMap = KeyMap{
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c", "ctrl+b", "esc"),
		key.WithHelp("ctrl+c/esc", "quit"),
	),
	Select: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	FilterAll: key.NewBinding(
		key.WithKeys("ctrl+a"),
		key.WithHelp("ctrl+a", "all sessions"),
	),
	FilterTmux: key.NewBinding(
		key.WithKeys("ctrl+t"),
		key.WithHelp("ctrl+t", "tmux only"),
	),
	FilterConfig: key.NewBinding(
		key.WithKeys("ctrl+g"),
		key.WithHelp("ctrl+g", "config only"),
	),
	FilterZoxide: key.NewBinding(
		key.WithKeys("ctrl+x"),
		key.WithHelp("ctrl+x", "zoxide only"),
	),
	ToggleZoxide: key.NewBinding(
		key.WithKeys("ctrl+o"),
		key.WithHelp("ctrl+o", "toggle zoxide"),
	),
	Delete: key.NewBinding(
		key.WithKeys("ctrl+d", "ctrl+k"),
		key.WithHelp("ctrl+d/ctrl+k", "delete session"),
	),
	GoToWorktreeRoot: key.NewBinding(
		key.WithKeys("ctrl+0"),
		key.WithHelp("ctrl+0", "go to worktree root"),
	),
}
