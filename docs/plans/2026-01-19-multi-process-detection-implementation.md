# Multi-Process Detection Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add detection and display for npm, yarn, and node processes in tmux sessions with branded Nerd Font icons.

**Architecture:** Expand existing single-process detection to collect multiple processes per session, store as slice, and render multiple branded icons.

**Tech Stack:** Go, Bubble Tea TUI framework, tmux, Nerd Fonts (Devicons)

---

## Task 1: Update Message Data Structure

**Files:**
- Modify: `tui/messages.go:22-25`

**Step 1: Change ProcessDetectedMsg to support multiple processes**

Edit `tui/messages.go` lines 22-25:

```go
type ProcessDetectedMsg struct {
	SessionName string
	Processes   []string // Changed from "Process string" to support multiple
}
```

**Step 2: Verify syntax**

Run: `go build -o ./sesh`
Expected: Build succeeds (no syntax errors)

**Step 3: Commit**

```bash
git add tui/messages.go
git commit -m "refactor(tui): change ProcessDetectedMsg to support multiple processes

- Change Process string to Processes []string
- Prepares for multi-process detection (node, npm, yarn)"
```

---

## Task 2: Update Model Data Structure

**Files:**
- Modify: `tui/model.go:36,67`

**Step 1: Change processInfo map value type**

Edit `tui/model.go` line 36:

```go
processInfo    map[string][]string // session.Name -> []processes (e.g., ["node", "npm"])
```

**Step 2: Update processInfo initialization**

Edit `tui/model.go` line 67:

```go
processInfo := make(map[string][]string)
```

**Step 3: Verify syntax**

