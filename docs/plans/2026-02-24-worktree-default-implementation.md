# Worktree Default & Smart Grouping Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Worktree groups show a user-picked default branch as the title. Enter opens the default directly. Tab expands to browse siblings. Ctrl+P sets defaults. Ctrl+T focuses on a single repo's worktrees.

**Architecture:** New `state/` package handles persistent defaults storage (JSON file in XDG_STATE_HOME). Grouping logic updated to accept defaults map and produce new title format. Key bindings reorganized: Tab for expand/collapse, Ctrl+P for set default, Ctrl+T for repo focus, Ctrl+E for process detection.

**Tech Stack:** Go 1.23, Bubble Tea, bubbles/list, lipgloss, encoding/json, os (XDG paths)

---

### Task 1: State Package — Defaults Read/Write

**Files:**
- Create: `state/defaults.go`
- Create: `state/defaults_test.go`

**Context:** This package manages persistent worktree defaults stored as a JSON file at `$XDG_STATE_HOME/sesh/worktree-defaults.json` (fallback: `~/.local/state/sesh/worktree-defaults.json`). The file is a flat map: `{"chase-monorepo": "develop", "geoip": "main"}`.

**Step 1: Write the failing tests**

In `state/defaults_test.go`:

```go
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Run("returns empty map when file does not exist", func(t *testing.T) {
		defaults, err := LoadDefaults("/nonexistent/path/defaults.json")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(defaults) != 0 {
			t.Errorf("expected empty map, got %d entries", len(defaults))
		}
	})

	t.Run("loads valid JSON file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "defaults.json")
		data := map[string]string{"chase-monorepo": "develop", "geoip": "main"}
		b, _ := json.Marshal(data)
		os.WriteFile(path, b, 0644)

		defaults, err := LoadDefaults(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if defaults["chase-monorepo"] != "develop" {
			t.Errorf("expected 'develop', got %q", defaults["chase-monorepo"])
		}
		if defaults["geoip"] != "main" {
			t.Errorf("expected 'main', got %q", defaults["geoip"])
		}
	})

	t.Run("returns empty map for corrupted file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "defaults.json")
		os.WriteFile(path, []byte("not json"), 0644)

		defaults, err := LoadDefaults(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(defaults) != 0 {
			t.Errorf("expected empty map for corrupted file, got %d entries", len(defaults))
		}
	})
}

func TestSaveDefaults(t *testing.T) {
	t.Run("writes JSON file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "defaults.json")
		defaults := map[string]string{"repo-a": "main", "repo-b": "dev"}

		err := SaveDefaults(path, defaults)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		b, _ := os.ReadFile(path)
		var loaded map[string]string
		json.Unmarshal(b, &loaded)
		if loaded["repo-a"] != "main" || loaded["repo-b"] != "dev" {
			t.Errorf("loaded data doesn't match: %v", loaded)
		}
	})

	t.Run("creates parent directories", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "nested", "deep", "defaults.json")

		err := SaveDefaults(path, map[string]string{"x": "y"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Error("expected file to exist")
		}
	})
}

func TestDefaultsPath(t *testing.T) {
	t.Run("uses XDG_STATE_HOME when set", func(t *testing.T) {
		path := DefaultsPath("/custom/state")
		expected := "/custom/state/sesh/worktree-defaults.json"
		if path != expected {
			t.Errorf("expected %q, got %q", expected, path)
		}
	})

	t.Run("falls back to home/.local/state", func(t *testing.T) {
		path := DefaultsPath("")
		// Should contain .local/state/sesh
		if filepath.Base(path) != "worktree-defaults.json" {
			t.Errorf("expected filename 'worktree-defaults.json', got %q", filepath.Base(path))
		}
	})
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./state/... -v`
Expected: compilation errors (package doesn't exist yet)

**Step 3: Write the implementation**

In `state/defaults.go`:

```go
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// DefaultsPath returns the path to the worktree defaults JSON file.
// Uses xdgStateHome if non-empty, otherwise falls back to ~/.local/state.
func DefaultsPath(xdgStateHome string) string {
	if xdgStateHome != "" {
		return filepath.Join(xdgStateHome, "sesh", "worktree-defaults.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".local", "state", "sesh", "worktree-defaults.json")
}

// LoadDefaults reads the worktree defaults from the JSON file.
// Returns an empty map if the file doesn't exist or is invalid.
func LoadDefaults(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return make(map[string]string), nil
	}

	var defaults map[string]string
	if err := json.Unmarshal(data, &defaults); err != nil {
		return make(map[string]string), nil
	}

	return defaults, nil
}

// SaveDefaults writes the worktree defaults to the JSON file.
// Creates parent directories if they don't exist.
func SaveDefaults(path string, defaults map[string]string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(defaults, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./state/... -v`
Expected: all PASS

---

### Task 2: Key Binding Updates + Ctrl+E Migration

**Files:**
- Modify: `tui/keys.go`
- Modify: `tui/model.go:184` (remove ctrl+p from CursorUp)
- Modify: `tui/update.go` (update Tab and process detection references)

**Context:** Reorganize key bindings: Tab becomes expand/collapse (handled in task 6), Ctrl+P becomes set default (handled in task 7), Ctrl+T becomes repo focus (handled in task 8). Ctrl+E takes over process detection from Tab. Remove FilterTmux key binding entirely.

**Step 1: Update `tui/keys.go`**

Replace the full `KeyMap` and `DefaultKeyMap`:

```go
package tui

import "github.com/charmbracelet/bubbles/key"

type KeyMap struct {
	Quit             key.Binding
	Select           key.Binding
	FilterAll        key.Binding
	FilterConfig     key.Binding
	FilterZoxide     key.Binding
	ToggleZoxide     key.Binding
	Delete           key.Binding
	GoToWorktreeRoot key.Binding
	DetectProcesses  key.Binding
	ExpandGroup      key.Binding
	SetDefault       key.Binding
	RepoFocus        key.Binding
}

var DefaultKeyMap = KeyMap{
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c", "ctrl+b", "esc"),
		key.WithHelp("ctrl+c/esc", "quit"),
	),
	Select: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	FilterAll: key.NewBinding(
		key.WithKeys("ctrl+a"),
		key.WithHelp("ctrl+a", "all sessions"),
	),
	FilterConfig: key.NewBinding(
		key.WithKeys("ctrl+g"),
		key.WithHelp("ctrl+g", "config only"),
	),
	FilterZoxide: key.NewBinding(
		key.WithKeys("ctrl+x"),
		key.WithHelp("ctrl+x", "zoxide only"),
	),
	ToggleZoxide: key.NewBinding(
		key.WithKeys("ctrl+o"),
		key.WithHelp("ctrl+o", "toggle zoxide"),
	),
	Delete: key.NewBinding(
		key.WithKeys("ctrl+d", "ctrl+k"),
		key.WithHelp("ctrl+d/ctrl+k", "delete session"),
	),
	GoToWorktreeRoot: key.NewBinding(
		key.WithKeys("ctrl+0"),
		key.WithHelp("ctrl+0", "go to worktree root"),
	),
	DetectProcesses: key.NewBinding(
		key.WithKeys("ctrl+e"),
		key.WithHelp("ctrl+e", "detect processes"),
	),
	ExpandGroup: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "expand/collapse group"),
	),
	SetDefault: key.NewBinding(
		key.WithKeys("ctrl+p"),
		key.WithHelp("ctrl+p", "set default worktree"),
	),
	RepoFocus: key.NewBinding(
		key.WithKeys("ctrl+t"),
		key.WithHelp("ctrl+t", "focus repo worktrees"),
	),
}
```

**Step 2: Update `tui/model.go:184` — remove ctrl+p from CursorUp**

Change line 184 from:
```go
listKeys.CursorUp.SetKeys("up", "ctrl+p")
```
to:
```go
listKeys.CursorUp.SetKeys("up")
```

**Step 3: Update `tui/update.go` — migrate Tab→Ctrl+E for process detection**

Replace the Tab handler at lines 256-260:
```go
// Handle Tab key explicitly FIRST (before any other checks)
if msg.Type == tea.KeyTab {
    logDebug("DEBUG: Tab key detected (KeyTab type)")
    return m, detectAllProcesses()
}
```
with:
```go
// Handle Ctrl+E for process detection
if key.Matches(msg, m.keys.DetectProcesses) {
    logDebug("DEBUG: Ctrl+E pressed, triggering process detection")
    return m, detectAllProcesses()
}
```

Remove the `FilterTmux` case (lines 337-340):
```go
case key.Matches(msg, m.keys.FilterTmux):
    m.currentFilter = FilterTmux
    m.lastFilter = "" // Reset filter tracking
    return m, loadSessionsWithFilter(m.lister, FilterTmux)
```

Remove the old DetectProcesses case (lines 410-413):
```go
case key.Matches(msg, m.keys.DetectProcesses):
    logDebug("DEBUG: Tab key pressed, triggering process detection")
    return m, detectAllProcesses()
```

In the filtering mode intercept section (lines 418-448), replace the `"tab"` case:
```go
case "tab":
    // Trigger process detection when Tab is pressed
    logDebug("DEBUG: Tab key pressed in filtering mode, triggering process detection")
    return m, detectAllProcesses()
```
with nothing (remove it entirely — Tab will be handled by ExpandGroup in task 6).

**Step 4: Run tests to verify**

Run: `go test ./tui/... -v`
Expected: all PASS (the FilterTmux constant still exists in messages.go, just the key binding is removed)

---

### Task 3: Grouping Logic for Default Branch

**Files:**
- Modify: `tui/grouping.go`
- Modify: `tui/grouping_test.go`
- Modify: `tui/item.go`

**Context:** Update the grouping logic to accept a defaults map and produce the new title format: `repo/branch (+N)` when default is set, `repo (+N)` when not.

**Step 1: Update `worktreeGroup` struct and `buildWorktreeGroups` signature**

In `tui/grouping.go`, update `worktreeGroup`:

```go
type worktreeGroup struct {
	repoName      string
	defaultBranch string          // from user defaults ("" = no default)
	worktrees     []sessionItem
	tmuxNames     map[string]bool
}
```

Update `buildWorktreeGroups` to accept defaults:

```go
func buildWorktreeGroups(items []list.Item, defaults map[string]string) map[string]*worktreeGroup {
```

After creating the group in the second pass, set `defaultBranch`:

```go
if groups[repoName] == nil {
    groups[repoName] = &worktreeGroup{
        repoName:      repoName,
        defaultBranch: defaults[repoName],
        tmuxNames:     make(map[string]bool),
    }
}
```

**Step 2: Update `formatGroupDisplay` for new title format**

Replace the existing function:

```go
// formatGroupDisplay creates the display string for a collapsed worktree group.
// With default: " repo/branch (+N)" where N is other dormant worktrees
// Without default: " repo (+N)" where N is total dormant worktrees
func formatGroupDisplay(repoName string, defaultBranch string, extraCount int) string {
	icon := fmt.Sprintf("\033[32m%s\033[39m", "")
	if defaultBranch != "" {
		name := repoName + "/" + defaultBranch
		if extraCount > 0 {
			badge := fmt.Sprintf("\033[240m(+%d)\033[39m", extraCount)
			return fmt.Sprintf("%s %s %s", icon, name, badge)
		}
		return fmt.Sprintf("%s %s", icon, name)
	}
	badge := fmt.Sprintf("\033[240m(+%d)\033[39m", extraCount)
	return fmt.Sprintf("%s %s %s", icon, repoName, badge)
}
```

**Step 3: Update `worktreeGroupItem` in `tui/item.go`**

Add `defaultBranch` field:

```go
type worktreeGroupItem struct {
	repoName      string
	defaultBranch string        // "" = no default set
	dormantCount  int
	totalCount    int
	worktrees     []sessionItem
	displayName   string
}
```

**Step 4: Update `buildDisplayItems` to use new format**

In the group item creation section, replace:

```go
groupItem := worktreeGroupItem{
    repoName:     repoName,
    dormantCount: dormant,
    totalCount:   total,
    worktrees:    group.worktrees,
    displayName:  formatGroupDisplay(repoName, dormant, total),
}
```

with:

```go
// Calculate extra count based on whether default is set
extraCount := dormant
if group.defaultBranch != "" {
    // Subtract 1 for the default (shown in title) if it's dormant
    if !group.tmuxNames[repoName+"/"+group.defaultBranch] {
        extraCount = dormant - 1
    }
}

groupItem := worktreeGroupItem{
    repoName:      repoName,
    defaultBranch: group.defaultBranch,
    dormantCount:  dormant,
    totalCount:    total,
    worktrees:     group.worktrees,
    displayName:   formatGroupDisplay(repoName, group.defaultBranch, extraCount),
}
```

**Step 5: Update all callers of `buildWorktreeGroups`**

In `tui/model.go` line 142:
```go
worktreeGroups := buildWorktreeGroups(items, make(map[string]string))
```
(Will be updated with real defaults in Task 4)

In `tui/update.go` line 75:
```go
m.worktreeGroups = buildWorktreeGroups(items, make(map[string]string))
```
(Will be updated with real defaults in Task 4)

**Step 6: Update tests in `tui/grouping_test.go`**

Update all `buildWorktreeGroups` calls to pass empty defaults:
```go
groups := buildWorktreeGroups(items, make(map[string]string))
```

Update `TestFormatGroupDisplay`:
```go
func TestFormatGroupDisplay(t *testing.T) {
	t.Run("format with default branch", func(t *testing.T) {
		got := formatGroupDisplay("chase-monorepo", "develop", 3)
		if !strings.Contains(got, "chase-monorepo/develop") {
			t.Errorf("expected 'chase-monorepo/develop' in display, got %q", got)
		}
		if !strings.Contains(got, "(+3)") {
			t.Errorf("expected (+3) badge, got %q", got)
		}
	})

	t.Run("format without default branch", func(t *testing.T) {
		got := formatGroupDisplay("chase-monorepo", "", 4)
		if !strings.Contains(got, "chase-monorepo") {
			t.Errorf("expected repo name in display, got %q", got)
		}
		if !strings.Contains(got, "(+4)") {
			t.Errorf("expected (+4) badge, got %q", got)
		}
		if strings.Contains(got, "/") {
			t.Errorf("should not contain / when no default, got %q", got)
		}
	})

	t.Run("no badge when extra count is zero", func(t *testing.T) {
		got := formatGroupDisplay("repo", "main", 0)
		if strings.Contains(got, "(+") {
			t.Errorf("should not contain (+N) when count is 0, got %q", got)
		}
	})
}
```

Add test for defaults in `buildWorktreeGroups`:
```go
t.Run("populates default branch from defaults map", func(t *testing.T) {
    items := []list.Item{
        sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}},
        sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "projects"}},
    }

    defaults := map[string]string{"repo": "main"}
    groups := buildWorktreeGroups(items, defaults)

    if groups["repo"].defaultBranch != "main" {
        t.Errorf("expected defaultBranch 'main', got %q", groups["repo"].defaultBranch)
    }
})
```

**Step 7: Run tests**

Run: `go test ./tui/... -v`
Expected: all PASS

---

### Task 4: Model Wiring — Load Defaults and Pass Through

**Files:**
- Modify: `tui/model.go`
- Modify: `tui/tui.go`
- Modify: `tui/update.go`
- Modify: `tui/messages.go`

**Context:** Load defaults at startup, store in model, pass to grouping functions. Add new message type for async save confirmation.

**Step 1: Add new message type in `tui/messages.go`**

Add at the end:
```go
type DefaultsSavedMsg struct {
	Err error // nil on success
}
```

**Step 2: Add fields to `Model` in `tui/model.go`**

Add to the Model struct:
```go
worktreeDefaults map[string]string // repo → default branch (loaded from state file)
defaultsPath     string            // path to defaults JSON file
repoFocusFilter  string            // repo name for Ctrl+T focus ("" = no focus)
```

**Step 3: Update `newModel` to accept defaults**

Change the `newModel` signature:
```go
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
```

Update the call to `buildWorktreeGroups`:
```go
worktreeGroups := buildWorktreeGroups(items, worktreeDefaults)
```

Add the new fields to the Model initialization:
```go
m := Model{
    sessions:         sessions,
    width:            80,
    height:           24,
    currentFilter:    FilterAll,
    previewContent:   "",
    pendingPreview:   "",
    lastPreviewKey:   "",
    processInfo:      make(map[string]string),
    allItems:         items,
    worktreeGroups:   worktreeGroups,
    expandedGroup:    "",
    worktreeDefaults: worktreeDefaults,
    defaultsPath:     defaultsPath,
    repoFocusFilter:  "",
}
```

**Step 4: Update `tui/tui.go` to load defaults at startup**

Add import for the state package and os:
```go
import (
    "os"

    "github.com/charmbracelet/bubbletea"
    "github.com/joshmedeski/sesh/v2/connector"
    "github.com/joshmedeski/sesh/v2/icon"
    "github.com/joshmedeski/sesh/v2/lister"
    "github.com/joshmedeski/sesh/v2/model"
    "github.com/joshmedeski/sesh/v2/previewer"
    "github.com/joshmedeski/sesh/v2/state"
    "github.com/joshmedeski/sesh/v2/tmux"
)
```

In `Run()`, before creating the model:
```go
// Load worktree defaults (sub-millisecond, never fails)
defaultsPath := state.DefaultsPath(os.Getenv("XDG_STATE_HOME"))
worktreeDefaults, _ := state.LoadDefaults(defaultsPath)

m := newModel(t.lister, t.connector, t.icon, t.tmux, t.config, t.previewer, sessions, worktreeDefaults, defaultsPath)
```

**Step 5: Update `SessionsLoadedMsg` handler in `tui/update.go`**

Update the `buildWorktreeGroups` call (line 75) to pass defaults:
```go
m.worktreeGroups = buildWorktreeGroups(items, m.worktreeDefaults)
```

**Step 6: Run tests**

Run: `go test ./tui/... -v && go test ./state/... -v`
Expected: all PASS

---

### Task 5: Enter Handler — Default Opens Session

**Files:**
- Modify: `tui/update.go`

**Context:** When Enter is pressed on a `worktreeGroupItem`:
- If it has a `defaultBranch` → set `m.selected` to `repoName/defaultBranch` and quit (connects to session)
- If no default → expand the group (current behavior)

**Step 1: Update the `worktreeGroupItem` case in the Enter handler**

Replace the existing `worktreeGroupItem` case in `case key.Matches(msg, m.keys.Select):` with:

```go
case worktreeGroupItem:
    if item.defaultBranch != "" {
        // Has default — connect to default worktree directly
        m.selected = item.repoName + "/" + item.defaultBranch
        return m, tea.Quit
    }

    // No default — expand the group
    m.expandedGroup = item.repoName
    displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, m.expandedGroup)

    m.list.ResetFilter()
    m.list.SetItems(displayItems)
    m.list.Filter = tmuxFirstFilter(displayItems)

    // Position cursor on first worktree in expanded group
    var previewCmd tea.Cmd
    for i, listItem := range displayItems {
        if si, ok := listItem.(sessionItem); ok {
            if si.session.Src == "projects" && strings.Contains(si.session.Name, "/") {
                repoName := strings.SplitN(si.session.Name, "/", 2)[0]
                if repoName == item.repoName {
                    m.list.Select(i)
                    var newModel Model
                    newModel, previewCmd = m.loadPreviewDebounced(si)
                    m = newModel
                    break
                }
            }
        }
    }

    filterCmd := func() tea.Msg {
        return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
    }
    return m, tea.Batch(previewCmd, filterCmd)
```

**Step 2: Run tests**

Run: `go test ./tui/... -v`
Expected: all PASS

---

### Task 6: Tab Expand/Collapse Handler

**Files:**
- Modify: `tui/update.go`

**Context:** Tab toggles expand/collapse on worktree groups. When the selected item is a `worktreeGroupItem`, Tab expands it. When a group is already expanded, Tab collapses it. On non-group items, Tab does nothing.

**Step 1: Add Tab handler in the KeyMsg switch**

Add a new case in the `switch` block (after the `DetectProcesses` handler at the top of KeyMsg, before the filtering mode section):

```go
// Handle Tab for group expand/collapse
if key.Matches(msg, m.keys.ExpandGroup) {
    // If a group is expanded, collapse it
    if m.expandedGroup != "" {
        groupRepo := m.expandedGroup
        m.expandedGroup = ""
        displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, "")

        m.list.ResetFilter()
        m.list.SetItems(displayItems)
        m.list.Filter = tmuxFirstFilter(displayItems)

        for i, item := range displayItems {
            if gi, ok := item.(worktreeGroupItem); ok && gi.repoName == groupRepo {
                m.list.Select(i)
                break
            }
        }
        m.previewPort.SetContent("")
        m.previewContent = ""

        return m, func() tea.Msg {
            return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
        }
    }

    // If on a group item, expand it
    if item, ok := m.list.SelectedItem().(worktreeGroupItem); ok {
        m.expandedGroup = item.repoName
        displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, m.expandedGroup)

        m.list.ResetFilter()
        m.list.SetItems(displayItems)
        m.list.Filter = tmuxFirstFilter(displayItems)

        var previewCmd tea.Cmd
        for i, listItem := range displayItems {
            if si, ok := listItem.(sessionItem); ok {
                if si.session.Src == "projects" && strings.Contains(si.session.Name, "/") {
                    repoName := strings.SplitN(si.session.Name, "/", 2)[0]
                    if repoName == item.repoName {
                        m.list.Select(i)
                        var newModel Model
                        newModel, previewCmd = m.loadPreviewDebounced(si)
                        m = newModel
                        break
                    }
                }
            }
        }

        filterCmd := func() tea.Msg {
            return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
        }
        return m, tea.Batch(previewCmd, filterCmd)
    }

    return m, nil
}
```

Also add a `"tab"` case in the filtering mode intercept section to handle Tab during filtering:
```go
case "tab":
    // Same Tab logic as above — delegate to ExpandGroup handler
    // by converting to a key match
    if m.expandedGroup != "" || func() bool {
        _, ok := m.list.SelectedItem().(worktreeGroupItem)
        return ok
    }() {
        // Re-dispatch as ExpandGroup — but simpler to just inline the collapse logic
        if m.expandedGroup != "" {
            groupRepo := m.expandedGroup
            m.expandedGroup = ""
            displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, "")
            m.list.ResetFilter()
            m.list.SetItems(displayItems)
            m.list.Filter = tmuxFirstFilter(displayItems)
            for i, item := range displayItems {
                if gi, ok := item.(worktreeGroupItem); ok && gi.repoName == groupRepo {
                    m.list.Select(i)
                    break
                }
            }
            m.previewPort.SetContent("")
            m.previewContent = ""
            return m, func() tea.Msg {
                return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
            }
        }
    }
    return m, nil
```

**Note:** There is duplication between the Tab handler outside filtering and inside filtering. Consider extracting a helper method `(m Model) toggleGroupExpand() (Model, tea.Cmd)` to DRY this up. The implementer should extract this helper to keep the code clean.

**Step 2: Run tests**

Run: `go test ./tui/... -v`
Expected: all PASS

---

### Task 7: Ctrl+P Set Default Handler

**Files:**
- Modify: `tui/update.go`
- Modify: `tui/commands.go`

**Context:** Ctrl+P on any project worktree sets (or clears) it as the default for its repo. Updates the in-memory map instantly, fires async save to disk. Collapses the group with updated title.

**Step 1: Add async save command in `tui/commands.go`**

```go
// saveDefaults writes worktree defaults to disk asynchronously
func saveDefaults(path string, defaults map[string]string) tea.Cmd {
	return func() tea.Msg {
		// Copy the map to avoid concurrent access
		copied := make(map[string]string, len(defaults))
		for k, v := range defaults {
			copied[k] = v
		}
		err := state.SaveDefaults(path, copied)
		return DefaultsSavedMsg{Err: err}
	}
}
```

Add the import for state package at the top of commands.go:
```go
"github.com/joshmedeski/sesh/v2/state"
```

**Step 2: Add Ctrl+P handler in `tui/update.go`**

Add a new case in the `switch` block (in the key matches section):

```go
case key.Matches(msg, m.keys.SetDefault):
    // Set/clear default worktree for a repo
    if item, ok := m.list.SelectedItem().(sessionItem); ok {
        if item.session.Src == "projects" && strings.Contains(item.session.Name, "/") {
            parts := strings.SplitN(item.session.Name, "/", 2)
            repoName := parts[0]
            branchName := parts[1]

            // Toggle: if already default, clear it
            if m.worktreeDefaults[repoName] == branchName {
                delete(m.worktreeDefaults, repoName)
            } else {
                m.worktreeDefaults[repoName] = branchName
            }

            // Rebuild groups with updated defaults
            m.worktreeGroups = buildWorktreeGroups(m.allItems, m.worktreeDefaults)
            m.expandedGroup = ""
            displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, "")

            m.list.ResetFilter()
            m.list.SetItems(displayItems)
            m.list.Filter = tmuxFirstFilter(displayItems)

            // Position cursor on the updated group
            for i, listItem := range displayItems {
                if gi, ok := listItem.(worktreeGroupItem); ok && gi.repoName == repoName {
                    m.list.Select(i)
                    break
                }
            }
            m.previewPort.SetContent("")
            m.previewContent = ""

            // Re-enter filter mode + async save
            filterCmd := func() tea.Msg {
                return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
            }
            saveCmd := saveDefaults(m.defaultsPath, m.worktreeDefaults)
            return m, tea.Batch(filterCmd, saveCmd)
        }
    }
    return m, nil
```

**Step 3: Add DefaultsSavedMsg handler**

In the main `switch msg := msg.(type)` block, add:
```go
case DefaultsSavedMsg:
    if msg.Err != nil {
        logDebug("DEBUG: Failed to save defaults: %v", msg.Err)
    }
    return m, nil
```

**Step 4: Run tests**

Run: `go test ./tui/... -v`
Expected: all PASS

---

### Task 8: Ctrl+T Repo Focus Handler

**Files:**
- Modify: `tui/update.go`

**Context:** Ctrl+T toggles a repo focus filter. When active, the list shows only worktrees (tmux + projects) belonging to the focused repo. Pressing Ctrl+T again clears the focus.

**Step 1: Add Ctrl+T handler in the key matches section of `tui/update.go`**

```go
case key.Matches(msg, m.keys.RepoFocus):
    // Determine repo name from selected item
    var repoName string
    switch item := m.list.SelectedItem().(type) {
    case sessionItem:
        if strings.Contains(item.session.Name, "/") {
            repoName = strings.SplitN(item.session.Name, "/", 2)[0]
        }
    case worktreeGroupItem:
        repoName = item.repoName
    }

    if repoName == "" {
        return m, nil // Not a worktree item
    }

    // Toggle focus
    if m.repoFocusFilter == repoName {
        // Clear focus — restore normal view
        m.repoFocusFilter = ""
        m.expandedGroup = ""
        displayItems := buildDisplayItems(m.allItems, m.worktreeGroups, "")

        m.list.ResetFilter()
        m.list.SetItems(displayItems)
        m.list.Filter = tmuxFirstFilter(displayItems)

        // Position on the group
        for i, listItem := range displayItems {
            if gi, ok := listItem.(worktreeGroupItem); ok && gi.repoName == repoName {
                m.list.Select(i)
                break
            }
        }

        m.list.Title = getFilterTitle(m.currentFilter)
    } else {
        // Set focus — filter to only this repo's worktrees
        m.repoFocusFilter = repoName
        m.expandedGroup = ""

        focusedItems := make([]list.Item, 0)
        for _, item := range m.allItems {
            if si, ok := item.(sessionItem); ok {
                if strings.Contains(si.session.Name, "/") {
                    itemRepo := strings.SplitN(si.session.Name, "/", 2)[0]
                    if itemRepo == repoName {
                        focusedItems = append(focusedItems, item)
                    }
                } else if si.session.Name == repoName {
                    // Include the bare repo session too
                    focusedItems = append(focusedItems, item)
                }
            }
        }

        m.list.ResetFilter()
        m.list.SetItems(focusedItems)
        m.list.Filter = tmuxFirstFilter(focusedItems)
        if len(focusedItems) > 0 {
            m.list.Select(0)
        }

        m.list.Title = "🔍 " + repoName
    }

    m.previewPort.SetContent("")
    m.previewContent = ""

    filterCmd := func() tea.Msg {
        return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
    }

    var previewCmd tea.Cmd
    if item, ok := m.list.SelectedItem().(sessionItem); ok {
        var newModel Model
        newModel, previewCmd = m.loadPreviewDebounced(item)
        m = newModel
    }

    return m, tea.Batch(filterCmd, previewCmd)
```

**Step 2: Run tests**

Run: `go test ./tui/... -v`
Expected: all PASS

---

### Task 9: Delegate ★ Marker for Default Worktree

**Files:**
- Modify: `tui/delegate.go`

**Context:** When a group is expanded, the default worktree should show a ★ prefix. The delegate needs access to `worktreeDefaults` to know which worktree is the default.

**Step 1: Add worktreeDefaults to delegate**

Update `compactDelegate`:
```go
type compactDelegate struct {
	processInfo      *map[string]string
	expandedGroup    *string
	worktreeDefaults *map[string]string
}
```

**Step 2: Update the Render method**

In the `sessionItem` case, after the existing indent logic for expanded worktrees, add the ★ marker:

```go
// Indent if this is an expanded worktree child
if d.expandedGroup != nil && *d.expandedGroup != "" &&
    v.session.Src == "projects" && strings.Contains(v.session.Name, "/") {
    repoName := strings.SplitN(v.session.Name, "/", 2)[0]
    if repoName == *d.expandedGroup {
        branchName := strings.SplitN(v.session.Name, "/", 2)[1]
        // Check if this is the default worktree
        if d.worktreeDefaults != nil {
            if defaultBranch, ok := (*d.worktreeDefaults)[repoName]; ok && defaultBranch == branchName {
                str = "★ " + str
            } else {
                str = "  " + str
            }
        } else {
            str = "  " + str
        }
    }
}
```

Add a cached style for the star at the top of the file:
```go
var (
    nodeIndicatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
    selectedItemStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("170"))
    defaultStarStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
    nodeIndicatorStr   = nodeIndicatorStyle.Render(" ⬢")
    defaultStarStr     = defaultStarStyle.Render("★")
)
```

Then use `defaultStarStr` instead of plain `"★"`:
```go
str = defaultStarStr + " " + str
```

**Step 3: Update delegate creation in `tui/model.go`**

Update line 164:
```go
delegate := compactDelegate{
    processInfo:      &m.processInfo,
    expandedGroup:    &m.expandedGroup,
    worktreeDefaults: &m.worktreeDefaults,
}
```

**Step 4: Run tests**

Run: `go test ./tui/... -v`
Expected: all PASS

---

### Task 10: Build, Verify, and Clean Up

**Files:**
- All modified files from previous tasks

**Step 1: Run full test suite**

Run: `go test ./... -v`
Expected: all PASS

**Step 2: Run go vet**

Run: `go vet ./...`
Expected: no issues

**Step 3: Build the binary**

Run:
```bash
rm /Users/igorpetrovic/Projects/sesh/bin/sesh
CGO_ENABLED=1 go build -ldflags "-X 'main.version=$(git describe --tags --abbrev=0)'" -o /Users/igorpetrovic/Projects/sesh/bin/sesh
chmod +x /Users/igorpetrovic/Projects/sesh/bin/sesh
```

**Step 4: Manual verification checklist**

1. Open sesh TUI — groups show with `(+N)` format
2. Arrow to a group, press Tab — expands to show worktrees
3. Tab again — collapses back
4. Arrow to a worktree in expanded view, press Ctrl+P — group collapses with new default in title
5. Arrow to group with default, press Enter — connects to the default session
6. Arrow to group without default, press Enter — expands the group
7. Ctrl+T on a worktree — filters to only that repo's worktrees
8. Ctrl+T again — returns to full list
9. Ctrl+E — triggers process detection
10. Verify `~/.local/state/sesh/worktree-defaults.json` was created after Ctrl+P
