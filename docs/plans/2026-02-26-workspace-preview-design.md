# Workspace Sub-Project Preview Design

## Problem

Workspace sessions (monorepo sub-projects like `chase-web/packages/ui/main`) show no meaningful preview — just a directory listing. Their paths don't contain `.git` directly (it lives at the monorepo root), so `isGitRepo()` returns false and they skip the rich git preview.

This affects both:
- **Inactive** workspace sessions (`src="workspace"`)
- **Active** workspace tmux sessions (`src="tmux"` with a workspace sub-project path, shown via duplicate hiding)

## Design

### Detection

In `loadPreview`, after computing `isGit`, add:

```go
isSubdirGit := !isGit && path != "" && isInsideGitWorkTree(path)
```

`isInsideGitWorkTree` runs `git -C <path> rev-parse --is-inside-work-tree` (~1-2ms). When true, route to `GenerateWorkspacePreview` — this takes priority over pane capture for active sessions.

### Preview Generation

`GenerateWorkspacePreview` runs **6 commands in parallel** (all use `git -C <subpath>` so git walks up to find the repo root):

| # | Command | Purpose |
|---|---------|---------|
| 1 | `git branch --show-current` | Branch name (reuse `getGitBranch`) |
| 2 | `git rev-list --left-right --count @{upstream}...HEAD` | Ahead/behind (reuse `getGitTracking`) |
| 3 | `git status --short -- .` | Changes in sub-project (displayed) |
| 4 | `git status --short` | Total change count (for "other" hint) |
| 5 | `git log --oneline --graph --decorate --color=always -3 -- .` | Commits touching this folder |
| 6 | Claude sessions DB query | Same as current |

Additional parallel command:
- `git rev-parse --show-prefix` → relative path for display (e.g., `packages/ui/`)

### Output Format

Identical to current rich git preview, except status and log are folder-scoped:

```
󰘬 main ↑2

━━━ Claude Sessions ━━━
 ● fix button hover  @main  2m

━━━ Status ━━━
 M packages/ui/src/Button.tsx
 M packages/ui/src/Input.tsx
  5 other files changed outside packages/ui

━━━ Recent Commits ━━━
* abc1234 fix: button hover state
* def5678 feat: add Input component
* ghi9012 refactor: shared styles
```

- "other files" line is dimmed, only shown when count > 0
- Relative path in hint comes from `git rev-parse --show-prefix` (trimmed of trailing slash)
- Active/inactive dimming works the same as regular rich preview

## Implementation

All changes in `tui/` — pure preview layer, no model/lister/config changes.

### File: `tui/preview_generator.go`

New functions:

1. **`isInsideGitWorkTree(path string) bool`** — runs `git -C <path> rev-parse --is-inside-work-tree`, returns true if output is "true"

2. **`getGitRelativePrefix(path string) string`** — runs `git -C <path> rev-parse --show-prefix`, trims trailing slash. Returns e.g. `packages/ui`

3. **`getGitStatusFiltered(path string, isActive bool) (filtered string, otherCount int)`** — runs `git status --short -- .` and `git status --short` in parallel via goroutines. Returns the filtered output for display and `total_lines - filtered_lines` as the other count.

4. **`getGitLogFiltered(path string, isActive bool) string`** — runs `git -C <path> log --oneline --graph --decorate --color=always -3 -- .`. Applies dimming for inactive.

5. **`GenerateWorkspacePreview(sessionName, path string, isActive bool) string`** — launches all 7 parallel commands (branch, tracking, filtered status, total status, filtered log, relative prefix, claude sessions). Assembles output in same format as `GenerateRichPreview` with the "other files" hint line.

### File: `tui/commands.go`

In `loadPreview`, add early check:

```go
isSubdirGit := !isGit && path != "" && isInsideGitWorkTree(path)
if isSubdirGit {
    content := GenerateWorkspacePreview(session.Name, path, isActive)
    return PreviewLoadedMsg{Content: content}
}
```

This goes before the existing `isActive && isGit` check, so workspace sub-projects always get the filtered preview regardless of active/inactive state.

## Performance

- All 7 git/DB commands run in parallel via goroutines
- `git -C <subpath>` avoids needing to resolve git root separately
- Expected total time: ~8-15ms (bounded by slowest git command, same as current rich preview)
- `isInsideGitWorkTree` detection adds ~1-2ms only when `isGitRepo` is false
