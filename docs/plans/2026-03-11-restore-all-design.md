# Restore All Sessions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add "restore all" functionality to the sesh TUI, mirroring the existing "save all" UX — triggered from restore preview mode via `Ctrl+A`.

**Architecture:** Extend restore preview mode with a `Ctrl+A` keybinding that iterates all sessions with saved state, restores non-current sessions sequentially with progress UI, then exits via the existing single-restore flow for the current session. Mirrors the save-all pattern exactly.

**Tech Stack:** Go, Bubble Tea v2, tmux-session-saver CLI

---

## Context

**Existing save-all flow (to mirror):**
1. `Ctrl+S` → enters save preview mode for selected session
2. `Ctrl+A` → starts save-all: sets `saveAllSessions`/`saveAllCompleted`, shows progress UI
3. `SessionSavedMsg` handler chains: updates progress, saves next, or exits when done
4. Uses `SaveAllNextMsg` to trigger delayed exit after completion

**Existing single-restore flow:**
1. `Ctrl+R` → enters restore preview for selected session (shows saved state details)
2. `Enter` → sets `m.selected` + `m.restoreRequested = true`, quits TUI
3. `seshcli/tui.go` handles post-quit: runs `tmux run-shell -b "sleep 0.5 && tmux-session-saver restore '<name>' && rm -f <save-file>"`, then connects

**Key files:**
- `tui/model.go` — Model fields
- `tui/messages.go` — Message types
- `tui/keys.go` — Keybindings
- `tui/update.go` — Event handling (restore preview mode, save-all chain)
- `tui/view.go` — Status bar text
- `tui/commands.go` — Async commands (`saveSessionState`, `deleteSavedState`)
- `tui/preview_generator.go` — Preview pane content generators
- `seshcli/tui.go` — Post-TUI restore dispatch

---

### Task 1: Add Model Fields for Restore-All State

**Files:**
- Modify: `tui/model.go:185-186` (after `saveAllCompleted`)

**Step 1: Add restore-all fields to Model**

Add these fields after `saveAllCompleted` (line 186):

```go
	restoreAllSessions     []string // sessions being restored in restore-all mode
	restoreAllCompleted    []string // sessions that finished restoring in restore-all mode
	restoreAllCurrentSession string // current tmux session name (restored last via quit flow)
```

**Step 2: Commit**

```bash
git add tui/model.go
git commit -m "feat: add model fields for restore-all state"
```

---

### Task 2: Add Message Types

**Files:**
- Modify: `tui/messages.go`

**Step 1: Add SessionRestoredMsg and RestoreAllNextMsg**

Add after `SaveAllNextMsg` (line 60):

```go
// SessionRestoredMsg is sent after tmux-session-saver restore completes for one session.
type SessionRestoredMsg struct {
	SessionName string
	Err         error
}

// RestoreAllNextMsg triggers restoring the next session in a restore-all batch,
// or exits to restore the current session.
type RestoreAllNextMsg struct{}
```

**Step 2: Commit**

```bash
git add tui/messages.go
git commit -m "feat: add SessionRestoredMsg and RestoreAllNextMsg types"
```

---

### Task 3: Add RestoreAll Keybinding

**Files:**
- Modify: `tui/keys.go`

**Step 1: Add RestoreAll to KeyMap struct**

Add after `SaveAll` (line 23):

```go
	RestoreAll        key.Binding
```

**Step 2: Add RestoreAll to DefaultKeyMap**

Add after the SaveAll binding (line 98):

```go
	RestoreAll: key.NewBinding(
		key.WithKeys("ctrl+a"),
		key.WithHelp("ctrl+a", "restore all sessions"),
	),
```

Note: `ctrl+a` is context-dependent — in save preview mode it triggers SaveAll, in restore preview mode it triggers RestoreAll. They never conflict because each modal handler checks independently.

**Step 3: Commit**

```bash
git add tui/keys.go
git commit -m "feat: add RestoreAll keybinding (ctrl+a in restore preview)"
```

---

### Task 4: Add restoreSessionState Command

**Files:**
- Modify: `tui/commands.go`

**Step 1: Add restoreSessionState function**

Add after `saveSessionState` (after line 273):

```go
// restoreSessionState runs tmux-session-saver restore for a tmux session.
func restoreSessionState(sessionName string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("tmux-session-saver", "restore", sessionName)
		err := cmd.Run()
		return SessionRestoredMsg{SessionName: sessionName, Err: err}
	}
}
```

**Step 2: Add getCurrentTmuxSession function**

Add after the new function:

```go
// getCurrentTmuxSession returns the name of the current tmux session.
func getCurrentTmuxSession() string {
	cmd := exec.Command("tmux", "display-message", "-p", "#S")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
```

**Step 3: Commit**

```bash
git add tui/commands.go
git commit -m "feat: add restoreSessionState and getCurrentTmuxSession commands"
```

