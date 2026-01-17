package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshmedeski/sesh/v2/lister"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/previewer"
)

func loadSessionsWithFilter(l lister.Lister, filter FilterType) tea.Cmd {
	return func() tea.Msg {
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
