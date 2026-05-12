package tui

import (
	"context"
	"database/sql"
	"encoding/json"
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

// claudeDBPool holds a reusable SQLite connection for Claude sessions queries.
// Opened lazily on first use, shared across all preview loads (thread-safe via database/sql).
var (
	claudeDBPool     *sql.DB
	claudeDBOnce     sync.Once
	claudeDBPath     string
)

func getClaudeDB() *sql.DB {
	claudeDBOnce.Do(func() {
		home := os.Getenv("HOME")
		if home == "" {
			return
		}
		claudeDBPath = filepath.Join(home, ".claude", "sessions.db")
		if _, err := os.Stat(claudeDBPath); os.IsNotExist(err) {
			return
		}
		db, err := sql.Open("sqlite", claudeDBPath)
		if err != nil {
			return
		}
		// Read-only access: never block Claude Code's writes
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		db.Exec("PRAGMA query_only = ON")
		db.Exec("PRAGMA wal_autocheckpoint = 0")
		db.Exec("PRAGMA busy_timeout = 50")
		claudeDBPool = db
	})
	return claudeDBPool
}

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

// gitData holds results from git commands for a regular git repo.
type gitData struct {
	branch   string
	tracking string
	status   string
	commits  string
}

// workspaceGitData holds results from filtered git commands for workspace sub-projects.
type workspaceGitData struct {
	branch          string
	tracking        string
	filteredStatus  string
	otherCount      int
	filteredCommits string
	relPrefix       string
}

// Pre-compiled regex for dimANSI (avoids recompilation per preview)
var dimANSIRegex = regexp.MustCompile(`\x1b\[([1-9][0-9;]*)m`)

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
func getGitRelativePrefix(ctx context.Context, path string) string {
	cmd := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "--show-prefix")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(string(output)), "/")
}

