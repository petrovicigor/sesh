package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// loadPreviewDebounced handles preview loading with debouncing
// Returns updated model and command to execute
func (m Model) loadPreviewDebounced(item sessionItem) (Model, tea.Cmd) {
	sessionName := item.session.Name

	// If this is the first selection, load immediately (no debounce)
	if m.lastPreviewKey == "" {
		m.previewPort.SetContent("") // Blank while loading
		m.lastPreviewKey = sessionName
		return m, loadPreview(m.previewer, item.session)
	}

	// Otherwise, debounce the preview load
	m.pendingPreview = sessionName
	m.previewPort.SetContent("") // Blank while waiting
	return m, debouncePreview(sessionName)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Split width: 45% for list, 55% for preview
		// Each box decoration: border(2) + padding(2) = 4 chars total
		listBoxWidth := (msg.Width * 45) / 100
		previewBoxWidth := msg.Width - listBoxWidth

		// Content width = box width - decoration (4 chars)
		listContentWidth := listBoxWidth - 4
		previewContentWidth := previewBoxWidth - 4

		m.list.SetSize(listContentWidth, msg.Height-4)
		m.previewPort.Width = previewContentWidth
		m.previewPort.Height = msg.Height - 4

		// Re-wrap existing preview content for new width
		if m.previewContent != "" {
			wrappedContent := lipgloss.NewStyle().Width(m.previewPort.Width).Render(m.previewContent)
			m.previewPort.SetContent(wrappedContent)
		}

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

		// Partition items so tmux sessions appear first
		items = partitionItemsByTmux(items)
		m.list.SetItems(items)

		// Update filter function with new items
		m.list.Filter = tmuxFirstFilter(items)

		// Update title based on current filter
		m.list.Title = getFilterTitle(m.currentFilter)

		// Check if we should preserve state (from session deletion)
		if msg.PreserveCursorIndex >= 0 {
			// PRESERVING STATE - from ctrl+d deletion
			logDebug("DEBUG: Preserving state - filter=%q, cursor=%d, items=%d", msg.PreserveFilterText, msg.PreserveCursorIndex, len(items))

			// Update lastFilter to prevent cursor reset on filter change detection
			m.lastFilter = msg.PreserveFilterText

			// Set cursor position (clamped to valid range)
			targetIndex := msg.PreserveCursorIndex
			if targetIndex >= len(items) {
				targetIndex = len(items) - 1
			}
			if targetIndex < 0 {
				targetIndex = 0
			}
			logDebug("DEBUG: Target index after clamp: %d", targetIndex)

			var cmds []tea.Cmd

			if msg.PreserveFilterText != "" {
				// Had a filter - restore it
				m.list.ResetFilter()
				m.list.SetFilterText(msg.PreserveFilterText)
				m.list.SetFilterState(list.Filtering)

				logDebug("DEBUG: After filtering, visible items: %d", len(m.list.VisibleItems()))

				// Clamp again based on filtered items
				visibleCount := len(m.list.VisibleItems())
				if targetIndex >= visibleCount {
					targetIndex = visibleCount - 1
				}
				if targetIndex < 0 {
					targetIndex = 0
				}
				logDebug("DEBUG: Will set cursor to %d for filtered list", targetIndex)

				// Set restoring state to prevent any cursor resets
				m.restoringState = true

				// Use sequence to set cursor and then complete restoration
				cmds = append(cmds, tea.Sequence(
					// Set cursor position
					func() tea.Msg {
						return setCursorMsg{index: targetIndex}
					},
					// Clear restoration flag
					func() tea.Msg {
						return RestorationCompleteMsg{}
					},
				))
				cmds = append(cmds, tea.ClearScreen)
			} else {
				// No filter - restore cursor position AFTER entering filter mode
				m.list.ResetFilter()

				logDebug("DEBUG: Will set cursor to %d for no-filter case", targetIndex)

				// Set restoring state flag to prevent cursor resets
				m.restoringState = true

				// Use sequence to ensure cursor is set AFTER filter mode is entered
				cmds = append(cmds, tea.Sequence(
					// First, enter filter mode
					func() tea.Msg {
						return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
					},
					// Then set cursor position
					func() tea.Msg {
						return setCursorMsg{index: targetIndex}
					},
					// Finally, clear restoration flag
					func() tea.Msg {
						return RestorationCompleteMsg{}
					},
				))
				cmds = append(cmds, tea.ClearScreen)
			}

			return m, tea.Batch(cmds...)
		}

		// NOT preserving - normal reload behavior
		// Reset list filter and cursor
		m.list.ResetFilter()
		if len(items) > 0 {
			m.list.Select(0)
		}

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
		// Wrap content to viewport width
		wrappedContent := lipgloss.NewStyle().Width(m.previewPort.Width).Render(msg.Content)
		m.previewPort.SetContent(wrappedContent)
		return m, nil

	case DebounceTickMsg:
		// Only load preview if this session is still pending
		// (user might have moved cursor away during debounce)
		if msg.SessionName != m.pendingPreview {
			return m, nil
		}

		// Find the session and load its preview
		for _, key := range m.sessions.OrderedIndex {
			session := m.sessions.Directory[key]
			if session.Name == msg.SessionName {
				m.lastPreviewKey = session.Name
				m.pendingPreview = ""
				return m, loadPreview(m.previewer, session)
			}
		}
		return m, nil

	case RestorationCompleteMsg:
		// Filter restoration complete, re-enable cursor reset on filter changes
		m.restoringState = false
		return m, nil

	case StartProcessDetectionMsg:
		return m, detectAllProcesses()

	case ProcessInfoMsg:
		// Update map in-place to preserve delegate's pointer reference
		// Clear existing entries
		for k := range m.processInfo {
			delete(m.processInfo, k)
		}
		// Copy new entries
		for k, v := range msg.Processes {
			m.processInfo[k] = v
		}
		return m, nil

	case setCursorMsg:
		// Set cursor position and load preview
		m.list.Select(msg.index)
		if item, ok := m.list.SelectedItem().(sessionItem); ok {
			m.previewPort.SetContent("")
			return m, loadPreview(m.previewer, item.session)
		}
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
					// Capture current state BEFORE killing
					filterText := m.list.FilterValue()
					cursorIndex := m.list.Index()

					// Move cursor up after deletion (go to previous item)
					newCursorIndex := cursorIndex - 1
					if newCursorIndex < 0 {
						newCursorIndex = 0
					}

					_, err := m.tmux.KillSession(item.session.Name)
					if err == nil {
						// Reload sessions with state preservation
						return m, loadSessionsPreservingState(m.lister, m.currentFilter, filterText, newCursorIndex)
					}
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.GoToWorktreeRoot):
			// Jump to worktree root if current session is a worktree
			if item, ok := m.list.SelectedItem().(sessionItem); ok {
				sessionName := item.session.Name
				// Check if this is a worktree session (contains "/")
				if strings.Contains(sessionName, "/") {
					// Extract root name (everything before the first "/")
					rootName := strings.Split(sessionName, "/")[0]
					// Find the root session in the list
					items := m.list.Items()
					for i, listItem := range items {
						if rootItem, ok := listItem.(sessionItem); ok {
							if rootItem.session.Name == rootName {
								m.list.Select(i)
								// Load preview for the root session
								return m.loadPreviewDebounced(rootItem)
							}
						}
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
				// Load preview for newly selected session with cache/debounce
				if item, ok := m.list.SelectedItem().(sessionItem); ok {
					return m.loadPreviewDebounced(item)
				}
				return m, nil
			case "down":
				m.list.CursorDown()
				// Load preview for newly selected session with cache/debounce
				if item, ok := m.list.SelectedItem().(sessionItem); ok {
					return m.loadPreviewDebounced(item)
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

			// Skip cursor reset if we're restoring state
			if !m.restoringState {
				m.list.Select(0)
				// Load preview for top item with cache/debounce
				if item, ok := m.list.SelectedItem().(sessionItem); ok {
					newModel, previewCmd := m.loadPreviewDebounced(item)
					m = newModel
					return m, tea.Batch(cmd, previewCmd)
				}
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

// partitionItemsByTmux groups tmux sessions first, then all other sessions
// Preserves the fuzzy match ordering within each group
func partitionItemsByTmux(items []list.Item) []list.Item {
	if len(items) <= 1 {
		return items
	}

	tmuxItems := make([]list.Item, 0, len(items))
	otherItems := make([]list.Item, 0, len(items))

	for _, item := range items {
		if sessionItem, ok := item.(sessionItem); ok {
			if sessionItem.session.Src == "tmux" {
				tmuxItems = append(tmuxItems, item)
			} else {
				otherItems = append(otherItems, item)
			}
		}
	}

	// Concatenate: tmux first, then others
	result := make([]list.Item, 0, len(items))
	result = append(result, tmuxItems...)
	result = append(result, otherItems...)
	return result
}
