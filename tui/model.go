package tui

import (
	"time"

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

// tmuxFirstFilter returns a custom FilterFunc that groups tmux sessions first
func tmuxFirstFilter(items []list.Item) list.FilterFunc {
	return func(term string, targets []string) []list.Rank {
		// Use default fuzzy filter to get matches
		ranks := list.DefaultFilter(term, targets)

		// Partition ranks by tmux sessions vs others
		estimatedSize := len(ranks) / 2
		tmuxRanks := make([]list.Rank, 0, estimatedSize)
		otherRanks := make([]list.Rank, 0, estimatedSize)

		for _, rank := range ranks {
			if rank.Index < len(items) {
				if item, ok := items[rank.Index].(sessionItem); ok {
					if item.session.Src == "tmux" {
						tmuxRanks = append(tmuxRanks, rank)
					} else {
						otherRanks = append(otherRanks, rank)
					}
				}
			}
		}

		// Concatenate: tmux first, then others
		result := make([]list.Rank, 0, len(ranks))
		result = append(result, tmuxRanks...)
		result = append(result, otherRanks...)
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
	processInfo      map[string]string // session -> detected process
	previewWrapWidth int               // Track last width used for preview wrapping
}

func newModel(
	lister lister.Lister,
	connector connector.Connector,
	icon icon.Icon,
	tmux tmux.Tmux,
	config model.Config,
	previewer previewer.Previewer,
	sessions model.SeshSessions,
) Model {
	// Build list items with pre-computed display names
	items := make([]list.Item, 0, len(sessions.OrderedIndex))
	if sessions.Directory != nil && sessions.OrderedIndex != nil {
		for _, key := range sessions.OrderedIndex {
			if session, ok := sessions.Directory[key]; ok {
				items = append(items, sessionItem{
					session:     session,
					displayName: icon.AddIcon(session),
				})
			}
		}
	}

	// Partition items so tmux sessions appear first
	items = partitionItemsByTmux(items)

	// Create model instance first (we need to pass processInfo pointer to delegate)
	m := Model{
		sessions:       sessions,
		width:          80,
		height:         24,
		currentFilter:  FilterAll,
		previewContent: "",
		pendingPreview: "",
		lastPreviewKey: "",
		processInfo:    make(map[string]string),
	}

	// Create list with items using compact delegate
	// Start with reasonable defaults, will be resized on WindowSizeMsg
	listWidth := 60
	previewWidth := 100
	delegate := compactDelegate{processInfo: &m.processInfo}
	l := list.New(items, delegate, listWidth, 24)
	l.Title = ""               // Empty title to avoid any spacing
	l.SetShowStatusBar(false)  // Hide item count
	l.SetShowPagination(false)
	l.SetFilteringEnabled(true)
	l.SetShowTitle(false) // Hide default title, we'll render custom one
	l.SetShowHelp(false)  // Hide help to keep UI clean

	// Use custom filter that groups tmux sessions first
	l.Filter = tmuxFirstFilter(items)

	// Create preview viewport
	vp := viewport.New(previewWidth, 24)
	vp.SetContent("")

	// Disable j/k shortcuts, keep arrow keys only
	l.DisableQuitKeybindings()
	listKeys := l.KeyMap
	// Set cursor keys for both normal and filtering mode
	listKeys.CursorUp.SetKeys("up", "ctrl+p")
	listKeys.CursorDown.SetKeys("down", "ctrl+n")
	// Clear accept/cancel filter keys so arrows don't exit filter mode
	listKeys.AcceptWhileFiltering.SetKeys("enter", "tab")
	listKeys.CancelWhileFiltering.SetKeys("esc", "ctrl+c", "ctrl+b")
	l.KeyMap = listKeys

	// Customize filter styling - simple gray prompt
	styles := list.DefaultStyles()
	styles.FilterPrompt = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styles.FilterCursor = lipgloss.NewStyle().Foreground(lipgloss.Color("170"))
	l.Styles = styles

	// Remove filter input prompt entirely
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

	return m
}

func (m Model) Init() tea.Cmd {
	logDebug("DEBUG: TUI Init() called with %d sessions", len(m.sessions.OrderedIndex))

	// Start with filter active and load preview for first session
	cmds := []tea.Cmd{
		func() tea.Msg {
			return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
		},
	}

	// Load preview for first item if available
	if m.list.SelectedItem() != nil {
		if item, ok := m.list.SelectedItem().(sessionItem); ok {
			cmds = append(cmds, loadPreview(m.previewer, item.session))
		}
	}

	// Schedule process detection after first render (10ms delay)
	cmds = append(cmds, tea.Tick(10*time.Millisecond, func(time.Time) tea.Msg {
		return StartProcessDetectionMsg{}
	}))

	return tea.Batch(cmds...)
}
