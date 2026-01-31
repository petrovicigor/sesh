package tui

import (
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshmedeski/sesh/v2/lister"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/previewer"
)

func loadSessionsWithFilter(l lister.Lister, filter FilterType) tea.Cmd {
	return func() tea.Msg {
		// Invalidate tmux cache to get fresh session data
		l.InvalidateTmuxCache()

		opts := lister.ListOptions{
			HideDuplicates: true, // Hide duplicate sessions
		}

		switch filter {
		case FilterTmux:
			opts.Tmux = true
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
		case FilterTmux:
			opts.Tmux = true
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
		isActive := (session.Src == "tmux")
		path := session.Path

		// For active tmux sessions in git repos, show git info
		if isActive && path != "" && isGitRepo(path) {
			content := GenerateRichPreview(session.Name, path, isActive)
			return PreviewLoadedMsg{Content: content}
		}

		// For active tmux sessions NOT in git repos, capture pane content
		if isActive {
			content, err := p.Preview(session.Name)
			if err == nil && content != "" {
				return PreviewLoadedMsg{Content: content}
			}
			// Fallback to rich preview if capture fails
		}

		// For non-tmux sessions, show git/directory info
		if path != "" {
			content := GenerateRichPreview(session.Name, path, isActive)
			return PreviewLoadedMsg{Content: content}
		}

		// Final fallback to default previewer
		content, err := p.Preview(session.Name)
		if err != nil {
			return PreviewLoadedMsg{Content: "Error loading preview: " + err.Error()}
		}
		if content == "" {
			return PreviewLoadedMsg{Content: "No preview available"}
		}
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

// restoreFilterMode re-enters filter mode and optionally types filter text
func restoreFilterMode(filterText string) tea.Cmd {
	// Create sequence: first '/', then each character of filter text, then completion message
	cmds := make([]tea.Cmd, 0, len(filterText)+2)

	// Enter filter mode
	cmds = append(cmds, func() tea.Msg {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	})

	// Type each character of the filter text
	if filterText != "" {
		for _, r := range filterText {
			r := r // capture loop variable
			cmds = append(cmds, func() tea.Msg {
				return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
			})
		}
	}

	// Signal restoration complete
	cmds = append(cmds, func() tea.Msg {
		return RestorationCompleteMsg{}
	})

	return tea.Sequence(cmds...)
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

