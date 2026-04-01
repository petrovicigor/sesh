package claude

import (
	"database/sql"
	"os"
	"path/filepath"
	"syscall"

	_ "modernc.org/sqlite"
)

// dbPath returns the path to Claude's sessions database given a home directory.
func dbPath(homeDir string) string {
	return filepath.Join(homeDir, ".claude", "sessions.db")
}

// openReadOnly opens Claude's sessions.db for read-only access.
// Configures the connection to never block writers:
//   - query_only: prevents accidental writes
//   - wal_autocheckpoint=0: prevents checkpointing on close (the main cause of hangs)
//   - busy_timeout=50: bail fast if any residual lock contention
func openReadOnly(homeDir string) (*sql.DB, error) {
	path := dbPath(homeDir)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.Exec("PRAGMA query_only = ON")
	db.Exec("PRAGMA wal_autocheckpoint = 0")
	db.Exec("PRAGMA busy_timeout = 50")

	return db, nil
}

// SessionsNeedingAttention returns tmux session names that have at least one
// active Claude Code session with "awaiting:*" status AND a live process.
// The process check prevents stale entries from dead/crashed sessions.
func SessionsNeedingAttention(homeDir string) (map[string]bool, error) {
	db, err := openReadOnly(homeDir)
	if err != nil {
		return nil, nil
	}
	defer db.Close()

	rows, err := db.Query(`SELECT tmux_session, pid FROM sessions
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
		var pid int
		if err := rows.Scan(&name, &pid); err == nil {
			if pid > 0 && syscall.Kill(pid, 0) == nil {
				result[name] = true
			}
		}
	}
	return result, nil
}
