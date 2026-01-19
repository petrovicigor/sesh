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

func loadPreview(p previewer.Previewer, session model.SeshSession) tea.Cmd {
	return func() tea.Msg {
		// Determine if this is an active tmux session
		isActive := (session.Src == "tmux" && session.Attached > 0)

		// Use the session path
		path := session.Path

		// Generate rich preview if we have a path
		if path != "" {
			content := GenerateRichPreview(session.Name, path, isActive)
			return PreviewLoadedMsg{Content: content}
		}

		// Fallback to default previewer
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

// detectProcessForSession checks if a tmux session has specific processes running
func detectProcessForSession(session model.SeshSession) tea.Cmd {
	return func() tea.Msg {
		// Only check tmux sessions
		if session.Src != "tmux" {
			return nil
		}

		// Get all pane commands for this session
		start := time.Now()
		cmd := exec.Command("tmux", "list-panes", "-t", session.Name, "-F", "#{pane_current_command}")
		output, err := cmd.Output()
		elapsed := time.Since(start)
		logDebug("DEBUG: list-panes for %s took %v", session.Name, elapsed)
		if err != nil {
			return nil
		}

		// Collect all detected processes
		detected := make(map[string]bool)

		// Check each pane's command
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(strings.ToLower(line))
			if line == "" {
				continue
			}

			// Detect Node.js processes
			if line == "node" || strings.HasPrefix(line, "node ") {
				detected["node"] = true
				logDebug("DEBUG: Node detected in session: %s (line: %s)", session.Name, line)
			}

			// Detect npm processes
			if line == "npm" || strings.HasPrefix(line, "npm ") {
				detected["npm"] = true
				logDebug("DEBUG: npm detected in session: %s (line: %s)", session.Name, line)
			}

			// Detect yarn processes
			if line == "yarn" || strings.HasPrefix(line, "yarn ") {
				detected["yarn"] = true
				logDebug("DEBUG: Yarn detected in session: %s (line: %s)", session.Name, line)
			}
		}

		// Return all detected processes
		if len(detected) > 0 {
			processes := make([]string, 0, len(detected))
			for proc := range detected {
				processes = append(processes, proc)
			}
			return ProcessDetectedMsg{
				SessionName: session.Name,
				Processes:   processes,
			}
		}

		return nil
	}
}

// detectProcessesForAllSessions launches async detection for all tmux sessions
func detectProcessesForAllSessions(sessions model.SeshSessions) tea.Cmd {
	var cmds []tea.Cmd
	var tmuxSessions []string
	for _, key := range sessions.OrderedIndex {
		session := sessions.Directory[key]
		// Check all tmux sessions (attached or not)
		if session.Src == "tmux" {
			tmuxSessions = append(tmuxSessions, session.Name)
			cmds = append(cmds, detectProcessForSession(session))
		}
	}
	// DEBUG: Log which sessions we're checking
	logDebug("DEBUG: Checking %d tmux sessions for processes: %v", len(tmuxSessions), tmuxSessions)
	return tea.Batch(cmds...)
}

// debouncePreview creates a debounced preview loading command
// This prevents preview flickering during rapid cursor navigation
func debouncePreview(sessionName string) tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return DebounceTickMsg{SessionName: sessionName}
	})
}
