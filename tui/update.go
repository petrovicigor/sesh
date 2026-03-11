package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/joshmedeski/sesh/v2/state"
)

// handleCursorMove loads preview for the current item immediately.
// Context cancellation kills any in-flight preview load — no debounce needed.
func (m Model) handleCursorMove() (Model, tea.Cmd) {
	switch item := m.list.SelectedItem().(type) {
	case sessionItem:
		return m.loadPreviewForItem(item)
	case worktreeGroupItem:
		if rep, ok := representativeSession(item); ok {
			return m.loadPreviewForItem(rep)
		}
	}
	return m, nil
}

// cancelInflightPreview cancels any in-flight preview load goroutine.
func (m *Model) cancelInflightPreview() {
	if m.previewCancel != nil {
		m.previewCancel()
		m.previewCancel = nil
	}
}

// loadPreviewForItem cancels any in-flight preview and starts a new load immediately.
// Context cancellation kills stale git subprocesses — no debounce needed.
// Old preview stays visible until the new one arrives (no blank flash).
func (m Model) loadPreviewForItem(item sessionItem) (Model, tea.Cmd) {
	m.cancelInflightPreview()

	if m.lastPreviewKey == "" {
		m.previewPort.SetContent("") // Blank on very first selection
	}

	m.lastPreviewKey = item.session.Name
	ctx, cancel := context.WithCancel(context.Background())
	m.previewCancel = cancel
	return m, loadPreview(ctx, m.previewer, item.session)
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
	displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, *m.expandedGroup, m.workspacePrefixes, m.groupMode)

	// Swap items in-place without leaving filter mode — avoids layout shift
	m.lastFilter = "" // Prevent filter transition from overwriting items
	m.list.SetItems(displayItems)
	m.list.Filter = seshFilter(displayItems, m.frecencyScores, m.workspacePrefixes, m.groupMode)
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
				// Match worktree items belonging to this group (works for both package-first and branch-first)
				itemKey := groupKeyForItem(v.session.Name, v.session.Src, m.workspacePrefixes, m.groupMode)
				if itemKey == repoName || v.session.Name == repoName {
					targetIndex = i
					goto found
				}
			}
			// For active groups with no dormant: target first active item
			if group != nil && !foundBadgeOrHeader && strings.Contains(v.session.Name, "/") {
				itemKey := groupKeyForItem(v.session.Name, v.session.Src, m.workspacePrefixes, m.groupMode)
				if itemKey == repoName && !group.tmuxNames[v.session.Name] {
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
			return m, loadPreview(context.Background(), m.previewer,item.session)
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
	displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, "", m.workspacePrefixes, m.groupMode)

	// Swap items in-place without leaving filter mode — avoids layout shift
	m.lastFilter = "" // Prevent filter transition from overwriting items
	m.list.SetItems(displayItems)
	m.list.Filter = seshFilter(displayItems, m.frecencyScores, m.workspacePrefixes, m.groupMode)
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
			return m, loadPreview(context.Background(), m.previewer,item.session)
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
	m.list.Title = getFilterTitle(m.currentFilter, m.groupMode)

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

// enterRestorePreview enters restore preview mode, showing saved state details in the preview pane.
func (m Model) enterRestorePreview(sessionName string) (Model, tea.Cmd) {
	m.restorePreviewMode = true
	m.deleteConfirmPending = false
	m.restorePreviewSession = sessionName
	content := generateRestorePreview(sessionName)
	m.previewContent = content
	m.previewPort.SetContent(content)
	m.previewPort.GotoTop()
	return m, nil
}

// exitRestorePreview exits restore preview mode and reloads the normal preview.
func (m Model) exitRestorePreview() (Model, tea.Cmd) {
	m.restorePreviewMode = false
	m.deleteConfirmPending = false
	m.restorePreviewSession = ""
	// Reload normal preview for currently selected item
	if item, ok := m.list.SelectedItem().(sessionItem); ok {
		m.previewPort.SetContent("")
		return m, loadPreview(context.Background(), m.previewer,item.session)
	}
	m.previewPort.SetContent("")
	m.previewContent = ""
	return m, nil
}

// tmuxSessionNames returns the names of all active tmux sessions from allItems.
func (m Model) tmuxSessionNames() []string {
	var names []string
	for _, item := range m.allItems {
		if si, ok := item.(sessionItem); ok && si.session.Src == "tmux" {
			names = append(names, si.session.Name)
		}
	}
	return names
}

// enterSavePreview enters save preview mode, showing save confirmation in the preview pane.
func (m Model) enterSavePreview(sessionName string) (Model, tea.Cmd) {
	m.savePreviewMode = true
	m.savePreviewSession = sessionName
	m.saveAllSessions = nil
	m.saveAllCompleted = nil
	tmuxNames := m.tmuxSessionNames()
	content := generateSavePreview(sessionName, tmuxNames)
	m.previewContent = content
	m.previewPort.SetContent(content)
	m.previewPort.GotoTop()
	return m, nil
}

// exitSavePreview exits save preview mode and reloads the normal preview.
func (m Model) exitSavePreview() (Model, tea.Cmd) {
	m.savePreviewMode = false
	m.savePreviewSession = ""
	m.saveAllSessions = nil
	m.saveAllCompleted = nil
	// Reload normal preview for currently selected item
	if item, ok := m.list.SelectedItem().(sessionItem); ok {
		m.previewPort.SetContent("")
		return m, loadPreview(context.Background(), m.previewer,item.session)
	}
	m.previewPort.SetContent("")
	m.previewContent = ""
	return m, nil
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
		m.previewPort.SetWidth(previewContentWidth)
		m.previewPort.SetHeight(msg.Height - 4)

		// Re-set preview content (viewport handles width/truncation)
		if m.previewContent != "" {
			m.previewPort.SetContent(m.previewContent)
		}

		return m, nil

	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		// Rebuild list styles with correct dark/light mode
		styles := list.DefaultStyles(m.isDark)
		styles.Title = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170")).
			MarginBottom(1).
			MarginLeft(2)
		m.list.Styles = styles
		return m, nil

	case enterFilterMsg:
		// Forward a '/' key press to the list so it enters filter mode
		// through its own internal handler (focuses input, shows all items).
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
		return m, cmd

	case SessionsLoadedMsg:
		// Build new list items from loaded sessions
		items := make([]list.Item, 0, len(msg.Sessions.OrderedIndex))
		if msg.Sessions.Directory != nil && msg.Sessions.OrderedIndex != nil {
			for _, key := range msg.Sessions.OrderedIndex {
				if session, ok := msg.Sessions.Directory[key]; ok {
					displayName := m.icon.AddIcon(session)
					items = append(items, sessionItem{
						session:       session,
						displayName:   displayName,
						iconPrefix:    extractIconPrefix(displayName, session.Name),
						sanitizedName: SanitizeSessionName(session.Name),
					})
				}
			}
		}
		m.sessions = msg.Sessions

		// Partition items so tmux sessions appear first
		items = partitionItemsByTmux(items)

		// Build worktree groups
		m.allItems = items
		m.worktreeGroups = buildWorktreeGroups(items, *m.worktreeDefaults, m.workspacePrefixes, m.groupMode)
		*m.expandedGroup = ""

		// Use collapsed display items
		displayItems := buildDisplayItems(items, m.worktreeGroups, "", m.workspacePrefixes, m.groupMode)
		m.list.SetItems(displayItems)

		// Update filter function with display items
		m.list.Filter = seshFilter(displayItems, m.frecencyScores, m.workspacePrefixes, m.groupMode)

		// Update title based on current filter
		m.list.Title = getFilterTitle(m.currentFilter, m.groupMode)

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
				previewCmd = loadPreview(context.Background(), m.previewer,item.session)
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
				previewCmd = loadPreview(context.Background(), m.previewer,firstItem.session)
			}
		} else {
			// No items - clear preview
			m.previewPort.SetContent("")
		}

		// Re-enable filter mode
		filterCmd := func() tea.Msg {
			return enterFilterMsg{}
		}

		return m, tea.Batch(previewCmd, filterCmd)

	case PreviewLoadedMsg:
		// Don't let async preview overwrite the restore confirmation screen
		if m.restorePreviewMode {
			return m, nil
		}
		logDebug("PreviewLoadedMsg received (%d bytes)", len(msg.Content))
		m.previewContent = msg.Content
		// Let the viewport handle width/truncation directly —
		// no pre-wrapping with lipgloss.Width() which was double-processing
		// and amplifying width miscalculations for ANSI/emoji content.
		m.previewPort.SetContent(msg.Content)
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

	case claudeAttentionTickMsg:
		// Periodic re-check: read @claude_icon from tmux windows and schedule next tick
		return m, tea.Batch(checkClaudeAttention(), scheduleClaudeAttentionTick())

	case ClaudeAttentionMsg:
		// Update map in-place to preserve delegate's pointer reference
		for k := range *m.claudeAttention {
			delete(*m.claudeAttention, k)
		}
		for k, v := range msg.Sessions {
			(*m.claudeAttention)[k] = v
		}
		return m, nil

	case SavedStateMsg:
		// Update map in-place to preserve delegate's pointer reference
		for k := range *m.savedState {
			delete(*m.savedState, k)
		}
		for k, v := range msg.Sessions {
			(*m.savedState)[k] = v
		}
		return m, nil

	case SessionKilledMsg:
		if msg.Err != nil {
			logDebug("DEBUG ctrl+d: kill error: %v", msg.Err)
			return m, nil
		}
		*m.expandedGroup = ""
		logDebug("DEBUG ctrl+d: killed ok, reloading with preserved state")
		return m, loadSessionsPreservingState(m.lister, m.currentFilter, m.pendingDeleteFilterText, m.pendingDeleteCursorIndex)

	case SavedStateDeletedMsg:
		if msg.Err != nil {
			logDebug("DEBUG: Failed to delete saved state for %s: %v", msg.SessionName, msg.Err)
		} else {
			delete(*m.savedState, msg.SessionName)
		}
		// Exit restore preview since the saved state is gone
		if m.restorePreviewMode {
			return m.exitRestorePreview()
		}
		return m, nil

	case SessionSavedMsg:
		if msg.Err != nil {
			logDebug("DEBUG: Failed to save session state for %s: %v", msg.SessionName, msg.Err)
		} else {
			(*m.savedState)[SanitizeSessionName(msg.SessionName)] = true
		}

		// Save-all mode: track progress and save next session
		if m.savePreviewMode && m.saveAllSessions != nil {
			m.saveAllCompleted = append(m.saveAllCompleted, msg.SessionName)
			content := generateSaveAllProgress(m.saveAllSessions, m.saveAllCompleted)
			m.previewContent = content
			m.previewPort.SetContent(content)
			m.previewPort.GotoTop()

			// If all done, show completion and exit after delay
			if len(m.saveAllCompleted) >= len(m.saveAllSessions) {
				m.statusMessage = fmt.Sprintf("💾 saved %d/%d sessions", len(m.saveAllCompleted), len(m.saveAllSessions))
				return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
					return SaveAllNextMsg{} // reuse to trigger exit
				})
			}

			// Save the next session
			nextIdx := len(m.saveAllCompleted)
			return m, saveSessionState(m.saveAllSessions[nextIdx])
		}

		// Single save mode: show result and exit save preview
		if m.savePreviewMode {
			if msg.Err != nil {
				m.statusMessage = "save failed: " + msg.Err.Error()
			} else {
				m.statusMessage = "💾 saved " + msg.SessionName
			}
			m.savePreviewMode = false
			m.savePreviewSession = ""
			// Reload normal preview
			if item, ok := m.list.SelectedItem().(sessionItem); ok {
				m.previewPort.SetContent("")
				return m, tea.Batch(
					loadPreview(context.Background(), m.previewer,item.session),
					tea.Tick(2*time.Second, func(time.Time) tea.Msg { return clearStatusMsg{} }),
				)
			}
			return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return clearStatusMsg{} })
		}

		// Not in save preview mode (shouldn't normally happen, but handle gracefully)
		if msg.Err != nil {
			m.statusMessage = "save failed: " + msg.Err.Error()
		} else {
			m.statusMessage = "💾 saved " + msg.SessionName
		}
		return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
			return clearStatusMsg{}
		})

	case SaveAllNextMsg:
		// Save-all completed — exit save preview and show status
		if m.savePreviewMode {
			m.savePreviewMode = false
			m.savePreviewSession = ""
			m.saveAllSessions = nil
			m.saveAllCompleted = nil
			if item, ok := m.list.SelectedItem().(sessionItem); ok {
				m.previewPort.SetContent("")
				return m, tea.Batch(
					loadPreview(context.Background(), m.previewer,item.session),
					tea.Tick(2*time.Second, func(time.Time) tea.Msg { return clearStatusMsg{} }),
				)
			}
			return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return clearStatusMsg{} })
		}
		return m, nil

	case clearStatusMsg:
		m.statusMessage = ""
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

	case tea.KeyPressMsg:
		// Restore preview mode is fully modal — only Enter, Esc, Ctrl+C, Ctrl+B handled
		if m.restorePreviewMode {
			switch {
			case key.Matches(msg, m.keys.Select): // Enter → confirm restore
				m.selected = m.restorePreviewSession
				m.restoreRequested = true
				return m, tea.Quit
			case msg.Code == tea.KeyBackspace: // Backspace → delete saved state
				if m.deleteConfirmPending {
					m.deleteConfirmPending = false
					return m, deleteSavedState(m.restorePreviewSession)
				}
				// First press — show confirmation in preview
				m.deleteConfirmPending = true
				content := generateDeleteConfirmPreview(m.restorePreviewSession)
				m.previewContent = content
				m.previewPort.SetContent(content)
				m.previewPort.GotoTop()
				return m, nil
			case msg.Code == tea.KeyEscape: // Esc → cancel
				m.deleteConfirmPending = false
				return m.exitRestorePreview()
			case msg.String() == "ctrl+c" || msg.String() == "ctrl+b": // hard quit
				return m, tea.Quit
			default:
				return m, nil // swallow all other keys
			}
		}

		// Save preview mode is fully modal — only Enter, Ctrl+A, Esc, Ctrl+C, Ctrl+B handled
		if m.savePreviewMode {
			// During save-all, swallow everything (progress is automatic)
			if m.saveAllSessions != nil {
				switch {
				case msg.String() == "ctrl+c" || msg.String() == "ctrl+b":
					return m, tea.Quit
				default:
					return m, nil
				}
			}

			switch {
			case key.Matches(msg, m.keys.Select): // Enter → save selected session
				m.statusMessage = "saving " + m.savePreviewSession + "..."
				return m, saveSessionState(m.savePreviewSession)
			case key.Matches(msg, m.keys.SaveAll): // Ctrl+A → save all tmux sessions
				tmuxNames := m.tmuxSessionNames()
				if len(tmuxNames) == 0 {
					return m.exitSavePreview()
				}
				m.saveAllSessions = tmuxNames
				m.saveAllCompleted = nil
				content := generateSaveAllProgress(tmuxNames, nil)
				m.previewContent = content
				m.previewPort.SetContent(content)
				m.previewPort.GotoTop()
				// Start saving the first session
				return m, saveSessionState(tmuxNames[0])
			case msg.Code == tea.KeyEscape: // Esc → cancel
				return m.exitSavePreview()
			case msg.String() == "ctrl+c" || msg.String() == "ctrl+b": // hard quit
				return m, tea.Quit
			default:
				return m, nil // swallow all other keys
			}
		}

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
				if msg.Code == tea.KeyEscape {
					return m.exitWorkspaceManager()
				}
				return m, tea.Quit
			case key.Matches(msg, m.keys.Select) || msg.String() == "space":
				return m.toggleWorkspaceItem()
			}
			// Fall through to normal key handling (up/down, typing)
		}

		// Handle Shift+Enter or Ctrl+R: show restore preview (any session with saved state)
		if key.Matches(msg, m.keys.RestoreConnect) || key.Matches(msg, m.keys.RestoreSession) {
			switch item := m.list.SelectedItem().(type) {
			case sessionItem:
				if (*m.savedState)[item.sanitizedName] {
					return m.enterRestorePreview(item.session.Name)
				}
			case worktreeGroupItem:
				// Check if any session in the group has saved state
				for _, wt := range item.worktrees {
					if (*m.savedState)[wt.sanitizedName] {
						return m.enterRestorePreview(wt.session.Name)
					}
				}
			}
			return m, nil
		}

		// Handle Ctrl+S: enter save preview mode for tmux sessions
		if key.Matches(msg, m.keys.SaveSession) {
			if item, ok := m.list.SelectedItem().(sessionItem); ok {
				if item.session.Src == "tmux" {
					return m.enterSavePreview(item.session.Name)
				}
				// Non-tmux session — show feedback
				m.statusMessage = "ctrl+s saves tmux sessions only"
				return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
					return clearStatusMsg{}
				})
			}
			return m, nil
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
					groupKey := groupKeyForItem(item.session.Name, item.session.Src, m.workspacePrefixes, m.groupMode)
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
			if *m.expandedGroup != "" && msg.Code == tea.KeyEscape {
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

		case key.Matches(msg, m.keys.ToggleGroupMode):
			// Toggle workspace grouping between package-first and branch-first.
			// Remember current session name for cursor restoration.
			// For group items, save a worktree session name (not the group key)
			// so it can be found in the new grouping mode.
			var prevSessionName string
			var prevIsWorkspace bool
			switch sel := m.list.SelectedItem().(type) {
			case sessionItem:
				prevSessionName = sel.session.Name
				prevIsWorkspace = sel.session.Src == "workspace" || isWorkspaceTmuxSession(sel.session.Name, m.workspacePrefixes)
			case worktreeGroupItem:
				if len(sel.worktrees) > 0 {
					prevSessionName = sel.worktrees[0].session.Name
					prevIsWorkspace = sel.worktrees[0].session.Src == "workspace" || isWorkspaceTmuxSession(sel.worktrees[0].session.Name, m.workspacePrefixes)
				}
			}

			if m.groupMode == GroupByPackage {
				m.groupMode = GroupByBranch
			} else {
				m.groupMode = GroupByPackage
			}
			m.repoFocusFilter = ""
			m.worktreeGroups = buildWorktreeGroups(m.allItems, *m.worktreeDefaults, m.workspacePrefixes, m.groupMode)

			// Auto-expand the workspace group the user was looking at so the
			// pivot shows the same sessions reorganized, not hidden behind a badge.
			autoExpandGroup := ""
			if prevIsWorkspace && prevSessionName != "" {
				newKey := groupKeyForItem(prevSessionName, "workspace", m.workspacePrefixes, m.groupMode)
				if _, exists := m.worktreeGroups[newKey]; exists {
					autoExpandGroup = newKey
				}
			}
			*m.expandedGroup = autoExpandGroup

			displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, autoExpandGroup, m.workspacePrefixes, m.groupMode)
			m.list.SetItems(displayItems)
			m.list.Filter = seshFilter(displayItems, m.frecencyScores, m.workspacePrefixes, m.groupMode)
			m.list.SetFilterText("")
			m.list.SetFilterState(list.Filtering)
			m.list.SetSize(m.list.Width(), m.list.Height())

			// Find the previous session in the (now expanded) display items.
			targetIndex := 0
			logDebug("DEBUG toggleGroup: prevSessionName=%q autoExpand=%q displayItems=%d mode=%d", prevSessionName, autoExpandGroup, len(displayItems), m.groupMode)
			if prevSessionName != "" {
				// Exact session name match (works well with auto-expanded groups)
				for i, listItem := range displayItems {
					switch v := listItem.(type) {
					case sessionItem:
						if v.session.Name == prevSessionName {
							targetIndex = i
							goto toggleFound
						}
					case worktreeGroupItem:
						if v.repoName == prevSessionName {
							targetIndex = i
							goto toggleFound
						}
						for _, wt := range v.worktrees {
							if wt.session.Name == prevSessionName {
								targetIndex = i
								goto toggleFound
							}
						}
					}
				}

				// Fallback: find any item from the same workspace prefix
				for _, prefix := range m.workspacePrefixes {
					if strings.HasPrefix(prevSessionName, prefix+"/") {
						for i, listItem := range displayItems {
							switch v := listItem.(type) {
							case sessionItem:
								if strings.HasPrefix(v.session.Name, prefix+"/") {
									targetIndex = i
									goto toggleFound
								}
							case worktreeGroupItem:
								if strings.HasPrefix(v.repoName, prefix+"/") || v.repoName == prefix {
									targetIndex = i
									goto toggleFound
								}
							}
						}
					}
				}
				logDebug("DEBUG toggleGroup: NO MATCH found, defaulting to 0")
			}
		toggleFound:
			logDebug("DEBUG toggleGroup: selecting index %d", targetIndex)
			m.list.Select(targetIndex)
			m.list.Title = getFilterTitle(m.currentFilter, m.groupMode)
			// Load preview for selected item
			switch item := m.list.SelectedItem().(type) {
			case sessionItem:
				return m.loadPreviewForItem(item)
			case worktreeGroupItem:
				if rep, ok := representativeSession(item); ok {
					return m.loadPreviewForItem(rep)
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.Delete):
			// Delete session if it's a tmux session
			if item, ok := m.list.SelectedItem().(sessionItem); ok {
				if item.session.Src == "tmux" {
					cursorIndex := m.list.Index()
					// Move to previous item after deletion
					if cursorIndex > 0 {
						cursorIndex--
					}
					m.pendingDeleteFilterText = m.list.FilterValue()
					m.pendingDeleteCursorIndex = cursorIndex
					logDebug("DEBUG ctrl+d: killing session=%s targetIndex=%d filterText=%q expandedGroup=%q currentFilter=%d", item.session.Name, cursorIndex, m.pendingDeleteFilterText, *m.expandedGroup, m.currentFilter)
					return m, killSessionWithCleanup(m.tmux, item.session.Name)
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
								return m.loadPreviewForItem(rootItem)
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
					groupKey := groupKeyForItem(item.session.Name, item.session.Src, m.workspacePrefixes, m.groupMode)
					if _, isGrouped := m.worktreeGroups[groupKey]; isGrouped {
						repoName = groupKey
						if strings.HasPrefix(item.session.Name, groupKey+"/") {
							branchName = item.session.Name[len(groupKey)+1:]
						} else {
							// Branch-first mode: last path segment is the branch
							lastSlash := strings.LastIndex(item.session.Name, "/")
							if lastSlash > 0 {
								branchName = item.session.Name[lastSlash+1:]
							}
						}
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
			m.worktreeGroups = buildWorktreeGroups(m.allItems, *m.worktreeDefaults, m.workspacePrefixes, m.groupMode)
			displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, *m.expandedGroup, m.workspacePrefixes, m.groupMode)

			// Swap items in-place without leaving filter mode
			m.lastFilter = ""
			m.list.SetItems(displayItems)
			m.list.Filter = seshFilter(displayItems, m.frecencyScores, m.workspacePrefixes, m.groupMode)
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
					repoName = groupKeyForItem(item.session.Name, item.session.Src, m.workspacePrefixes, m.groupMode)
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
				displayItems = buildDisplayItems(m.allItems, m.worktreeGroups, "", m.workspacePrefixes, m.groupMode)

				for i, listItem := range displayItems {
					if gi, ok := listItem.(worktreeGroupItem); ok && gi.repoName == repoName {
						targetIndex = i
						break
					}
				}

				m.list.Title = getFilterTitle(m.currentFilter, m.groupMode)
			} else {
				// Set focus — filter to only this repo's worktrees
				m.repoFocusFilter = repoName
				*m.expandedGroup = ""

				displayItems = make([]list.Item, 0)
				for _, item := range m.allItems {
					if si, ok := item.(sessionItem); ok {
						itemKey := groupKeyForItem(si.session.Name, si.session.Src, m.workspacePrefixes, m.groupMode)
						if itemKey == repoName || si.session.Name == repoName {
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
			m.list.Filter = seshFilter(displayItems, m.frecencyScores, m.workspacePrefixes, m.groupMode)
			m.list.SetFilterText("")
			m.list.SetFilterState(list.Filtering)
			// SetFilterState doesn't call updatePagination(), but it changes titleView()
			// height (filter input vs title). Force recalculation to prevent 1-line jump.
			m.list.SetSize(m.list.Width(), m.list.Height())

			m.list.Select(targetIndex)

			// Load preview for target item
			if targetIndex < len(displayItems) {
				if item, ok := displayItems[targetIndex].(sessionItem); ok {
					return m, loadPreview(context.Background(), m.previewer,item.session)
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
				return m.handleCursorMove()
			case "down":
				m.list.CursorDown()
				// Skip separator items
				if _, ok := m.list.SelectedItem().(separatorItem); ok {
					m.list.CursorDown()
				}
				return m.handleCursorMove()
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
				m.list.Filter = seshFilter(m.allItems, m.frecencyScores, m.workspacePrefixes, m.groupMode)
				m.list.SetItems(m.allItems)
				// Re-apply filter text after item swap
				m.list.SetFilterText(currentFilter)
				m.list.SetFilterState(list.Filtering)
			}

			// Transition: non-empty → empty (cleared filter)
			// Must return early to avoid the stale async filter command from
			// m.list.Update(msg) overriding our grouped displayItems.
			if prevFilter != "" && currentFilter == "" {
				displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, "", m.workspacePrefixes, m.groupMode)
				m.list.SetItems(displayItems)
				m.list.Filter = seshFilter(displayItems, m.frecencyScores, m.workspacePrefixes, m.groupMode)
				m.list.SetFilterText("")
				m.list.SetFilterState(list.Filtering)
				m.list.SetSize(m.list.Width(), m.list.Height())

				m.list.Select(0)
				switch item := m.list.SelectedItem().(type) {
				case sessionItem:
					return m.loadPreviewForItem(item)
				case worktreeGroupItem:
					if rep, ok := representativeSession(item); ok {
						return m.loadPreviewForItem(rep)
					}
				}
				return m, nil
			}

			m.list.Select(0)
			// Load preview for top item or group representative
			switch item := m.list.SelectedItem().(type) {
			case sessionItem:
				newModel, previewCmd := m.loadPreviewForItem(item)
				m = newModel
				return m, tea.Batch(cmd, previewCmd)
			case worktreeGroupItem:
				if rep, ok := representativeSession(item); ok {
					newModel, previewCmd := m.loadPreviewForItem(rep)
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

func getFilterTitle(filter FilterType, mode GroupMode) string {
	var base string
	switch filter {
	case FilterConfig:
		base = "⚙️ config"
	case FilterZoxide:
		base = "📁 zoxide"
	default:
		base = "⚡ Sesh Sessions"
	}
	if mode == GroupByBranch {
		base += " \033[240m[branch]\033[39m"
	}
	return base
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
