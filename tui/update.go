package tui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Split width: 40% for list, 60% for preview (with borders)
		// Account for borders (2 chars each side) and gap
		listWidth := (msg.Width * 4) / 10
		previewWidth := msg.Width - listWidth - 6 // 6 for borders and gap

		m.list.SetSize(listWidth-4, msg.Height-4)      // -4 for border padding
		m.previewPort.Width = previewWidth - 4
		m.previewPort.Height = msg.Height - 4

		return m, nil

	case SessionsLoadedMsg:
		// Build new list items from loaded sessions
		items := make([]list.Item, 0, len(msg.Sessions.OrderedIndex))
		if msg.Sessions.Directory != nil && msg.Sessions.OrderedIndex != nil {
			for _, key := range msg.Sessions.OrderedIndex {
				if session, ok := msg.Sessions.Directory[key]; ok {
					items = append(items, sessionItem{
						session:     session,
						displayName: m.icon.AddIcon(session),
					})
				}
			}
		}
		m.sessions = msg.Sessions
		m.list.SetItems(items)

		// Reset list filter and cursor
		m.list.ResetFilter()
		if len(items) > 0 {
			m.list.Select(0)
		}

		// Update title based on current filter
		m.list.Title = getFilterTitle(m.currentFilter)

		// Load preview for first session
		var previewCmd tea.Cmd
		if len(items) > 0 {
			if firstItem, ok := items[0].(sessionItem); ok {
				// Blank preview while loading
				m.previewPort.SetContent("")
				previewCmd = loadPreview(m.previewer, firstItem.session)
			}
		} else {
			// No items - clear preview
			m.previewPort.SetContent("")
		}

		// Re-enable filter mode
		filterCmd := func() tea.Msg {
			return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
		}

		return m, tea.Batch(previewCmd, filterCmd)

	case PreviewLoadedMsg:
		m.previewContent = msg.Content
		m.previewPort.SetContent(msg.Content)
		return m, nil

	case tea.KeyMsg:
		// Handle quit/select/filter keys first
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, m.keys.Select):
			if item, ok := m.list.SelectedItem().(sessionItem); ok {
				m.selected = item.session.Name
				return m, tea.Quit
			}

		case key.Matches(msg, m.keys.FilterAll):
			m.currentFilter = FilterAll
			m.lastFilter = "" // Reset filter tracking
			return m, loadSessionsWithFilter(m.lister, FilterAll)

		case key.Matches(msg, m.keys.FilterTmux):
			m.currentFilter = FilterTmux
			m.lastFilter = "" // Reset filter tracking
			return m, loadSessionsWithFilter(m.lister, FilterTmux)

		case key.Matches(msg, m.keys.FilterConfig):
			m.currentFilter = FilterConfig
			m.lastFilter = "" // Reset filter tracking
			return m, loadSessionsWithFilter(m.lister, FilterConfig)

		case key.Matches(msg, m.keys.FilterZoxide):
			m.currentFilter = FilterZoxide
			m.lastFilter = "" // Reset filter tracking
			return m, loadSessionsWithFilter(m.lister, FilterZoxide)

		case key.Matches(msg, m.keys.ToggleZoxide):
			// Toggle between zoxide and all
			if m.currentFilter == FilterZoxide {
				m.currentFilter = FilterAll
				m.lastFilter = ""
				return m, loadSessionsWithFilter(m.lister, FilterAll)
			} else {
				m.currentFilter = FilterZoxide
				m.lastFilter = ""
				return m, loadSessionsWithFilter(m.lister, FilterZoxide)
			}

		case key.Matches(msg, m.keys.Delete):
			// Delete session if it's a tmux session
			if item, ok := m.list.SelectedItem().(sessionItem); ok {
				if item.session.Src == "tmux" {
					_, err := m.tmux.KillSession(item.session.Name)
					if err == nil {
						// Reload sessions after deletion
						return m, loadSessionsWithFilter(m.lister, m.currentFilter)
					}
				}
			}
			return m, nil
		}

		// When filtering, intercept arrow keys and handle them ourselves
		// This prevents the list from exiting filter mode
		if m.list.FilterState() == list.Filtering {
			switch msg.String() {
			case "up":
				m.list.CursorUp()
				// Load preview for newly selected session
				if item, ok := m.list.SelectedItem().(sessionItem); ok {
					// Blank preview while loading
					m.previewPort.SetContent("")
					return m, loadPreview(m.previewer, item.session)
				}
				return m, nil
			case "down":
				m.list.CursorDown()
				// Load preview for newly selected session
				if item, ok := m.list.SelectedItem().(sessionItem); ok {
					// Blank preview while loading
					m.previewPort.SetContent("")
					return m, loadPreview(m.previewer, item.session)
				}
				return m, nil
			}
		}

		// For all other cases, delegate to list
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)

		// If filter text changed, reset cursor to top and load preview
		currentFilter := m.list.FilterValue()
		if currentFilter != m.lastFilter {
			m.lastFilter = currentFilter
			m.list.Select(0)
			// Load preview for top item
			if item, ok := m.list.SelectedItem().(sessionItem); ok {
				// Blank preview while loading
				m.previewPort.SetContent("")
				return m, tea.Batch(cmd, loadPreview(m.previewer, item.session))
			}
		}

		return m, cmd
	}

	// Delegate to list for other messages
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func getFilterTitle(filter FilterType) string {
	switch filter {
	case FilterTmux:
		return " tmux"
	case FilterConfig:
		return "⚙️ config"
	case FilterZoxide:
		return "📁 zoxide"
	default:
		return "⚡ Sesh Sessions"
	}
}
