# Worktree Default & Smart Grouping Design

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Worktree groups show a user-picked default branch as the title. Enter opens the default directly. Tab expands to browse siblings. Ctrl+P sets defaults. Ctrl+T focuses on a single repo's worktrees.

**Builds on:** The existing worktree grouping implementation (collapsed groups, accordion expand/collapse, grouped filter results).

---

## Core Behavior

### Group States

A worktree group has two display states: **collapsed** (single line) and **expanded** (individual worktrees).

**Collapsed with default set:**
- Title: `chase-monorepo/develop (+3)` — default branch in name, dimmed `(+N)` showing remaining dormant count
- Enter: opens the default worktree session directly (connects)
- Tab: expands the group to show all dormant worktrees

**Collapsed without default:**
- Title: `chase-monorepo (+4)` — repo name only, count is total dormant worktrees
- Enter: expands the group (same as Tab) so user can browse and pick
- Tab: also expands

**Expanded:**
- Collapsed line replaced by individual worktree items
- Default worktree marked with ★ prefix
- Non-default worktrees indented 2 spaces
- Enter on any worktree: connects to it
- Tab: collapses back to single line
- Escape: also collapses (or quits if nothing expanded)

### Active Worktrees

Worktrees with active tmux sessions always appear as individual items at the top of the list. They are never bundled into groups. Groups only contain dormant worktrees.

### Grouping Threshold

- Only repos with 2+ dormant worktrees form a group
- Single dormant worktree shows as a regular item (no `(+0)`)
- All worktrees active → no group shown

---

## Key Bindings

| Key | Action |
|-----|--------|
| Enter | Open session (collapsed+default) / Expand (collapsed+no default) / Connect (individual worktree) |
| Tab | Toggle expand/collapse on a worktree group |
| Ctrl+P | Set selected worktree as default for its repo (toggle: pressing on current default clears it) |
| Ctrl+T | Toggle repo focus — filter list to show only this repo's worktrees |
| Ctrl+E | Process detection (moved from Tab) |
| Escape | Collapse expanded group / quit |

---

## Visual Design

### Collapsed Group (has default)

```
  📁 chase-monorepo/develop (+3)
```

Green folder icon (consistent with projects source). Default branch name in the title. `(+3)` in dimmed gray — 3 more dormant worktrees besides the default.

### Collapsed Group (no default)

```
  📁 chase-monorepo (+4)
```

Repo name only. Count is total dormant worktrees.

### Expanded Group

```
  ★ 📁 chase-monorepo/develop
    📁 chase-monorepo/review
    📁 chase-monorepo/main
    📁 chase-monorepo/hotfix
```

Default gets ★ prefix (gold/yellow). Non-default siblings indented 2 spaces. Each is a regular selectable item.

### Repo Focus View (Ctrl+T)

```
  🟢 chase-monorepo/feature-cdk    (active tmux session)
  ★ 📁 chase-monorepo/develop      (dormant, default)
    📁 chase-monorepo/review        (dormant)
    📁 chase-monorepo/main          (dormant)
```

Shows ALL worktrees for the repo — active tmux sessions at top, dormant below. Flat list, no grouping. Ctrl+T again returns to full list.

### Count Logic

- **Has default:** `(+N)` where N = dormant worktrees minus the default
- **No default:** `(+N)` where N = total dormant worktrees
- **N = 0:** no `(+0)` suffix shown (but group still shows if default is set and there are other active worktrees)

---

## Setting Defaults

### Ctrl+P Behavior

- Works on any project worktree anywhere in the list (expanded group, individual item, repo focus view)
- Sets the worktree's branch as default for its repo
- Pressing Ctrl+P on the current default clears it (toggle)
- On non-worktree items (tmux sessions, regular projects): does nothing

### Visual Feedback

- Group collapses instantly showing new default in title
- Title change IS the feedback — no toast/notification needed

---

## Storage

### File Location

```
~/.local/state/sesh/worktree-defaults.json
```

Respects `$XDG_STATE_HOME` if set, falls back to `~/.local/state`. This is mutable application state, NOT config — keeps dotfiles repo clean.

