package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/joshmedeski/sesh/v2/connector"
	"github.com/joshmedeski/sesh/v2/icon"
	"github.com/joshmedeski/sesh/v2/lister"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/previewer"
	"github.com/joshmedeski/sesh/v2/tmux"
)

// tmuxFirstFilter returns a custom FilterFunc that groups results by repo.
// Within each group, tmux sessions appear before other sources.
// Groups are ordered by the best fuzzy match score of any item in the group.
func tmuxFirstFilter(items []list.Item) list.FilterFunc {
	return func(term string, targets []string) []list.Rank {
		ranks := list.DefaultFilter(term, targets)

		if len(ranks) == 0 {
			return ranks
		}

		// Group ranks by repo name
		type repoGroup struct {
			repo       string
			tmuxRanks  []list.Rank
			otherRanks []list.Rank
		}

		groupMap := make(map[string]*repoGroup)
		groupOrder := make([]string, 0) // preserve first-seen order by score

		for _, rank := range ranks {
			if rank.Index >= len(items) {
				continue
			}

			// Determine item name and source
			var name, src string
			switch v := items[rank.Index].(type) {
			case sessionItem:
				name = v.session.Name
				src = v.session.Src
			case worktreeGroupItem:
				name = v.repoName
				src = "projects"
			default:
				continue
			}

			// Determine repo name
			repo := name
			if strings.Contains(repo, "/") {
				repo = strings.SplitN(repo, "/", 2)[0]
			}

			g, exists := groupMap[repo]
			if !exists {
				g = &repoGroup{repo: repo}
				groupMap[repo] = g
				groupOrder = append(groupOrder, repo)
			}

			if src == "tmux" {
				g.tmuxRanks = append(g.tmuxRanks, rank)
			} else {
				g.otherRanks = append(g.otherRanks, rank)
			}
		}

		// Build result: for each group (in score order), tmux first then others
		result := make([]list.Rank, 0, len(ranks))
		for _, repo := range groupOrder {
			g := groupMap[repo]
			result = append(result, g.tmuxRanks...)
			result = append(result, g.otherRanks...)
		}

		return result
	}
}

type Model struct {
	lister    lister.Lister
	connector connector.Connector
	icon      icon.Icon
	tmux      tmux.Tmux
	config    model.Config
	previewer previewer.Previewer

	list             list.Model
	previewPort      viewport.Model
	sessions         model.SeshSessions
	selected         string
	width            int
	height           int
	currentFilter    FilterType
	keys             KeyMap
	lastFilter       string            // Track last filter text to detect changes
	previewContent   string            // Current preview text
	pendingPreview   string            // Session name waiting for debounce
	lastPreviewKey   string            // Last session name that had preview loaded
	restoringState   bool              // True when restoring filter text after ctrl+d
	processInfo      *map[string]string       // shared with delegate (heap-allocated)
	allItems         []list.Item              // Full item list (no groups, used for filter mode)
	worktreeGroups   map[string]*worktreeGroup // Grouped worktrees
	expandedGroup    *string                  // shared with delegate (heap-allocated)
	worktreeDefaults *map[string]string       // shared with delegate (heap-allocated)
	defaultsPath     string                   // path to defaults JSON file
	repoFocusFilter  string                   // repo name for Ctrl+T focus ("" = no focus)
}

