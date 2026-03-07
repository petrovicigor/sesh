package tui

import "github.com/joshmedeski/sesh/v2/model"

// sessionItem implements list.Item interface for bubbles/list
type sessionItem struct {
	session        model.SeshSession
	displayName    string // Pre-computed with icon and ⎇ formatting
	iconPrefix     string // Just the ANSI icon prefix (e.g., "\033[34m\033[39m ")
	sanitizedName  string // Pre-computed SanitizeSessionName(session.Name) for savedState lookups
	groupBadge     string // Dormant count badge (e.g., "\033[240m(+2)\033[39m") — set on last active tmux in group
	groupRepo      string // Repo name if this item carries expand/collapse for a group
	groupChild     bool   // True if this is an expanded child of a worktree group
	groupLastChild bool   // True if this is the last expanded child (for └ connector)
	bareRoot       bool   // True if this is the bare repo root of a worktree group
}

// Title returns the display name shown in the list
func (i sessionItem) Title() string {
	return i.displayName
}

// Description returns empty string (no description line needed)
func (i sessionItem) Description() string {
	return ""
}

// FilterValue returns the value used for fuzzy filtering
func (i sessionItem) FilterValue() string {
	return i.session.Name
}

// worktreeGroupItem represents a collapsed group of worktrees in the list
type worktreeGroupItem struct {
	repoName      string
	defaultBranch string        // "" = no default set
	activeCount   int           // active tmux worktrees
	dormantCount  int           // dormant project worktrees
	totalCount    int           // unique worktrees in group
	worktrees     []sessionItem // ALL unique worktree items (tmux + projects, deduplicated)
	displayName   string        // pre-computed with icon and badge
}

// Title returns the display name shown in the list
func (g worktreeGroupItem) Title() string { return g.displayName }

// Description returns empty string (no description line needed)
func (g worktreeGroupItem) Description() string { return "" }

// FilterValue returns the repo name used for fuzzy filtering
func (g worktreeGroupItem) FilterValue() string { return g.repoName }

// workspaceToggleItem represents a sub-project toggle in workspace manager mode
type workspaceToggleItem struct {
	workspaceName string // e.g., "mono"
	subProject    string // e.g., "packages/auth"
	excluded      bool   // true = excluded from session list
}

// Title returns the display name with checkbox
func (w workspaceToggleItem) Title() string {
	check := "[x]"
	if w.excluded {
		check = "[ ]"
	}
	return check + " " + w.workspaceName + "/" + w.subProject
}

// Description returns empty string
func (w workspaceToggleItem) Description() string { return "" }

// FilterValue returns the full path for fuzzy filtering
func (w workspaceToggleItem) FilterValue() string {
	return w.workspaceName + "/" + w.subProject
}

// separatorItem is a non-selectable visual divider between active and inactive sessions.
type separatorItem struct{}

func (s separatorItem) Title() string       { return "" }
func (s separatorItem) Description() string { return "" }
func (s separatorItem) FilterValue() string { return "" }
