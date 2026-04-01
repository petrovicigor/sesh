package claude

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, ".claude", "sessions.db")
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE sessions (
		session_id TEXT PRIMARY KEY,
		tmux_session TEXT,
		status TEXT,
		ended_at TEXT,
		replaced_by_session_id TEXT,
		pid INTEGER
	)`)
	require.NoError(t, err)
	return dir
}

func insertSession(t *testing.T, dir, sessionID, tmuxSession, status string, ended bool, pid int) {
	t.Helper()
	dbPath := filepath.Join(dir, ".claude", "sessions.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	var endedAt *string
	if ended {
		v := "2026-01-01"
		endedAt = &v
	}
	_, err = db.Exec(
		"INSERT INTO sessions (session_id, tmux_session, status, ended_at, pid) VALUES (?, ?, ?, ?, ?)",
		sessionID, tmuxSession, status, endedAt, pid)
	require.NoError(t, err)
}

func TestSessionsNeedingAttention_NoDB(t *testing.T) {
	result, err := SessionsNeedingAttention("/nonexistent")
	assert.NoError(t, err)
	assert.Empty(t, result)
}

func TestSessionsNeedingAttention_NoAwaitingSessions(t *testing.T) {
	dir := setupTestDB(t)
	insertSession(t, dir, "s1", "myproject", "thinking", false, os.Getpid())
	insertSession(t, dir, "s2", "other", "running:tool", false, os.Getpid())

	result, err := SessionsNeedingAttention(dir)
	assert.NoError(t, err)
	assert.Empty(t, result)
}

func TestSessionsNeedingAttention_WithLiveProcess(t *testing.T) {
	dir := setupTestDB(t)
	// Use current PID (alive) for the awaiting session
	insertSession(t, dir, "s1", "myproject", "awaiting:permission", false, os.Getpid())
	insertSession(t, dir, "s2", "other", "thinking", false, os.Getpid())

	result, err := SessionsNeedingAttention(dir)
	assert.NoError(t, err)
	assert.True(t, result["myproject"])
	assert.False(t, result["other"])
}

func TestSessionsNeedingAttention_SkipsDeadProcess(t *testing.T) {
	dir := setupTestDB(t)
	// PID 999999 almost certainly doesn't exist
	insertSession(t, dir, "s1", "myproject", "awaiting:permission", false, 999999)

	result, err := SessionsNeedingAttention(dir)
	assert.NoError(t, err)
	assert.Empty(t, result)
}

func TestSessionsNeedingAttention_IgnoresEndedSessions(t *testing.T) {
	dir := setupTestDB(t)
	insertSession(t, dir, "s1", "myproject", "awaiting:permission", true, os.Getpid())

	result, err := SessionsNeedingAttention(dir)
	assert.NoError(t, err)
	assert.Empty(t, result)
}
