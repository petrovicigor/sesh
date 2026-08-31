package tui

import (
	"context"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/joshmedeski/sesh/v2/state"
)

// handleCursorMove loads preview for the current item immediately.
// Context cancellation kills any in-flight preview load — no debounce needed.
func (m Model) handleCursorMove() (Model, tea.Cmd) {
	if !m.showPreview {
		return m, nil
	}
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

// applyPanelSize derives width/height from the screen dims. In scrim mode the
// UI is a panel boxed to what the popup used to be opened at (45% compact /
// 80% with preview, 75% tall) inside the full-client popup; otherwise the UI
// fills the screen as before.
func (m *Model) applyPanelSize() {
	if !m.scrimMode {
		m.width, m.height = m.screenW, m.screenH
		return
	}
	pct := 45
	if m.showPreview {
		pct = 80
	}
	m.width = m.screenW * pct / 100
	m.height = m.screenH * 75 / 100
}

// toggleAndRelaunch serializes current TUI state (filter text, selected session,
// target preview flag), asks tmux to queue a new popup at the opposite size
// via tmux run-shell -b, and exits. The queued popup opens after the current
// one closes, and the new sesh process reads the state file.
//
// Scrim mode skips the whole dance: the full-client popup freed the panel
// from tmux's fixed popup geometry, so the preview toggles in place and the
// panel just re-lays-out at the other width.
func (m Model) toggleAndRelaunch() (Model, tea.Cmd) {
	if m.scrimMode {
		m.showPreview = !m.showPreview
		if !m.showPreview {
			m.cancelInflightPreview()
		}
		m.applyPanelSize()
		return m.applyLayout()
	}
	var sessionName string
	switch item := m.list.SelectedItem().(type) {
	case sessionItem:
		sessionName = item.session.Name
	case worktreeGroupItem:
		if rep, ok := representativeSession(item); ok {
			sessionName = rep.session.Name
		} else {
			sessionName = item.repoName
		}
	}

	state := RestoreState{
		Filter:      m.list.FilterValue(),
		Cursor:      m.list.Index(),
		ShowPreview: !m.showPreview,
		SessionName: sessionName,
	}

	if err := ScheduleRelaunch(state); err != nil {
		// If the relaunch can't be scheduled, fall back to in-place toggle
		// so the user isn't left in a broken state.
		logDebug("toggleAndRelaunch: ScheduleRelaunch failed: %v", err)
		m.showPreview = !m.showPreview
		if !m.showPreview {
			m.cancelInflightPreview()
		}
		return m.applyLayout()
	}
	return m, tea.Quit
}

// boxChromeV is the rows a column box spends on its own vertical chrome:
// border plus padding when bordered (full-screen mode), padding only for
// scrim mode's borderless boxes. Horizontal chrome is 4 in both modes, so
// the width arithmetic stays shared.
func (m Model) boxChromeV() int {
	if m.scrimMode {
		return 2
	}
	return 4
}

// listInnerHeight returns the list's inner content height, reserving two rows for
// the blank spacer + hint line when a worktree group is expanded.
func (m Model) listInnerHeight() int {
	h := m.height - m.boxChromeV()
	if m.expandedGroup != nil && *m.expandedGroup != "" {
		h -= 2
	}
	return h
}

// applyLayout recalculates list and preview sizes based on showPreview state.
// When toggling preview on, loads preview for the current item.
func (m Model) applyLayout() (Model, tea.Cmd) {
	listHeight := m.listInnerHeight()
	if m.showPreview {
		listBoxWidth := (m.width * 45) / 100
		previewBoxWidth := m.width - listBoxWidth
		m.list.SetSize(listBoxWidth-4, listHeight)
		m.previewPort.SetWidth(previewBoxWidth - 4)
		m.previewPort.SetHeight(m.height - m.boxChromeV())
	} else {
		m.list.SetSize(m.width-4, listHeight)
	}
	if m.showPreview && m.previewContent != "" {
		m.previewPort.SetContent(m.previewContent)
	}
	if m.showPreview {
		switch item := m.list.SelectedItem().(type) {
		case sessionItem:
			return m.loadPreviewForItem(item)
		case worktreeGroupItem:
			if rep, ok := representativeSession(item); ok {
				return m.loadPreviewForItem(rep)
			}
		}
	}
	return m, nil
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
			// Shrink list to make room for the in-box hint line.
			m.list.SetSize(m.list.Width(), m.listInnerHeight())
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
	// Also shrink by 1 row to reserve space for the in-box hint line.
	m.list.SetSize(m.list.Width(), m.listInnerHeight())

	// Find cursor target. Preference order, so expanding lands on the most
	// useful child instead of the badge carrier (which is the parent-summary row):
	//   1. dormant child whose branch matches the active tmux session
	//   2. dormant child whose branch matches the user's default for this repo
	//   3. first non-bare-root dormant child
	//   4. first dormant child (bare root only as last resort)
	//   5. badge carrier
	targetIndex := 0
	badgeCarrierIndex := -1
	activeBranch := ""
	if group != nil {
		for name := range group.tmuxNames {
			if idx := strings.LastIndex(name, "/"); idx >= 0 {
				activeBranch = name[idx+1:]
				break
			}
		}
	}
	defaultBranch := ""
	if m.worktreeDefaults != nil {
		defaultBranch = (*m.worktreeDefaults)[repoName]
	}

	matchActiveIdx := -1
	matchDefaultIdx := -1
	firstNonBareIdx := -1
	firstDormantIdx := -1
	foundBadgeOrHeader := false
	for i, listItem := range displayItems {
		switch v := listItem.(type) {
		case worktreeGroupItem:
			if v.repoName == repoName {
				foundBadgeOrHeader = true
			}
		case sessionItem:
			if v.groupRepo == repoName {
				if badgeCarrierIndex == -1 {
					badgeCarrierIndex = i
				}
				foundBadgeOrHeader = true
				continue
			}
			if !foundBadgeOrHeader || !v.groupChild {
				continue
			}
			itemKey := groupKeyForItem(v.session.Name, v.session.Src, m.workspacePrefixes, m.groupMode)
			if itemKey != repoName && v.session.Name != repoName {
				continue
			}
			if firstDormantIdx == -1 {
				firstDormantIdx = i
			}
			if !v.bareRoot && firstNonBareIdx == -1 {
				firstNonBareIdx = i
			}
			branch := ""
			if idx := strings.LastIndex(v.session.Name, "/"); idx >= 0 {
				branch = v.session.Name[idx+1:]
			}
			if activeBranch != "" && branch == activeBranch && matchActiveIdx == -1 {
				matchActiveIdx = i
			}
			if defaultBranch != "" && branch == defaultBranch && matchDefaultIdx == -1 {
				matchDefaultIdx = i
			}
		}
	}
	switch {
	case matchActiveIdx >= 0:
		targetIndex = matchActiveIdx
	case matchDefaultIdx >= 0:
		targetIndex = matchDefaultIdx
	case firstNonBareIdx >= 0:
		targetIndex = firstNonBareIdx
	case firstDormantIdx >= 0:
		targetIndex = firstDormantIdx
	case badgeCarrierIndex >= 0:
		targetIndex = badgeCarrierIndex
	}
	logDebug("DEBUG expandGroup: repoName=%s targetIndex=%d totalItems=%d", repoName, targetIndex, len(displayItems))
	m.list.Select(targetIndex)

	// Load preview for target item
	if m.showPreview && targetIndex < len(displayItems) {
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
			// Restore full height now that the in-box hint is gone.
			m.list.SetSize(m.list.Width(), m.listInnerHeight())
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
	// Restore full height now that the in-box hint is gone.
	m.list.SetSize(m.list.Width(), m.listInnerHeight())

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
	if m.showPreview && targetIndex < len(displayItems) {
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
	m.list.SetSize(m.list.Width(), m.listInnerHeight())
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
	m.list.SetSize(m.list.Width(), m.listInnerHeight())

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
		m.screenW, m.screenH = msg.Width, msg.Height
		m.applyPanelSize()
		return m.applyLayout()

	case ScrimMsg:
		m.snap = msg.Snap
		return m, nil

	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		// Rebuild list styles with correct dark/light mode
		styles := list.DefaultStyles(m.isDark)
		styles.Title = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#5f875f")).
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

	case applyRestoreStateMsg:
		// Rehydrate state after a kill-and-relaunch toggle. See state_restore.go.
		// showPreview was already set in newModel; here we restore filter text,
		// cursor position, and trigger any pending save/restore action.
		if m.pendingRestore == nil {
			return m, nil
		}
		rs := m.pendingRestore
		m.pendingRestore = nil

		// Enter filter mode via the list's own '/' handler so the filter input
		// gets focused properly. Must happen BEFORE SetFilterText — otherwise
		// if we set text first, the '/' would be appended as a literal char.
		var listCmd tea.Cmd
		m.list, listCmd = m.list.Update(tea.KeyPressMsg{Code: '/', Text: "/"})

		// Mirror the "empty → non-empty" transition that happens when the user
		// starts typing: swap to full item set for fuzzy search, then apply text.
		// Without this swap, SetFilterText alone doesn't trigger filter evaluation
		// (items show but aren't filtered).
		if rs.Filter != "" {
			*m.expandedGroup = ""
			m.list.Filter = seshFilter(m.allItems, m.frecencyScores, m.workspacePrefixes, m.groupMode)
			m.list.SetItems(m.allItems)
			m.list.SetFilterText(rs.Filter)
			m.list.SetFilterState(list.Filtering)
			m.list.SetSize(m.list.Width(), m.listInnerHeight())
			m.lastFilter = rs.Filter
		}

		// Prefer to re-select by session name (stable across filter evaluation).
		// Fall back to cursor index, clamped to visible range.
		targetIndex := rs.Cursor
		if rs.SessionName != "" {
			for i, listItem := range m.list.VisibleItems() {
				switch v := listItem.(type) {
				case sessionItem:
					if v.session.Name == rs.SessionName {
						targetIndex = i
					}
				case worktreeGroupItem:
					if v.repoName == rs.SessionName {
						targetIndex = i
					}
				}
			}
		}
		visibleCount := len(m.list.VisibleItems())
		if visibleCount == 0 {
			targetIndex = 0
		} else if targetIndex >= visibleCount {
			targetIndex = visibleCount - 1
		} else if targetIndex < 0 {
			targetIndex = 0
		}
		m.list.Select(targetIndex)

		cmds := []tea.Cmd{}
		if listCmd != nil {
			cmds = append(cmds, listCmd)
		}
		// Load preview for the restored item when preview is on.
		if m.showPreview {
			switch item := m.list.SelectedItem().(type) {
			case sessionItem:
				var previewCmd tea.Cmd
				m, previewCmd = m.loadPreviewForItem(item)
				if previewCmd != nil {
					cmds = append(cmds, previewCmd)
				}
			case worktreeGroupItem:
				if rep, ok := representativeSession(item); ok {
					var previewCmd tea.Cmd
					m, previewCmd = m.loadPreviewForItem(rep)
					if previewCmd != nil {
						cmds = append(cmds, previewCmd)
					}
				}
			}
		}

		return m, tea.Batch(cmds...)

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
			m.list.SetSize(m.list.Width(), m.listInnerHeight())

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
			if m.showPreview {
				if item, ok := m.list.SelectedItem().(sessionItem); ok {
					m.previewPort.SetContent("")
					previewCmd = loadPreview(context.Background(), m.previewer,item.session)
				}
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
		if m.showPreview && len(items) > 0 {
			if firstItem, ok := items[0].(sessionItem); ok {
				// Blank preview while loading
				m.previewPort.SetContent("")
				previewCmd = loadPreview(context.Background(), m.previewer,firstItem.session)
			}
		} else {
			// No items or preview hidden - clear preview
			m.previewPort.SetContent("")
		}

		// Re-enable filter mode
		filterCmd := func() tea.Msg {
			return enterFilterMsg{}
		}

		return m, tea.Batch(previewCmd, filterCmd)

	case PreviewLoadedMsg:
		if !m.showPreview {
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

	case GitChangesMsg:
		// Update map in-place to preserve delegate's pointer reference
		for k := range *m.gitChanges {
			delete(*m.gitChanges, k)
		}
		for k, v := range msg.Changes {
			(*m.gitChanges)[k] = v
		}
		return m, nil

	case SessionKilledMsg:
		// Reload even on error: kill-session commonly returns exit 1 when the
		// session was already destroyed by GracefulPaneCleanup's SIGTERM to
		// the last nvim pane. Skipping the reload leaves a ghost row in the
		// picker until something else triggers a refresh.
		if msg.Err != nil {
			logDebug("DEBUG ctrl+d: kill error: %v (reloading anyway)", msg.Err)
		} else {
			logDebug("DEBUG ctrl+d: killed ok, reloading with preserved state")
		}
		*m.expandedGroup = ""
		return m, loadSessionsPreservingState(m.lister, m.currentFilter, m.pendingDeleteFilterText, m.pendingDeleteCursorIndex)

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
			m.list.SetSize(m.list.Width(), m.listInnerHeight())

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
			if m.showPreview {
				switch item := m.list.SelectedItem().(type) {
				case sessionItem:
					return m.loadPreviewForItem(item)
				case worktreeGroupItem:
					if rep, ok := representativeSession(item); ok {
						return m.loadPreviewForItem(rep)
					}
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.TogglePreview):
			// Kill-and-relaunch toggle: tmux popups can't be resized in place,
			// so we persist state, ask tmux to queue a new popup at the opposite
			// size, and exit. See state_restore.go for the mechanism.
			return m.toggleAndRelaunch()

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
								if m.showPreview {
									return m.loadPreviewForItem(rootItem)
								}
								return m, nil
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
			m.list.SetSize(m.list.Width(), m.listInnerHeight())

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
			m.list.SetSize(m.list.Width(), m.listInnerHeight())

			m.list.Select(targetIndex)

			// Load preview for target item
			if m.showPreview && targetIndex < len(displayItems) {
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
				m.list.SetSize(m.list.Width(), m.listInnerHeight())

				m.list.Select(0)
				if m.showPreview {
					switch item := m.list.SelectedItem().(type) {
					case sessionItem:
						return m.loadPreviewForItem(item)
					case worktreeGroupItem:
						if rep, ok := representativeSession(item); ok {
							return m.loadPreviewForItem(rep)
						}
					}
				}
				return m, nil
			}

			m.list.Select(0)
			// Load preview for top item or group representative
			if m.showPreview {
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