// getGitStatusFiltered returns git status filtered to the current directory and the count
// of other changes outside this directory. Both commands run in parallel.
func getGitStatusFiltered(ctx context.Context, path string, isActive bool) (filtered string, otherCount int) {
	var wg sync.WaitGroup
	var filteredOut, totalOut []byte

	wg.Add(2)
	go func() {
		defer wg.Done()
		cmd := exec.CommandContext(ctx, "git", "-C", path, "-c", "color.status=always", "status", "--short", "--", ".")
		filteredOut, _ = cmd.Output()
	}()
	go func() {
		defer wg.Done()
		cmd := exec.CommandContext(ctx, "git", "-C", path, "status", "--short")
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
func getGitLogFiltered(ctx context.Context, path string, isActive bool) string {
	cmd := exec.CommandContext(ctx, "git", "-C", path, "log", "--oneline", "--graph", "--decorate", "--color=always", "-3", "--", ".")
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


// savedStateData holds parsed data from a tmux-session-saver JSON file.
type savedStateData struct {
	savedAt     string
	windows     []savedWindowInfo
	windowCount int
}

// savedWindowInfo is per-window data extracted from the saved state file,
// used to annotate window names in the preview with `(claude)` /
// `(npm run dev)` markers so the user can see what each window will
// actually restore to.
type savedWindowInfo struct {
	name           string
	hasClaude      bool
	processCommand string // empty if no long-running process captured
}

// sanitizeReplacer is shared across all SanitizeSessionName calls to avoid per-call allocation.
// Mapping must stay in sync with tmux-session-saver's internal/state.sanitizeName.
var sanitizeReplacer = strings.NewReplacer("/", "_", " ", "+")

// SanitizeSessionName applies the same sanitization as tmux-session-saver
// (replaces / and spaces with _) to match saved state filenames.
func SanitizeSessionName(name string) string {
	return sanitizeReplacer.Replace(name)
}

// rawSavedFile is the minimal schema needed to extract per-window info
// from a tmux-session-saver JSON file. Mirrors the shape of
// internal/state.SessionState; only fields we render are decoded.
type rawSavedFile struct {
	SavedAt string `json:"saved_at"`
	Windows []struct {
		Name  string `json:"name"`
		Panes []struct {
			Process *struct {
				Command     string `json:"command"`
				LongRunning bool   `json:"long_running"`
			} `json:"process"`
			ClaudeSession *struct {
				Title string `json:"title"`
			} `json:"claude_session"`
		} `json:"panes"`
	} `json:"windows"`
}

// parseSavedState reads and parses the tmux-session-saver JSON file for a session.
// Returns nil if no saved state exists or the file is malformed.
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

	var raw rawSavedFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}

	// Normalise the saved_at timestamp: "2026-05-12T17:30:00.123+02:00" → "2026-05-12 17:30:00"
	savedAt := raw.SavedAt
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

	wins := make([]savedWindowInfo, 0, len(raw.Windows))
	for _, w := range raw.Windows {
		info := savedWindowInfo{name: w.Name}
		for _, p := range w.Panes {
			if p.ClaudeSession != nil {
				info.hasClaude = true
			}
			if p.Process != nil && p.Process.LongRunning && info.processCommand == "" {
				info.processCommand = p.Process.Command
			}
		}
		wins = append(wins, info)
	}

	return &savedStateData{
		savedAt:     savedAt,
		windows:     wins,
		windowCount: len(wins),
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

	for i, w := range sd.windows {
		name := w.name
		if len(name) > 30 {
			name = name[:27] + "..."
		}
		prefix := "├"
		if i == len(sd.windows)-1 {
			prefix = "└"
		}
		// Tag per window so the user can see WHAT each window will restore
		// to (not just a count of how many sessions exist in aggregate):
		//   "  · claude"            — pane runs claude --resume
		//   "  · npm run dev"       — long-running captured process
		//   (no tag)                — plain shell / editor
		var tag string
		switch {
		case w.hasClaude:
			tag = "  · claude"
		case w.processCommand != "":
			cmd := w.processCommand
			if len(cmd) > 24 {
				cmd = cmd[:21] + "..."
			}
			tag = "  · " + cmd
		}
		if tag != "" {
			out.WriteString(fmt.Sprintf(" %s%s%s %s%s%s%s\n", dim, prefix, colorReset, name, dim, tag, colorReset))
		} else {
			out.WriteString(fmt.Sprintf(" %s%s%s %s%s\n", dim, prefix, colorReset, name, colorReset))
		}
	}

	return out.String()
}

// fetchGitData runs git commands in parallel and returns cacheable data.
// Does NOT fetch Claude sessions (those are always fetched fresh).
// Respects context cancellation to abort early when the user navigates away.
func fetchGitData(ctx context.Context, path string, isActive bool) *gitData {
	var wg sync.WaitGroup
	var data gitData
	wg.Add(4)
	go func() { defer wg.Done(); data.branch = getGitBranch(ctx, path) }()
	go func() { defer wg.Done(); data.tracking = getGitTracking(ctx, path) }()
	go func() { defer wg.Done(); data.status = getGitStatus(ctx, path, isActive) }()
	go func() { defer wg.Done(); data.commits = getGitLog(ctx, path, isActive) }()
	wg.Wait()
	return &data
}

// fetchWorkspaceGitData runs filtered git commands in parallel for workspace sub-projects.
// Does NOT fetch Claude sessions (those are always fetched fresh).
func fetchWorkspaceGitData(ctx context.Context, path string, isActive bool) *workspaceGitData {
	var wg sync.WaitGroup
	var data workspaceGitData
	wg.Add(5)
	go func() { defer wg.Done(); data.branch = getGitBranch(ctx, path) }()
	go func() { defer wg.Done(); data.tracking = getGitTracking(ctx, path) }()
	go func() {
		defer wg.Done()
		data.filteredStatus, data.otherCount = getGitStatusFiltered(ctx, path, isActive)
	}()
	go func() { defer wg.Done(); data.filteredCommits = getGitLogFiltered(ctx, path, isActive) }()
	go func() { defer wg.Done(); data.relPrefix = getGitRelativePrefix(ctx, path) }()
	wg.Wait()
	return &data
}

// composeGitPreview assembles a git preview from cached data and fresh Claude sessions.
func composeGitPreview(sessionName string, isActive bool, data *gitData, claudeSessions string) string {
	cyan, green := colorCyan, colorGreen
	if !isActive {
		cyan, green = colorCyanDim, colorGreenDim
	}
	var output strings.Builder
	output.WriteString(fmt.Sprintf("%s󰘬 %s%s%s%s\n\n", cyan, data.branch, green, data.tracking, colorReset))
	if claudeSessions != "" {
		output.WriteString(claudeSessions)
		output.WriteString("\n")
	}
	// Saved tmux-session-saver state for non-active sessions: shows what
	// restore-or would rebuild when you enter the session. Reading the JSON
	// file is sub-ms so it's safe in the preview hot path.
	if !isActive {
		if saved := getSavedStateInfo(sessionName, isActive); saved != "" {
			output.WriteString(saved)
			output.WriteString("\n")
		}
	}
	output.WriteString(fmt.Sprintf("%s━━━ Status ━━━%s\n", colorDim, colorReset))
	if data.status == "" {
		output.WriteString(fmt.Sprintf("%sclean%s\n", colorDim, colorReset))
	} else {
		output.WriteString(data.status + "\n")
	}
	output.WriteString("\n")
	output.WriteString(fmt.Sprintf("%s━━━ Recent Commits ━━━%s\n", colorDim, colorReset))
	output.WriteString(data.commits)
	return output.String()
}

// composeWorkspacePreviewFromCache assembles a workspace preview from cached data and fresh Claude sessions.
func composeWorkspacePreviewFromCache(sessionName string, isActive bool, data *workspaceGitData, claudeSessions string) string {
	cyan, green := colorCyan, colorGreen
	if !isActive {
		cyan, green = colorCyanDim, colorGreenDim
	}
	var output strings.Builder
	output.WriteString(fmt.Sprintf("%s󰘬 %s%s%s%s\n\n", cyan, data.branch, green, data.tracking, colorReset))
	if claudeSessions != "" {
		output.WriteString(claudeSessions)
		output.WriteString("\n")
	}
	if !isActive {
		if saved := getSavedStateInfo(sessionName, isActive); saved != "" {
			output.WriteString(saved)
			output.WriteString("\n")
		}
	}
	output.WriteString(fmt.Sprintf("%s━━━ Status ━━━%s\n", colorDim, colorReset))
	if data.filteredStatus == "" {
		output.WriteString(fmt.Sprintf("%sclean%s\n", colorDim, colorReset))
	} else {
		output.WriteString(data.filteredStatus + "\n")
	}
	if data.otherCount > 0 {
		label := data.relPrefix
		if label == "" {
			label = "this directory"
		}
		output.WriteString(fmt.Sprintf("  %s%d other files changed outside %s%s\n", colorDim, data.otherCount, label, colorReset))
	}
	output.WriteString("\n")
	output.WriteString(fmt.Sprintf("%s━━━ Recent Commits ━━━%s\n", colorDim, colorReset))
	output.WriteString(data.filteredCommits)
	return output.String()
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

func getGitBranch(ctx context.Context, path string) string {
	cmd := exec.CommandContext(ctx, "git", "-C", path, "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func getGitTracking(ctx context.Context, path string) string {
	cmd := exec.CommandContext(ctx, "git", "-C", path, "rev-list", "--left-right", "--count", "@{upstream}...HEAD")
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

func getGitStatus(ctx context.Context, path string, isActive bool) string {
	cmd := exec.CommandContext(ctx, "git", "-C", path, "-c", "color.status=always", "status", "--short")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	status := string(output)
	if !isActive {
		status = dimANSI(status)
	}
	return strings.TrimSpace(status)
}

func getGitLog(ctx context.Context, path string, isActive bool) string {
	cmd := exec.CommandContext(ctx, "git", "-C", path, "log", "--oneline", "--graph", "--decorate", "--color=always", "-3")
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
	db := getClaudeDB()
	if db == nil {
		return ""
	}

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

	// Status icons and display — match miasma icons used in claude-sessions picker.
	var statusIcon string
	if status == "failed" {
		statusIcon = colorRed + "✗" + colorReset
	} else if status == "starting" {
		statusIcon = colorYellow + "◈" + colorReset
	} else if isActive {
		statusIcon = colorGreen + "◆" + colorReset
	} else {
		statusIcon = colorGray + "◇" + colorReset
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