### Format

```json
{
  "chase-monorepo": "develop",
  "geoip": "main",
  "homestory-platform": "staging"
}
```

Flat map: repo name → branch name.

### Read Path (startup)

- Single `os.ReadFile` + `json.Unmarshal` on ~100 bytes
- Loaded inline at startup alongside config
- Sub-millisecond, no impact on first render
- File doesn't exist → empty map, no error

### Write Path (Ctrl+P)

- Model updates `defaults` map in memory — instant UI update
- Fire-and-forget `tea.Cmd` writes JSON to disk in background
- If write fails, state is still correct in memory (just won't persist across sessions)
- Creates `~/.local/state/sesh/` directory if missing

---

## Interaction Flows

### First Time (no defaults)

1. Open sesh — collapsed groups: `chase-monorepo (+4)`, `geoip (+2)`
2. Enter on `chase-monorepo (+4)` — expands to show all 4 worktrees
3. Navigate to `develop`, Ctrl+P — group collapses to `chase-monorepo/develop (+3)` with ★ saved
4. Next session: Enter on that group opens develop directly

### Daily Workflow (defaults set)

1. Open sesh — see `chase-monorepo/develop (+3)`
2. Enter — connects to develop instantly

### Exploring Siblings

1. Tab on `chase-monorepo/develop (+3)` — expands all dormant worktrees
2. Navigate to `review`, Enter — connects to review (default unchanged)
3. Or: Ctrl+P on `review` — sets new default, collapses to `chase-monorepo/review (+3)`

### Repo Focus

1. Cursor on any chase-monorepo item, press Ctrl+T
2. List filters to only chase-monorepo worktrees (active + dormant, flat)
3. Browse, connect, set defaults
4. Ctrl+T again — back to full list

---

## Data Model Changes

### New/Modified Fields on Model

```go
worktreeDefaults map[string]string  // repo → default branch (loaded from JSON)
repoFocusFilter  string             // repo name when Ctrl+T is active ("" = no focus)
```

### Changes to worktreeGroup

```go
type worktreeGroup struct {
    repoName      string
    defaultBranch string          // from worktreeDefaults map
    worktrees     []sessionItem
    tmuxNames     map[string]bool
}
```

### New Files

- `state/defaults.go` — read/write worktree defaults JSON (XDG_STATE_HOME path resolution, atomic write)

### Modified Files

- `tui/model.go` — new Model fields, load defaults at init, pass to grouping
- `tui/grouping.go` — `buildWorktreeGroups` accepts defaults, `formatGroupDisplay` uses default branch in title
- `tui/update.go` — new key handlers (Tab, Ctrl+P, Ctrl+T, Ctrl+E), updated Enter logic
- `tui/keys.go` — new key bindings
- `tui/delegate.go` — ★ marker for default worktree in expanded view
- `tui/item.go` — `worktreeGroupItem` gets `defaultBranch` field
- `tui/commands.go` — async command for writing defaults to disk

### No Changes

- `lister/`, `connector/`, `icon/`, `model/` packages untouched
- Session loading, config parsing, preview generation unchanged

---

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| Only 1 dormant worktree | No group — shows as regular item |
| All worktrees active | No group — all shown individually as tmux sessions |
| Default worktree becomes active | Group still shows (with default in title), count adjusts. Enter still connects. |
| Default worktree deleted/removed | Default cleared from map, group reverts to no-default format |
| Repo has no worktrees (regular project) | Not grouped, Ctrl+P/Ctrl+T do nothing |
| Ctrl+T on non-worktree item | Does nothing |
| defaults.json corrupted/invalid | Treated as empty — all groups show no-default format |

---

## Performance

All operations are pure in-memory except the defaults file:

- **Read:** <0.5ms at startup (tiny JSON file)
- **Write:** async background command, never blocks UI
- **Grouping:** same O(n) passes as before, just uses defaults map for title format
- **Ctrl+T filter:** in-memory filter of existing items list
- **Tab expand/collapse:** same item swap as current implementation
