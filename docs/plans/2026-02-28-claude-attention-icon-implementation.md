# Claude Attention Icon Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show a 🖐️ icon on tmux sessions that have a Claude Code session awaiting user confirmation — in the TUI session list and the tmux status bar.

**Architecture:** New `claude/` package with shared SQLite query logic. TUI fires an async command on Init, receives a map of sessions needing attention, and the delegate overrides the icon at render time. New `sesh status` CLI command outputs 🖐️ for tmux status-right integration.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), Bubble Tea, Cobra

---

### Task 1: Create `claude/attention.go` — shared query logic

**Files:**
- Create: `claude/attention.go`
- Test: `claude/attention_test.go`

**Step 1: Write the failing tests**

```go
// claude/attention_test.go
package claude

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, ".claude", "sessions.db")
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE sessions (
		session_id TEXT PRIMARY KEY,
		tmux_session TEXT,
		status TEXT,
		ended_at TEXT,
		replaced_by_session_id TEXT
	)`)
	require.NoError(t, err)
	return dir
}

func insertSession(t *testing.T, dir, sessionID, tmuxSession, status string, ended bool) {
	t.Helper()
	dbPath := filepath.Join(dir, ".claude", "sessions.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	endedAt := "NULL"
	if ended {
		endedAt = "'2026-01-01'"
	}
	_, err = db.Exec("INSERT INTO sessions (session_id, tmux_session, status, ended_at) VALUES (?, ?, ?, "+endedAt+")",
		sessionID, tmuxSession, status)
	require.NoError(t, err)
}

func TestSessionsNeedingAttention_NoDB(t *testing.T) {
	result, err := SessionsNeedingAttention("/nonexistent")
	assert.NoError(t, err)
	assert.Empty(t, result)
}

func TestSessionsNeedingAttention_NoAwaitingSessions(t *testing.T) {
	dir := setupTestDB(t)
	insertSession(t, dir, "s1", "myproject", "thinking", false)
	insertSession(t, dir, "s2", "other", "running:tool", false)

	result, err := SessionsNeedingAttention(dir)
	assert.NoError(t, err)
	assert.Empty(t, result)
}

func TestSessionsNeedingAttention_WithAwaitingSessions(t *testing.T) {
	dir := setupTestDB(t)
	insertSession(t, dir, "s1", "myproject", "awaiting:permission", false)
	insertSession(t, dir, "s2", "other", "thinking", false)
	insertSession(t, dir, "s3", "myproject", "running:tool", false)

	result, err := SessionsNeedingAttention(dir)
	assert.NoError(t, err)
	assert.True(t, result["myproject"])
	assert.False(t, result["other"])
}

func TestSessionsNeedingAttention_IgnoresEndedSessions(t *testing.T) {
	dir := setupTestDB(t)
	insertSession(t, dir, "s1", "myproject", "awaiting:permission", true) // ended

	result, err := SessionsNeedingAttention(dir)
	assert.NoError(t, err)
	assert.Empty(t, result)
}

func TestNeedsAttention_True(t *testing.T) {
	dir := setupTestDB(t)
	insertSession(t, dir, "s1", "myproject", "awaiting:permission", false)

	result, err := NeedsAttention(dir, "myproject")
	assert.NoError(t, err)
	assert.True(t, result)
}

func TestNeedsAttention_False(t *testing.T) {
	dir := setupTestDB(t)
	insertSession(t, dir, "s1", "myproject", "thinking", false)

	result, err := NeedsAttention(dir, "myproject")
	assert.NoError(t, err)
	assert.False(t, result)
}

func TestNeedsAttention_NoDB(t *testing.T) {
	result, err := NeedsAttention("/nonexistent", "myproject")
	assert.NoError(t, err)
	assert.False(t, result)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./claude/... -v`
Expected: FAIL — package does not exist

**Step 3: Write minimal implementation**

```go
// claude/attention.go
package claude

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// dbPath returns the path to Claude's sessions database given a home directory.
func dbPath(homeDir string) string {
	return filepath.Join(homeDir, ".claude", "sessions.db")
}

// SessionsNeedingAttention returns a set of tmux session names that have
// at least one active Claude Code session with "awaiting:*" status.
// homeDir is the user's home directory (used to locate ~/.claude/sessions.db).
// Returns an empty map (not error) if the DB doesn't exist.
func SessionsNeedingAttention(homeDir string) (map[string]bool, error) {
	path := dbPath(homeDir)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, nil
	}
	defer db.Close()

	rows, err := db.Query(`SELECT DISTINCT tmux_session FROM sessions
		WHERE ended_at IS NULL
		  AND status LIKE 'awaiting:%'
		  AND replaced_by_session_id IS NULL`)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			result[name] = true
		}
	}
	return result, nil
}

