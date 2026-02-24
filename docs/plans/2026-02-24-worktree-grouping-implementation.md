# Worktree Grouping Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Collapse git worktrees into grouped entries in the TUI session list, with accordion expand/collapse and smart filter-mode grouping.

**Architecture:** Two display modes sharing the same underlying data. Default (no filter text) shows collapsed worktree groups as synthetic `worktreeGroupItem` entries. Filter mode (typing) shows all items with results grouped by repo. Item lists are swapped on transition between modes. All logic lives in `tui/` package — no changes to data loading or connection.

**Tech Stack:** Go 1.23, Bubble Tea (bubbletea), Bubbles list component, lipgloss styling

**Design doc:** `docs/plans/2026-02-24-worktree-grouping-design.md`

---

### Task 1: Add worktreeGroupItem type

**Files:**
- Modify: `tui/item.go`
- Test: `tui/item_test.go` (create)

**Step 1: Write the failing test**

Create `tui/item_test.go`:

```go
package tui

import (
	"testing"

	"github.com/joshmedeski/sesh/v2/model"
)

func TestWorktreeGroupItemInterface(t *testing.T) {
	group := worktreeGroupItem{
		repoName:     "chase-monorepo",
		dormantCount: 2,
		totalCount:   4,
		displayName:  " chase-monorepo [2/4]",
	}

	t.Run("FilterValue returns repo name", func(t *testing.T) {
		if got := group.FilterValue(); got != "chase-monorepo" {
			t.Errorf("FilterValue() = %q, want %q", got, "chase-monorepo")
		}
	})

	t.Run("Title returns displayName", func(t *testing.T) {
		if got := group.Title(); got != " chase-monorepo [2/4]" {
			t.Errorf("Title() = %q, want %q", got, " chase-monorepo [2/4]")
		}
	})

	t.Run("Description returns empty", func(t *testing.T) {
		if got := group.Description(); got != "" {
			t.Errorf("Description() = %q, want %q", got, "")
		}
	})
}

func TestSessionItemInterface(t *testing.T) {
	item := sessionItem{
		session:     model.SeshSession{Name: "test", Src: "tmux"},
		displayName: " test",
	}

	if got := item.FilterValue(); got != "test" {
		t.Errorf("FilterValue() = %q, want %q", got, "test")
	}
	if got := item.Title(); got != " test" {
		t.Errorf("Title() = %q, want %q", got, " test")
	}
	if got := item.Description(); got != "" {
		t.Errorf("Description() = %q, want %q", got, "")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestWorktreeGroupItemInterface ./tui/...`
Expected: FAIL — `worktreeGroupItem` undefined

**Step 3: Write minimal implementation**

Add to `tui/item.go`:

```go
// worktreeGroupItem represents a collapsed group of worktrees in the list
type worktreeGroupItem struct {
	repoName     string
	dormantCount int           // worktrees without active tmux session
	totalCount   int           // total worktrees in group
	worktrees    []sessionItem // all worktree items in this group
	displayName  string        // pre-computed with icon and badge
}

func (g worktreeGroupItem) Title() string       { return g.displayName }
func (g worktreeGroupItem) Description() string  { return "" }
func (g worktreeGroupItem) FilterValue() string  { return g.repoName }
```

**Step 4: Run tests to verify they pass**

Run: `go test -run "TestWorktreeGroupItemInterface|TestSessionItemInterface" ./tui/...`
Expected: PASS

---

### Task 2: Add worktree grouping logic

**Files:**
- Create: `tui/grouping.go`
- Test: `tui/grouping_test.go` (create)

**Step 1: Write the failing tests**

Create `tui/grouping_test.go`:

