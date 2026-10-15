package claude

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addLanesTable creates claude-sessions' lanes table (the columns it has
// today) in the test DB and inserts the given (path, owner_label) rows; a nil
// label inserts NULL.
func addLanesTable(t *testing.T, dir string, rows map[string]*string) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(dir, ".claude", "sessions.db"))
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE lanes (
		path        TEXT PRIMARY KEY,
		repo_dir    TEXT NOT NULL,
		num         INTEGER NOT NULL,
		owner_key   TEXT,
		owner_label TEXT,
		route_id    INTEGER,
		generation  INTEGER NOT NULL DEFAULT 0,
		assigned_at TEXT,
		released_at TEXT,
		created_at  TEXT NOT NULL
	)`)
	require.NoError(t, err)

	n := 0
	for path, label := range rows {
		n++
		_, err = db.Exec(
			`INSERT INTO lanes (path, repo_dir, num, owner_label, created_at) VALUES (?, ?, ?, ?, ?)`,
			path, "/repo/.git", n, label, "2026-10-15T00:00:00Z")
		require.NoError(t, err)
	}
}

func strp(s string) *string { return &s }

func TestLaneOwners_MissingDB(t *testing.T) {
	owners := LaneOwners("/nonexistent")
	assert.NotNil(t, owners)
	assert.Empty(t, owners)
}

func TestLaneOwners_NoLanesTable(t *testing.T) {
	dir := setupTestDB(t) // has a sessions table, no lanes table
	owners := LaneOwners(dir)
	assert.NotNil(t, owners)
	assert.Empty(t, owners)
}

func TestLaneOwners_ReturnsOwnedLanes(t *testing.T) {
	dir := setupTestDB(t)
	addLanesTable(t, dir, map[string]*string{
		"/p/usaa-customer/lane-1": strp("USAA-3351"),
		"/p/usaa-customer/lane-2": strp("Fix the login redirect"),
		"/p/usaa-customer/lane-3": nil,         // never taken
		"/p/usaa-customer/lane-4": strp(""),    // owner cleared
		"/p/usaa-customer/lane-5": strp(" \t"), // blank
	})

	owners := LaneOwners(dir)
	assert.Equal(t, map[string]string{
		"/p/usaa-customer/lane-1": "USAA-3351",
		"/p/usaa-customer/lane-2": "Fix the login redirect",
	}, owners)
}

func TestLaneOwners_SanitizesControlCharacters(t *testing.T) {
	dir := setupTestDB(t)
	addLanesTable(t, dir, map[string]*string{
		"/p/r/lane-1": strp("USAA-1\n\x1b[2Jevil\r\t  label"),
		"/p/r/lane-2": strp("\x1b\x07"), // nothing left once cleaned
	})

	owners := LaneOwners(dir)
	assert.Equal(t, map[string]string{"/p/r/lane-1": "USAA-1 [2Jevil label"}, owners)
}
