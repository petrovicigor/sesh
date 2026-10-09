package model

type ConnectOpts struct {
	Command    string
	Switch     bool
	Tmuxinator bool
	SkipRecent bool   // Don't record this connection in recent sessions
	Pane       string // Land on this pane (%N) of the session
}