```go
package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/joshmedeski/sesh/v2/model"
)

func TestBuildWorktreeGroups(t *testing.T) {
	t.Run("identifies worktree groups from projects", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "chase-monorepo", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "chase-monorepo/develop", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "chase-monorepo/feature-cdk", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "chase-monorepo/review", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "other-project", Src: "projects"}},
		}

		groups := buildWorktreeGroups(items)

		if len(groups) != 1 {
			t.Fatalf("expected 1 group, got %d", len(groups))
		}
		group, ok := groups["chase-monorepo"]
		if !ok {
			t.Fatal("expected group 'chase-monorepo'")
		}
		if len(group.worktrees) != 3 {
			t.Errorf("expected 3 worktrees, got %d", len(group.worktrees))
		}
	})

	t.Run("tracks active tmux sessions", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "geoip/develop", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "geoip/develop", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "geoip/feature-x", Src: "projects"}},
		}

		groups := buildWorktreeGroups(items)

		group := groups["geoip"]
		if group == nil {
			t.Fatal("expected group 'geoip'")
		}
		if len(group.tmuxNames) != 1 {
			t.Errorf("expected 1 active tmux session, got %d", len(group.tmuxNames))
		}
		if !group.tmuxNames["geoip/develop"] {
			t.Error("expected geoip/develop to be active")
		}
	})

	t.Run("no groups when no worktrees", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "sesh", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "dotfiles", Src: "config"}},
			sessionItem{session: model.SeshSession{Name: "myproject", Src: "projects"}},
		}

		groups := buildWorktreeGroups(items)

		if len(groups) != 0 {
			t.Errorf("expected 0 groups, got %d", len(groups))
		}
	})

	t.Run("multiple repos each get their own group", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo-a/main", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "repo-a/develop", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "repo-b/main", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "repo-b/feature", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "repo-b/hotfix", Src: "projects"}},
		}

		groups := buildWorktreeGroups(items)

		if len(groups) != 2 {
			t.Fatalf("expected 2 groups, got %d", len(groups))
		}
		if len(groups["repo-a"].worktrees) != 2 {
			t.Errorf("repo-a: expected 2 worktrees, got %d", len(groups["repo-a"].worktrees))
		}
		if len(groups["repo-b"].worktrees) != 3 {
			t.Errorf("repo-b: expected 3 worktrees, got %d", len(groups["repo-b"].worktrees))
		}
	})

	t.Run("ignores non-projects worktree-like names", func(t *testing.T) {
		items := []list.Item{
			// tmux session with "/" in name — NOT a project worktree for grouping purposes
			sessionItem{session: model.SeshSession{Name: "geoip/develop", Src: "tmux"}},
			// config with "/" — not a worktree
			sessionItem{session: model.SeshSession{Name: "igorpetrovic/dotfiles", Src: "config"}},
		}

		groups := buildWorktreeGroups(items)

		if len(groups) != 0 {
			t.Errorf("expected 0 groups (only projects source creates groups), got %d", len(groups))
		}
	})

	t.Run("single worktree does not create group", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}},
		}

		groups := buildWorktreeGroups(items)

		if len(groups) != 0 {
			t.Errorf("expected 0 groups (single worktree = no group), got %d", len(groups))
		}
	})
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestBuildWorktreeGroups ./tui/...`
Expected: FAIL — `buildWorktreeGroups` undefined

**Step 3: Write minimal implementation**

Create `tui/grouping.go`:

```go
package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
)

// worktreeGroup holds metadata about a group of worktrees from the same repo
type worktreeGroup struct {
	repoName  string
	worktrees []sessionItem
	tmuxNames map[string]bool // active tmux session names matching this repo's worktrees
}

// buildWorktreeGroups identifies worktree clusters from a flat session list.
// Only projects-source items with "/" in the name are considered worktrees.
// Groups with only 1 worktree are excluded (no benefit to collapsing).
func buildWorktreeGroups(items []list.Item) map[string]*worktreeGroup {
	// First pass: collect active tmux session names
	tmuxSessions := make(map[string]bool)
	for _, item := range items {
		si, ok := item.(sessionItem)
		if !ok {
			continue
		}
		if si.session.Src == "tmux" {
			tmuxSessions[si.session.Name] = true
		}
	}

	// Second pass: identify worktree groups from projects source
	groups := make(map[string]*worktreeGroup)
	for _, item := range items {
		si, ok := item.(sessionItem)
		if !ok {
			continue
		}
		if si.session.Src != "projects" {
			continue
		}
		if !strings.Contains(si.session.Name, "/") {
			continue
		}

		repoName := strings.SplitN(si.session.Name, "/", 2)[0]
		if groups[repoName] == nil {
			groups[repoName] = &worktreeGroup{
				repoName:  repoName,
				tmuxNames: make(map[string]bool),
			}
		}
		groups[repoName].worktrees = append(groups[repoName].worktrees, si)

		if tmuxSessions[si.session.Name] {
			groups[repoName].tmuxNames[si.session.Name] = true
		}
	}

	// Remove groups with only 1 worktree (no benefit to collapsing)
	for name, group := range groups {
		if len(group.worktrees) <= 1 {
			delete(groups, name)
		}
	}

	return groups
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -run TestBuildWorktreeGroups ./tui/...`
Expected: PASS