// NeedsAttention checks if a specific tmux session has any active Claude Code
// session with "awaiting:*" status.
// Returns false (not error) if the DB doesn't exist.
func NeedsAttention(homeDir, tmuxSession string) (bool, error) {
	path := dbPath(homeDir)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false, nil
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return false, nil
	}
	defer db.Close()

	var exists int
	err = db.QueryRow(`SELECT 1 FROM sessions
		WHERE tmux_session = ?
		  AND ended_at IS NULL
		  AND status LIKE 'awaiting:%'
		  AND replaced_by_session_id IS NULL
		LIMIT 1`, tmuxSession).Scan(&exists)
	if err != nil {
		return false, nil
	}
	return true, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./claude/... -v`
Expected: All PASS

---

### Task 2: Create `seshcli/status.go` — CLI command

**Files:**
- Create: `seshcli/status.go`
- Modify: `seshcli/root_command.go:110-120` (add subcommand)

**Step 1: Write the CLI command**

```go
// seshcli/status.go
package seshcli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/joshmedeski/sesh/v2/claude"
	"github.com/spf13/cobra"
)

func NewStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show attention indicator for current tmux session",
		Long:  "Outputs 🖐️ if any Claude Code session in the current tmux session needs user confirmation. Designed for tmux status-right: set -g status-right '#(sesh status)'",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get current tmux session name
			out, err := exec.Command("tmux", "display-message", "-p", "#S").Output()
			if err != nil {
				return nil // Not in tmux — silent exit
			}
			sessionName := strings.TrimSpace(string(out))
			if sessionName == "" {
				return nil
			}

			homeDir, err := os.UserHomeDir()
			if err != nil {
				return nil
			}

			needs, _ := claude.NeedsAttention(homeDir, sessionName)
			if needs {
				fmt.Print("🖐️")
			}
			return nil
		},
	}
}
```

**Step 2: Register the subcommand**

In `seshcli/root_command.go`, add `NewStatusCommand()` to the `rootCmd.AddCommand(...)` block:

```go
// After NewFindByPathCommand(lister),
NewStatusCommand(),
// Before NewTuiCommand(...)
```

**Step 3: Build and test manually**

Run: `go build -o /dev/null && echo "builds"`
Expected: builds

Run: `go run . status` (outside tmux should print nothing)
Expected: No output, exit 0

---

### Task 3: TUI message type and model field

**Files:**
- Modify: `tui/messages.go` (add message type)
- Modify: `tui/model.go:167` (add field)
- Modify: `tui/delegate.go:59` (add field to delegate struct)

**Step 1: Add the message type**

In `tui/messages.go`, add after `ExcludesSavedMsg`:

```go
type ClaudeAttentionMsg struct {
	Sessions map[string]bool // tmux session name -> needs attention
}
```

**Step 2: Add field to Model struct**

In `tui/model.go`, add after `groupMode GroupMode` (line ~176):

```go
claudeAttention *map[string]bool // shared with delegate: tmux session name -> needs attention
```

**Step 3: Initialize the field in `newModel`**

In `tui/model.go`, inside the `m := Model{...}` block (around line 237), add:

```go
claudeAttention: &map[string]bool{},
```

**Step 4: Add field to delegate struct**

In `tui/delegate.go`, add to `compactDelegate` struct (after `worktreeDefaults`):

```go
claudeAttention *map[string]bool
```

**Step 5: Pass the pointer when creating the delegate**

In `tui/model.go`, update the delegate creation (around line 263):

```go
delegate := compactDelegate{
	processInfo:      m.processInfo,
	expandedGroup:    m.expandedGroup,
	worktreeDefaults: m.worktreeDefaults,
	claudeAttention:  m.claudeAttention,
}
```

**Step 6: Build to verify**

Run: `go build -o /dev/null && echo "builds"`
Expected: builds

---

### Task 4: TUI async command and update handler

**Files:**
- Modify: `tui/commands.go` (add command)
- Modify: `tui/update.go` (add handler for ClaudeAttentionMsg)
- Modify: `tui/model.go:320-337` (add to Init batch)

**Step 1: Add the async command**

In `tui/commands.go`, add after `detectAllProcesses()`:

```go
// checkClaudeAttention queries Claude's sessions.db for sessions needing user attention.
// Returns a map of tmux session names that have awaiting CC sessions.
func checkClaudeAttention() tea.Cmd {
	return func() tea.Msg {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return ClaudeAttentionMsg{Sessions: nil}
		}
		sessions, _ := claude.SessionsNeedingAttention(homeDir)
		return ClaudeAttentionMsg{Sessions: sessions}
	}
}
```

Add these imports to `tui/commands.go` if not already present:

```go
"os"
"github.com/joshmedeski/sesh/v2/claude"
```

**Step 2: Add the handler in Update**

In `tui/update.go`, add a new case after the `ProcessInfoMsg` handler (around line 487):

```go
case ClaudeAttentionMsg:
	// Update map in-place to preserve delegate's pointer reference
	for k := range *m.claudeAttention {
		delete(*m.claudeAttention, k)
	}
	for k, v := range msg.Sessions {
		(*m.claudeAttention)[k] = v
	}
	return m, nil
