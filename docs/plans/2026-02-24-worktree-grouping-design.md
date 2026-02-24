# Worktree Grouping Design

**Date:** 2026-02-24
**Status:** Approved for Implementation

## Overview

Collapse git worktrees into grouped entries in the TUI session list to reduce visual clutter. Worktrees expand inline on selection (accordion) and auto-group when filtering by search.

## Problem

Bare repos with multiple worktrees flood the session list. A search for "monorepo" scatters `chase-monorepo` worktrees across the results, mixed with other repos. The list becomes hard to scan.

## Goals

- Clean default list: worktrees collapsed under parent repo
- Smart filtering: search results grouped by repo
- Zero added latency on first render
- No changes outside `tui/` package — purely presentation

## Core Behavior

### Default View (No Filter)

- Worktrees collapse into a single entry: `chase-monorepo [1/3]`
- Format: `[dormant/total]` — dormant = no active tmux session, total = all worktrees
- Active worktree sessions appear separately in the tmux section with their own icons
- The collapsed entry uses the projects icon (green folder)
- If all worktrees are active (showing as tmux sessions), the collapsed entry is hidden entirely

### Expand (Select Collapsed Entry)

- Worktrees expand inline below the parent
- Only dormant worktrees shown (active ones already visible in tmux section)
- Accordion: expanding one group collapses any previously expanded group

### Collapse Back

- Selecting a worktree connects — state resets
- Pressing Escape on expanded group collapses it
- Expanding another group auto-collapses the current one

### Filtering (Typing)

- Bypasses collapse logic — all matching items visible
- Results grouped by repo: parent anchors position by fuzzy score, worktrees follow
- Both active (tmux) and dormant (projects) results cluster together within a group
- Tmux icon/color distinguishes active from dormant within the group

## Interaction Flow

### Startup

1. Lister loads sessions as today — no change
2. TUI model builds `worktreeGroups` from loaded sessions (string split on `/`)
3. List renders with collapsed entries replacing worktree clusters
4. Zero added latency — pure in-memory grouping

### Browsing (No Filter)

1. User scrolls list — tmux sessions first, then projects with collapsed groups
2. Cursor lands on `chase-monorepo [1/3]`
3. Enter → group expands inline, showing dormant worktrees indented below
4. Navigate to `chase-monorepo ⎇ develop`, Enter → connects
5. Or Escape → group collapses back

### Searching (Typing Filter)

1. User types "monorepo"
2. Fuzzy filter matches all sessions containing "monorepo"
3. Results re-sorted: grouped by repo, parent anchors position
4. No collapse in filter mode — all matching worktrees visible and grouped
5. Select one → connects

### Accordion

1. `chase-monorepo [1/3]` is expanded
2. Scroll to `frontend-monorepo [2/2]`, Enter
3. `chase-monorepo` auto-collapses, `frontend-monorepo` expands
4. Only one group open at a time

## Visual Design

### Collapsed Entry

```
  󰉋 chase-monorepo [1/3]
```

- Green folder icon (existing projects icon)
- Repo name in normal text
- `[1/3]` as a dimmed/subtle badge

### Expanded Entry

```
  󰉋 chase-monorepo [1/3]
     󰉋 chase-monorepo ⎇ review
     󰉋 chase-monorepo ⎇ develop
```

- Parent stays visible as the group header
- Worktrees indented slightly (2-3 spaces) to show hierarchy
- Only dormant worktrees listed

### Filtered Results (Typing)

```
  󰆍 chase-monorepo                    ← tmux (active)
  󰆍 chase-monorepo ⎇ feature-cdk     ← tmux (active)
  󰉋 chase-monorepo ⎇ review          ← project (dormant)
  󰉋 chase-monorepo ⎇ develop         ← project (dormant)
  󰉋 frontend-monorepo
  󰉋 frontend-monorepo ⎇ develop
```

- Grouped by repo, mixed sources within group
- Icons/colors distinguish tmux vs projects

## Data Model Changes

No new data sources or subprocess calls. All grouping computed in-memory from existing session lists.

### New State in TUI Model

- `worktreeGroups`: map of repo name → list of worktree sessions
- `expandedGroup`: string tracking which group is currently expanded (empty = all collapsed)
- Built once when sessions load by splitting names on `/`
- Count of active tmux sessions per group via in-memory map lookup

### List Item Changes

- Collapsed: worktree group replaced by a single synthetic `groupItem`
- Expanded: synthetic item replaced by actual worktree items
- Filter mode: collapse logic bypassed, items re-sorted by repo grouping
- Expanded group + start typing: collapse immediately, switch to filter mode

## Edge Cases

| Case | Behavior |
|------|----------|
| Single worktree repo | No grouping — shows as normal entry |
| Bare repo with no worktrees | Shows normally as project entry, no badge |
| All worktrees active | Collapsed entry hidden (all already in tmux section) |
| Ctrl+0 (GoToWorktreeRoot) | Works in filter mode; not needed in collapsed mode |
| Worktrees added/removed between opens | Groups rebuild fresh every TUI launch |

## Implementation

### Files to Modify

| File | Change |
|------|--------|
| `tui/model.go` | Add `worktreeGroups` map, `expandedGroup` string, grouping logic on session load |
| `tui/item.go` | New `groupItem` type for collapsed entries (synthetic, not a real session) |
| `tui/update.go` | Handle Enter on group items (expand/collapse), accordion logic, Escape to collapse |
| `tui/delegate.go` | Render collapsed entries with `[1/3]` badge, indent expanded worktrees |
| `tui/model.go` | Filter mode: bypass collapse, re-sort results by repo grouping |

### Files NOT Modified

| File | Reason |
|------|--------|
| `lister/projects.go` | Data loading unchanged |
| `connector/` | Connection logic unchanged |
| `icon/icon.go` | Existing icons sufficient |
| `model/sesh_session.go` | Session data structure unchanged |

### Configuration

None needed. Purely a smarter default. `include_worktrees = true` in `sesh.toml` still controls whether worktrees are detected at all.

## Testing Strategy

- Group building from session list (unit test)
- Active count computation (unit test)
- Expand/collapse state transitions (unit test)
- Filter mode grouping sort (unit test)
- All worktrees active → group hidden (unit test)
- Single worktree → no group, show normally (unit test)

## Performance

- **First render**: Zero added latency. Grouping is pure in-memory string operations on already-loaded data.
- **Expand/collapse**: Instant — swapping items in an in-memory list.
- **Filter grouping**: Re-sort after fuzzy match — negligible cost on filtered (small) result sets.