---

### Task 3: Add buildDisplayItems and formatGroupDisplay

**Files:**
- Modify: `tui/grouping.go`
- Modify: `tui/grouping_test.go`

**Step 1: Write the failing tests**

Add to `tui/grouping_test.go`:

```go
func TestFormatGroupDisplay(t *testing.T) {
	t.Run("formats with dormant and total counts", func(t *testing.T) {
		got := formatGroupDisplay("chase-monorepo", 2, 4)
		// Should contain repo name and [2/4] badge
		if !strings.Contains(got, "chase-monorepo") {
			t.Errorf("expected repo name in display, got %q", got)
		}
		if !strings.Contains(got, "[2/4]") {
			t.Errorf("expected [2/4] badge in display, got %q", got)
		}
	})
}

func TestBuildDisplayItems(t *testing.T) {
	t.Run("collapses worktree groups", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "sesh", Src: "tmux"}, displayName: " sesh"},
			sessionItem{session: model.SeshSession{Name: "chase-monorepo/develop", Src: "projects"}, displayName: " chase-monorepo ⎇ develop"},
			sessionItem{session: model.SeshSession{Name: "chase-monorepo/feature-cdk", Src: "projects"}, displayName: " chase-monorepo ⎇ feature-cdk"},
			sessionItem{session: model.SeshSession{Name: "chase-monorepo/review", Src: "projects"}, displayName: " chase-monorepo ⎇ review"},
			sessionItem{session: model.SeshSession{Name: "other-project", Src: "projects"}, displayName: " other-project"},
		}

		groups := buildWorktreeGroups(items)
		display := buildDisplayItems(items, groups, "")

		// Should have: sesh, chase-monorepo group, other-project = 3 items
		if len(display) != 3 {
			t.Fatalf("expected 3 items, got %d", len(display))
		}

		// First item: tmux session
		if si, ok := display[0].(sessionItem); !ok || si.session.Name != "sesh" {
			t.Errorf("expected first item to be 'sesh' sessionItem")
		}

		// Second item: group
		gi, ok := display[1].(worktreeGroupItem)
		if !ok {
			t.Fatalf("expected second item to be worktreeGroupItem, got %T", display[1])
		}
		if gi.repoName != "chase-monorepo" {
			t.Errorf("expected repoName 'chase-monorepo', got %q", gi.repoName)
		}
		if gi.dormantCount != 3 {
			t.Errorf("expected dormantCount 3, got %d", gi.dormantCount)
		}
		if gi.totalCount != 3 {
			t.Errorf("expected totalCount 3, got %d", gi.totalCount)
		}

		// Third item: other-project
		if si, ok := display[2].(sessionItem); !ok || si.session.Name != "other-project" {
			t.Errorf("expected third item to be 'other-project' sessionItem")
		}
	})

	t.Run("hides group when all worktrees are active", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "geoip/develop", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "geoip/feature", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "geoip/develop", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "geoip/feature", Src: "projects"}},
		}

		groups := buildWorktreeGroups(items)
		display := buildDisplayItems(items, groups, "")

		// Only tmux sessions should remain, group is hidden
		if len(display) != 2 {
			t.Fatalf("expected 2 items (tmux only), got %d", len(display))
		}
		for _, item := range display {
			si, ok := item.(sessionItem)
			if !ok {
				t.Error("expected only sessionItems (no group)")
			}
			if si.session.Src != "tmux" {
				t.Errorf("expected tmux source, got %q", si.session.Src)
			}
		}
	})

	t.Run("expands selected group", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}, displayName: " repo ⎇ main"},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "projects"}, displayName: " repo ⎇ develop"},
			sessionItem{session: model.SeshSession{Name: "repo/feature", Src: "projects"}, displayName: " repo ⎇ feature"},
		}

		groups := buildWorktreeGroups(items)
		display := buildDisplayItems(items, groups, "repo") // "repo" is expanded

		// Should have: group header + 3 worktrees = 4 items
		if len(display) != 4 {
			t.Fatalf("expected 4 items, got %d", len(display))
		}

		// First: group header
		if _, ok := display[0].(worktreeGroupItem); !ok {
			t.Error("expected first item to be worktreeGroupItem")
		}

		// Rest: session items
		for i := 1; i < 4; i++ {
			if _, ok := display[i].(sessionItem); !ok {
				t.Errorf("expected item %d to be sessionItem", i)
			}
		}
	})

	t.Run("expanded group only shows dormant worktrees", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}, displayName: " repo ⎇ main"},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "projects"}, displayName: " repo ⎇ develop"},
			sessionItem{session: model.SeshSession{Name: "repo/feature", Src: "projects"}, displayName: " repo ⎇ feature"},
		}

		groups := buildWorktreeGroups(items)
		display := buildDisplayItems(items, groups, "repo")

		// Should have: tmux session + group header + 2 dormant worktrees = 4 items
		// (repo/main is active, so not shown under expanded group)
		if len(display) != 4 {
			t.Fatalf("expected 4 items, got %d", len(display))
		}

		// Verify no dormant duplicate of the active session
		dormantNames := []string{}
		for i := 2; i < len(display); i++ {
			if si, ok := display[i].(sessionItem); ok {
				dormantNames = append(dormantNames, si.session.Name)
			}
		}
		for _, name := range dormantNames {
			if name == "repo/main" {
				t.Error("repo/main should not appear as dormant — it has active tmux session")
			}
		}
	})

	t.Run("counts reflect active sessions", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "repo/feature", Src: "projects"}},
		}

		groups := buildWorktreeGroups(items)
		display := buildDisplayItems(items, groups, "")

		// Find the group item
		for _, item := range display {
			if gi, ok := item.(worktreeGroupItem); ok {
				if gi.dormantCount != 2 {
					t.Errorf("expected dormantCount 2, got %d", gi.dormantCount)
				}
				if gi.totalCount != 3 {
					t.Errorf("expected totalCount 3, got %d", gi.totalCount)
				}
			}
		}
	})
}
```

