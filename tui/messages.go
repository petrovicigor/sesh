package tui

import "github.com/joshmedeski/sesh/v2/model"

type FilterType int

const (
	FilterAll FilterType = iota
	FilterTmux
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

type DebounceTickMsg struct {
	SessionName string
}

type RestorationCompleteMsg struct{}

type setCursorMsg struct {
	index int
}

type StartProcessDetectionMsg struct{}

type ProcessInfoMsg struct {
	Processes map[string]string // session name -> process type ("node", etc)
}
