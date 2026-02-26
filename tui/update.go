package tui

import (
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshmedeski/sesh/v2/state"
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
	// Keep old preview visible until new one loads (no blank flash)
	m.pendingPreview = sessionName
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
	displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, *m.expandedGroup, m.workspacePrefixes)

	// Swap items in-place without leaving filter mode — avoids layout shift
	m.lastFilter = "" // Prevent filter transition from overwriting items
	m.list.SetItems(displayItems)
	m.list.Filter = seshFilter(displayItems, m.frecencyScores, m.workspacePrefixes)
	m.list.SetFilterText("")
	m.list.SetFilterState(list.Filtering)
	// SetFilterState doesn't call updatePagination(), but it changes titleView()
	// height (filter input vs title). Force recalculation to prevent 1-line jump.
	m.list.SetSize(m.list.Width(), m.list.Height())

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
				if strings.HasPrefix(v.session.Name, repoName+"/") || v.session.Name == repoName {
					targetIndex = i
					goto found
				}
			}
			// For active groups with no dormant: target first active item
			if group != nil && !foundBadgeOrHeader && strings.Contains(v.session.Name, "/") {
				if strings.HasPrefix(v.session.Name, repoName+"/") && !group.tmuxNames[v.session.Name] {
					targetIndex = i
					goto found
				}
			}
		}
	}
found:
	logDebug("DEBUG expandGroup: repoName=%s targetIndex=%d totalItems=%d", repoName, targetIndex, len(displayItems))
	m.list.Select(targetIndex)

	// Load preview for target item
	if targetIndex < len(displayItems) {
		if item, ok := displayItems[targetIndex].(sessionItem); ok {
			m.previewPort.SetContent("")
			return m, loadPreview(m.previewer, item.session)
		}
	}
	return m, nil
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
	displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, "", m.workspacePrefixes)

	// Swap items in-place without leaving filter mode — avoids layout shift
	m.lastFilter = "" // Prevent filter transition from overwriting items
	m.list.SetItems(displayItems)
	m.list.Filter = seshFilter(displayItems, m.frecencyScores, m.workspacePrefixes)
	m.list.SetFilterText("")
	m.list.SetFilterState(list.Filtering)
	// SetFilterState doesn't call updatePagination(), but it changes titleView()
	// height (filter input vs title). Force recalculation to prevent 1-line jump.
	m.list.SetSize(m.list.Width(), m.list.Height())

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
	m.list.Select(targetIndex)

	// Load preview for target item
	if targetIndex < len(displayItems) {
		if item, ok := displayItems[targetIndex].(sessionItem); ok {
			return m, loadPreview(m.previewer, item.session)
		}
	}
	return m, nil
}

// enterWorkspaceManager enters workspace manager mode, showing toggle checkboxes.
func (m Model) enterWorkspaceManager() (Model, tea.Cmd) {
	// No-op if no workspace configs
	if len(m.config.WorkspaceConfigs) == 0 {
		return m, nil
	}

	// Discover all sub-projects (config excludes applied, NOT state excludes)
	subProjects := m.lister.ListWorkspaceSubProjects()
	if len(subProjects) == 0 {
		return m, nil
	}

	// Load current state excludes
	excludes, _ := state.LoadExcludes(m.excludesPath)

	m.workspaceManagerMode = true
	m.workspaceSubProjects = subProjects
	m.workspaceExcludes = excludes

	// Build toggle items
	toggleItems := buildWorkspaceToggleItems(subProjects, excludes)

	// Swap list items
	m.lastFilter = ""
	m.list.SetItems(toggleItems)
	m.list.Filter = list.DefaultFilter
	m.list.SetFilterText("")
	m.list.SetFilterState(list.Filtering)
	m.list.SetSize(m.list.Width(), m.list.Height())
	m.list.Title = "📦 Workspace Manager"
	m.list.Select(0)

	// Clear preview
	m.previewPort.SetContent("")
	m.previewContent = ""

	return m, nil
}

// exitWorkspaceManager exits workspace manager mode, saves excludes, and reloads sessions.
func (m Model) exitWorkspaceManager() (Model, tea.Cmd) {
	m.workspaceManagerMode = false
	m.workspaceSubProjects = nil

	// Save excludes and reload sessions
	saveCmd := saveExcludes(m.excludesPath, m.workspaceExcludes)
	reloadCmd := loadSessionsWithFilter(m.lister, m.currentFilter)

	// Restore title
	m.list.Title = getFilterTitle(m.currentFilter)

	return m, tea.Batch(saveCmd, reloadCmd)
}