Add `"strings"` to the imports in `tui/grouping_test.go`.

**Step 2: Run tests to verify they fail**

Run: `go test -run "TestFormatGroupDisplay|TestBuildDisplayItems" ./tui/...`
Expected: FAIL — functions undefined

**Step 3: Write minimal implementation**

Add to `tui/grouping.go`:

```go
import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
)

// formatGroupDisplay creates the display string for a collapsed worktree group.
// Format: " repoName [dormant/total]" with dimmed badge
func formatGroupDisplay(repoName string, dormant, total int) string {
	icon := fmt.Sprintf("\033[32m%s\033[39m", "") // green folder icon (same as projects)
	badge := fmt.Sprintf("\033[240m[%d/%d]\033[39m", dormant, total) // dimmed gray badge
	return fmt.Sprintf("%s %s %s", icon, repoName, badge)
}

// buildDisplayItems creates the list items for display, collapsing worktree groups.
// expandedGroup is the repo name of the currently expanded group ("" = all collapsed).
func buildDisplayItems(items []list.Item, groups map[string]*worktreeGroup, expandedGroup string) []list.Item {
	result := make([]list.Item, 0, len(items))
	insertedGroups := make(map[string]bool)

	for _, item := range items {
		si, ok := item.(sessionItem)
		if !ok {
			result = append(result, item)
			continue
		}

		// Check if this is a project worktree belonging to a group
		if si.session.Src == "projects" && strings.Contains(si.session.Name, "/") {
			repoName := strings.SplitN(si.session.Name, "/", 2)[0]
			group, isGrouped := groups[repoName]
			if !isGrouped {
				// Single worktree or not grouped — show normally
				result = append(result, item)
				continue
			}

			if insertedGroups[repoName] {
				// Already handled this group
				continue
			}
			insertedGroups[repoName] = true

			dormant := len(group.worktrees) - len(group.tmuxNames)
			total := len(group.worktrees)

			// Hide group entirely if all worktrees are active
			if dormant == 0 {
				continue
			}

			groupItem := worktreeGroupItem{
				repoName:     repoName,
				dormantCount: dormant,
				totalCount:   total,
				worktrees:    group.worktrees,
				displayName:  formatGroupDisplay(repoName, dormant, total),
			}

			result = append(result, groupItem)

			// If this group is expanded, show dormant worktrees below
			if expandedGroup == repoName {
				for _, wt := range group.worktrees {
					if !group.tmuxNames[wt.session.Name] {
						result = append(result, wt)
					}
				}
			}

			continue
		}

		result = append(result, item)
	}

	return result
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -run "TestFormatGroupDisplay|TestBuildDisplayItems|TestBuildWorktreeGroups" ./tui/...`
Expected: PASS

---

### Task 4: Update delegate to render worktreeGroupItem

**Files:**
- Modify: `tui/delegate.go`

