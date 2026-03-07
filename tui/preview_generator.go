package tui

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"

	_ "modernc.org/sqlite"
)

// ANSI color codes
const (
	colorReset   = "\033[0m"
	colorDim     = "\033[2m"
	colorCyan    = "\033[0;36m"
	colorGreen   = "\033[0;32m"
	colorYellow  = "\033[0;33m"
	colorMagenta = "\033[0;35m"
	colorRed     = "\033[0;31m"
	colorGray    = "\033[90m"
	// Dimmed versions for inactive sessions
	colorCyanDim    = "\033[2;36m"
	colorGreenDim   = "\033[2;32m"
	colorYellowDim  = "\033[2;33m"
	colorMagentaDim = "\033[2;35m"
)

// Pre-compiled regex for dimANSI (avoids recompilation per preview)
var dimANSIRegex = regexp.MustCompile(`\x1b\[([1-9][0-9;]*)m`)

// GenerateRichPreview creates a rich preview similar to preview.sh
// Git commands run in parallel for better performance.
// isGit indicates whether the path is a git repo (caller checks to avoid double stat).
func GenerateRichPreview(sessionName string, path string, isActive bool, isGit bool) string {
	logDebug("GenerateRichPreview: start for %q (active=%v, git=%v)", sessionName, isActive, isGit)
	var output strings.Builder

	// Select colors based on active/inactive
	cyan := colorCyan
	green := colorGreen
	if !isActive {
		cyan = colorCyanDim
		green = colorGreenDim
	}

	if isGit {
		logDebug("GenerateRichPreview: is git repo, launching parallel commands")
		// Run all git commands in parallel
		var wg sync.WaitGroup
		var branch, tracking, status, commits, claudeSessions string

		wg.Add(5)

		// Parallel git command execution
		go func() {
			defer wg.Done()
			branch = getGitBranch(path)
		}()

		go func() {
			defer wg.Done()
			tracking = getGitTracking(path)
		}()

		go func() {
			defer wg.Done()
			status = getGitStatus(path, isActive)
		}()

		go func() {
			defer wg.Done()
			commits = getGitLog(path, isActive)
		}()

		go func() {
			defer wg.Done()
			claudeSessions = getClaudeSessions(path, sessionName, isActive)
		}()

		// Wait for all commands to complete
		wg.Wait()
		logDebug("GenerateRichPreview: all parallel commands done")

		// Build output with results
		output.WriteString(fmt.Sprintf("%s󰘬 %s%s%s%s\n\n", cyan, branch, green, tracking, colorReset))

		if claudeSessions != "" {
			output.WriteString(claudeSessions)
			output.WriteString("\n")
		}

		output.WriteString(fmt.Sprintf("%s━━━ Status ━━━%s\n", colorDim, colorReset))
		if status == "" {
			output.WriteString(fmt.Sprintf("%sclean%s\n", colorDim, colorReset))
		} else {
			output.WriteString(status + "\n")
		}
		output.WriteString("\n")

		output.WriteString(fmt.Sprintf("%s━━━ Recent Commits ━━━%s\n", colorDim, colorReset))
		output.WriteString(commits)
	} else {
		// Non-git directory
		if isActive {
			// For active sessions, show directory path
			output.WriteString(fmt.Sprintf("%s📁 %s%s\n", cyan, path, colorReset))
		} else {
			// For inactive sessions, show directory tree
			output.WriteString(fmt.Sprintf("%s📁 %s%s\n", cyan, path, colorReset))
			tree := getDirectoryTree(path, isActive)
			output.WriteString(tree)
		}

	}

	return output.String()
}

// isInsideGitWorkTree checks if a path is inside a git work tree (but not necessarily at the root).
// Used to detect workspace sub-projects that live inside a monorepo.
func isInsideGitWorkTree(path string) bool {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--is-inside-work-tree")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "true"
}

// getGitRelativePrefix returns the path relative to the git root (e.g., "packages/ui").
func getGitRelativePrefix(path string) string {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--show-prefix")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(string(output)), "/")
}

