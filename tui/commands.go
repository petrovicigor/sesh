package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/joshmedeski/sesh/v2/claude"
	"github.com/joshmedeski/sesh/v2/lister"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/previewer"
	"github.com/joshmedeski/sesh/v2/state"
	"github.com/joshmedeski/sesh/v2/tmux"
)

// Floating-pane geometry for the bind-o float. Height matches the bind's -y;
// the width swaps on the preview toggle (compact = preview off), and each
// width carries the -X that keeps it horizontally centred — the toggle keeps
// the TOP edge fixed (bind -Y 6%, shared with the claude-sessions floats)
// instead of re-centring vertically.
const (
	floatCompactWidth = "45%"
	floatCompactX     = "27%"
	floatWideWidth    = "80%"
	floatWideX        = "10%"
	floatHeight       = "75%"
)

// resizeOwnFloat resizes sesh's own floating pane to the width matching the
// preview state, horizontally re-centred, top edge fixed. Called
// synchronously from togglePreview — see the ordering rationale there. On
// error the pane keeps its old size, which the layout (driven by real
// WindowSizeMsg values) is still correct for.
func resizeOwnFloat(t tmux.Tmux, showPreview bool) error {
	width, x := floatCompactWidth, floatCompactX
	if showPreview {
		width, x = floatWideWidth, floatWideX
	}
	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		return nil
	}
	_, err := t.ResizeFloatingPane(pane, width, floatHeight, x)
	return err
}

func loadSessionsWithFilter(l lister.Lister, filter FilterType) tea.Cmd {
	return func() tea.Msg {
		// Invalidate tmux cache to get fresh session data
		l.InvalidateTmuxCache()

		opts := lister.ListOptions{
			HideDuplicates: true, // Hide duplicate sessions
		}

		switch filter {
		case FilterConfig:
			opts.Config = true
		case FilterZoxide:
			opts.Zoxide = true
		case FilterAll:
			// No filter - load all
		}

		sessions, err := l.List(opts)
		if err != nil {
			return nil
		}
		return SessionsLoadedMsg{Sessions: sessions}
	}
}

func loadSessionsPreservingState(l lister.Lister, filter FilterType, filterText string, cursorIndex int) tea.Cmd {
	return func() tea.Msg {
		// Invalidate tmux cache to get fresh session data after deletion
		l.InvalidateTmuxCache()

		opts := lister.ListOptions{
			HideDuplicates: true, // Hide duplicate sessions
		}

		switch filter {
		case FilterConfig:
			opts.Config = true
		case FilterZoxide:
			opts.Zoxide = true
		case FilterAll:
			// No filter - load all
		}

		sessions, err := l.List(opts)
		if err != nil {
			return nil
		}
		return SessionsLoadedMsg{
			Sessions:            sessions,
			PreserveFilterText:  filterText,
			PreserveCursorIndex: cursorIndex,
		}
	}
}

func loadPreview(ctx context.Context, p previewer.Previewer, session model.SeshSession) tea.Cmd {
	return func() tea.Msg {
		logDebug("loadPreview: starting for %q (src=%s, path=%s)", session.Name, session.Src, session.Path)
		isActive := (session.Src == "tmux")
		path := session.Path

		// No path → fallback previewer
		if path == "" {
			content, err := p.Preview(session.Name)
			if err != nil {
				return PreviewLoadedMsg{Content: "Error loading preview: " + err.Error()}
			}
			if content == "" {
				return PreviewLoadedMsg{Content: "No preview available"}
			}
			return PreviewLoadedMsg{Content: content}
		}

		// Check cancellation before expensive git operations
		if ctx.Err() != nil {
			return nil
		}

		// Detect git status
		isGit := isGitRepo(path)
		isSubdirGit := !isGit && isInsideGitWorkTree(path)

		if ctx.Err() != nil {
			return nil
		}

		if isSubdirGit {
			var wg sync.WaitGroup
			var data *workspaceGitData
			var claudeSessions string
			wg.Add(2)
			go func() { defer wg.Done(); data = fetchWorkspaceGitData(ctx, path, isActive) }()
			go func() { defer wg.Done(); claudeSessions = getClaudeSessions(path, session.Name, isActive) }()
			wg.Wait()
			if ctx.Err() != nil {
				return nil
			}
			content := composeWorkspacePreviewFromCache(session.Name, isActive, data, claudeSessions)
			return PreviewLoadedMsg{Content: content}
		}

		if isGit {
			var wg sync.WaitGroup
			var data *gitData
			var claudeSessions string
			wg.Add(2)
			go func() { defer wg.Done(); data = fetchGitData(ctx, path, isActive) }()
			go func() { defer wg.Done(); claudeSessions = getClaudeSessions(path, session.Name, isActive) }()
			wg.Wait()
			if ctx.Err() != nil {
				return nil
			}
			content := composeGitPreview(session.Name, isActive, data, claudeSessions)
			return PreviewLoadedMsg{Content: content}
		}

		// Active non-git: try pane capture
		if isActive {
			content, err := p.Preview(session.Name)
			if err == nil && content != "" {
				return PreviewLoadedMsg{Content: content}
			}
		}

		// Non-git with path: directory tree
		dirTree := getDirectoryTree(path, isActive)
		cyan := colorCyan
		if !isActive {
			cyan = colorCyanDim
		}
		var saved string
		if !isActive {
			saved = getSavedStateInfo(session.Name, isActive)
			if saved != "" {
				saved += "\n"
			}
		}
		content := fmt.Sprintf("%s📁 %s%s\n%s%s", cyan, path, colorReset, saved, dirTree)
		return PreviewLoadedMsg{Content: content}
	}
}

