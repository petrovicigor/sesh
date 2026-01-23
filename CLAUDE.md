# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Sesh is a smart terminal session manager written in Go that helps users create and manage tmux sessions quickly and easily using zoxide. It's a CLI tool that integrates with tmux and zoxide to provide intelligent session management.

## Core Architecture

- **Module**: `github.com/joshmedeski/sesh/v2`
- **Go Version**: 1.23.0 (toolchain 1.24.3)
- **Main Entry Point**: `main.go` → `seshcli.App()`

### Key Packages

- `seshcli/` - CLI command implementations (connect, list, preview, clone, last, tui)
- `lister/` - Lists sessions from various sources (tmux, zoxide, config)
- `connector/` - Handles session connections
- `tmux/` - Tmux integration
- `zoxide/` - Zoxide integration
- `configurator/` - Configuration management (`sesh.toml` in `$XDG_CONFIG_HOME/sesh`)
- `tui/` - Terminal UI implementation using Bubble Tea (currently under development)

### Session Sources

1. **Tmux**: Active tmux sessions
2. **Zoxide**: Frequently used directories
3. **Config**: User-defined sessions in configuration
4. **Tmuxinator**: Tmuxinator project configurations
5. **Projects**: Directory scanning from configured project paths (e.g., `~/Projects`)

## Common Development Commands

### Build and Install

**IMPORTANT**: After making any code changes, always build the binary:

```bash
# Build to the symlink target (ALWAYS USE THIS)
go build -o /Users/igorpetrovic/Projects/sesh/bin/sesh
```

**IMPORTANT**: `/Users/igorpetrovic/Dotfiles/bin/sesh` is a **symlink** to `/Users/igorpetrovic/Projects/sesh/bin/sesh`. DO NOT copy the binary - just build to the symlink target location. The symlink will automatically point to the new binary.

**Never do this:**
```bash
# WRONG - Don't copy, the file is a symlink!
go build -o ./sesh && cp ./sesh /Users/igorpetrovic/Dotfiles/bin/sesh
```

### Test
```bash
make test
# Or directly:
go test -cover -bench=. -benchmem -race ./... -coverprofile=coverage.out
```

### Run a single test
```bash
go test -run TestFunctionName ./package/...
```

### Generate mocks
```bash
# Uses mockery v2.52.3
mockery --all
```

## Projects Source - Implementation Details

### Overview

The **Projects** source scans configured directory paths (e.g., `~/Projects`) and exposes all subdirectories as available sessions. It includes automatic git worktree detection and uses sesh's native custom naming pattern (separate display name and filesystem path).

### Architecture Pattern: Name/Path Separation

Projects source follows the same pattern as Config sessions:

```go
// Display name vs. Filesystem path
model.SeshSession{
    Src:  "projects",
    Name: "chase-cognito",           // Short display name (shown in list)
    Path: "/Users/user/Projects/chase-cognito", // Full path (used for connection)
}
```

**Benefits:**
- Clean, short names in the UI
- Full paths preserved for connection
- No hardcoded path construction in shell scripts
- Consistent with existing sesh patterns

### Key Files

#### Core Implementation

**`lister/projects.go`** - Main projects source logic
- `listProjects()`: Scans configured paths, detects worktrees, builds sessions
- `scanDirectoryFast()`: Optimized directory scanning (depth=1 fast path)
- `detectWorktreesFast()`: Filesystem-based worktree detection (checks if `.git` is file vs directory)
- `getProjectDisplayName()`: Generates short names (worktrees: `repo/branch`, regular: `basename`)
- `FindProjectSession()`: Lookup method for path resolution

**`connector/projects.go`** - Connection strategy
- `projectStrategy()`: Resolves project names to paths for connection

**`seshcli/path.go`** - Path resolution command
- `NewPathCommand()`: CLI command that resolves session names to filesystem paths
- Tries all sources in order: projects → config → tmux → zoxide → tmuxinator

#### Configuration

**`model/config.go`** - Configuration data structures
```go
type ProjectsConfig struct {
    Paths            []string       // e.g., ["~/Projects"]
    MaxDepth         int            // Scan depth (default: 1)
    IncludeWorktrees bool           // Auto-detect git worktrees
    Exclude          []string       // Patterns to exclude
    Saved            []SavedProject // Additional paths to include
}
```

**Example `sesh.toml`:**
```toml
[projects]
paths = ["~/Projects"]
max_depth = 1
include_worktrees = true
exclude = ["node_modules", ".git", "vendor", "build", "dist"]
```

#### Integration Points

**`lister/lister.go`** - Interface definition
- Added `FindProjectSession(name string) (model.SeshSession, bool)` to Lister interface

**`connector/connect.go`** - Connection strategy order
- Projects checked after config, before dir/zoxide strategies

**`icon/icon.go`** - Visual differentiation
- Projects use green folder icon (color code 32) vs. cyan for zoxide (36)

### Display Name Logic

**Regular projects**: Just the basename
```
/Users/user/Projects/chase-cognito → "chase-cognito"
```

