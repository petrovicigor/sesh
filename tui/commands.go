package tui

import (
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshmedeski/sesh/v2/lister"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/previewer"
	"github.com/joshmedeski/sesh/v2/state"
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