```

**Step 3: Fire the command in Init**

In `tui/model.go`, update `Init()` to batch `checkClaudeAttention()`:

The current Init (line 320):
```go
func (m Model) Init() tea.Cmd {
	logDebug("Init() called with %d sessions", len(m.sessions.OrderedIndex))
	filterCmd := func() tea.Msg {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	}
	if m.list.SelectedItem() != nil {
		if item, ok := m.list.SelectedItem().(sessionItem); ok {
			logDebug("Init: queuing preview load for %q", item.session.Name)
			return tea.Batch(filterCmd, loadPreview(m.previewer, item.session))
		}
	}
	return filterCmd
}
```

Updated Init:
```go
func (m Model) Init() tea.Cmd {
	logDebug("Init() called with %d sessions", len(m.sessions.OrderedIndex))
	filterCmd := func() tea.Msg {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	}
	if m.list.SelectedItem() != nil {
		if item, ok := m.list.SelectedItem().(sessionItem); ok {
			logDebug("Init: queuing preview load for %q", item.session.Name)
			return tea.Batch(filterCmd, loadPreview(m.previewer, item.session), checkClaudeAttention())
		}
	}
	return tea.Batch(filterCmd, checkClaudeAttention())
}
```

**Step 4: Build and run tests**

Run: `go build -o /dev/null && go test ./tui/... -v -run Test`
Expected: builds, existing tests pass

---

### Task 5: Delegate icon override at render time

**Files:**
- Modify: `tui/delegate.go:72-136` (Render method, sessionItem case)

**Step 1: Add the attention icon constant**

In `tui/delegate.go`, add near the top with the other cached styles (around line 14):

```go
attentionIconStr = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Render("🖐️") + " " // magenta hand
```

**Step 2: Add icon override in Render**

In `tui/delegate.go`, inside the `case sessionItem:` block, after the existing icon rendering and before `// Add process indicator if available` (around line 131), add the attention icon override:

```go
// Override tmux icon with attention indicator if CC session is awaiting
if v.session.Src == "tmux" && d.claudeAttention != nil {
	if (*d.claudeAttention)[v.session.Name] {
		// Replace icon prefix: swap blue  with magenta 🖐️
		if v.iconPrefix != "" {
			str = attentionIconStr + strings.TrimPrefix(str, v.iconPrefix)
		}
	}
}
```

This goes right after line 129 (`str = v.displayName`), but needs to also handle the filtered case. The cleanest spot is right before `// Add process indicator if available` (line 131), since at that point `str` is fully built for both filtered and non-filtered paths.

**Step 3: Build and test manually**

Run: `go build -o /dev/null && echo "builds"`
Expected: builds

---

### Task 6: Build binary and manual verification

**Files:** None (testing only)

**Step 1: Build the binary**

Run: `rm ~/Dotfiles/bin/sesh && go build -ldflags "-X 'main.version=$(git describe --tags --abbrev=0)'" -o ~/Dotfiles/bin/sesh && chmod +x ~/Dotfiles/bin/sesh`

**Step 2: Test `sesh status` from tmux**

Run: `sesh status`
Expected: 🖐️ if any CC session in current tmux session is awaiting, nothing otherwise

**Step 3: Test TUI**

Run: `sesh tui`
Expected: Tmux sessions with awaiting CC sessions show 🖐️ instead of

**Step 4: Run full test suite**

Run: `make test`
Expected: All tests pass
