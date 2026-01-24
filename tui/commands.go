package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshmedeski/sesh/v2/lister"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/previewer"
)

func loadSessionsWithFilter(l lister.Lister, filter FilterType) tea.Cmd {
	return func() tea.Msg {
		// Invalidate tmux cache to get fresh session data
		l.InvalidateTmuxCache()

		opts := lister.ListOptions{
			HideDuplicates: true, // Hide duplicate sessions
		}

		switch filter {
		case FilterTmux:
			opts.Tmux = true
		case FilterConfig:
			opts.Config = true
		case FilterZoxide:
			opts.Zoxide = true
		case FilterAll:
			// No filter - load all
		}

		sessions, err := l.List(opts)
		if err != nil {
			return nil
		}
		return SessionsLoadedMsg{Sessions: sessions}
	}
}

func loadSessionsPreservingState(l lister.Lister, filter FilterType, filterText string, cursorIndex int) tea.Cmd {
	return func() tea.Msg {
		// Invalidate tmux cache to get fresh session data after deletion
		l.InvalidateTmuxCache()

		opts := lister.ListOptions{
			HideDuplicates: true, // Hide duplicate sessions
		}

		switch filter {
		case FilterTmux:
			opts.Tmux = true
		case FilterConfig:
			opts.Config = true
		case FilterZoxide:
			opts.Zoxide = true
		case FilterAll:
			// No filter - load all
		}

		sessions, err := l.List(opts)
		if err != nil {
			return nil
		}
		return SessionsLoadedMsg{
			Sessions:            sessions,
			PreserveFilterText:  filterText,
			PreserveCursorIndex: cursorIndex,
		}
	}
}

func loadPreview(p previewer.Previewer, session model.SeshSession) tea.Cmd {
	return func() tea.Msg {
		// Determine if this is an active tmux session
		isActive := (session.Src == "tmux" && session.Attached > 0)

		// Use the session path
		path := session.Path

		// Generate rich preview if we have a path
		if path != "" {
			content := GenerateRichPreview(session.Name, path, isActive)
			return PreviewLoadedMsg{Content: content}
		}

		// Fallback to default previewer
		content, err := p.Preview(session.Name)
		if err != nil {
			return PreviewLoadedMsg{Content: "Error loading preview: " + err.Error()}
		}
		if content == "" {
			return PreviewLoadedMsg{Content: "No preview available"}
		}
		return PreviewLoadedMsg{Content: content}
	}
}

// debouncePreview creates a debounced preview loading command
// This prevents preview flickering during rapid cursor navigation
func debouncePreview(sessionName string) tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return DebounceTickMsg{SessionName: sessionName}
	})
}

// restoreFilterMode re-enters filter mode and optionally types filter text
func restoreFilterMode(filterText string) tea.Cmd {
	// Create sequence: first '/', then each character of filter text, then completion message
	cmds := make([]tea.Cmd, 0, len(filterText)+2)

	// Enter filter mode
	cmds = append(cmds, func() tea.Msg {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	})

	// Type each character of the filter text
	if filterText != "" {
		for _, r := range filterText {
			r := r // capture loop variable
			cmds = append(cmds, func() tea.Msg {
				return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
			})
		}
	}

	// Signal restoration complete
	cmds = append(cmds, func() tea.Msg {
		return RestorationCompleteMsg{}
	})

	return tea.Sequence(cmds...)
}

