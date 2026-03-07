package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/joshmedeski/sesh/v2/claude"
	"github.com/joshmedeski/sesh/v2/lister"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/previewer"
	"github.com/joshmedeski/sesh/v2/state"
	"github.com/joshmedeski/sesh/v2/tmux"
)

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

func loadPreview(p previewer.Previewer, session model.SeshSession) tea.Cmd {
	return func() tea.Msg {
		logDebug("loadPreview: starting for %q (src=%s, path=%s)", session.Name, session.Src, session.Path)
		isActive := (session.Src == "tmux")
		path := session.Path

		// Check git status once (avoid double stat)
		isGit := path != "" && isGitRepo(path)

		// For workspace sub-projects (path inside a git repo but not at root),
		// show filtered git preview regardless of active/inactive state.
		isSubdirGit := !isGit && path != "" && isInsideGitWorkTree(path)
		logDebug("loadPreview: isGit=%v, isSubdirGit=%v, path=%q", isGit, isSubdirGit, path)
		if isSubdirGit {
			logDebug("loadPreview: workspace sub-project detected, generating filtered preview")
			content := GenerateWorkspacePreview(session.Name, path, isActive)
			return PreviewLoadedMsg{Content: content}
		}

		// For active tmux sessions in git repos, show git info
		if isActive && isGit {
			logDebug("loadPreview: active tmux git repo path")
			content := GenerateRichPreview(session.Name, path, isActive, true)
			logDebug("loadPreview: GenerateRichPreview done (%d bytes)", len(content))
			return PreviewLoadedMsg{Content: content}
		}

		// For active tmux sessions NOT in git repos, capture pane content
		if isActive {
			logDebug("loadPreview: active tmux non-git, trying pane capture")
			content, err := p.Preview(session.Name)
			if err == nil && content != "" {
				logDebug("loadPreview: pane capture done (%d bytes)", len(content))
				return PreviewLoadedMsg{Content: content}
			}
			logDebug("loadPreview: pane capture failed, falling through")
			// Fallback to rich preview if capture fails
		}

		// For non-tmux sessions, show git/directory info
		if path != "" {
			logDebug("loadPreview: non-tmux with path, generating rich preview")
			content := GenerateRichPreview(session.Name, path, isActive, isGit)
			logDebug("loadPreview: GenerateRichPreview done (%d bytes)", len(content))
			return PreviewLoadedMsg{Content: content}
		}

		// Final fallback to default previewer
		logDebug("loadPreview: fallback to default previewer")
		content, err := p.Preview(session.Name)
		if err != nil {
			return PreviewLoadedMsg{Content: "Error loading preview: " + err.Error()}
		}
		if content == "" {
			return PreviewLoadedMsg{Content: "No preview available"}
		}
		logDebug("loadPreview: default previewer done (%d bytes)", len(content))
		return PreviewLoadedMsg{Content: content}
	}
}

// debouncePreview creates a debounced preview loading command
// This prevents preview flickering during rapid cursor navigation
func debouncePreview(sessionName string) tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return DebounceTickMsg{SessionName: sessionName}
	})
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

// checkSavedState scans ~/.local/share/tmux-session-saver/ for .json files.
// Returns a map of sanitized session names that have saved state available for restore.
// Keys are sanitized (/ and spaces → _) to match tmux-session-saver's filename convention.
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
		sessions := make(map[string]bool)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if strings.HasSuffix(name, ".json") {
				sessions[strings.TrimSuffix(name, ".json")] = true
			}
		}
		return SavedStateMsg{Sessions: sessions}
	}
}

// deleteSavedState removes the tmux-session-saver save file for a session.
func deleteSavedState(sessionName string) tea.Cmd {
	sanitized := SanitizeSessionName(sessionName)
	return func() tea.Msg {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return SavedStateDeletedMsg{SessionName: sanitized, Err: err}
		}
		path := filepath.Join(homeDir, ".local", "share", "tmux-session-saver", sanitized+".json")
		err = os.Remove(path)
		return SavedStateDeletedMsg{SessionName: sanitized, Err: err}
	}
}

// saveSessionState runs tmux-session-saver save for a tmux session.
func saveSessionState(sessionName string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("tmux-session-saver", "save", sessionName)
		err := cmd.Run()
		return SessionSavedMsg{SessionName: sessionName, Err: err}
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

// checkClaudeAttention queries Claude's sessions.db for sessions needing user attention.
// Returns a map of tmux session names that have awaiting CC sessions.
func checkClaudeAttention() tea.Cmd {
	return func() tea.Msg {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return ClaudeAttentionMsg{Sessions: nil}
		}
		sessions, _ := claude.SessionsNeedingAttention(homeDir)
		return ClaudeAttentionMsg{Sessions: sessions}
	}
}