**Step 1: Update delegate struct**

Add `expandedGroup *string` field to `compactDelegate`:

```go
type compactDelegate struct {
	processInfo   *map[string]string
	expandedGroup *string
}
```

**Step 2: Update Render to handle both item types**

Replace the `Render` method in `tui/delegate.go`:

```go
func (d compactDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	var str string
	var nodeIndicator string

	switch v := item.(type) {
	case sessionItem:
		str = v.displayName

		// Add process indicator if available
		if d.processInfo != nil {
			if process, ok := (*d.processInfo)[v.session.Name]; ok && process == "node" {
				nodeIndicator = nodeIndicatorStr
			}
		}

		// Indent if this is an expanded worktree child
		if d.expandedGroup != nil && *d.expandedGroup != "" &&
			v.session.Src == "projects" && strings.Contains(v.session.Name, "/") {
			repoName := strings.SplitN(v.session.Name, "/", 2)[0]
			if repoName == *d.expandedGroup {
				str = "  " + str // 2-space indent for child worktrees
			}
		}

	case worktreeGroupItem:
		str = v.displayName

	default:
		return
	}

	// Highlight selected item
	if index == m.Index() {
		str = selectedItemStyle.Render("❯ " + str + nodeIndicator)
	} else {
		str = "  " + str + nodeIndicator
	}

	fmt.Fprint(w, str)
}
```

Add `"strings"` to imports in `delegate.go`.

**Step 3: Verify build compiles**

