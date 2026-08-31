package tui

import "github.com/joshmedeski/sesh/v2/model"

type FilterType int

const (
	FilterAll FilterType = iota
	FilterConfig
	FilterZoxide
)

type SessionsLoadedMsg struct {
	Sessions            model.SeshSessions
	PreserveFilterText  string // Filter text to restore ("" = don't preserve)
	PreserveCursorIndex int    // Cursor index to restore (-1 = don't preserve)
}

type PreviewLoadedMsg struct {
	Content string
}

type ProcessInfoMsg struct {
	Processes map[string]string // session name -> process type ("node", etc)
}

type DefaultsSavedMsg struct {
	Err error // nil on success
}

type ExcludesSavedMsg struct {
	Err error // nil on success
}

type ClaudeAttentionMsg struct {
	Sessions map[string]bool // tmux session name -> needs attention
}

type SavedStateMsg struct {
	Sessions map[string]bool // session name -> has saved state in tmux-session-saver
}

// GitChangesMsg delivers the one-shot working-tree check for tmux-backed rows.
type GitChangesMsg struct {
	Changes map[string]gitChanges // absolute directory path -> change counts
}

// SessionKilledMsg is sent after a tmux session has been killed (with process cleanup).
type SessionKilledMsg struct {
	Err error
}

// enterFilterMsg is sent to enter filter mode without fabricating a KeyMsg.
type enterFilterMsg struct{}

// claudeAttentionTickMsg triggers a periodic re-check of Claude attention status.
type claudeAttentionTickMsg struct{}

// applyRestoreStateMsg applies persisted TUI state after a kill-and-relaunch
// toggle. Sent once from Init() when the model was created with restore state.
type applyRestoreStateMsg struct{}
