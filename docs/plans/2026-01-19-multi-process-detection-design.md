# Multi-Process Detection Design (Node/Yarn/npm)

**Date:** 2026-01-19
**Status:** Approved for Implementation

## Overview

Expand sesh's TUI process detection to identify npm, yarn, and node processes running in tmux sessions. Display branded Nerd Font icons next to sessions running these processes.

## Goals

- Detect multiple process types per session (node, npm, yarn)
- Show all detected processes with distinct branded icons
- Maintain async behavior (doesn't block initial render)
- Use official Nerd Font glyphs for visual consistency

## Current State

**Detection:** Only detects "node" processes, returns immediately on first match
**Icon:** Green hexagon `⬢` for node
**Storage:** Single process per session (`map[string]string`)

## Proposed Changes

### 1. Detection Logic

**File:** `tui/commands.go` (lines 69-108)

**Current behavior:**
- Scans panes with `tmux list-panes`
- Returns immediately when "node" is found
- Single process per session

**New behavior:**
```go
func detectProcessForSession(session model.SeshSession) tea.Cmd {
    return func() tea.Msg {
        if session.Src != "tmux" {
            return nil
        }

        cmd := exec.Command("tmux", "list-panes", "-t", session.Name, "-F", "#{pane_current_command}")
        output, err := cmd.Output()
        if err != nil {
            return nil
        }

        // Collect all detected processes
        detected := make(map[string]bool)

        lines := strings.Split(string(output), "\n")
        for _, line := range lines {
            line = strings.TrimSpace(strings.ToLower(line))
            if line == "" {
                continue
            }

            // Check for each process type
            if line == "node" || strings.HasPrefix(line, "node ") {
                detected["node"] = true
            }
            if line == "yarn" || strings.HasPrefix(line, "yarn ") {
                detected["yarn"] = true
            }
            if line == "npm" || strings.HasPrefix(line, "npm ") {
                detected["npm"] = true
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

**Key changes:**
- Scans all panes completely before returning
- Uses map to dedupe (if multiple panes run same process)
- Detects "npm" and "yarn" in addition to "node"
- Returns slice of processes instead of single process

### 2. Data Structure Changes

**File:** `tui/messages.go` (lines 22-25)

**Current:**
```go
type ProcessDetectedMsg struct {
    SessionName string
    Process     string
}
```

**New:**
```go
type ProcessDetectedMsg struct {
    SessionName string
    Processes   []string
}
```

---

**File:** `tui/model.go` (line 36)

**Current:**
```go
processInfo map[string]string  // sessionName -> process
```

**New:**
```go
processInfo map[string][]string  // sessionName -> []processes
```

---

**File:** `tui/update.go` (lines 103-111)

**Current:**
```go
case ProcessDetectedMsg:
    m.processInfo[msg.SessionName] = msg.Process
    return m, nil
```

**New:**
```go
case ProcessDetectedMsg:
    m.processInfo[msg.SessionName] = msg.Processes
    return m, nil
```

### 3. Icon Rendering

**File:** `tui/delegate.go` (lines 14, 31-37)

**Current:**
```go
// Field
processInfo map[string]string

// Rendering
if process, ok := d.processInfo[session.Name]; ok {
    processIcon := lipgloss.NewStyle().
        Foreground(lipgloss.Color("34")).
        Render("⬢")
    icon += processIcon + " "
}
```

**New:**
```go
// Field
processInfo map[string][]string

// Rendering
if processes, ok := d.processInfo[session.Name]; ok {
    // Map process names to Nerd Font icons and colors
    processIcons := map[string]struct{
        Icon  string
        Color string
    }{
        "node": {Icon: "\ue718", Color: "34"},   // green
        "npm":  {Icon: "\ue71e", Color: "196"},  // red
        "yarn": {Icon: "\ue6a7", Color: "39"},   // blue
    }

    // Render all detected processes
    for _, proc := range processes {
        if iconData, exists := processIcons[proc]; exists {
            processIcon := lipgloss.NewStyle().
                Foreground(lipgloss.Color(iconData.Color)).
                Render(iconData.Icon)
            icon += processIcon + " "
        }
    }
}
```

**Icon details:**
- **Node.js**: `` (U+E718) in green (#34)
- **npm**: `` (U+E71E) in red (#196)
- **Yarn**: `` (U+E6A7) in blue (#39)

These are official Nerd Font glyphs from the Devicons set.

## Files to Modify

| File | Lines | Changes |
|------|-------|---------|
| `tui/commands.go` | 69-108 | Update detection logic for multi-process |
| `tui/messages.go` | 22-25 | Change `Process` to `Processes []string` |
| `tui/model.go` | 36 | Change map value type to `[]string` |
| `tui/delegate.go` | 14, 31-37 | Update field type and rendering logic |
| `tui/update.go` | 103-111 | Update message handler |

## Testing Strategy

1. **Single process:** Session running only node
2. **Single process:** Session running only npm/yarn
3. **Multiple processes:** Session running `npm run dev` (spawns node)
4. **No processes:** Session with no JavaScript processes
5. **Visual:** Verify icons render with Nerd Font installed

## Edge Cases

- **Deduplication:** Multiple panes running same process → handled by map
- **No detection:** No processes found → no message, no icons
- **Non-tmux sessions:** Skipped entirely in detection
- **Missing Nerd Font:** Glyphs show as boxes/missing chars (acceptable)

## Performance

- **Async behavior preserved:** Detection doesn't block initial TUI render
- **Cost:** ~100ms per session (existing overhead, not increased)
- **Benefit:** Single scan detects all processes (no additional tmux calls)

## References

- [Nerd Fonts](https://www.nerdfonts.com) - Icon font collection
- [Nerd Fonts Cheat Sheet](https://www.nerdfonts.com/cheat-sheet) - Icon codes
- [Devicons](https://vorillaz.github.io/devicons/) - Development tool icons
- [npm logos](https://github.com/npm/logos) - Official npm branding
- [Yarn assets](https://github.com/yarnpkg/assets) - Official Yarn branding

## Implementation Notes

- Requires Nerd Font installed in user's terminal for proper icon display
- Icons use brand-accurate colors matching official logos
- Multiple icons appear side-by-side when multiple processes detected
- Detection pattern uses exact match or prefix (e.g., "npm" or "npm run dev")