---

### Task 5: Add Restore-All Progress Preview Generator

**Files:**
- Modify: `tui/preview_generator.go`

**Step 1: Add generateRestoreAllProgress function**

Add after `generateSaveAllProgress` (after line 439):

```go
// generateRestoreAllProgress creates a preview showing restore-all progress.
func generateRestoreAllProgress(allSessions []string, completed []string, currentSession string) string {
	var out strings.Builder
	out.WriteString(fmt.Sprintf("\n%s━━━ Restoring All Sessions ━━━%s\n\n", colorDim, colorReset))

	completedSet := make(map[string]bool, len(completed))
	for _, name := range completed {
		completedSet[name] = true
	}

	firstPending := true
	for _, name := range allSessions {
		if name == currentSession {
			out.WriteString(fmt.Sprintf(" %s★%s %s %s(restore on exit)%s\n", colorMagenta, colorReset, name, colorDim, colorReset))
			continue
		}

		sd := parseSavedState(name)
		suffix := ""
		if sd != nil {
			suffix = fmt.Sprintf(" %s(%d windows)%s", colorDim, sd.windowCount, colorReset)
		}

		if completedSet[name] {
			out.WriteString(fmt.Sprintf(" %s✓%s %s%s\n", colorGreen, colorReset, name, suffix))
		} else if firstPending {
			out.WriteString(fmt.Sprintf(" %s⏳%s %s%s\n", colorYellow, colorReset, name, suffix))
			firstPending = false
		} else {
			out.WriteString(fmt.Sprintf("    %s%s%s%s\n", colorDim, name, colorReset, suffix))
		}
	}

	// Count non-current sessions for progress
	total := 0
	done := 0
	for _, name := range allSessions {
		if name != currentSession {
			total++
		}
	}
	for _, name := range completed {
		if name != currentSession {
			done++
		}
	}

	out.WriteString(fmt.Sprintf("\n %s%d/%d restored%s\n", colorDim, done, total, colorReset))
	return out.String()
}
```

**Step 2: Update generateRestorePreview to include Ctrl+A hint**

In `generateRestorePreview` (line 339), replace the footer line:

```go
// Old:
out.WriteString(fmt.Sprintf(" %s%sEnter%s%s restore  |  %s%sBksp%s%s delete  |  %s%sEsc%s%s cancel%s\n",
    colorReset, colorGreen, colorReset, colorDim,
    colorReset, colorRed, colorReset, colorDim,
    colorReset, colorYellow, colorReset, colorDim, colorReset))

// New:
out.WriteString(fmt.Sprintf(" %s%sEnter%s%s restore  |  ", colorReset, colorGreen, colorReset, colorDim))
if savedCount > 1 {
    out.WriteString(fmt.Sprintf("%s%sCtrl+A%s%s restore all (%d)  |  ", colorReset, colorCyan, colorReset, colorDim, savedCount))
}
out.WriteString(fmt.Sprintf("%s%sBksp%s%s delete  |  %s%sEsc%s%s cancel%s\n",
    colorReset, colorRed, colorReset, colorDim,
    colorReset, colorYellow, colorReset, colorDim, colorReset))
```

To get `savedCount`, update the function signature to accept it:

```go
func generateRestorePreview(sessionName string, savedCount int) string {
```

And update the caller in `enterRestorePreview` to pass the count from `m.savedState`.

**Step 3: Commit**

```bash
git add tui/preview_generator.go
git commit -m "feat: add restore-all progress preview and update restore preview footer"
```

---

### Task 6: Update enterRestorePreview and Add exitRestoreAll

**Files:**
- Modify: `tui/update.go`

**Step 1: Update enterRestorePreview to reset restore-all state and pass savedCount**

Replace the `enterRestorePreview` function (lines 343-353):

```go
func (m Model) enterRestorePreview(sessionName string) (Model, tea.Cmd) {
	m.restorePreviewMode = true
	m.deleteConfirmPending = false
	m.restorePreviewSession = sessionName
	m.restoreAllSessions = nil
	m.restoreAllCompleted = nil
	m.restoreAllCurrentSession = ""
	content := generateRestorePreview(sessionName, len(*m.savedState))
	m.previewContent = content
	m.previewPort.SetContent(content)
	m.previewPort.GotoTop()
	return m, nil
}
```

**Step 2: Update exitRestorePreview to clear restore-all state**

Replace the `exitRestorePreview` function (lines 355-367):

```go
func (m Model) exitRestorePreview() (Model, tea.Cmd) {
	m.restorePreviewMode = false
	m.deleteConfirmPending = false
	m.restorePreviewSession = ""
	m.restoreAllSessions = nil
	m.restoreAllCompleted = nil
	m.restoreAllCurrentSession = ""
	if item, ok := m.list.SelectedItem().(sessionItem); ok {
		m.previewPort.SetContent("")
		return m, loadPreview(context.Background(), m.previewer, item.session)
	}
	m.previewPort.SetContent("")
	m.previewContent = ""
	return m, nil
}
```