Run: `go build -o ./sesh`
Expected: Build will FAIL with type mismatch errors in update.go and delegate.go (expected, we'll fix next)

**Step 4: Commit**

```bash
git add tui/model.go
git commit -m "refactor(tui): change processInfo to map[string][]string

- Change from single process to process slice per session
- Breaking change: update.go and delegate.go need fixes (next commits)"
```

---

## Task 3: Update Message Handler

**Files:**
- Modify: `tui/update.go:103-111`

**Step 1: Update ProcessDetectedMsg handler**

Edit `tui/update.go` lines 103-114 (replace entire case block):

```go
case ProcessDetectedMsg:
	// DEBUG: Log when message is received
	logDebug("DEBUG: ProcessDetectedMsg received for %s with processes %v", msg.SessionName, msg.Processes)

	// Just update the processInfo map - delegate will read it on next render
	m.processInfo[msg.SessionName] = msg.Processes

	// DEBUG: Log processInfo map
	logDebug("DEBUG: processInfo map: %+v", m.processInfo)

	// No list rebuild needed - just return to trigger re-render
	return m, nil
```

**Step 2: Verify syntax**

Run: `go build -o ./sesh`
Expected: Build still FAILS but fewer errors (delegate.go still needs fix)

**Step 3: Commit**

```bash
git add tui/update.go
git commit -m "refactor(tui): update ProcessDetectedMsg handler for slices

- Store msg.Processes instead of msg.Process
- Update debug logging to show process slice"
```

---

## Task 4: Update Delegate Field Type

**Files:**
- Modify: `tui/delegate.go:14`

**Step 1: Change processInfo field type**

Edit `tui/delegate.go` line 14:

```go
processInfo *map[string][]string // Pointer to model's processInfo map
```

**Step 2: Verify syntax**

Run: `go build -o ./sesh`
Expected: Build FAILS with error in Render() method (line 34) - we'll fix next

**Step 3: Commit**

```bash
git add tui/delegate.go
git commit -m "refactor(tui): update delegate processInfo field type

- Change to *map[string][]string for multi-process support
- Render() method needs update (next commit)"
```

---

## Task 5: Update Delegate Rendering Logic

**Files:**
- Modify: `tui/delegate.go:31-37`

**Step 1: Replace process icon rendering logic**

Edit `tui/delegate.go` lines 31-37 (replace entire block):

```go
// Add process indicators on the right if detected (lookup from model's map)
var processIcon string
if d.processInfo != nil {
	if processes, ok := (*d.processInfo)[sessionItem.session.Name]; ok {
		// Map process names to Nerd Font icons and colors
		processIcons := map[string]struct {
			Icon  string
			Color string
		}{
			"node": {Icon: "\ue718", Color: "34"},  // green
			"npm":  {Icon: "\ue71e", Color: "196"}, // red
			"yarn": {Icon: "\ue6a7", Color: "39"},  // blue
		}

		// Render all detected processes
		for _, proc := range processes {
			if iconData, exists := processIcons[proc]; exists {
				processIcon += fmt.Sprintf(" \033[38;5;%sm%s\033[0m", iconData.Color, iconData.Icon)
			}
		}
	}
}
```

**Step 2: Verify syntax**

Run: `go build -o ./sesh`
Expected: Build SUCCEEDS (all type mismatches resolved)

**Step 3: Commit**

```bash
git add tui/delegate.go
git commit -m "feat(tui): add multi-process icon rendering with Nerd Fonts

- Iterate over process slice instead of single process
- Add npm (red \ue71e) and yarn (blue \ue6a7) icons
- Keep node (green \ue718) icon
- Multiple icons render side-by-side when detected"
```

---

## Task 6: Update Detection Logic for Multi-Process

**Files:**
- Modify: `tui/commands.go:69-108`

**Step 1: Replace detectProcessForSession function**

Edit `tui/commands.go` lines 69-108 (replace entire function):

```go
// detectProcessForSession checks if a tmux session has specific processes running
func detectProcessForSession(session model.SeshSession) tea.Cmd {
	return func() tea.Msg {
		// Only check tmux sessions
		if session.Src != "tmux" {
			return nil
		}

		// Get all pane commands for this session
		start := time.Now()
		cmd := exec.Command("tmux", "list-panes", "-t", session.Name, "-F", "#{pane_current_command}")
		output, err := cmd.Output()
		elapsed := time.Since(start)
		logDebug("DEBUG: list-panes for %s took %v", session.Name, elapsed)
		if err != nil {
			return nil
		}

		// Collect all detected processes
		detected := make(map[string]bool)

		// Check each pane's command
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(strings.ToLower(line))
			if line == "" {
				continue
			}

			// Detect Node.js processes
			if line == "node" || strings.HasPrefix(line, "node ") {
				detected["node"] = true
				logDebug("DEBUG: Node detected in session: %s (line: %s)", session.Name, line)
			}

			// Detect npm processes
			if line == "npm" || strings.HasPrefix(line, "npm ") {
				detected["npm"] = true
				logDebug("DEBUG: npm detected in session: %s (line: %s)", session.Name, line)
			}

			// Detect yarn processes
			if line == "yarn" || strings.HasPrefix(line, "yarn ") {
				detected["yarn"] = true
				logDebug("DEBUG: Yarn detected in session: %s (line: %s)", session.Name, line)
			}
		}

		// Return all detected processes
		if len(detected) > 0 {
			processes := make([]string, 0, len(detected))
			for proc := range detected {
				processes = append(processes, proc)
			}
			return ProcessDetectedMsg{
				SessionName: session.Name,
				Processes:   processes,
			}
		}

		return nil
	}
}
```

**Step 2: Verify syntax and build**

Run: `go build -o ./sesh`
Expected: Build SUCCEEDS

**Step 3: Commit**

```bash
git add tui/commands.go
git commit -m "feat(tui): detect npm and yarn processes in addition to node

- Scan all panes completely before returning
- Use map to dedupe if multiple panes run same process
- Detect npm and yarn with exact match or prefix
- Return slice of all detected processes
- Add debug logging for each process type"
```

---

## Task 7: Build and Install

**Files:**
- Build: `sesh`

**Step 1: Build binary**

Run: `go build -o ./sesh`
Expected: Build SUCCEEDS with no errors

**Step 2: Install to user's bin**

Run: `cp ./sesh /Users/igorpetrovic/Dotfiles/bin/sesh`
Expected: File copied successfully

**Step 3: Verify installation**

Run: `which sesh && sesh --version`
Expected: Shows path and version

---

## Task 8: Manual Testing

**Test Cases:**

### Test 1: Session with only node
1. Create tmux session: `tmux new-session -d -s test-node 'node'`
2. Run: `sesh`
3. Expected: test-node session shows green  icon

### Test 2: Session with npm
1. Create tmux session in a project with package.json
2. Run: `npm run dev` in that session
3. Run: `sesh` in another terminal
4. Expected: Session shows red  icon (or both  if npm spawns node)

### Test 3: Session with yarn
1. Create tmux session in a project with yarn.lock
2. Run: `yarn dev` in that session
3. Run: `sesh` in another terminal
4. Expected: Session shows blue  icon (or multiple icons if yarn spawns node/npm)

### Test 4: Session with no JavaScript processes
1. Create tmux session: `tmux new-session -d -s test-bash 'bash'`
2. Run: `sesh`
3. Expected: test-bash session shows NO process icons

### Test 5: Multiple processes in same session
1. Create tmux session with multiple panes
2. Run `node` in one pane, `npm` in another
3. Run: `sesh` in another terminal
4. Expected: Session shows both   icons

### Test 6: Visual verification
1. Ensure Nerd Font is installed in terminal
2. Run: `sesh`
3. Expected: Icons render as branded glyphs, not boxes/missing chars

---

## Task 9: Cleanup and Final Commit

**Step 1: Clean up any debug outputs**

Review code for any temporary debug statements that should be removed.

**Step 2: Run tests**

Run: `go test ./tui/...`
Expected: All tests pass (if tests exist)

**Step 3: Final verification build**

Run: `go build -o ./sesh && cp ./sesh /Users/igorpetrovic/Dotfiles/bin/sesh`
Expected: Clean build and install

---

## Troubleshooting

### Issue: Icons show as boxes or missing chars
**Cause:** Terminal doesn't have Nerd Font installed
**Solution:** Install a Nerd Font (e.g., "FiraCode Nerd Font") and configure terminal to use it

### Issue: Process not detected
**Cause:** Process name doesn't match detection pattern
**Solution:** Check debug log (`/tmp/sesh-tui-debug.log`) for actual pane command, add detection pattern

### Issue: Icons not colored correctly
**Cause:** Terminal doesn't support 256 colors
**Solution:** Verify `$TERM` is set to `xterm-256color` or similar

---

## References

- Design doc: `docs/plans/2026-01-19-multi-process-detection-design.md`
- Nerd Fonts: https://www.nerdfonts.com
- Nerd Fonts Cheat Sheet: https://www.nerdfonts.com/cheat-sheet
- Devicons (icon set): https://vorillaz.github.io/devicons/

---

## Success Criteria

- ✅ Build succeeds with no errors
- ✅ Single process detection works (node only)
- ✅ Multiple process detection works (node + npm/yarn)
- ✅ Icons render with correct colors
- ✅ Multiple icons display side-by-side
- ✅ No performance regression (async behavior preserved)
- ✅ Debug logging shows detected processes
