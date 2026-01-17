package tui

import "github.com/joshmedeski/sesh/v2/model"

// sessionItem implements list.Item interface for bubbles/list
type sessionItem struct {
	session     model.SeshSession
	displayName string // Pre-computed with icon
}

// Title returns the display name shown in the list
func (i sessionItem) Title() string {
	return i.displayName
}

// Description returns empty string (no description line needed)
func (i sessionItem) Description() string {
	return ""
}

// FilterValue returns the value used for fuzzy filtering
func (i sessionItem) FilterValue() string {
	return i.session.Name
}
