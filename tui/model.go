package tui

import (
	"context"
	"slices"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/joshmedeski/sesh/v2/connector"
	"github.com/joshmedeski/sesh/v2/icon"
	"github.com/joshmedeski/sesh/v2/lister"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/previewer"
	"github.com/joshmedeski/sesh/v2/scrim"
	"github.com/joshmedeski/sesh/v2/tmux"
)

// seshFilter returns a custom FilterFunc with fzf-style fuzzy scoring.
// Ranks results by match quality, with frecency tiebreaking within 5% score bands,
// tmux sessions winning remaining ties.
// Worktree siblings (same repo prefix) are grouped together, ordered by
// the best score in the group.
func seshFilter(items []list.Item, frecencyScores map[string]float64, workspacePrefixes []string, mode GroupMode) list.FilterFunc {
	return func(term string, targets []string) []list.Rank {
		if term == "" {
			return nil
		}

		type scoredRank struct {
			rank     list.Rank
			score    float64
			src      string
			repo     string  // repo prefix for grouping ("" = ungrouped)
			frecency float64 // frecency score for tiebreaking
		}

		results := make([]scoredRank, 0, len(targets))
		for i, target := range targets {
			score, indices := fuzzyScore(term, target)
			if score <= 0 {
				continue
			}

			var src, name string
			if i < len(items) {
				switch v := items[i].(type) {
				case sessionItem:
					src = v.session.Src
					name = v.session.Name
				case worktreeGroupItem:
					src = "projects"
					name = v.repoName
				}
			}

			// Determine repo group: items with "/" get grouped by prefix
			repo := ""
			if strings.Contains(name, "/") {
				repo = groupKeyForItem(name, src, workspacePrefixes, mode)
			}

			// Look up frecency score for tiebreaking
			var frec float64
			if frecencyScores != nil {
				frec = frecencyScores[name]
			}

			results = append(results, scoredRank{
				rank: list.Rank{
					Index:          i,
					MatchedIndexes: indices,
				},
				score:    score,
				src:      src,
				repo:     repo,
				frecency: frec,
			})
		}

		// Sort by score, with frecency tiebreaking within 5% bands
		slices.SortStableFunc(results, func(a, b scoredRank) int {
			// If scores differ by more than 5%, sort by score alone
			maxScore := a.score
			if b.score > maxScore {
				maxScore = b.score
			}
			diff := a.score - b.score
			if diff < 0 {
				diff = -diff
			}
			if maxScore > 0 && diff/maxScore > 0.05 {
				if a.score > b.score {
					return -1
				}
				return 1
			}
			// Within 5% band: frecency tiebreaker
			if a.frecency != b.frecency {
				if a.frecency > b.frecency {
					return -1
				}
				return 1
			}
			// Tmux wins remaining ties
			aIsTmux := a.src == "tmux"
			bIsTmux := b.src == "tmux"
			if aIsTmux && !bIsTmux {
				return -1
			}
			if !aIsTmux && bIsTmux {
				return 1
			}
			return 0
		})

		// Group worktree siblings together: when we encounter the first item
		// from a repo, pull all other items from that repo to follow it.
		grouped := make([]scoredRank, 0, len(results))
		used := make(map[int]bool)
		for i, r := range results {
			if used[i] {
				continue
			}
			used[i] = true
			grouped = append(grouped, r)

			// If this item has a repo prefix, pull in all siblings
			if r.repo != "" {
				for j := i + 1; j < len(results); j++ {
					if !used[j] && results[j].repo == r.repo {
						used[j] = true
						grouped = append(grouped, results[j])
					}
				}
			}
		}

		ranks := make([]list.Rank, len(grouped))
		for i, r := range grouped {
			ranks[i] = r.rank
		}
		return ranks
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
	lastPreviewKey   string            // Last session name that had preview loaded
	processInfo      *map[string]string       // shared with delegate (heap-allocated)
	allItems         []list.Item              // Full item list (no groups, used for filter mode)
	worktreeGroups   map[string]*worktreeGroup // Grouped worktrees
	expandedGroup    *string                  // shared with delegate (heap-allocated)
	worktreeDefaults *map[string]string       // shared with delegate (heap-allocated)
	defaultsPath     string                   // path to defaults JSON file
	repoFocusFilter   string                   // repo name for Ctrl+T focus ("" = no focus)
	frecencyScores    map[string]float64       // frecency scores for filter tiebreaking
	workspacePrefixes []string                 // workspace config names for group key extraction
	groupMode         GroupMode                // current workspace grouping mode (package vs branch)
	claudeAttention   *map[string]bool         // shared with delegate: tmux session name -> needs attention
	savedState        *map[string]bool         // shared with delegate: session name -> has saved state
	gitChanges        *map[string]gitChanges   // shared with delegate: path -> working-tree change counts
	pendingDeleteFilterText  string // filter text to restore after async session kill
	pendingDeleteCursorIndex int    // cursor index to restore after async session kill
	showPreview            bool   // true when preview pane is visible (toggled with ctrl+p)
	isDark                 bool   // terminal dark mode (detected via BackgroundColorMsg)

	previewCancel        context.CancelFunc       // cancels the in-flight preview load goroutine

	// Workspace manager mode
	workspaceManagerMode bool                     // true when in workspace manager mode
	workspaceExcludes    map[string][]string       // workspace name -> excluded sub-project paths
	workspaceSubProjects map[string][]string       // cached discovered sub-projects (during manager mode)
	excludesPath         string                    // path to workspace-excludes.json

	// Pending state from a kill-and-relaunch toggle. nil on normal launch.
	// Consumed by Init() which fires applyRestoreStateMsg once.
	pendingRestore *RestoreState

	// Scrim mode: the tmux bind opened a FULL-CLIENT popup (`sesh tui --scrim
	// <window-id>`) and the TUI draws its classic box (45% compact / 80% with
	// preview, 75% tall) centered on a dimmed snapshot of the window behind
	// it. width/height above become the BOX; screenW/screenH are the real
	// popup dims. A direct `sesh tui` (the `t` alias, full-screen in a pane)
	// keeps the plain full-size rendering — scrimMode stays false.
	scrimMode   bool
	scrimTarget string // window to capture for the backdrop (async, in Init)
	screenW     int
	screenH     int
	snap        *scrim.Snapshot // dimmed backdrop; nil composes a plain dim
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
	frecencyScores map[string]float64,
	excludesPath string,
	restore *RestoreState,
	scrimMode bool,
	scrimTarget string,
) Model {
	logDebug("newModel: building list items")

	// Build list items with pre-computed display names
	items := make([]list.Item, 0, len(sessions.OrderedIndex))
	if sessions.Directory != nil && sessions.OrderedIndex != nil {
		for _, key := range sessions.OrderedIndex {
			if session, ok := sessions.Directory[key]; ok {
				displayName := icon.AddIcon(session)
				items = append(items, sessionItem{
					session:       session,
					displayName:   displayName,
					iconPrefix:    extractIconPrefix(displayName, session.Name),
					sanitizedName: SanitizeSessionName(session.Name),
				})
			}
		}
	}
	logDebug("newModel: %d items built", len(items))

	// Extract workspace prefixes from config for group key extraction
	workspacePrefixes := make([]string, 0, len(config.WorkspaceConfigs))
	for _, ws := range config.WorkspaceConfigs {
		if ws.Name != "" {
			workspacePrefixes = append(workspacePrefixes, ws.Name)
		}
	}

	// Partition items so tmux sessions appear first
	items = partitionItemsByTmux(items)
	logDebug("newModel: partitioned by tmux")

	// Build worktree groups and create collapsed display items
	worktreeGroups := buildWorktreeGroups(items, worktreeDefaults, workspacePrefixes, GroupByPackage)
	logDebug("newModel: %d worktree groups built", len(worktreeGroups))
	displayItems := buildDisplayItems(items, worktreeGroups, "", workspacePrefixes, GroupByPackage)
	logDebug("newModel: %d display items built", len(displayItems))

	// Create model instance first (we need to pass processInfo pointer to delegate)
	// width/height start at 0 — View() returns "" until WindowSizeMsg arrives,
	// preventing a wasted render at wrong default size (eliminates flicker).
	initialShowPreview := false // default: hidden, toggle with ctrl+p
	if restore != nil {
		initialShowPreview = restore.ShowPreview
	}

	m := Model{
		sessions:         sessions,
		width:            0,
		height:           0,
		scrimMode:        scrimMode,
		scrimTarget:      scrimTarget,
		showPreview:      initialShowPreview,
		pendingRestore:   restore,
		isDark:           true, // default until BackgroundColorMsg arrives
		currentFilter:    FilterAll,
		previewContent:   "",
		lastPreviewKey:   "",
		processInfo:      &map[string]string{},
		claudeAttention:  &map[string]bool{},
		savedState:       &map[string]bool{},
		gitChanges:       &map[string]gitChanges{},
		allItems:         items,
		worktreeGroups:   worktreeGroups,
		expandedGroup:    new(string),
		worktreeDefaults: &worktreeDefaults,
		defaultsPath:      defaultsPath,
		repoFocusFilter:   "",
		frecencyScores:    frecencyScores,
		workspacePrefixes: workspacePrefixes,
		excludesPath:      excludesPath,
	}

	logDebug("newModel: creating list widget")

	// Create list with items using compact delegate
	// Start with reasonable defaults, will be resized on WindowSizeMsg
	listWidth := 60
	previewWidth := 100
	delegate := compactDelegate{processInfo: m.processInfo, expandedGroup: m.expandedGroup, worktreeDefaults: m.worktreeDefaults, claudeAttention: m.claudeAttention, savedState: m.savedState, gitChanges: m.gitChanges}
	l := list.New(displayItems, delegate, listWidth, 24)
	l.Title = "⚡ Sesh Sessions" // Set initial title
	l.SetShowStatusBar(false)  // Hide item count
	l.SetShowPagination(false)
	l.SetFilteringEnabled(true)
	l.SetShowTitle(false)
	l.SetShowHelp(false)  // Hide help to keep UI clean

	// Use custom filter with fzf-style scoring, frecency tiebreaking
	l.Filter = seshFilter(displayItems, frecencyScores, workspacePrefixes, GroupByPackage)

	// Create preview viewport
	vp := viewport.New(viewport.WithWidth(previewWidth), viewport.WithHeight(24))
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

	// Customize list styling
	styles := list.DefaultStyles(true)
	// Make title visible with bold and color
	styles.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("170")).
		MarginBottom(1).
		MarginLeft(2)
	l.Styles = styles

	// Remove filter input prompt (will be set temporarily for indicator)
	l.FilterInput.Prompt = ""

	// Make filter input cursor a solid block (no blink) for better visibility.
	// Clear Cursor.Color so Reverse uses the terminal's default foreground,
	// giving a full-color block instead of the default dim ANSI 7 gray.
	inputStyles := l.FilterInput.Styles()
	inputStyles.Cursor.Blink = false
	inputStyles.Cursor.Color = lipgloss.NoColor{}
	l.FilterInput.SetStyles(inputStyles)

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

	// Request terminal background color for dark/light mode detection
	bgColorCmd := func() tea.Msg {
		return tea.RequestBackgroundColor()
	}

	cmds := []tea.Cmd{bgColorCmd, checkClaudeAttention(), scheduleClaudeAttentionTick(), checkSavedState()}

	// The backdrop capture rides along asynchronously: several tmux
	// round-trips, and done before the program started it held the popup
	// open on a bare cursor. Until it lands, the nil snapshot composes a
	// plain dimmed background.
	if m.scrimMode && m.scrimTarget != "" {
		target := m.scrimTarget
		cmds = append(cmds, func() tea.Msg {
			snap, err := scrim.Capture(target)
			if err != nil {
				logDebug("scrim capture failed: %v", err)
			}
			return ScrimMsg{Snap: snap}
		})
	}

	// When restoring from a kill-and-relaunch toggle, the restore handler owns
	// filter-mode entry itself (it must enter filter mode BEFORE setting text,
	// otherwise the '/' keypress lands in the filter input as a literal char).
	// On normal launch, enterFilterMsg does this.
	if m.pendingRestore != nil {
		cmds = append(cmds, func() tea.Msg { return applyRestoreStateMsg{} })
	} else {
		cmds = append(cmds, func() tea.Msg { return enterFilterMsg{} })
	}

	// Load preview for first item if available
	if m.list.SelectedItem() != nil && m.showPreview {
		if item, ok := m.list.SelectedItem().(sessionItem); ok {
			logDebug("Init: queuing preview load for %q", item.session.Name)
			cmds = append(cmds, loadPreview(context.Background(), m.previewer, item.session))
		}
	}

	return tea.Batch(cmds...)
}
