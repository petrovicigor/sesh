package claude

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// dbPath returns the path to Claude's sessions database given a home directory.
func dbPath(homeDir string) string {
	return filepath.Join(homeDir, ".claude", "sessions.db")
}

// SessionsNeedingAttention returns a set of tmux session names that have
// at least one active Claude Code session with "awaiting:*" status.
// homeDir is the user's home directory (used to locate ~/.claude/sessions.db).
// Returns an empty map (not error) if the DB doesn't exist.
func SessionsNeedingAttention(homeDir string) (map[string]bool, error) {
	path := dbPath(homeDir)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, nil
	}
	defer db.Close()

	// Wait up to 3s for Claude Code to release its write lock.
	// Without this, concurrent reads hit SQLITE_BUSY immediately.
	db.Exec("PRAGMA busy_timeout = 3000")

	rows, err := db.Query(`SELECT DISTINCT tmux_session FROM sessions
		WHERE ended_at IS NULL
		  AND status LIKE 'awaiting:%'
		  AND replaced_by_session_id IS NULL`)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			result[name] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil
	}
	return result, nil
}

// NeedsAttention checks if a specific tmux session has any active Claude Code
// session with "awaiting:*" status.
// Returns false (not error) if the DB doesn't exist.
func NeedsAttention(homeDir, tmuxSession string) (bool, error) {
	path := dbPath(homeDir)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false, nil
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return false, nil
	}
	defer db.Close()

	// Wait up to 3s for Claude Code to release its write lock.
	db.Exec("PRAGMA busy_timeout = 3000")

	var exists int
	err = db.QueryRow(`SELECT 1 FROM sessions
		WHERE tmux_session = ?
		  AND ended_at IS NULL
		  AND status LIKE 'awaiting:%'
		  AND replaced_by_session_id IS NULL
		LIMIT 1`, tmuxSession).Scan(&exists)
	if err != nil {
		return false, nil
	}
	return true, nil
}