// toggleWorkspaceItem toggles the excluded state of the selected workspace item.
func (m Model) toggleWorkspaceItem() (Model, tea.Cmd) {
	item, ok := m.list.SelectedItem().(workspaceToggleItem)
	if !ok {
		return m, nil
	}

	cursorIndex := m.list.Index()

	// Deep-copy the excludes map to preserve Elm architecture value semantics.
	// Maps are reference types, so mutating in-place would affect the original model.
	copied := make(map[string][]string, len(m.workspaceExcludes))
	for k, v := range m.workspaceExcludes {
		copied[k] = append([]string(nil), v...)
	}
	m.workspaceExcludes = copied

	// Toggle excluded state
	if item.excluded {
		// Remove from excludes
		if excludes, exists := m.workspaceExcludes[item.workspaceName]; exists {
			filtered := make([]string, 0, len(excludes))
			for _, sp := range excludes {
				if sp != item.subProject {
					filtered = append(filtered, sp)
				}
			}
			if len(filtered) == 0 {
				delete(m.workspaceExcludes, item.workspaceName)
			} else {
				m.workspaceExcludes[item.workspaceName] = filtered
			}
		}
	} else {
		// Add to excludes
		if m.workspaceExcludes == nil {
			m.workspaceExcludes = make(map[string][]string)
		}
		m.workspaceExcludes[item.workspaceName] = append(m.workspaceExcludes[item.workspaceName], item.subProject)
	}

	// Rebuild toggle items
	toggleItems := buildWorkspaceToggleItems(m.workspaceSubProjects, m.workspaceExcludes)
	filterText := m.list.FilterValue()

	m.lastFilter = filterText
	m.list.SetItems(toggleItems)
	m.list.Filter = list.DefaultFilter
	m.list.SetFilterText(filterText)
	m.list.SetFilterState(list.Filtering)
	m.list.SetSize(m.list.Width(), m.list.Height())

	// Restore cursor position
	if cursorIndex >= len(m.list.VisibleItems()) {
		cursorIndex = len(m.list.VisibleItems()) - 1
	}
	if cursorIndex < 0 {
		cursorIndex = 0
	}
	m.list.Select(cursorIndex)

	return m, nil
}

