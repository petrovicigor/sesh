# Claude Attention Icon Design

## Problem

When a Claude Code session running inside a tmux session needs user confirmation (status `awaiting:*`), there's no way to know without switching to that session. The user wants a visible 🖐️ icon in two places:

1. **TUI session list** — replace the `` tmux icon with 🖐️ for sessions needing attention
2. **tmux status-right** — always-visible indicator via `sesh status` command

Both must be ultra-fast and non-blocking on first render.

## Design

### Shared Query Logic — `claude/attention.go`

New package with two exported functions:

- `SessionsNeedingAttention() (map[string]bool, error)` — returns all tmux session names that have any active CC session with `awaiting:*` status. Used by TUI.
- `NeedsAttention(tmuxSession string) (bool, error)` — single session check. Used by `sesh status` CLI.

Both query `~/.claude/sessions.db`:

```sql
-- SessionsNeedingAttention (TUI)
SELECT DISTINCT tmux_session FROM sessions
WHERE ended_at IS NULL
  AND status LIKE 'awaiting:%'
  AND replaced_by_session_id IS NULL

-- NeedsAttention (CLI)
SELECT 1 FROM sessions
WHERE tmux_session = ?
  AND ended_at IS NULL
  AND status LIKE 'awaiting:%'
  AND replaced_by_session_id IS NULL
LIMIT 1
```

Silent failures — if no DB file, query error, or no results → return empty/false. Never surface errors to the user.

DB path: `~/.claude/sessions.db` via `os.Getenv("HOME") + "/.claude/sessions.db"`.

### TUI Async Icon Enrichment

**Flow:**

1. `Init()` fires `checkClaudeAttentionCmd` alongside existing commands
2. Command calls `claude.SessionsNeedingAttention()` — single SQLite query, returns `map[string]bool`
3. `claudeAttentionMsg` arrives in `Update()`, stored as `m.claudeAttention`
4. `delegate.Render()` checks the map at paint time: if `v.session.Src == "tmux"` and `m.claudeAttention[v.session.Name]` is true, render magenta 🖐️ instead of the normal blue ``

**First render is instant** — icons show as normal ``, then swap to 🖐️ when the async query resolves (typically <2ms, so effectively same frame in practice).

**No periodic refresh** — the TUI is short-lived. One check on load is sufficient.

**Icon swap is render-time only** — the `sessionItem.iconPrefix` is not mutated. The delegate overrides at paint time. No item rebuild needed.

### `sesh status` CLI Command — `seshcli/status.go`

Standalone command meant for tmux status-right:

```go
func NewStatusCommand() *cobra.Command {
    return &cobra.Command{
        Use:   "status",
        Short: "Show attention indicator for current tmux session",
        RunE: func(cmd *cobra.Command, args []string) error {
            // 1. Get current tmux session name via: tmux display-message -p "#S"
            // 2. Call claude.NeedsAttention(sessionName)
            // 3. Print 🖐️ if true, nothing if false
        },
    }
}
```

**Key decisions:**
- No dependency injection — doesn't need lister, connector, icon, or config. Standalone SQLite + tmux subprocess.
- Gets tmux session name itself via `exec.Command("tmux", "display-message", "-p", "#S")`
- Silent failures — no tmux, no DB, any error → print nothing, exit 0
- No ANSI colors — tmux status-right handles its own styling. Raw 🖐️ or empty string.

**tmux.conf integration:**
```
set -g status-right '#(sesh status)'
set -g status-interval 2
```

Worst case 2s lag between CC entering awaiting state and the icon appearing.

## Edge Cases

- **No `sessions.db` file** — `os.Stat` check, return empty/false
- **Multiple CC sessions in one tmux session** — `DISTINCT` / `LIMIT 1` handles it
- **Worktree group items in TUI** — only `sessionItem` with `Src == "tmux"` gets the swap
- **Workspace tmux sessions** — active workspace sessions have `Src == "tmux"` too, they get 🖐️ correctly
- **`sesh status` called outside tmux** — `tmux display-message` fails, print nothing, exit 0

## Files

| File | Change |
|------|--------|
| `claude/attention.go` | **New** — shared query logic (~40 lines) |
| `seshcli/status.go` | **New** — `sesh status` CLI command |
| `seshcli/root_command.go` | Add `status` subcommand registration |
| `tui/commands.go` | Add `checkClaudeAttentionCmd` |
| `tui/messages.go` | Add `claudeAttentionMsg` type |
| `tui/model.go` | Add `claudeAttention map[string]bool` field |
| `tui/update.go` | Handle `claudeAttentionMsg` |
| `tui/delegate.go` | Icon override in `Render()` for tmux sessions |

**Not touched:** `icon/icon.go`, `tui/preview_generator.go` (existing code stays as-is).

## Performance

- TUI: one SQLite query on init (~1-2ms), non-blocking first render
- CLI: one `tmux display-message` (~1ms) + one SQLite query (~1ms) = ~2ms total
- tmux status-interval of 2s means the `sesh status` process spawns every 2s — negligible overhead
