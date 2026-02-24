package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
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

// expandGroup expands a worktree group, showing its dormant worktrees.
// Returns updated model and commands to re-enter filter mode with preview.
func (m Model) expandGroup(repoName string) (Model, tea.Cmd) {
	// When all items are already visible (no dormant worktrees),
	// just toggle the star indicator — no item rebuild or cursor change needed.
	group := m.worktreeGroups[repoName]
	if group != nil {
		unique := deduplicateWorktrees(group)
		dormant := dormantWorktrees(unique, group.tmuxNames)
		if len(dormant) == 0 {
			*m.expandedGroup = repoName
			return m, nil
		}
	}

	*m.expandedGroup = repoName
	displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, *m.expandedGroup)

	m.list.ResetFilter()
	m.lastFilter = "" // Prevent filter transition from overwriting items
	m.list.SetItems(displayItems)
	m.list.Filter = tmuxFirstFilter(displayItems)

	// Find cursor target: first newly-revealed item from this group.
	// For active groups: first dormant (non-tmux) child after the badge carrier.
	// For dormant-only groups: first child after the group header.
	targetIndex := 0
	foundBadgeOrHeader := false
	for i, listItem := range displayItems {
		switch v := listItem.(type) {
		case worktreeGroupItem:
			if v.repoName == repoName {
				foundBadgeOrHeader = true
			}
		case sessionItem:
			if v.groupRepo == repoName {
				// This is the badge carrier — dormant items follow it
				foundBadgeOrHeader = true
				continue
			}
			if foundBadgeOrHeader {
				// Match worktree items (repo/branch) or bare repo items (exact name match)
				if strings.Contains(v.session.Name, "/") {
					itemRepo := strings.SplitN(v.session.Name, "/", 2)[0]
					if itemRepo == repoName {
						targetIndex = i
						goto found
					}
				} else if v.session.Name == repoName {
					targetIndex = i
					goto found
				}
			}
			// For active groups with no dormant: target first active item
			if group != nil && !foundBadgeOrHeader && strings.Contains(v.session.Name, "/") {
				itemRepo := strings.SplitN(v.session.Name, "/", 2)[0]
				if itemRepo == repoName && !group.tmuxNames[v.session.Name] {
					targetIndex = i
					goto found
				}
			}
		}
	}
found:
	logDebug("DEBUG expandGroup: repoName=%s targetIndex=%d totalItems=%d", repoName, targetIndex, len(displayItems))

	// Enter filter mode synchronously (no message round-trip, no jitter)
	m.list, _ = m.list.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m.list.Select(targetIndex)

	// Force clean redraw (item count changed, prevents terminal misalignment)
	cmds := []tea.Cmd{tea.ClearScreen}

	// Load preview for target item
	if targetIndex < len(displayItems) {
		if item, ok := displayItems[targetIndex].(sessionItem); ok {
			m.previewPort.SetContent("")
			cmds = append(cmds, loadPreview(m.previewer, item.session))
		}
	}
	return m, tea.Batch(cmds...)
}