**Step 3: Commit**

```bash
git add tui/update.go
git commit -m "feat: update enter/exit restore preview to handle restore-all state"
```

---

### Task 7: Handle SessionRestoredMsg in Update

**Files:**
- Modify: `tui/update.go`

**Step 1: Add SessionRestoredMsg handler**

Add a new case in the `Update` function's type switch, after the `SessionSavedMsg` handler (after line 688). Place it before `SaveAllNextMsg`:

```go
	case SessionRestoredMsg:
		if msg.Err != nil {
			logDebug("DEBUG: Failed to restore session %s: %v", msg.SessionName, msg.Err)
		} else {
			// Delete save file after successful restore
			sanitized := SanitizeSessionName(msg.SessionName)
			homeDir, _ := os.UserHomeDir()
			if homeDir != "" {
				os.Remove(filepath.Join(homeDir, ".local", "share", "tmux-session-saver", sanitized+".json"))
			}
			delete(*m.savedState, sanitized)
		}

		// Restore-all mode: track progress and restore next session
		if m.restorePreviewMode && m.restoreAllSessions != nil {
			m.restoreAllCompleted = append(m.restoreAllCompleted, msg.SessionName)
			content := generateRestoreAllProgress(m.restoreAllSessions, m.restoreAllCompleted, m.restoreAllCurrentSession)
			m.previewContent = content
			m.previewPort.SetContent(content)
			m.previewPort.GotoTop()

			// Find next non-current session to restore
			nextIdx := -1
			completedSet := make(map[string]bool, len(m.restoreAllCompleted))
			for _, name := range m.restoreAllCompleted {
				completedSet[name] = true
			}
			for i, name := range m.restoreAllSessions {
				if name == m.restoreAllCurrentSession {
					continue
				}
				if !completedSet[name] {
					nextIdx = i
					break
				}
			}

			if nextIdx >= 0 {
				// Restore next session
				return m, restoreSessionState(m.restoreAllSessions[nextIdx])
			}

			// All non-current sessions done — check if current session needs restore
			if m.restoreAllCurrentSession != "" {
				// Delay then exit via RestoreAllNextMsg to trigger current session restore
				m.statusMessage = fmt.Sprintf("restored %d sessions, switching to restore current...",
					len(m.restoreAllCompleted))
				return m, tea.Tick(1000*time.Millisecond, func(time.Time) tea.Msg {
					return RestoreAllNextMsg{}
				})
			}

			// No current session to restore — just exit
			m.statusMessage = fmt.Sprintf("restored %d sessions", len(m.restoreAllCompleted))
			return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
				return RestoreAllNextMsg{}
			})
		}

		return m, nil
```

**Step 2: Add RestoreAllNextMsg handler**

Add after the SessionRestoredMsg handler:

```go
	case RestoreAllNextMsg:
		if m.restorePreviewMode && m.restoreAllSessions != nil {
			// If current session has saved state, exit via single-restore flow
			if m.restoreAllCurrentSession != "" {
				m.selected = m.restoreAllCurrentSession
				m.restoreRequested = true
				m.restorePreviewMode = false
				m.restoreAllSessions = nil
				m.restoreAllCompleted = nil
				return m, tea.Quit
			}

			// No current session to restore — exit restore preview
			m.restorePreviewMode = false
			m.restorePreviewSession = ""
			m.restoreAllSessions = nil
			m.restoreAllCompleted = nil
			m.restoreAllCurrentSession = ""
			if item, ok := m.list.SelectedItem().(sessionItem); ok {
				m.previewPort.SetContent("")
				return m, tea.Batch(
					loadPreview(context.Background(), m.previewer, item.session),
					tea.Tick(2*time.Second, func(time.Time) tea.Msg { return clearStatusMsg{} }),
				)
			}
			return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return clearStatusMsg{} })
		}
		return m, nil
```

**Step 3: Commit**

```bash
git add tui/update.go
git commit -m "feat: handle SessionRestoredMsg and RestoreAllNextMsg in update loop"
```

---

### Task 8: Add Ctrl+A Handler in Restore Preview Mode

**Files:**
- Modify: `tui/update.go`

**Step 1: Update restore preview key handling**

In the restore preview mode key handler (lines 726-752), add `Ctrl+A` restore-all trigger. Replace the current block:

