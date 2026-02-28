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
		replaced_by_session_id TEXT
	)`)
	require.NoError(t, err)
	return dir
}

func insertSession(t *testing.T, dir, sessionID, tmuxSession, status string, ended bool) {
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
		"INSERT INTO sessions (session_id, tmux_session, status, ended_at) VALUES (?, ?, ?, ?)",
		sessionID, tmuxSession, status, endedAt)
	require.NoError(t, err)
}

func TestSessionsNeedingAttention_NoDB(t *testing.T) {
	result, err := SessionsNeedingAttention("/nonexistent")
	assert.NoError(t, err)
	assert.Empty(t, result)
}

func TestSessionsNeedingAttention_NoAwaitingSessions(t *testing.T) {
	dir := setupTestDB(t)
	insertSession(t, dir, "s1", "myproject", "thinking", false)
	insertSession(t, dir, "s2", "other", "running:tool", false)

	result, err := SessionsNeedingAttention(dir)
	assert.NoError(t, err)
	assert.Empty(t, result)
}

func TestSessionsNeedingAttention_WithAwaitingSessions(t *testing.T) {
	dir := setupTestDB(t)
	insertSession(t, dir, "s1", "myproject", "awaiting:permission", false)
	insertSession(t, dir, "s2", "other", "thinking", false)
	insertSession(t, dir, "s3", "myproject", "running:tool", false)

	result, err := SessionsNeedingAttention(dir)
	assert.NoError(t, err)
	assert.True(t, result["myproject"])
	assert.False(t, result["other"])
}

func TestSessionsNeedingAttention_IgnoresEndedSessions(t *testing.T) {
	dir := setupTestDB(t)
	insertSession(t, dir, "s1", "myproject", "awaiting:permission", true) // ended

	result, err := SessionsNeedingAttention(dir)
	assert.NoError(t, err)
	assert.Empty(t, result)
}

func TestNeedsAttention_True(t *testing.T) {
	dir := setupTestDB(t)
	insertSession(t, dir, "s1", "myproject", "awaiting:permission", false)

	result, err := NeedsAttention(dir, "myproject")
	assert.NoError(t, err)
	assert.True(t, result)
}

func TestNeedsAttention_False(t *testing.T) {
	dir := setupTestDB(t)
	insertSession(t, dir, "s1", "myproject", "thinking", false)

	result, err := NeedsAttention(dir, "myproject")
	assert.NoError(t, err)
	assert.False(t, result)
}

func TestNeedsAttention_NoDB(t *testing.T) {
	result, err := NeedsAttention("/nonexistent", "myproject")
	assert.NoError(t, err)
	assert.False(t, result)
}
