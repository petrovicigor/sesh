package tui

import (
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

type Model struct {
	lister    lister.Lister
	connector connector.Connector
	icon      icon.Icon
	tmux      tmux.Tmux
	config    model.Config
	previewer previewer.Previewer

	list           list.Model
	previewPort    viewport.Model
	sessions       model.SeshSessions
	selected       string
	width          int
	height         int
	currentFilter  FilterType
	keys           KeyMap
	lastFilter     string            // Track last filter text to detect changes
	previewContent string            // Current preview text
	processInfo    map[string]string // session.Name -> process indicator (e.g., "node", "python")
	pendingPreview string            // Session name waiting for debounce
	lastPreviewKey string            // Last session name that had preview loaded
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

	// Create list with items using compact delegate
	// Start with reasonable defaults, will be resized on WindowSizeMsg
	listWidth := 60
	previewWidth := 100
	processInfo := make(map[string]string)
	delegate := compactDelegate{processInfo: &processInfo}
	l := list.New(items, delegate, listWidth, 24)
	l.Title = "Sesh Sessions"
	l.SetShowStatusBar(false) // Hide item count
	l.SetShowPagination(false)
	l.SetFilteringEnabled(true)
	l.SetShowTitle(false) // Hide default title, we'll render custom one
	l.SetShowHelp(false)  // Hide help to keep UI clean

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

	// Remove "Filter:" text, just show cursor
	l.FilterInput.Prompt = ""
	l.FilterInput.PromptStyle = styles.FilterPrompt

	return Model{
		lister:         lister,
		connector:      connector,
		icon:           icon,
		tmux:           tmux,
		config:         config,
		previewer:      previewer,
		list:           l,
		previewPort:    vp,
		sessions:       sessions,
		width:          80,
		height:         24,
		currentFilter:  FilterAll,
		keys:           DefaultKeyMap,
		previewContent: "",
		processInfo:    processInfo,
		pendingPreview: "",
		lastPreviewKey: "",
	}
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

	// Start async process detection for tmux sessions
	cmds = append(cmds, detectProcessesForAllSessions(m.sessions))

	return tea.Batch(cmds...)
}