// collapseGroup collapses any expanded group back to its single-line form.
// Returns updated model and command to re-enter filter mode.
func (m Model) collapseGroup() (Model, tea.Cmd) {
	groupRepo := *m.expandedGroup

	// When all items were already visible (star-only expand), just toggle off.
	group := m.worktreeGroups[groupRepo]
	if group != nil {
		unique := deduplicateWorktrees(group)
		dormant := dormantWorktrees(unique, group.tmuxNames)
		if len(dormant) == 0 {
			*m.expandedGroup = ""
			return m, nil
		}
	}

	*m.expandedGroup = ""
	displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, "")

	m.list.ResetFilter()
	m.lastFilter = "" // Prevent filter transition from overwriting items
	m.list.SetItems(displayItems)
	m.list.Filter = tmuxFirstFilter(displayItems)

	// Find the group item or badged session to position cursor on
	targetIndex := 0
	for i, item := range displayItems {
		if gi, ok := item.(worktreeGroupItem); ok && gi.repoName == groupRepo {
			targetIndex = i
			break
		}
		if si, ok := item.(sessionItem); ok && si.groupRepo == groupRepo {
			targetIndex = i
			break
		}
	}
	m.previewPort.SetContent("")
	m.previewContent = ""

	// Enter filter mode synchronously (no message round-trip, no jitter)
	m.list, _ = m.list.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m.list.Select(targetIndex)

	// Force clean redraw (item count changed, prevents terminal misalignment)
	cmds := []tea.Cmd{tea.ClearScreen}

	// Load preview for target item
	if targetIndex < len(displayItems) {
		if item, ok := displayItems[targetIndex].(sessionItem); ok {
			cmds = append(cmds, loadPreview(m.previewer, item.session))
		}
	}
	return m, tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		logDebug("WindowSizeMsg: %dx%d", msg.Width, msg.Height)
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

		// Re-set preview content (viewport handles width/truncation)
		if m.previewContent != "" {
			m.previewPort.SetContent(m.previewContent)
		}

		return m, nil

	case SessionsLoadedMsg:
		// Build new list items from loaded sessions
		items := make([]list.Item, 0, len(msg.Sessions.OrderedIndex))
		if msg.Sessions.Directory != nil && msg.Sessions.OrderedIndex != nil {
			for _, key := range msg.Sessions.OrderedIndex {
				if session, ok := msg.Sessions.Directory[key]; ok {
					displayName := m.icon.AddIcon(session)
					items = append(items, sessionItem{
						session:     session,
						displayName: displayName,
						iconPrefix:  extractIconPrefix(displayName, session.Name),
					})
				}
			}
		}
		m.sessions = msg.Sessions

		// Partition items so tmux sessions appear first
		items = partitionItemsByTmux(items)

		// Build worktree groups
		m.allItems = items
		m.worktreeGroups = buildWorktreeGroups(items, *m.worktreeDefaults)
		*m.expandedGroup = ""

		// Use collapsed display items
		displayItems := buildDisplayItems(items, m.worktreeGroups, "")
		m.list.SetItems(displayItems)

		// Update filter function with display items
		m.list.Filter = tmuxFirstFilter(displayItems)

		// Update title based on current filter
		m.list.Title = getFilterTitle(m.currentFilter)

		// Check if we should preserve state (from session deletion)
		if msg.PreserveCursorIndex >= 0 {
			// PRESERVING STATE - from ctrl+d deletion
			logDebug("DEBUG: Preserving state - filter=%q, cursor=%d, items=%d", msg.PreserveFilterText, msg.PreserveCursorIndex, len(items))

			// Update lastFilter to prevent cursor reset on filter change detection
			m.lastFilter = msg.PreserveFilterText

			// Set cursor position (clamped to valid range of display items)
			targetIndex := msg.PreserveCursorIndex
			if targetIndex >= len(displayItems) {
				targetIndex = len(displayItems) - 1
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
		logDebug("PreviewLoadedMsg received (%d bytes)", len(msg.Content))
		m.previewContent = msg.Content
		// Let the viewport handle width/truncation directly —
		// no pre-wrapping with lipgloss.Width() which was double-processing
		// and amplifying width miscalculations for ANSI/emoji content.
		m.previewPort.SetContent(msg.Content)
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

	case ProcessInfoMsg:
		logDebug("DEBUG: ProcessInfoMsg received with %d processes", len(msg.Processes))
		// Update map in-place to preserve delegate's pointer reference
		// Clear existing entries
		for k := range *m.processInfo {
			delete(*m.processInfo, k)
		}
		// Copy new entries
		for k, v := range msg.Processes {
			(*m.processInfo)[k] = v
			logDebug("DEBUG: Process detected - session: %s, process: %s", k, v)
		}

		// Process detection complete
		logDebug("DEBUG: Process detection complete, %d processes found", len(msg.Processes))
		return m, nil

	case DefaultsSavedMsg:
		if msg.Err != nil {
			logDebug("DEBUG: Failed to save defaults: %v", msg.Err)
		}
		return m, nil

	case setCursorMsg:
		// Set cursor position and load preview
		logDebug("DEBUG setCursorMsg: index=%d currentIndex=%d totalItems=%d", msg.index, m.list.Index(), len(m.list.Items()))
		m.list.Select(msg.index)
		logDebug("DEBUG setCursorMsg: after Select, index=%d", m.list.Index())
		switch item := m.list.SelectedItem().(type) {
		case sessionItem:
			m.previewPort.SetContent("")
			return m, loadPreview(m.previewer, item.session)
		case worktreeGroupItem:
			m.previewPort.SetContent("")
			m.previewContent = ""
		}
		return m, nil

	case tea.KeyMsg:
		// Handle Ctrl+E for process detection
		if key.Matches(msg, m.keys.DetectProcesses) {
			logDebug("DEBUG: Ctrl+E pressed, triggering process detection")
			return m, detectAllProcesses()
		}

		// Handle Tab for group expand/collapse
		if key.Matches(msg, m.keys.ExpandGroup) {
			// Determine which group the cursor is on (if any)
			var targetGroup string
			switch item := m.list.SelectedItem().(type) {
			case worktreeGroupItem:
				targetGroup = item.repoName
			case sessionItem:
				if item.groupRepo != "" {
					targetGroup = item.groupRepo
				} else if strings.Contains(item.session.Name, "/") {
					repo := strings.SplitN(item.session.Name, "/", 2)[0]
					if _, isGrouped := m.worktreeGroups[repo]; isGrouped {
						targetGroup = repo
					}
				} else if _, isGrouped := m.worktreeGroups[item.session.Name]; isGrouped {
					targetGroup = item.session.Name
				}
			}

			if targetGroup == "" {
				// Not on a group item — collapse if expanded, otherwise no-op
				if *m.expandedGroup != "" {
					return m.collapseGroup()
				}
				return m, nil
			}

			if *m.expandedGroup == targetGroup {
				// Same group — toggle collapse
				return m.collapseGroup()
			}

			// Different group (or none expanded) — collapse old, expand new
			if *m.expandedGroup != "" {
				*m.expandedGroup = ""
			}
			return m.expandGroup(targetGroup)
		}

		// Handle quit/select/filter keys first
		switch {
		case key.Matches(msg, m.keys.Quit):
			// Only Escape collapses expanded group; ctrl+c/ctrl+b always quit
			if *m.expandedGroup != "" && msg.Type == tea.KeyEscape {
				return m.collapseGroup()
			}
			return m, tea.Quit

		case key.Matches(msg, m.keys.Select):
			switch item := m.list.SelectedItem().(type) {
			case sessionItem:
				m.selected = item.session.Name
				return m, tea.Quit
			case worktreeGroupItem:
				if item.defaultBranch != "" {
					// Has default — connect to default worktree directly
					m.selected = item.repoName + "/" + item.defaultBranch
					return m, tea.Quit
				}

				// No default — expand the group
				return m.expandGroup(item.repoName)
			}

		case key.Matches(msg, m.keys.FilterAll):
			m.currentFilter = FilterAll
			m.lastFilter = "" // Reset filter tracking
			return m, loadSessionsWithFilter(m.lister, FilterAll)

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
					cursorIndex := m.list.Index()
					// Move to previous item after deletion
					if cursorIndex > 0 {
						cursorIndex--
					}
					filterText := m.list.FilterValue()
					logDebug("DEBUG ctrl+d: killing session=%s targetIndex=%d filterText=%q expandedGroup=%q currentFilter=%d", item.session.Name, cursorIndex, filterText, *m.expandedGroup, m.currentFilter)
					_, err := m.tmux.KillSession(item.session.Name)
					if err == nil {
						*m.expandedGroup = ""
						logDebug("DEBUG ctrl+d: killed ok, reloading with preserved state")
						return m, loadSessionsPreservingState(m.lister, m.currentFilter, filterText, cursorIndex)
					}
					logDebug("DEBUG ctrl+d: kill error: %v", err)
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

		case key.Matches(msg, m.keys.SetDefault):
			// Set/clear default worktree for a repo
			var repoName, branchName string
			switch item := m.list.SelectedItem().(type) {
			case sessionItem:
				if strings.Contains(item.session.Name, "/") {
					parts := strings.SplitN(item.session.Name, "/", 2)
					repo := parts[0]
					if _, isGrouped := m.worktreeGroups[repo]; isGrouped {
						repoName = repo
						branchName = parts[1]
					}
				} else if _, isGrouped := m.worktreeGroups[item.session.Name]; isGrouped {
					// Bare repo item in a worktree group — clear default
					repoName = item.session.Name
					branchName = ""
				}
			case worktreeGroupItem:
				// Group header — clear default
				repoName = item.repoName
				branchName = ""
			}

			if repoName == "" {
				return m, nil
			}

			// Toggle: if already default, clear it; bare repo always clears
			if branchName == "" || (*m.worktreeDefaults)[repoName] == branchName {
				delete(*m.worktreeDefaults, repoName)
			} else {
				(*m.worktreeDefaults)[repoName] = branchName
			}

			// Rebuild groups with updated defaults
			m.worktreeGroups = buildWorktreeGroups(m.allItems, *m.worktreeDefaults)
			*m.expandedGroup = ""
			displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, "")

			m.list.ResetFilter()
			m.lastFilter = "" // Prevent filter transition from overwriting items
			m.list.SetItems(displayItems)
			m.list.Filter = tmuxFirstFilter(displayItems)

			// Find the group item to position cursor on
			targetIndex := 0
			for i, listItem := range displayItems {
				if gi, ok := listItem.(worktreeGroupItem); ok && gi.repoName == repoName {
					targetIndex = i
					break
				}
			}
			m.previewPort.SetContent("")
			m.previewContent = ""

			// Sequence: enter filter mode, then set cursor + async save
			seqCmd := tea.Sequence(
				func() tea.Msg {
					return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
				},
				func() tea.Msg {
					return setCursorMsg{index: targetIndex}
				},
			)
			saveCmd := saveDefaults(m.defaultsPath, *m.worktreeDefaults)
			return m, tea.Batch(seqCmd, saveCmd)

		case key.Matches(msg, m.keys.RepoFocus):
			// Determine repo name from selected item
			var repoName string
			switch item := m.list.SelectedItem().(type) {
			case sessionItem:
				if strings.Contains(item.session.Name, "/") {
					repoName = strings.SplitN(item.session.Name, "/", 2)[0]
				} else if _, isGrouped := m.worktreeGroups[item.session.Name]; isGrouped {
					repoName = item.session.Name
				}
			case worktreeGroupItem:
				repoName = item.repoName
			}

			if repoName == "" {
				return m, nil
			}

			// Toggle focus
			var targetIndex int
			if m.repoFocusFilter == repoName {
				// Clear focus — restore normal view
				m.repoFocusFilter = ""
				*m.expandedGroup = ""
				displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, "")

				m.list.ResetFilter()
				m.lastFilter = "" // Prevent filter transition from overwriting items
				m.list.SetItems(displayItems)
				m.list.Filter = tmuxFirstFilter(displayItems)

				for i, listItem := range displayItems {
					if gi, ok := listItem.(worktreeGroupItem); ok && gi.repoName == repoName {
						targetIndex = i
						break
					}
				}

				m.list.Title = getFilterTitle(m.currentFilter)
			} else {
				// Set focus — filter to only this repo's worktrees
				m.repoFocusFilter = repoName
				*m.expandedGroup = ""

				focusedItems := make([]list.Item, 0)
				for _, item := range m.allItems {
					if si, ok := item.(sessionItem); ok {
						if strings.Contains(si.session.Name, "/") {
							itemRepo := strings.SplitN(si.session.Name, "/", 2)[0]
							if itemRepo == repoName {
								focusedItems = append(focusedItems, item)
							}
						} else if si.session.Name == repoName {
							focusedItems = append(focusedItems, item)
						}
					}
				}

				m.list.ResetFilter()
				m.lastFilter = "" // Prevent filter transition from overwriting items
				m.list.SetItems(focusedItems)
				m.list.Filter = tmuxFirstFilter(focusedItems)
				targetIndex = 0

				m.list.Title = "🔍 " + repoName
			}

			m.previewPort.SetContent("")
			m.previewContent = ""

			return m, tea.Sequence(
				func() tea.Msg {
					return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
				},
				func() tea.Msg {
					return setCursorMsg{index: targetIndex}
				},
			)
		}

		// When filtering, intercept arrow keys and handle them ourselves
		// This prevents the list from exiting filter mode
		if m.list.FilterState() == list.Filtering {
			switch msg.String() {
			case "up":
				m.list.CursorUp()
				// Load preview for newly selected session, clear for group items
				switch item := m.list.SelectedItem().(type) {
				case sessionItem:
					return m.loadPreviewDebounced(item)
				case worktreeGroupItem:
					m.previewPort.SetContent("")
					m.previewContent = ""
				}
				return m, nil
			case "down":
				m.list.CursorDown()
				// Load preview for newly selected session, clear for group items
				switch item := m.list.SelectedItem().(type) {
				case sessionItem:
					return m.loadPreviewDebounced(item)
				case worktreeGroupItem:
					m.previewPort.SetContent("")
					m.previewContent = ""
				}
				return m, nil
			}
		}

		// For all other cases, delegate to list
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)

		// If filter text changed, handle item swapping and cursor reset
		currentFilter := m.list.FilterValue()
		if currentFilter != m.lastFilter {
			prevFilter := m.lastFilter
			m.lastFilter = currentFilter
			logDebug("DEBUG filterChange: prev=%q curr=%q restoringState=%v", prevFilter, currentFilter, m.restoringState)

			// Skip all this if we're restoring state
			if !m.restoringState {
				// Transition: empty → non-empty (start typing)
				if prevFilter == "" && currentFilter != "" {
					// Swap to full items for fuzzy search
					*m.expandedGroup = ""
					m.list.SetItems(m.allItems)
					m.list.Filter = tmuxFirstFilter(m.allItems)
					// Re-apply filter text after item swap
					m.list.SetFilterText(currentFilter)
					m.list.SetFilterState(list.Filtering)
				}

				// Transition: non-empty → empty (cleared filter)
				if prevFilter != "" && currentFilter == "" {
					// Swap back to collapsed display
					displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, "")
					m.list.SetItems(displayItems)
					m.list.Filter = tmuxFirstFilter(displayItems)
				}

				m.list.Select(0)
				// Load preview for top item, clear for group items
				switch item := m.list.SelectedItem().(type) {
				case sessionItem:
					newModel, previewCmd := m.loadPreviewDebounced(item)
					m = newModel
					return m, tea.Batch(cmd, previewCmd)
				case worktreeGroupItem:
					m.previewPort.SetContent("")
					m.previewContent = ""
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

	result := make([]list.Item, 0, len(items))
	otherItems := make([]list.Item, 0, len(items)/2)

	for _, item := range items {
		if sessionItem, ok := item.(sessionItem); ok {
			if sessionItem.session.Src == "tmux" {
				result = append(result, item)
			} else {
				otherItems = append(otherItems, item)
			}
		}
	}

	return append(result, otherItems...)
}