func newModel(
	lister lister.Lister,
	connector connector.Connector,
	icon icon.Icon,
	tmux tmux.Tmux,
	config model.Config,
	previewer previewer.Previewer,
	sessions model.SeshSessions,
	worktreeDefaults map[string]string,
	defaultsPath string,
) Model {
	logDebug("newModel: building list items")

	// Build list items with pre-computed display names
	items := make([]list.Item, 0, len(sessions.OrderedIndex))
	if sessions.Directory != nil && sessions.OrderedIndex != nil {
		for _, key := range sessions.OrderedIndex {
			if session, ok := sessions.Directory[key]; ok {
				displayName := icon.AddIcon(session)
				items = append(items, sessionItem{
					session:     session,
					displayName: displayName,
					iconPrefix:  extractIconPrefix(displayName, session.Name),
				})
			}
		}
	}
	logDebug("newModel: %d items built", len(items))

	// Partition items so tmux sessions appear first
	items = partitionItemsByTmux(items)
	logDebug("newModel: partitioned by tmux")

	// Build worktree groups and create collapsed display items
	worktreeGroups := buildWorktreeGroups(items, worktreeDefaults)
	logDebug("newModel: %d worktree groups built", len(worktreeGroups))
	displayItems := buildDisplayItems(items, worktreeGroups, "")
	logDebug("newModel: %d display items built", len(displayItems))

	// Create model instance first (we need to pass processInfo pointer to delegate)
	// width/height start at 0 — View() returns "" until WindowSizeMsg arrives,
	// preventing a wasted render at wrong default size (eliminates flicker).
	m := Model{
		sessions:         sessions,
		width:            0,
		height:           0,
		currentFilter:    FilterAll,
		previewContent:   "",
		pendingPreview:   "",
		lastPreviewKey:   "",
		processInfo:      &map[string]string{},
		allItems:         items,
		worktreeGroups:   worktreeGroups,
		expandedGroup:    new(string),
		worktreeDefaults: &worktreeDefaults,
		defaultsPath:     defaultsPath,
		repoFocusFilter:  "",
	}

	logDebug("newModel: creating list widget")

	// Create list with items using compact delegate
	// Start with reasonable defaults, will be resized on WindowSizeMsg
	listWidth := 60
	previewWidth := 100
	delegate := compactDelegate{processInfo: m.processInfo, expandedGroup: m.expandedGroup, worktreeDefaults: m.worktreeDefaults}
	l := list.New(displayItems, delegate, listWidth, 24)
	l.Title = "⚡ Sesh Sessions" // Set initial title
	l.SetShowStatusBar(false)  // Hide item count
	l.SetShowPagination(false)
	l.SetFilteringEnabled(true)
	l.SetShowTitle(false)
	l.SetShowHelp(false)  // Hide help to keep UI clean

	// Use custom filter that groups tmux sessions first
	l.Filter = tmuxFirstFilter(displayItems)

	// Create preview viewport
	vp := viewport.New(previewWidth, 24)
	vp.SetContent("")

	// Disable j/k shortcuts, keep arrow keys only
	l.DisableQuitKeybindings()
	listKeys := l.KeyMap
	// Set cursor keys for both normal and filtering mode
	listKeys.CursorUp.SetKeys("up")
	listKeys.CursorDown.SetKeys("down", "ctrl+n")
	// Clear accept/cancel filter keys so arrows don't exit filter mode
	listKeys.AcceptWhileFiltering.SetKeys("enter")
	listKeys.CancelWhileFiltering.SetKeys("esc", "ctrl+c", "ctrl+b")
	l.KeyMap = listKeys

	// Customize filter styling - simple gray prompt
	styles := list.DefaultStyles()
	styles.FilterPrompt = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styles.FilterCursor = lipgloss.NewStyle().Foreground(lipgloss.Color("170"))
	// Make title visible with bold and color
	styles.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("170")).
		MarginBottom(1).
		MarginLeft(2)
	l.Styles = styles

	// Remove filter input prompt (will be set temporarily for indicator)
	l.FilterInput.Prompt = ""

	// Complete model initialization with remaining fields
	m.lister = lister
	m.connector = connector
	m.icon = icon
	m.tmux = tmux
	m.config = config
	m.previewer = previewer
	m.list = l
	m.previewPort = vp
	m.keys = DefaultKeyMap

	logDebug("newModel: complete")
	return m
}

func (m Model) Init() tea.Cmd {
	logDebug("Init() called with %d sessions", len(m.sessions.OrderedIndex))

	// Enter filter mode asynchronously (allows typing to filter immediately)
	filterCmd := func() tea.Msg {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	}

	// Load preview for first item if available
	if m.list.SelectedItem() != nil {
		if item, ok := m.list.SelectedItem().(sessionItem); ok {
			logDebug("Init: queuing preview load for %q", item.session.Name)
			return tea.Batch(filterCmd, loadPreview(m.previewer, item.session))
		}
	}

	return filterCmd
}