// saveDefaults writes worktree defaults to disk asynchronously
func saveDefaults(path string, defaults map[string]string) tea.Cmd {
	// Copy the map to avoid concurrent access
	copied := make(map[string]string, len(defaults))
	for k, v := range defaults {
		copied[k] = v
	}
	return func() tea.Msg {
		err := state.SaveDefaults(path, copied)
		return DefaultsSavedMsg{Err: err}
	}
}

// saveExcludes writes workspace excludes to disk asynchronously
func saveExcludes(path string, excludes map[string][]string) tea.Cmd {
	// Copy the map to avoid concurrent access
	copied := make(map[string][]string, len(excludes))
	for k, v := range excludes {
		vCopy := make([]string, len(v))
		copy(vCopy, v)
		copied[k] = vCopy
	}
	return func() tea.Msg {
		err := state.SaveExcludes(path, copied)
		return ExcludesSavedMsg{Err: err}
	}
}

// detectAllProcesses runs a single tmux command to detect processes in all sessions
func detectAllProcesses() tea.Cmd {
	return func() tea.Msg {
		logDebug("DEBUG: detectAllProcesses called")
		// Single command for all sessions
		cmd := exec.Command("tmux", "list-panes", "-a", "-F", "#{session_name}\t#{pane_current_command}")
		output, err := cmd.Output()
		if err != nil {
			logDebug("DEBUG: tmux command failed: %v", err)
			return ProcessInfoMsg{Processes: nil}
		}

		logDebug("DEBUG: tmux output:\n%s", string(output))
		processes := make(map[string]string)
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) != 2 {
				continue
			}
			sessionName := parts[0]
			command := strings.ToLower(strings.TrimSpace(parts[1]))

			// Already detected? Skip
			if _, exists := processes[sessionName]; exists {
				continue
			}

			// Detect Node.js
			if command == "node" || strings.HasPrefix(command, "node ") {
				processes[sessionName] = "node"
				logDebug("DEBUG: Found node process in session: %s", sessionName)
			}
		}
		logDebug("DEBUG: Detected %d processes", len(processes))
		return ProcessInfoMsg{Processes: processes}
	}
}

// savedStateFreshness is how recent a state file must be to qualify for the
// (+) indicator. Stale snapshots are likely irrelevant — the daemon updates
// every 30s, so anything older than this means the session hasn't been
// touched in a day.
const savedStateFreshness = 24 * time.Hour

// checkSavedState scans ~/.local/share/tmux-session-saver/ for .json files.
// Returns a map of sanitized session names that have a *recent* (≤ 24h)
// saved state available for restore. Keys are sanitized (/ and spaces → _)
// to match tmux-session-saver's filename convention.
func checkSavedState() tea.Cmd {
	return func() tea.Msg {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return SavedStateMsg{Sessions: nil}
		}
		dir := homeDir + "/.local/share/tmux-session-saver"
		entries, err := os.ReadDir(dir)
		if err != nil {
			return SavedStateMsg{Sessions: nil}
		}
		cutoff := time.Now().Add(-savedStateFreshness)
		sessions := make(map[string]bool)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasSuffix(name, ".json") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				continue
			}
			sessions[strings.TrimSuffix(name, ".json")] = true
		}
		return SavedStateMsg{Sessions: sessions}
	}
}

// killSessionWithCleanup gracefully cleans up panes, kills the session, then cleans up orphans.
// Runs entirely async so the TUI stays responsive.
func killSessionWithCleanup(t tmux.Tmux, sessionName string) tea.Cmd {
	return func() tea.Msg {
		// Graceful cleanup: notify claude-sessions + send :qa! to neovim panes
		tmux.GracefulPaneCleanup(sessionName)

		// Collect pane PIDs while tmux metadata still exists
		pids := tmux.CollectPanePids(sessionName)

		// Kill the session
		_, err := t.KillSession(sessionName)

		// Clean up orphaned processes in background (SIGTERM + SIGKILL)
		if len(pids) > 0 {
			go tmux.KillProcessTrees(pids)
		}

		return SessionKilledMsg{Err: err}
	}
}

// scheduleClaudeAttentionTick returns a command that fires a re-check after 2 seconds.
func scheduleClaudeAttentionTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return claudeAttentionTickMsg{}
	})
}

// checkClaudeAttention detects sessions needing user attention using a hybrid
// approach: tmux @claude_state for fast detection (set instantly by hooks),
// then DB + process liveness to filter stale state from dead processes or
// debounce bugs where the state wasn't cleared after approval.
//
// @claude_state carries the bare status word ("waiting"/"done"/…) that
// claude-sessions writes alongside the colored @claude_icon — matching the
// word instead of sniffing the icon's theme hex means a theme retune can't
// silently break attention detection.
func checkClaudeAttention() tea.Cmd {
	return func() tea.Msg {
		// Fast path: read @claude_state from tmux windows
		out, err := exec.Command("tmux", "list-windows", "-a", "-F", "#{session_name}\t#{@claude_state}").Output()
		if err != nil {
			return ClaudeAttentionMsg{Sessions: nil}
		}

		candidates := make(map[string]bool)
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) != 2 {
				continue
			}
			if parts[1] == "waiting" {
				candidates[parts[0]] = true
			}
		}
		if len(candidates) == 0 {
			return ClaudeAttentionMsg{Sessions: nil}
		}

		// Validate with DB + process liveness check
		homeDir, _ := os.UserHomeDir()
		if homeDir == "" {
			return ClaudeAttentionMsg{Sessions: nil}
		}
		verified, _ := claude.SessionsNeedingAttention(homeDir)

		result := make(map[string]bool)
		for session := range candidates {
			if verified[session] {
				result[session] = true
			}
		}
		return ClaudeAttentionMsg{Sessions: result}
	}
}