```go
	if m.restorePreviewMode {
		// During restore-all, swallow everything (progress is automatic)
		if m.restoreAllSessions != nil {
			switch {
			case msg.String() == "ctrl+c" || msg.String() == "ctrl+b":
				return m, tea.Quit
			default:
				return m, nil
			}
		}

		switch {
		case key.Matches(msg, m.keys.Select): // Enter → confirm restore
			m.selected = m.restorePreviewSession
			m.restoreRequested = true
			return m, tea.Quit
		case key.Matches(msg, m.keys.RestoreAll): // Ctrl+A → restore all sessions with saved state
			// Build list of sessions that have saved state
			var sessionsToRestore []string
			for _, item := range m.allItems {
				if si, ok := item.(sessionItem); ok && si.session.Src == "tmux" {
					if (*m.savedState)[si.sanitizedName] {
						sessionsToRestore = append(sessionsToRestore, si.session.Name)
					}
				}
			}
			if len(sessionsToRestore) == 0 {
				return m.exitRestorePreview()
			}
			// Detect current tmux session
			m.restoreAllCurrentSession = getCurrentTmuxSession()
			m.restoreAllSessions = sessionsToRestore
			m.restoreAllCompleted = nil
			content := generateRestoreAllProgress(sessionsToRestore, nil, m.restoreAllCurrentSession)
			m.previewContent = content
			m.previewPort.SetContent(content)
			m.previewPort.GotoTop()
			// Find first non-current session to restore
			for _, name := range sessionsToRestore {
				if name != m.restoreAllCurrentSession {
					return m, restoreSessionState(name)
				}
			}
			// Only current session has saved state — just do single restore
			m.selected = m.restoreAllCurrentSession
			m.restoreRequested = true
			return m, tea.Quit
		case msg.Code == tea.KeyBackspace: // Backspace → delete saved state
			if m.deleteConfirmPending {
				m.deleteConfirmPending = false
				return m, deleteSavedState(m.restorePreviewSession)
			}
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
```

**Step 2: Commit**

```bash
git add tui/update.go
git commit -m "feat: handle Ctrl+A in restore preview for restore-all"
```

---

### Task 9: Update View Status Bar

**Files:**
- Modify: `tui/view.go`

**Step 1: Update restore preview status bar to show restore-all state**

Replace the restore preview status bar line (line 55-56):

```go
if m.restorePreviewMode {
    if m.restoreAllSessions != nil {
        columns += "\n " + statusMsgStyle.Render("restoring all sessions...")
    } else {
        columns += "\n " + statusMsgStyle.Render("Enter restore | Ctrl+A restore all | Esc cancel")
    }
}
```

**Step 2: Commit**

```bash
git add tui/view.go
git commit -m "feat: update status bar for restore-all mode"
```

---

### Task 10: Add os Import to update.go If Missing

**Files:**
- Modify: `tui/update.go`

**Step 1: Ensure `os` and `path/filepath` are imported**

The `SessionRestoredMsg` handler uses `os.UserHomeDir()`, `os.Remove()`, and `filepath.Join()`. Check if these are already imported in update.go. If not, add them to the import block.

**Step 2: Commit (if changes needed)**

```bash
git add tui/update.go
git commit -m "fix: add missing imports for restore-all handler"
```

---

### Task 11: Build and Manual Test

**Step 1: Build sesh**

```bash
cd ~/projects/sesh && go build -o ~/dotfiles/bin/sesh . && codesign -f -s - ~/dotfiles/bin/sesh
```

**Step 2: Manual test checklist**

1. Open sesh picker (`prefix+o`)
2. Save all sessions (`Ctrl+S` then `Ctrl+A`)
3. Verify `⟲` badges appear on all sessions
4. On a session with saved state, press `Ctrl+R`
5. Verify restore preview shows "Ctrl+A restore all (N)" in footer
6. Press `Ctrl+A` — verify progress UI appears
7. Verify non-current sessions show ✓ as they complete
8. Verify current session shows ★ (restore on exit)
9. Verify TUI exits and connects to current session with restore
10. Verify save files are deleted after restore
11. Test edge case: only current session has saved state → should do single restore immediately

**Step 3: Commit any fixes**

---

## Summary of Changes

| File | Change |
|------|--------|
| `tui/model.go` | Add `restoreAllSessions`, `restoreAllCompleted`, `restoreAllCurrentSession` fields |
| `tui/messages.go` | Add `SessionRestoredMsg`, `RestoreAllNextMsg` types |
| `tui/keys.go` | Add `RestoreAll` keybinding (ctrl+a) |
| `tui/commands.go` | Add `restoreSessionState()`, `getCurrentTmuxSession()` |
| `tui/preview_generator.go` | Add `generateRestoreAllProgress()`, update `generateRestorePreview()` footer |
| `tui/update.go` | Handle `SessionRestoredMsg`, `RestoreAllNextMsg`; add Ctrl+A in restore preview; update enter/exit restore preview |
| `tui/view.go` | Update status bar for restore-all mode |
| `seshcli/tui.go` | No changes needed — existing `restoreRequested` flow handles current session |
