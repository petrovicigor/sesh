package tui

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3"
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

// GenerateRichPreview creates a rich preview similar to preview.sh
// Git commands run in parallel for better performance
func GenerateRichPreview(sessionName string, path string, isActive bool) string {
	var output strings.Builder

	// Select colors based on active/inactive
	cyan := colorCyan
	green := colorGreen
	if !isActive {
		cyan = colorCyanDim
		green = colorGreenDim
	}

	// Check if it's a git repository
	if isGitRepo(path) {
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

func isGitRepo(path string) bool {
	gitDir := filepath.Join(path, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return true
	}
	// Check if we're in a git repo by running git command
	cmd := exec.Command("git", "-C", path, "rev-parse", "--git-dir")
	return cmd.Run() == nil
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
func dimANSI(s string) string {
	// Replace \033[m with \033[0;2m (reset with dim)
	s = strings.ReplaceAll(s, "\033[m", "\033[0;2m")
	// Add dim (2) to existing color codes: \033[31m -> \033[2;31m
	// This is a simplified version - the bash script has more complex regex
	return s
}

func getClaudeSessions(projectPath string, tmuxSession string, isActive bool) string {
	dbPath := filepath.Join(os.Getenv("HOME"), ".claude", "sessions.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return ""
	}

	db, err := sql.Open("sqlite3", dbPath)
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
			WHEN (strftime('%s', 'now') - strftime('%s', last_activity)) < 60 THEN 'now'
			WHEN (strftime('%s', 'now') - strftime('%s', last_activity)) < 3600 THEN
				CAST((strftime('%s', 'now') - strftime('%s', last_activity)) / 60 AS TEXT) || 'm'
			WHEN (strftime('%s', 'now') - strftime('%s', last_activity)) < 86400 THEN
				CAST((strftime('%s', 'now') - strftime('%s', last_activity)) / 3600 AS TEXT) || 'h'
			ELSE
				CAST((strftime('%s', 'now') - strftime('%s', last_activity)) / 86400 AS TEXT) || 'd'
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

		var sessionID, title, lastActivity, status, startedAt, timeAgo, createdAgo, compactTimeAgo string
		var endedAt, gitBranch, pinnedAt sql.NullString
		var pid, compactionCount sql.NullInt64
		var compactedAt sql.NullString

		err := rows.Scan(&sessionID, &title, &lastActivity, &endedAt, &pid, &status,
			&compactedAt, &compactionCount, &startedAt, &gitBranch, &pinnedAt,
			&timeAgo, &createdAgo, &compactTimeAgo)
		if err != nil {
			continue
		}

		sessionLine := formatClaudeSession(
			title, endedAt.Valid, pid.Int64, status, timeAgo, createdAgo,
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
		// Check if process exists
		process, err := os.FindProcess(int(pid))
		if err == nil {
			// On Unix, FindProcess always succeeds, so we try to signal it
			err = process.Signal(os.Signal(nil))
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
		toolName := ""
		if len(parts) > 1 {
			toolName = parts[1]
		}

		yellow := colorYellow
		magenta := colorMagenta
		if !isActiveSession {
			yellow = colorYellowDim
			magenta = colorMagentaDim
		}

		if status == "thinking" {
			statusDisplay = fmt.Sprintf(" %s💭%s", yellow, colorReset)
		} else if statusType == "running" {
			statusDisplay = fmt.Sprintf(" %s⚙️ %s%s", yellow, toolName, colorReset)
		} else if statusType == "awaiting" {
			statusDisplay = fmt.Sprintf(" %s⏳ %s%s", magenta, toolName, colorReset)
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

	return fmt.Sprintf(" %s %s%s%s%s%s %s%s%s%s%s%s%s",
		statusIcon, pinIndicator, titleColor, title, colorReset, branchDisplay,
		colorDim, timeAgo, colorReset, ageDisplay, compactDisplay, statusDisplay, staleDisplay)
}
