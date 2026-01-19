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
	Sessions model.SeshSessions
}

type PreviewLoadedMsg struct {
	Content string
}

type DebounceTickMsg struct {
	SessionName string
}
