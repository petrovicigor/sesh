package tui

import "charm.land/bubbles/v2/key"

type KeyMap struct {
	Quit             key.Binding
	Select           key.Binding
	FilterAll        key.Binding
	FilterConfig     key.Binding
	FilterZoxide     key.Binding
	ToggleZoxide     key.Binding
	Delete           key.Binding
	GoToWorktreeRoot key.Binding
	DetectProcesses  key.Binding
	ExpandGroup      key.Binding
	SetDefault       key.Binding
	RepoFocus         key.Binding
	WorkspaceManager  key.Binding
	ToggleGroupMode   key.Binding
	RestoreConnect    key.Binding
	RestoreSession    key.Binding
	SaveSession       key.Binding
	SaveAll           key.Binding
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
	FilterConfig: key.NewBinding(
		key.WithHelp("", "config only"),
		// Unbound — ctrl+g reassigned to ToggleGroupMode
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
	DetectProcesses: key.NewBinding(
		key.WithKeys("ctrl+e"),
		key.WithHelp("ctrl+e", "detect processes"),
	),
	ExpandGroup: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "expand/collapse group"),
	),
	SetDefault: key.NewBinding(
		key.WithKeys("ctrl+p"),
		key.WithHelp("ctrl+p", "set default worktree"),
	),
	RepoFocus: key.NewBinding(
		key.WithKeys("ctrl+t"),
		key.WithHelp("ctrl+t", "focus repo worktrees"),
	),
	WorkspaceManager: key.NewBinding(
		key.WithKeys("ctrl+w"),
		key.WithHelp("ctrl+w", "workspace manager"),
	),
	ToggleGroupMode: key.NewBinding(
		key.WithKeys("ctrl+g"),
		key.WithHelp("ctrl+g", "toggle group mode"),
	),
	RestoreConnect: key.NewBinding(
		key.WithKeys("shift+enter"),
		key.WithHelp("shift+enter", "restore & connect"),
	),
	RestoreSession: key.NewBinding(
		key.WithKeys("ctrl+r"),
		key.WithHelp("ctrl+r", "restore session"),
	),
	SaveSession: key.NewBinding(
		key.WithKeys("ctrl+s"),
		key.WithHelp("ctrl+s", "save session state"),
	),
	SaveAll: key.NewBinding(
		key.WithKeys("ctrl+a"),
		key.WithHelp("ctrl+a", "save all tmux sessions"),
	),
}