// getGitStatusFiltered returns git status filtered to the current directory and the count
// of other changes outside this directory. Both commands run in parallel.
func getGitStatusFiltered(path string, isActive bool) (filtered string, otherCount int) {
	var wg sync.WaitGroup
	var filteredOut, totalOut []byte

	wg.Add(2)
	go func() {
		defer wg.Done()
		cmd := exec.Command("git", "-C", path, "-c", "color.status=always", "status", "--short", "--", ".")
		filteredOut, _ = cmd.Output()
	}()
	go func() {
		defer wg.Done()
		cmd := exec.Command("git", "-C", path, "status", "--short")
		totalOut, _ = cmd.Output()
	}()
	wg.Wait()

	filteredStr := string(filteredOut)
	if !isActive {
		filteredStr = dimANSI(filteredStr)
	}
	filtered = strings.TrimSpace(filteredStr)

	// Count lines to determine other changes
	filteredLines := countNonEmptyLines(string(filteredOut))
	totalLines := countNonEmptyLines(string(totalOut))
	otherCount = totalLines - filteredLines
	return
}

// getGitLogFiltered returns git log filtered to changes in the current directory.
func getGitLogFiltered(path string, isActive bool) string {
	cmd := exec.Command("git", "-C", path, "log", "--oneline", "--graph", "--decorate", "--color=always", "-3", "--", ".")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	log := string(output)
	if !isActive {
		log = dimANSI(log)
	}
	return log
}

