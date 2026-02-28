# Group-by Pivot Design (Package-first vs Branch-first)

## Problem

Workspace sessions in monorepos form a 2D grid: packages × worktrees/branches. The current TUI groups by package (e.g., `mono/packages/ui` → branches underneath). But when doing cross-cutting work on a feature branch touching multiple packages, the user wants to see all packages on that branch together.

## Design

### Group Mode

New `GroupMode` type with two values:
- `GroupByPackage` (default) — key = everything before last `/` → `mono/packages/ui`
- `GroupByBranch` — key = `{workspace}/{branch}` → `mono/develop`

Toggle via `ctrl+g`. Package-first is always the starting mode (no persistence). Only affects workspace groups — regular worktree groups stay repo-first regardless.

### Key Extraction

Session name format: `{workspace}/{subproject}/{branch}`

**Package-first key** (current):
- `frontend-monorepo/packages/ui/develop` → `frontend-monorepo/packages/ui`

**Branch-first key** (new):
- `frontend-monorepo/packages/ui/develop` → `frontend-monorepo/develop`
- Uses `workspacePrefixes` to identify the workspace name, last segment is branch

### Display

**Package-first (current):**
```
📦 mono/packages/ui ⎇ develop (+)
  ├ packages/ui ⎇ develop
  └ packages/ui ⎇ feature-x
```

**Branch-first:**
```
📦 mono ⎇ develop (+3)
  ├ packages/ui
  ├ packages/api
  └ packages/shared
```

Children in branch-first mode show just the sub-project path (middle part of session name).

### Toggle Behavior

`ctrl+g` handler:
1. Flip `m.groupMode`
2. Collapse any expanded group
3. Clear repo focus filter
4. Rebuild `m.worktreeGroups` with new mode
5. Rebuild display items
6. Reset cursor to top, load preview

Title bar shows `[pkg]` or `[branch]` indicator (dimmed).

### Edge Cases

- Single-worktree workspace: branch-first creates one group with all packages as children (useful)
- Non-workspace worktree groups: unaffected, always repo-first
- Active tmux workspace sessions: grouped same as inactive, just with tmux source preference

## Implementation

All changes in `tui/` — pure presentation layer.

### `tui/grouping.go`
- Add `GroupMode` type and constants
- Modify `groupKeyForItem(name, src, workspacePrefixes, mode)` — branch-first path for workspace items
- Add `branchFirstKey(name, workspacePrefixes)` — extracts `{workspace}/{branch}`
- Add `childDisplayName(sessionName, groupKey, mode, workspacePrefixes)` — returns sub-project for branch-first, branch for package-first
- Modify `buildWorktreeGroups(items, defaults, workspacePrefixes, mode)` — passes mode through
- In `buildDisplayItems()`, reformat child display names based on mode

### `tui/model.go`
- Add `groupMode GroupMode` field
- Pass through to `buildWorktreeGroups` and `buildDisplayItems` calls

### `tui/update.go`
- Add `ToggleGroupMode` to key map bound to `ctrl+g`
- Handler: flip mode, rebuild, reset
- Update all existing calls to pass `groupMode`

### `tui/view.go`
- Append dimmed `[pkg]` or `[branch]` to filter title