**Git worktrees**: `parent/name` format (detected via `.git` being a file, not directory)
```
/Users/user/Projects/geoip/develop → "geoip/develop"
```

**Edge case**: Worktrees directly under project root show as `Projects/name` (acceptable trade-off for simplicity)

### The `sesh path` Command

**Purpose**: Resolves session display names to filesystem paths for shell scripts

**Usage:**
```bash
$ sesh path "chase-cognito"
/Users/user/Projects/chase-cognito

$ sesh path "geoip/develop"
/Users/user/Projects/geoip/develop
```

**Lookup order**: projects → config → tmux → zoxide → tmuxinator

### Shell Script Integration

Both `~/.config/sesh/connect-wrapper.sh` and `~/.config/sesh/preview.sh` follow this pattern:

```bash
# 1. Strip ANSI codes and icon characters
session_name=$(echo "$session" | sed $'s/\033\[[0-9;]*m//g' | tr -cd '[:alnum:][:space:]/_-' | xargs | sed 's/ /\//g')

# 2. Resolve to filesystem path using sesh path
expanded_path=$(sesh path "$session_name" 2>/dev/null)

# 3. Fallback to regex extraction if sesh path fails
if [ -z "$expanded_path" ]; then
    # Extract from display string or tmux session
fi

# 4. Use path for command decision or preview generation
```

**Why this works:**
- Fast: Single subprocess call to `sesh path`
- Reliable: Uses sesh's internal session lookup logic
- No hardcoding: Works with any configured project paths
- Maintainable: Changes to path logic only need to happen in Go code

### Performance Optimizations

**Filesystem-only worktree detection** (~2-3ms):
- Uses `os.Stat()` to check if `.git` is a file (worktree) vs directory (main repo)
- No subprocess calls to `git worktree list` - pure filesystem operations
- 50x faster than previous implementation (~129ms with git commands)

**Fast path for depth=1**: Most common case (single level deep) uses optimized `os.ReadDir` without recursion

### Worktree Detection Technical Details

**How it works:**
```go
// Main git repo: .git is a directory
info, _ := os.Stat("/path/to/repo/.git")
info.IsDir() // true

// Git worktree: .git is a file containing gitdir reference
info, _ := os.Stat("/path/to/repo/branch/.git")
info.IsDir() // false
```

**Edge case - Submodules:**
- Git submodules also have `.git` as a file (not directory)
- Current implementation treats submodules as worktrees
- In practice, submodules are rarely nested directly under project root
- If strict distinction needed, read `.git` file and check for `/worktrees/` string
- This would add ~10-20µs per worktree (negligible vs 125ms savings)

**Trade-off:** Simplicity and speed over 100% accuracy for rare edge case

### Adding New Source Types

To add a new source following this pattern:

1. **Create `lister/newsource.go`**:
   ```go
   func listNewSource(l *RealLister) (model.SeshSessions, error) {
       // Build sessions with Name (display) and Path (filesystem)
   }

   func (l *RealLister) FindNewSourceSession(name string) (model.SeshSession, bool) {
       // Lookup logic for path resolution
   }
   ```

2. **Update `lister/lister.go`**: Add `FindNewSourceSession` to interface

3. **Create `connector/newsource.go`**: Add connection strategy

4. **Update `connector/connect.go`**: Add to strategies list

5. **Update `seshcli/path.go`**: Add to lookup order in `NewPathCommand()`

6. **Update `icon/icon.go`**: Add unique color code for visual differentiation

7. **Update `model/config.go`**: Add configuration struct if needed

## Development Notes

- The project uses dependency injection with interfaces for testability
- Mock files follow the pattern `mock_*.go` generated by mockery
- Configuration is stored in TOML format (`sesh.toml`)
- The TUI feature is currently under active development on the `tui` branch
- When editing code, follow existing patterns for error handling, logging, and interface design
- **Name/Path separation**: Sources that show shortened names must implement a `Find*Session()` method for path resolution
- **Shell integration**: External scripts should use `sesh path` command rather than parsing display strings or hardcoding paths

### Performance Debugging

**Timing instrumentation** is built into `lister/list.go:56-58`:
```go
start := time.Now()
sessions, err := srcStrategies[s](l)
fmt.Fprintf(os.Stderr, "[TIMING] %s: %v\n", s, time.Since(start))
```

This logs each session source's load time to stderr:
```bash
$ sesh list 2>&1 | grep TIMING
[TIMING] config: 18.583µs
[TIMING] tmuxinator: 119.292µs
[TIMING] projects: 2.17725ms
[TIMING] tmux: 8.61225ms
```

**Use this to:**
- Identify bottlenecks when adding new features
- Verify performance improvements
- Debug slow session loading

**Typical timings (as of 2026-01-18):**
- config: <50µs (fast)
- tmuxinator: ~100µs (fast)
- projects: ~2-3ms (filesystem only)
- tmux: ~8-15ms (subprocess overhead)