// countNonEmptyLines counts lines that are not empty.
func countNonEmptyLines(s string) int {
	count := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

// GenerateWorkspacePreview creates a rich preview for workspace sub-projects (monorepo packages/apps).
// Similar to GenerateRichPreview but filters git status and log to the sub-project directory,
// and shows a count of other changes outside the sub-project.
func GenerateWorkspacePreview(sessionName string, path string, isActive bool) string {
	logDebug("GenerateWorkspacePreview: start for %q (active=%v)", sessionName, isActive)
	var output strings.Builder

	cyan := colorCyan
	green := colorGreen
	if !isActive {
		cyan = colorCyanDim
		green = colorGreenDim
	}

	var wg sync.WaitGroup
	var branch, tracking, filteredStatus, commits, claudeSessions, relPrefix string
	var otherCount int

	wg.Add(6)

	go func() {
		defer wg.Done()
		branch = getGitBranch(path)
	}()

	go func() {
		defer wg.Done()
		tracking = getGitTracking(path)
	}()

	go func() {
		defer wg.Done()
		filteredStatus, otherCount = getGitStatusFiltered(path, isActive)
	}()

	go func() {
		defer wg.Done()
		commits = getGitLogFiltered(path, isActive)
	}()

	go func() {
		defer wg.Done()
		claudeSessions = getClaudeSessions(path, sessionName, isActive)
	}()

	go func() {
		defer wg.Done()
		relPrefix = getGitRelativePrefix(path)
	}()

	wg.Wait()
	logDebug("GenerateWorkspacePreview: all parallel commands done")

	// Build output
	output.WriteString(fmt.Sprintf("%s󰘬 %s%s%s%s\n\n", cyan, branch, green, tracking, colorReset))

	if claudeSessions != "" {
		output.WriteString(claudeSessions)
		output.WriteString("\n")
	}

	output.WriteString(fmt.Sprintf("%s━━━ Status ━━━%s\n", colorDim, colorReset))
	if filteredStatus == "" {
		output.WriteString(fmt.Sprintf("%sclean%s\n", colorDim, colorReset))
	} else {
		output.WriteString(filteredStatus + "\n")
	}
	if otherCount > 0 {
		label := relPrefix
		if label == "" {
			label = "this directory"
		}
		output.WriteString(fmt.Sprintf("  %s%d other files changed outside %s%s\n", colorDim, otherCount, label, colorReset))
	}
	output.WriteString("\n")

	output.WriteString(fmt.Sprintf("%s━━━ Recent Commits ━━━%s\n", colorDim, colorReset))
	output.WriteString(commits)

	return output.String()
}

// savedStateData holds parsed data from a tmux-session-saver JSON file.
type savedStateData struct {
	savedAt      string
	windowNames  []string
	windowCount  int
	processCount int
	claudeCount  int
}

// sanitizeReplacer is shared across all SanitizeSessionName calls to avoid per-call allocation.
var sanitizeReplacer = strings.NewReplacer("/", "_", " ", "_")

// SanitizeSessionName applies the same sanitization as tmux-session-saver
// (replaces / and spaces with _) to match saved state filenames.
func SanitizeSessionName(name string) string {
	return sanitizeReplacer.Replace(name)
}

// parseSavedState reads and parses the tmux-session-saver JSON file for a session.
// Returns nil if no saved state exists.
func parseSavedState(sessionName string) *savedStateData {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	filePath := filepath.Join(homeDir, ".local", "share", "tmux-session-saver", SanitizeSessionName(sessionName)+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	content := string(data)

	// Count windows (windows have "layout" field, panes don't)
	windowCount := strings.Count(content, `"layout":`)

	// Extract saved_at timestamp
	savedAt := ""
	if idx := strings.Index(content, `"saved_at": "`); idx >= 0 {
		start := idx + len(`"saved_at": "`)
		if end := strings.Index(content[start:], `"`); end >= 0 {
			savedAt = content[start : start+end]
			if tIdx := strings.Index(savedAt, "T"); tIdx >= 0 {
				date := savedAt[:tIdx]
				timePart := savedAt[tIdx+1:]
				if dotIdx := strings.Index(timePart, "."); dotIdx >= 0 {
					timePart = timePart[:dotIdx]
				}
				if plusIdx := strings.Index(timePart, "+"); plusIdx >= 0 {
					timePart = timePart[:plusIdx]
				}
				savedAt = date + " " + timePart
			}
		}
	}

	// Extract window names
	var windowNames []string
	remaining := content
	for {
		idx := strings.Index(remaining, `"name": "`)
		if idx < 0 {
			break
		}
		start := idx + len(`"name": "`)
		end := strings.Index(remaining[start:], `"`)
		if end < 0 {
			break
		}
		name := remaining[start : start+end]
		windowNames = append(windowNames, name)
		remaining = remaining[start+end:]
	}
	if len(windowNames) > windowCount {
		windowNames = windowNames[:windowCount]
	}

	return &savedStateData{
		savedAt:      savedAt,
		windowNames:  windowNames,
		windowCount:  windowCount,
		processCount: strings.Count(content, `"long_running": true`),
		claudeCount:  strings.Count(content, `"session_id": "`),
	}
}

// getSavedStateInfo reads the tmux-session-saver JSON file and returns a summary string.
// Returns empty string if no saved state exists.
func getSavedStateInfo(sessionName string, isActive bool) string {
	sd := parseSavedState(sessionName)
	if sd == nil {
		return ""
	}

	dim := colorDim

	var out strings.Builder
	out.WriteString(fmt.Sprintf("%s━━━ Saved State 💾 ━━━%s\n", dim, colorReset))
	if sd.savedAt != "" {
		out.WriteString(fmt.Sprintf(" %ssaved %s%s\n", dim, sd.savedAt, colorReset))
	}

	for i, name := range sd.windowNames {
		if len(name) > 30 {
			name = name[:27] + "..."
		}
		prefix := "├"
		if i == len(sd.windowNames)-1 {
			prefix = "└"
		}
		out.WriteString(fmt.Sprintf(" %s%s%s %s%s\n", dim, prefix, colorReset, name, colorReset))
	}

	if sd.processCount > 0 || sd.claudeCount > 0 {
		var badges []string
		if sd.processCount > 0 {
			badges = append(badges, fmt.Sprintf("%d process", sd.processCount))
		}
		if sd.claudeCount > 0 {
			badges = append(badges, fmt.Sprintf("%d claude", sd.claudeCount))
		}
		out.WriteString(fmt.Sprintf(" %s%s%s\n", dim, strings.Join(badges, ", "), colorReset))
	}

	return out.String()
}

// generateRestorePreview creates a full-pane preview for the restore confirmation screen.
func generateRestorePreview(sessionName string) string {
	sd := parseSavedState(sessionName)

	var out strings.Builder
	out.WriteString(fmt.Sprintf("\n%s━━━ Restore: %s ━━━%s\n\n", colorDim, sessionName, colorReset))

	if sd == nil {
		out.WriteString(fmt.Sprintf(" %sNo saved state found%s\n", colorDim, colorReset))
		out.WriteString(fmt.Sprintf("\n %sEsc to go back%s\n", colorDim, colorReset))
		return out.String()
	}

	if sd.savedAt != "" {
		out.WriteString(fmt.Sprintf(" %sSaved: %s%s\n\n", colorDim, sd.savedAt, colorReset))
	}

	if len(sd.windowNames) > 0 {
		out.WriteString(fmt.Sprintf(" %sWindows:%s\n", colorDim, colorReset))
		for i, name := range sd.windowNames {
			if len(name) > 40 {
				name = name[:37] + "..."
			}
			prefix := "├"
			if i == len(sd.windowNames)-1 {
				prefix = "└"
			}
			out.WriteString(fmt.Sprintf(" %s%s%s %s%s\n", colorDim, prefix, colorReset, name, colorReset))
		}
		out.WriteString("\n")
	}

	if sd.processCount > 0 || sd.claudeCount > 0 {
		var badges []string
		if sd.processCount > 0 {
			badges = append(badges, fmt.Sprintf("%d process(es)", sd.processCount))
		}
		if sd.claudeCount > 0 {
			badges = append(badges, fmt.Sprintf("%d claude session(s)", sd.claudeCount))
		}
		out.WriteString(fmt.Sprintf(" %s%s%s\n\n", colorDim, strings.Join(badges, ", "), colorReset))
	}

	out.WriteString(fmt.Sprintf(" %s%sEnter%s%s restore  |  %s%sBksp%s%s delete  |  %s%sEsc%s%s cancel%s\n",
		colorReset, colorGreen, colorReset, colorDim,
		colorReset, colorRed, colorReset, colorDim,
		colorReset, colorYellow, colorReset, colorDim, colorReset))

	return out.String()
}

// generateDeleteConfirmPreview shows a confirmation prompt before deleting saved state.
func generateDeleteConfirmPreview(sessionName string) string {
	var out strings.Builder
	out.WriteString(fmt.Sprintf("\n%s━━━ Delete saved state: %s ━━━%s\n\n", colorDim, sessionName, colorReset))
	out.WriteString(fmt.Sprintf(" %sPress %s%sBksp%s%s again to confirm  |  %s%sEsc%s%s cancel%s\n",
		colorDim,
		colorReset, colorRed, colorReset, colorDim,
		colorReset, colorYellow, colorReset, colorDim, colorReset))
	return out.String()
}

// generateSavePreview creates a compact save confirmation preview for the preview pane.
// Shows: session name, window count, process/claude counts, last saved timestamp.
func generateSavePreview(sessionName string, tmuxSessionNames []string) string {
	var out strings.Builder
	out.WriteString(fmt.Sprintf("\n%s━━━ Save: %s ━━━%s\n\n", colorDim, sessionName, colorReset))

	// Show existing saved state timestamp if available
	sd := parseSavedState(sessionName)
	if sd != nil && sd.savedAt != "" {
		out.WriteString(fmt.Sprintf(" %sLast saved: %s%s\n\n", colorDim, sd.savedAt, colorReset))
	}

	// Get live tmux windows for the selected session
	windowNames := getTmuxWindowNames(sessionName)
	if len(windowNames) > 0 {
		out.WriteString(fmt.Sprintf(" %sWindows:%s\n", colorDim, colorReset))
		for i, name := range windowNames {
			if len(name) > 40 {
				name = name[:37] + "..."
			}
			prefix := "├"
			if i == len(windowNames)-1 {
				prefix = "└"
			}
			out.WriteString(fmt.Sprintf(" %s%s%s %s%s\n", colorDim, prefix, colorReset, name, colorReset))
		}
		out.WriteString("\n")
	}

	// Footer with actions
	tmuxCount := len(tmuxSessionNames)
	out.WriteString(fmt.Sprintf(" %s%sEnter%s%s save  |  ", colorReset, colorGreen, colorReset, colorDim))
	if tmuxCount > 1 {
		out.WriteString(fmt.Sprintf("%s%sCtrl+A%s%s save all (%d)  |  ", colorReset, colorCyan, colorReset, colorDim, tmuxCount))
	}
	out.WriteString(fmt.Sprintf("%s%sEsc%s%s cancel%s\n", colorReset, colorYellow, colorReset, colorDim, colorReset))

	return out.String()
}

// getTmuxWindowNames returns window names for a live tmux session.
func getTmuxWindowNames(sessionName string) []string {
	cmd := exec.Command("tmux", "list-windows", "-t", sessionName, "-F", "#{window_name}")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	raw := strings.TrimSpace(string(output))
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

// generateSaveAllProgress creates a preview showing save-all progress.
func generateSaveAllProgress(allSessions []string, completed []string) string {
	var out strings.Builder
	out.WriteString(fmt.Sprintf("\n%s━━━ Saving All Sessions ━━━%s\n\n", colorDim, colorReset))

	completedSet := make(map[string]bool, len(completed))
	for _, name := range completed {
		completedSet[name] = true
	}

	firstPending := true
	for _, name := range allSessions {
		windowCount := len(getTmuxWindowNames(name))
		suffix := fmt.Sprintf(" %s(%d windows)%s", colorDim, windowCount, colorReset)

		if completedSet[name] {
			out.WriteString(fmt.Sprintf(" %s✓%s %s%s\n", colorGreen, colorReset, name, suffix))
		} else if firstPending {
			out.WriteString(fmt.Sprintf(" %s⏳%s %s%s\n", colorYellow, colorReset, name, suffix))
			firstPending = false
		} else {
			out.WriteString(fmt.Sprintf("    %s%s%s%s\n", colorDim, name, colorReset, suffix))
		}
	}

	out.WriteString(fmt.Sprintf("\n %s%d/%d saved%s\n", colorDim, len(completed), len(allSessions), colorReset))
	return out.String()
}

func isGitRepo(path string) bool {
	gitDir := filepath.Join(path, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		return false
	}
	// .git directory without index = bare repo, skip it
	if info.IsDir() {
		if _, err := os.Stat(filepath.Join(gitDir, "index")); err != nil {
			return false
		}
	}
	return true
}

func getGitBranch(path string) string {
	cmd := exec.Command("git", "-C", path, "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func getGitTracking(path string) string {
	cmd := exec.Command("git", "-C", path, "rev-list", "--left-right", "--count", "@{upstream}...HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	parts := strings.Fields(strings.TrimSpace(string(output)))
	if len(parts) != 2 {
		return ""
	}

	behind, _ := strconv.Atoi(parts[0])
	ahead, _ := strconv.Atoi(parts[1])

	var tracking string
	if ahead > 0 {
		tracking += fmt.Sprintf(" ↑%d", ahead)
	}
	if behind > 0 {
		tracking += fmt.Sprintf(" ↓%d", behind)
	}
	return tracking
}

func getGitStatus(path string, isActive bool) string {
	cmd := exec.Command("git", "-C", path, "-c", "color.status=always", "status", "--short")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	status := string(output)
	if !isActive {
		// Dim colors for inactive sessions
		status = dimANSI(status)
	}
	return strings.TrimSpace(status)
}

func getGitLog(path string, isActive bool) string {
	cmd := exec.Command("git", "-C", path, "log", "--oneline", "--graph", "--decorate", "--color=always", "-3")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	log := string(output)
	if !isActive {
		// Dim colors for inactive sessions
		log = dimANSI(log)
	}
	return log
}

func getDirectoryTree(path string, isActive bool) string {
	// Try lsd first, fallback to ls
	cmd := exec.Command("lsd", "--tree", "--depth", "1", "--icon", "always", path)
	output, err := cmd.Output()
	if err != nil {
		// Fallback to ls
		cmd = exec.Command("ls", "-la", path)
		output, _ = cmd.Output()
	}

	tree := string(output)
	if !isActive {
		tree = dimANSI(tree)
	}
	return tree
}

// dimANSI adds dim attribute to ANSI color codes
// Matches bash: sed $'s/\033\\[m/\033[0;2m/g; s/\033\\[\\([1-9][0-9;]*\\)m/\033[2;\\1m/g'
func dimANSI(s string) string {
	// Replace \033[m with \033[0;2m (reset with dim)
	s = strings.ReplaceAll(s, "\033[m", "\033[0;2m")

	// Add dim (2) to existing color codes: \033[31m -> \033[2;31m
	s = dimANSIRegex.ReplaceAllString(s, "\x1b[2;${1}m")

	return s
}

func getClaudeSessions(projectPath string, tmuxSession string, isActive bool) string {
	dbPath := filepath.Join(os.Getenv("HOME"), ".claude", "sessions.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return ""
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return ""
	}
	defer db.Close()

	// Use tmux session name or project name for matching
	tmuxMatch := tmuxSession
	if tmuxMatch == "" {
		tmuxMatch = filepath.Base(projectPath)
	}

	query := `SELECT
		session_id, custom_title, last_activity, ended_at, pid, status,
		compacted_at, compaction_count, started_at, git_branch, pinned_at,
		CASE
			WHEN ended_at IS NULL THEN
				CASE
					WHEN (strftime('%s', 'now') - strftime('%s', COALESCE(last_activity, started_at))) < 60 THEN 'now'
					WHEN (strftime('%s', 'now') - strftime('%s', COALESCE(last_activity, started_at))) < 3600 THEN
						CAST((strftime('%s', 'now') - strftime('%s', COALESCE(last_activity, started_at))) / 60 AS TEXT) || 'm'
					WHEN (strftime('%s', 'now') - strftime('%s', COALESCE(last_activity, started_at))) < 86400 THEN
						CAST((strftime('%s', 'now') - strftime('%s', COALESCE(last_activity, started_at))) / 3600 AS TEXT) || 'h'
					ELSE
						CAST((strftime('%s', 'now') - strftime('%s', COALESCE(last_activity, started_at))) / 86400 AS TEXT) || 'd'
				END
			ELSE
				CASE
					WHEN (strftime('%s', 'now') - strftime('%s', ended_at)) < 60 THEN 'now'
					WHEN (strftime('%s', 'now') - strftime('%s', ended_at)) < 3600 THEN
						CAST((strftime('%s', 'now') - strftime('%s', ended_at)) / 60 AS TEXT) || 'm'
					WHEN (strftime('%s', 'now') - strftime('%s', ended_at)) < 86400 THEN
						CAST((strftime('%s', 'now') - strftime('%s', ended_at)) / 3600 AS TEXT) || 'h'
					ELSE
						CAST((strftime('%s', 'now') - strftime('%s', ended_at)) / 86400 AS TEXT) || 'd'
				END
		END AS time_ago,
		CASE
			WHEN (strftime('%s', 'now') - strftime('%s', started_at)) < 60 THEN 'now'
			WHEN (strftime('%s', 'now') - strftime('%s', started_at)) < 3600 THEN
				CAST((strftime('%s', 'now') - strftime('%s', started_at)) / 60 AS TEXT) || 'm'
			WHEN (strftime('%s', 'now') - strftime('%s', started_at)) < 86400 THEN
				CAST((strftime('%s', 'now') - strftime('%s', started_at)) / 3600 AS TEXT) || 'h'
			ELSE
				CAST((strftime('%s', 'now') - strftime('%s', started_at)) / 86400 AS TEXT) || 'd'
		END AS created_ago,
		CASE
			WHEN compacted_at IS NULL THEN ''
			WHEN (strftime('%s', 'now') - strftime('%s', compacted_at)) < 60 THEN 'now'
			WHEN (strftime('%s', 'now') - strftime('%s', compacted_at)) < 3600 THEN
				CAST((strftime('%s', 'now') - strftime('%s', compacted_at)) / 60 AS TEXT) || 'm'
			WHEN (strftime('%s', 'now') - strftime('%s', compacted_at)) < 86400 THEN
				CAST((strftime('%s', 'now') - strftime('%s', compacted_at)) / 3600 AS TEXT) || 'h'
			ELSE
				CAST((strftime('%s', 'now') - strftime('%s', compacted_at)) / 86400 AS TEXT) || 'd'
		END AS compact_time_ago
	FROM sessions
	WHERE tmux_session = ?
	  AND replaced_by_session_id IS NULL
	ORDER BY
		CASE
			WHEN (ended_at IS NULL) AND (pinned_at IS NOT NULL) THEN 4
			WHEN (ended_at IS NULL) AND (pinned_at IS NULL) THEN 3
			WHEN (ended_at IS NOT NULL) AND (pinned_at IS NOT NULL) THEN 2
			ELSE 1
		END DESC,
		last_activity DESC
	LIMIT 3`

	rows, err := db.Query(query, tmuxMatch)
	if err != nil {
		return ""
	}
	defer rows.Close()

	var output strings.Builder
	hasRows := false

	for rows.Next() {
		if !hasRows {
			output.WriteString(fmt.Sprintf("%s━━━ Claude Sessions ━━━%s\n", colorDim, colorReset))
			hasRows = true
		}

		var sessionID, title, lastActivity, startedAt, timeAgo, createdAgo, compactTimeAgo string
		var endedAt, gitBranch, pinnedAt, status sql.NullString
		var pid, compactionCount sql.NullInt64
		var compactedAt sql.NullString

		err := rows.Scan(&sessionID, &title, &lastActivity, &endedAt, &pid, &status,
			&compactedAt, &compactionCount, &startedAt, &gitBranch, &pinnedAt,
			&timeAgo, &createdAgo, &compactTimeAgo)
		if err != nil {
			continue
		}

		sessionLine := formatClaudeSession(
			title, endedAt.Valid, pid.Int64, status.String, timeAgo, createdAgo,
			compactedAt.Valid, int(compactionCount.Int64), compactTimeAgo,
			gitBranch.String, pinnedAt.Valid, isActive,
		)
		output.WriteString(sessionLine + "\n")
	}

	return output.String()
}

func formatClaudeSession(title string, hasEnded bool, pid int64, status string,
	timeAgo string, createdAgo string, hasCompacted bool, compactionCount int,
	compactTimeAgo string, gitBranch string, isPinned bool, isActiveSession bool) string {

	if title == "" {
		title = "Untitled"
	}

	// Check if process is active
	isActive := false
	if !hasEnded && pid > 0 {
		// Check if process exists using signal 0 (same as kill -0)
		process, err := os.FindProcess(int(pid))
		if err == nil {
			// On Unix, FindProcess always succeeds, so we try to signal it with signal 0
			err = process.Signal(syscall.Signal(0))
			isActive = (err == nil)
		}
	}

	// Status icons and display
	var statusIcon string
	if status == "failed" {
		statusIcon = colorRed + "✗" + colorReset
	} else if status == "starting" {
		statusIcon = colorYellow + "◐" + colorReset
	} else if isActive {
		statusIcon = colorGreen + "●" + colorReset
	} else {
		statusIcon = colorGray + "○" + colorReset
	}

	// Status display
	statusDisplay := ""
	if isActive && status != "idle" && status != "" {
		parts := strings.Split(status, ":")
		statusType := parts[0]

		yellow := colorYellow
		magenta := colorMagenta
		if !isActiveSession {
			yellow = colorYellowDim
			magenta = colorMagentaDim
		}

		if status == "thinking" {
			statusDisplay = fmt.Sprintf("%s🔮%s ", yellow, colorReset)
		} else if statusType == "running" {
			statusDisplay = fmt.Sprintf("%s🔧%s ", yellow, colorReset)
		} else if statusType == "awaiting" {
			statusDisplay = fmt.Sprintf("%s🖐️%s ", magenta, colorReset)
		}
	}

	// Title color
	titleColor := colorCyan
	if !isActive {
		titleColor = colorGray
	} else if strings.Contains(timeAgo, "h") || strings.Contains(timeAgo, "d") {
		titleColor = colorCyanDim
	}

	// Branch display
	branchDisplay := ""
	if gitBranch != "" {
		branchDisplay = fmt.Sprintf(" %s@%s%s", colorDim, gitBranch, colorReset)
	}

	// Age display (only if older than 1 day)
	ageDisplay := ""
	if strings.Contains(createdAgo, "d") {
		ageDisplay = fmt.Sprintf(" %s(%s old)%s", colorDim, createdAgo, colorReset)
	}

	// Compaction display
	compactDisplay := ""
	if hasCompacted && compactionCount > 0 {
		compactDisplay = fmt.Sprintf(" %s📦×%d (%s)%s", colorDim, compactionCount, compactTimeAgo, colorReset)
	}

	// Pin indicator
	pinIndicator := ""
	if isPinned {
		pinIndicator = "📌 "
	}

	// Stale warning (inactive + 7+ days)
	staleDisplay := ""
	if !isActive && strings.Contains(timeAgo, "d") {
		daysStr := strings.TrimSuffix(timeAgo, "d")
		days, _ := strconv.Atoi(daysStr)
		if days >= 7 {
			staleDisplay = fmt.Sprintf(" %s⚠️%s", colorYellow, colorReset)
		}
	}

	// Fix "now" for inactive sessions
	if !isActive && timeAgo == "now" {
		timeAgo = "1m"
	}

	return fmt.Sprintf(" %s %s%s%s%s%s%s %s%s%s%s%s%s",
		statusIcon, statusDisplay, pinIndicator, titleColor, title, colorReset, branchDisplay,
		colorDim, timeAgo, colorReset, ageDisplay, compactDisplay, staleDisplay)
}