Run: `go build ./tui/...`
Expected: Build succeeds (delegate references `expandedGroup` field which we'll wire up in Task 5)

---

### Task 5: Wire up grouping in model

**Files:**
- Modify: `tui/model.go`

This task wires the grouping logic into the model, modifying `newModel` and the `SessionsLoadedMsg` handler.

**Step 1: Add new fields to Model struct**

In `tui/model.go`, add to the `Model` struct:

```go
type Model struct {
	// ... existing fields ...

	allItems       []list.Item              // Full item list (no groups)
	worktreeGroups map[string]*worktreeGroup // Grouped worktrees
	expandedGroup  string                   // Currently expanded group ("" = all collapsed)
}
```

**Step 2: Update newModel to build groups**

In `newModel`, after building `items` and calling `partitionItemsByTmux`, add grouping:

```go
	// Partition items so tmux sessions appear first
	items = partitionItemsByTmux(items)

	// Build worktree groups and create collapsed display items
	worktreeGroups := buildWorktreeGroups(items)
	displayItems := buildDisplayItems(items, worktreeGroups, "")
```

Update the model initialization to store `allItems` and `worktreeGroups`:

```go
	m := Model{
		sessions:       sessions,
		width:          80,
		height:         24,
		currentFilter:  FilterAll,
		previewContent: "",
		pendingPreview: "",
		lastPreviewKey: "",
		processInfo:    make(map[string]string),
		allItems:       items,
		worktreeGroups: worktreeGroups,
		expandedGroup:  "",
	}
```

Update the delegate to receive `expandedGroup` pointer:

```go
	delegate := compactDelegate{processInfo: &m.processInfo, expandedGroup: &m.expandedGroup}
```

Use `displayItems` instead of `items` for the list:

```go
	l := list.New(displayItems, delegate, listWidth, 24)
```

Update the filter to use `displayItems`:

```go
	l.Filter = tmuxFirstFilter(displayItems)
```

**Step 3: Update SessionsLoadedMsg handler**

In `tui/update.go`, in the `SessionsLoadedMsg` case, after building and partitioning items:

```go
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

		// Build worktree groups
		m.allItems = items
		m.worktreeGroups = buildWorktreeGroups(items)
		m.expandedGroup = ""

		// Use collapsed display items
		displayItems := buildDisplayItems(items, m.worktreeGroups, "")
		m.list.SetItems(displayItems)

		// Update filter function with new items
		m.list.Filter = tmuxFirstFilter(displayItems)
```

The rest of the `SessionsLoadedMsg` handler stays the same (title update, state preservation, preview loading).

**Step 4: Verify build and existing tests pass**

Run: `go build ./tui/... && go test ./tui/...`
Expected: Build succeeds, all existing tests pass

---

### Task 6: Handle Enter on worktreeGroupItem (expand)

**Files:**
- Modify: `tui/update.go`

**Step 1: Update Enter/Select handler**

In the `key.Matches(msg, m.keys.Select)` case in `tui/update.go`, handle both item types:

```go
		case key.Matches(msg, m.keys.Select):
			switch item := m.list.SelectedItem().(type) {
			case sessionItem:
				m.selected = item.session.Name
				return m, tea.Quit
			case worktreeGroupItem:
				// Expand this group (accordion: auto-collapses any other)
				m.expandedGroup = item.repoName
				displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, m.expandedGroup)
				m.list.SetItems(displayItems)
				m.list.Filter = tmuxFirstFilter(displayItems)

				// Position cursor on first worktree in expanded group
				for i, listItem := range displayItems {
					if si, ok := listItem.(sessionItem); ok {
						if si.session.Src == "projects" && strings.Contains(si.session.Name, "/") {
							repoName := strings.SplitN(si.session.Name, "/", 2)[0]
							if repoName == item.repoName {
								m.list.Select(i)
								return m.loadPreviewDebounced(si)
							}
						}
					}
				}
				return m, nil
			}
```

**Step 2: Verify build compiles**

Run: `go build ./tui/...`
Expected: Build succeeds

---

### Task 7: Handle Escape to collapse expanded group

**Files:**
- Modify: `tui/update.go`

**Step 1: Update Quit/Escape handler**

The current Quit binding uses `ctrl+c`, `ctrl+b`, `esc`. When a group is expanded, Escape should collapse it first instead of quitting.

Replace the Quit case:

```go
		case key.Matches(msg, m.keys.Quit):
			// If a group is expanded, collapse it first instead of quitting
			if m.expandedGroup != "" {
				// Find the group item's position to return cursor there
				groupRepo := m.expandedGroup
				m.expandedGroup = ""
				displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, "")
				m.list.SetItems(displayItems)
				m.list.Filter = tmuxFirstFilter(displayItems)

				// Position cursor on the collapsed group
				for i, item := range displayItems {
					if gi, ok := item.(worktreeGroupItem); ok && gi.repoName == groupRepo {
						m.list.Select(i)
						break
					}
				}
				return m, nil
			}
			return m, tea.Quit
```

**Step 2: Verify build compiles**

Run: `go build ./tui/...`
Expected: Build succeeds

---

### Task 8: Item swapping on filter text transition

**Files:**
- Modify: `tui/update.go`

When the user starts typing (filter text goes from empty to non-empty), swap to `allItems` so fuzzy matching can find individual worktrees. When filter text is cleared (non-empty to empty), swap back to collapsed display items.

**Step 1: Update filter change detection**

In the filter text change detection block at the bottom of the `tea.KeyMsg` case (around line 379), add item swapping:

```go
		// For all other cases, delegate to list
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)

		// If filter text changed, handle item swapping and cursor reset
		currentFilter := m.list.FilterValue()
		if currentFilter != m.lastFilter {
			prevFilter := m.lastFilter
			m.lastFilter = currentFilter

			// Skip all this if we're restoring state
			if !m.restoringState {
				// Transition: empty → non-empty (start typing)
				if prevFilter == "" && currentFilter != "" {
					// Swap to full items for fuzzy search
					m.expandedGroup = ""
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
				// Load preview for top item
				if item, ok := m.list.SelectedItem().(sessionItem); ok {
					newModel, previewCmd := m.loadPreviewDebounced(item)
					m = newModel
					return m, tea.Batch(cmd, previewCmd)
				}
			}
		}

		return m, cmd
```

**Step 2: Verify build compiles**

Run: `go build ./tui/...`
Expected: Build succeeds

---

### Task 9: Filter mode repo grouping

**Files:**
- Modify: `tui/model.go`
- Modify: `tui/filter_test.go`

**Step 1: Write failing tests**

Add to `tui/filter_test.go`:

```go
func TestTmuxFirstFilterGroupsByRepo(t *testing.T) {
	items := []list.Item{
		sessionItem{session: model.SeshSession{Name: "chase-monorepo", Src: "tmux"}, displayName: "chase-monorepo"},
		sessionItem{session: model.SeshSession{Name: "frontend-monorepo", Src: "projects"}, displayName: "frontend-monorepo"},
		sessionItem{session: model.SeshSession{Name: "frontend-monorepo/develop", Src: "projects"}, displayName: "frontend-monorepo/develop"},
		sessionItem{session: model.SeshSession{Name: "chase-monorepo/feature-cdk", Src: "tmux"}, displayName: "chase-monorepo/feature-cdk"},
		sessionItem{session: model.SeshSession{Name: "chase-monorepo/review", Src: "projects"}, displayName: "chase-monorepo/review"},
		sessionItem{session: model.SeshSession{Name: "chase-monorepo/develop", Src: "projects"}, displayName: "chase-monorepo/develop"},
	}

	targets := make([]string, len(items))
	for i, item := range items {
		targets[i] = item.(sessionItem).FilterValue()
	}

	filter := tmuxFirstFilter(items)
	ranks := filter("monorepo", targets)

	if len(ranks) == 0 {
		t.Fatal("expected results")
	}

	// Verify grouping: items from same repo should be adjacent
	type result struct {
		name string
		repo string
	}
	results := make([]result, 0, len(ranks))
	for _, rank := range ranks {
		si := items[rank.Index].(sessionItem)
		repo := si.session.Name
		if strings.Contains(repo, "/") {
			repo = strings.SplitN(repo, "/", 2)[0]
		}
		results = append(results, result{name: si.session.Name, repo: repo})
	}

	// Check that once we leave a repo group, we don't come back to it
	seen := make(map[string]bool)
	lastRepo := ""
	for _, r := range results {
		if r.repo != lastRepo {
			if seen[r.repo] {
				t.Errorf("repo %q appeared again after leaving — results not grouped. Order: %v", r.repo, results)
				break
			}
			seen[r.repo] = true
			lastRepo = r.repo
		}
	}
}
```

Add `"strings"` to imports in `filter_test.go`.

**Step 2: Run test to verify it fails**

Run: `go test -run TestTmuxFirstFilterGroupsByRepo ./tui/...`
Expected: FAIL — results are scattered, not grouped by repo

**Step 3: Update tmuxFirstFilter to group by repo**

Replace `tmuxFirstFilter` in `tui/model.go`:

```go
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
			bestScore  int // lowest index in original ranks = highest relevance
		}

		groupMap := make(map[string]*repoGroup)
		groupOrder := make([]string, 0) // preserve first-seen order by score

		for i, rank := range ranks {
			if rank.Index >= len(items) {
				continue
			}
			item, ok := items[rank.Index].(sessionItem)
			if !ok {
				continue
			}

			// Determine repo name
			repo := item.session.Name
			if strings.Contains(repo, "/") {
				repo = strings.SplitN(repo, "/", 2)[0]
			}

			g, exists := groupMap[repo]
			if !exists {
				g = &repoGroup{repo: repo, bestScore: i}
				groupMap[repo] = g
				groupOrder = append(groupOrder, repo)
			}

			if item.session.Src == "tmux" {
				g.tmuxRanks = append(g.tmuxRanks, rank)
			} else {
				g.otherRanks = append(g.otherRanks, rank)
			}
		}

		// Sort groups by best score (first-seen order is already by score
		// since ranks come pre-sorted from DefaultFilter)
		// groupOrder is already in the right order.

		// Build result: for each group, tmux first then others
		result := make([]list.Rank, 0, len(ranks))
		for _, repo := range groupOrder {
			g := groupMap[repo]
			result = append(result, g.tmuxRanks...)
			result = append(result, g.otherRanks...)
		}

		return result
	}
}
```

**Step 4: Run all filter tests to verify they pass**

Run: `go test -run "TestTmuxFirstFilter" ./tui/...`
Expected: PASS

**Important:** Review existing `TestTmuxFirstFilter` tests — some may need adjustment since grouping changes the result order. The key invariant remains: tmux sessions for a given repo come before non-tmux for that same repo. But the *global* tmux-first ordering may change to per-repo tmux-first ordering. Review carefully and adjust expected values if needed.

---

### Task 10: Integration testing and edge cases

**Files:**
- Modify: `tui/grouping_test.go`

**Step 1: Add edge case tests**

Add to `tui/grouping_test.go`:

```go
func TestBuildDisplayItemsEdgeCases(t *testing.T) {
	t.Run("single worktree shows normally (no group)", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}, displayName: " repo ⎇ main"},
		}

		groups := buildWorktreeGroups(items)
		display := buildDisplayItems(items, groups, "")

		if len(display) != 1 {
			t.Fatalf("expected 1 item, got %d", len(display))
		}
		if _, ok := display[0].(sessionItem); !ok {
			t.Error("single worktree should show as sessionItem, not group")
		}
	})

	t.Run("non-worktree projects pass through unchanged", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "myproject", Src: "projects"}, displayName: " myproject"},
			sessionItem{session: model.SeshSession{Name: "another", Src: "config"}, displayName: " another"},
		}

		groups := buildWorktreeGroups(items)
		display := buildDisplayItems(items, groups, "")

		if len(display) != 2 {
			t.Fatalf("expected 2 items, got %d", len(display))
		}
	})

	t.Run("mixed sources preserved around groups", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "sesh", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "dotfiles", Src: "config"}},
			sessionItem{session: model.SeshSession{Name: "repo/a", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "repo/b", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "mydir", Src: "zoxide"}},
		}

		groups := buildWorktreeGroups(items)
		display := buildDisplayItems(items, groups, "")

		// sesh, dotfiles, repo group, mydir = 4 items
		if len(display) != 4 {
			t.Fatalf("expected 4 items, got %d", len(display))
		}

		// Verify order
		names := []string{}
		for _, item := range display {
			switch v := item.(type) {
			case sessionItem:
				names = append(names, v.session.Name)
			case worktreeGroupItem:
				names = append(names, v.repoName+"[group]")
			}
		}

		expected := []string{"sesh", "dotfiles", "repo[group]", "mydir"}
		for i, name := range names {
			if name != expected[i] {
				t.Errorf("position %d: expected %q, got %q", i, expected[i], name)
			}
		}
	})

	t.Run("partial active hides active from expanded list", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}, displayName: " repo ⎇ main"},
			sessionItem{session: model.SeshSession{Name: "repo/dev", Src: "projects"}, displayName: " repo ⎇ dev"},
			sessionItem{session: model.SeshSession{Name: "repo/feat", Src: "projects"}, displayName: " repo ⎇ feat"},
		}

		groups := buildWorktreeGroups(items)
		display := buildDisplayItems(items, groups, "repo")

		// tmux repo/main + group header + 2 dormant = 4
		if len(display) != 4 {
			t.Fatalf("expected 4 items, got %d", len(display))
		}

		// Verify the expanded items don't include repo/main
		expandedNames := []string{}
		for _, item := range display {
			if si, ok := item.(sessionItem); ok && si.session.Src == "projects" {
				expandedNames = append(expandedNames, si.session.Name)
			}
		}
		if len(expandedNames) != 2 {
			t.Fatalf("expected 2 expanded worktrees, got %d: %v", len(expandedNames), expandedNames)
		}
	})
}
```

**Step 2: Run all tests**

Run: `go test -v ./tui/...`
Expected: ALL PASS

**Step 3: Build the full binary and test manually**

Run: `CGO_ENABLED=1 go build -ldflags "-X 'main.version=$(git describe --tags --abbrev=0)'" -o /Users/igorpetrovic/Projects/sesh/bin/sesh`

Manual testing checklist:
- [ ] Open sesh TUI: worktree repos appear collapsed with `[N/M]` badge
- [ ] Select a collapsed group: expands to show dormant worktrees
- [ ] Press Escape: group collapses back
- [ ] Expand group A, then select group B: A collapses, B expands (accordion)
- [ ] Start typing: all items appear, grouped by repo
- [ ] Clear filter: collapsed view returns
- [ ] All worktrees active for a repo: no group entry shown
- [ ] Single worktree repo: no group, shows normally
- [ ] Select expanded worktree: connects to session

---

## Notes for Implementation

### Existing test adjustments

The changes to `tmuxFirstFilter` (Task 9) change the sort order from "all tmux first" to "grouped by repo, tmux first within each group". Some existing tests in `filter_test.go` may need updated expected values. The key invariant to preserve: **within a repo group, tmux comes before non-tmux.**

### File checklist

| File | Tasks |
|------|-------|
| `tui/item.go` | Task 1 |
| `tui/item_test.go` (new) | Task 1 |
| `tui/grouping.go` (new) | Tasks 2, 3 |
| `tui/grouping_test.go` (new) | Tasks 2, 3, 10 |
| `tui/delegate.go` | Task 4 |
| `tui/model.go` | Tasks 5, 9 |
| `tui/update.go` | Tasks 6, 7, 8 |
| `tui/filter_test.go` | Task 9 |

### Build command

After all changes:
```bash
rm /Users/igorpetrovic/Projects/sesh/bin/sesh
CGO_ENABLED=1 go build -ldflags "-X 'main.version=$(git describe --tags --abbrev=0)'" -o /Users/igorpetrovic/Projects/sesh/bin/sesh
chmod +x /Users/igorpetrovic/Projects/sesh/bin/sesh
```