// buildWorkspaceToggleItems creates toggle items from discovered sub-projects and excludes.
func buildWorkspaceToggleItems(subProjects map[string][]string, excludes map[string][]string) []list.Item {
	items := make([]list.Item, 0)

	// Build exclude lookup
	excludeSet := make(map[string]map[string]bool)
	for wsName, paths := range excludes {
		excludeSet[wsName] = make(map[string]bool, len(paths))
		for _, p := range paths {
			excludeSet[wsName][p] = true
		}
	}

	// Sort workspace names for stable ordering
	wsNames := make([]string, 0, len(subProjects))
	for name := range subProjects {
		wsNames = append(wsNames, name)
	}
	slices.Sort(wsNames)

	for _, wsName := range wsNames {
		sps := subProjects[wsName]
		for _, sp := range sps {
			excluded := false
			if ws, ok := excludeSet[wsName]; ok {
				excluded = ws[sp]
			}
			items = append(items, workspaceToggleItem{
				workspaceName: wsName,
				subProject:    sp,
				excluded:      excluded,
			})
		}
	}

	return items
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
		m.worktreeGroups = buildWorktreeGroups(items, *m.worktreeDefaults, m.workspacePrefixes)
		*m.expandedGroup = ""

		// Use collapsed display items
		displayItems := buildDisplayItems(items, m.worktreeGroups, "", m.workspacePrefixes)
		m.list.SetItems(displayItems)

		// Update filter function with display items
		m.list.Filter = seshFilter(displayItems, m.frecencyScores, m.workspacePrefixes)

		// Update title based on current filter
		m.list.Title = getFilterTitle(m.currentFilter)

		// Check if we should preserve state (from session deletion)
		if msg.PreserveCursorIndex >= 0 {
			// PRESERVING STATE - from ctrl+d deletion
			logDebug("DEBUG: Preserving state - filter=%q, cursor=%d, items=%d", msg.PreserveFilterText, msg.PreserveCursorIndex, len(items))

			// Swap items in-place without leaving filter mode
			m.lastFilter = msg.PreserveFilterText
			m.list.SetFilterText(msg.PreserveFilterText)
			m.list.SetFilterState(list.Filtering)
			m.list.SetSize(m.list.Width(), m.list.Height())

			// Clamp cursor to valid range
			targetIndex := msg.PreserveCursorIndex
			visibleCount := len(m.list.VisibleItems())
			if msg.PreserveFilterText != "" && visibleCount > 0 {
				if targetIndex >= visibleCount {
					targetIndex = visibleCount - 1
				}
			} else {
				if targetIndex >= len(displayItems) {
					targetIndex = len(displayItems) - 1
				}
			}
			if targetIndex < 0 {
				targetIndex = 0
			}
			m.list.Select(targetIndex)

			// Load preview for selected item
			var previewCmd tea.Cmd
			if item, ok := m.list.SelectedItem().(sessionItem); ok {
				m.previewPort.SetContent("")
				previewCmd = loadPreview(m.previewer, item.session)
			}
			return m, previewCmd
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

	case ExcludesSavedMsg:
		if msg.Err != nil {
			logDebug("DEBUG: Failed to save excludes: %v", msg.Err)
		}
		return m, nil

	case tea.KeyMsg:
		// Handle Ctrl+E for process detection
		if key.Matches(msg, m.keys.DetectProcesses) {
			logDebug("DEBUG: Ctrl+E pressed, triggering process detection")
			return m, detectAllProcesses()
		}

		// Handle Ctrl+W for workspace manager
		if key.Matches(msg, m.keys.WorkspaceManager) {
			if m.workspaceManagerMode {
				return m.exitWorkspaceManager()
			}
			return m.enterWorkspaceManager()
		}

		// Handle workspace manager mode keys
		if m.workspaceManagerMode {
			switch {
			case key.Matches(msg, m.keys.Quit):
				// Escape exits manager mode; ctrl+c/ctrl+b quit entirely
				if msg.Type == tea.KeyEscape {
					return m.exitWorkspaceManager()
				}
				return m, tea.Quit
			case key.Matches(msg, m.keys.Select) || msg.String() == " ":
				return m.toggleWorkspaceItem()
			}
			// Fall through to normal key handling (up/down, typing)
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
					groupKey := groupKeyForItem(item.session.Name, item.session.Src, m.workspacePrefixes)
					if _, isGrouped := m.worktreeGroups[groupKey]; isGrouped {
						targetGroup = groupKey
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
					groupKey := groupKeyForItem(item.session.Name, item.session.Src, m.workspacePrefixes)
					if _, isGrouped := m.worktreeGroups[groupKey]; isGrouped {
						repoName = groupKey
						branchName = item.session.Name[len(groupKey)+1:]
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

			// Rebuild groups with updated defaults, preserve expanded state
			cursorIndex := m.list.Index()
			m.worktreeGroups = buildWorktreeGroups(m.allItems, *m.worktreeDefaults, m.workspacePrefixes)
			displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, *m.expandedGroup, m.workspacePrefixes)

			// Swap items in-place without leaving filter mode
			m.lastFilter = ""
			m.list.SetItems(displayItems)
			m.list.Filter = seshFilter(displayItems, m.frecencyScores, m.workspacePrefixes)
			m.list.SetFilterText("")
			m.list.SetFilterState(list.Filtering)
			m.list.SetSize(m.list.Width(), m.list.Height())

			// Restore cursor position (clamped to new item count)
			if cursorIndex >= len(displayItems) {
				cursorIndex = len(displayItems) - 1
			}
			m.list.Select(cursorIndex)

			return m, saveDefaults(m.defaultsPath, *m.worktreeDefaults)

		case key.Matches(msg, m.keys.RepoFocus):
			// Determine repo name from selected item
			var repoName string
			switch item := m.list.SelectedItem().(type) {
			case sessionItem:
				if strings.Contains(item.session.Name, "/") {
					repoName = groupKeyForItem(item.session.Name, item.session.Src, m.workspacePrefixes)
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
			var displayItems []list.Item
			if m.repoFocusFilter == repoName {
				// Clear focus — restore normal view
				m.repoFocusFilter = ""
				*m.expandedGroup = ""
				displayItems = buildDisplayItems(m.allItems, m.worktreeGroups, "", m.workspacePrefixes)

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

				displayItems = make([]list.Item, 0)
				for _, item := range m.allItems {
					if si, ok := item.(sessionItem); ok {
						if strings.HasPrefix(si.session.Name, repoName+"/") || si.session.Name == repoName {
							displayItems = append(displayItems, item)
						}
					}
				}

				targetIndex = 0
				m.list.Title = "🔍 " + repoName
			}

			// Swap items in-place without leaving filter mode — avoids layout shift
			m.lastFilter = "" // Prevent filter transition from overwriting items
			m.list.SetItems(displayItems)
			m.list.Filter = seshFilter(displayItems, m.frecencyScores, m.workspacePrefixes)
			m.list.SetFilterText("")
			m.list.SetFilterState(list.Filtering)
			// SetFilterState doesn't call updatePagination(), but it changes titleView()
			// height (filter input vs title). Force recalculation to prevent 1-line jump.
			m.list.SetSize(m.list.Width(), m.list.Height())

			m.list.Select(targetIndex)

			// Load preview for target item
			if targetIndex < len(displayItems) {
				if item, ok := displayItems[targetIndex].(sessionItem); ok {
					return m, loadPreview(m.previewer, item.session)
				}
			}
			m.previewPort.SetContent("")
			m.previewContent = ""
			return m, nil
		}

		// When filtering, intercept arrow keys and handle them ourselves
		// This prevents the list from exiting filter mode
		if m.list.FilterState() == list.Filtering {
			switch msg.String() {
			case "up":
				m.list.CursorUp()
				// Skip separator items
				if _, ok := m.list.SelectedItem().(separatorItem); ok {
					m.list.CursorUp()
				}
				// Load preview for newly selected session or group representative
				switch item := m.list.SelectedItem().(type) {
				case sessionItem:
					return m.loadPreviewDebounced(item)
				case worktreeGroupItem:
					if rep, ok := representativeSession(item); ok {
						return m.loadPreviewDebounced(rep)
					}
				}
				return m, nil
			case "down":
				m.list.CursorDown()
				// Skip separator items
				if _, ok := m.list.SelectedItem().(separatorItem); ok {
					m.list.CursorDown()
				}
				// Load preview for newly selected session or group representative
				switch item := m.list.SelectedItem().(type) {
				case sessionItem:
					return m.loadPreviewDebounced(item)
				case worktreeGroupItem:
					if rep, ok := representativeSession(item); ok {
						return m.loadPreviewDebounced(rep)
					}
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
			logDebug("DEBUG filterChange: prev=%q curr=%q", prevFilter, currentFilter)

			// Transition: empty → non-empty (start typing)
			if prevFilter == "" && currentFilter != "" {
				// Swap to full items for fuzzy search
				*m.expandedGroup = ""
				m.list.Filter = seshFilter(m.allItems, m.frecencyScores, m.workspacePrefixes)
				m.list.SetItems(m.allItems)
				// Re-apply filter text after item swap
				m.list.SetFilterText(currentFilter)
				m.list.SetFilterState(list.Filtering)
			}

			// Transition: non-empty → empty (cleared filter)
			// Must return early to avoid the stale async filter command from
			// m.list.Update(msg) overriding our grouped displayItems.
			if prevFilter != "" && currentFilter == "" {
				displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, "", m.workspacePrefixes)
				m.list.SetItems(displayItems)
				m.list.Filter = seshFilter(displayItems, m.frecencyScores, m.workspacePrefixes)
				m.list.SetFilterText("")
				m.list.SetFilterState(list.Filtering)
				m.list.SetSize(m.list.Width(), m.list.Height())

				m.list.Select(0)
				switch item := m.list.SelectedItem().(type) {
				case sessionItem:
					return m.loadPreviewDebounced(item)
				case worktreeGroupItem:
					if rep, ok := representativeSession(item); ok {
						return m.loadPreviewDebounced(rep)
					}
				}
				return m, nil
			}

			m.list.Select(0)
			// Load preview for top item or group representative
			switch item := m.list.SelectedItem().(type) {
			case sessionItem:
				newModel, previewCmd := m.loadPreviewDebounced(item)
				m = newModel
				return m, tea.Batch(cmd, previewCmd)
			case worktreeGroupItem:
				if rep, ok := representativeSession(item); ok {
					newModel, previewCmd := m.loadPreviewDebounced(rep)
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
