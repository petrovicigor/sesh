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

type SessionSavedMsg struct {
	SessionName string // session that was saved
	Err         error  // nil on success
}

// SavedStateDeletedMsg is sent after deleting a tmux-session-saver save file.
type SavedStateDeletedMsg struct {
	SessionName string // sanitized session name (matches savedState map key)
	Err         error
}

// SessionKilledMsg is sent after a tmux session has been killed (with process cleanup).
type SessionKilledMsg struct {
	Err error
}

// SaveAllNextMsg triggers saving the next session in a save-all batch.
type SaveAllNextMsg struct{}

// SessionRestoredMsg is sent after tmux-session-saver restore completes for one session.
type SessionRestoredMsg struct {
	SessionName string
	Err         error
}

// RestoreAllNextMsg triggers restoring the next session in a restore-all batch,
// or exits to restore the current session.
type RestoreAllNextMsg struct{}

// enterFilterMsg is sent to enter filter mode without fabricating a KeyMsg.
type enterFilterMsg struct{}

// clearStatusMsg is sent after a timeout to clear the status message.
type clearStatusMsg struct{}

// claudeAttentionTickMsg triggers a periodic re-check of Claude attention status.
type claudeAttentionTickMsg struct{}

// applyRestoreStateMsg applies persisted TUI state after a kill-and-relaunch
// toggle. Sent once from Init() when the model was created with restore state.
type applyRestoreStateMsg struct{}
