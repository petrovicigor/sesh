package claude

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type trailRow struct {
	id, title, pane, tmuxSession, transcript, replacedBy, lastActivity string
	ended                                                              bool
	pid                                                                int
}

func setupTrailDB(t *testing.T, rows ...trailRow) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude", "sessions.db")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE sessions (
		session_id TEXT PRIMARY KEY, custom_title TEXT, tmux_pane TEXT,
		tmux_session TEXT, transcript_path TEXT, ended_at TEXT,
		replaced_by_session_id TEXT, pid INTEGER, last_activity TEXT)`)
	require.NoError(t, err)
	for _, r := range rows {
		var ended, replaced any
		if r.ended {
			ended = "2026-10-09T10:00:00Z"
		}
		if r.replacedBy != "" {
			replaced = r.replacedBy
		}
		_, err := db.Exec(`INSERT INTO sessions VALUES (?,?,?,?,?,?,?,?,?)`,
			r.id, r.title, r.pane, r.tmuxSession, r.transcript, ended, replaced, r.pid, r.lastActivity)
		require.NoError(t, err)
	}
	return dir
}

// stubPIDs makes the listed pids alive for the test.
func stubPIDs(t *testing.T, alive ...int) {
	t.Helper()
	orig := pidAlive
	set := map[int]bool{}
	for _, p := range alive {
		set[p] = true
	}
	pidAlive = func(pid int) bool { return set[pid] }
	t.Cleanup(func() { pidAlive = orig })
}

func TestPaneSession(t *testing.T) {
	home := setupTrailDB(t,
		trailRow{id: "stale", title: "Old server", pane: "%5", pid: 11, lastActivity: "2026-10-09T09:00:00Z"},
		trailRow{id: "live", title: "Fix login", pane: "%5", pid: 22, lastActivity: "2026-10-09T08:00:00Z"},
		trailRow{id: "ended", title: "Done", pane: "%6", pid: 33, ended: true},
		trailRow{id: "cleared", title: "Before /clear", pane: "%7", pid: 44, replacedBy: "x"},
	)
	stubPIDs(t, 22, 33, 44)

	id, title, ok := PaneSession(home, "%5")
	assert.True(t, ok)
	assert.Equal(t, "live", id, "a row whose process is gone (recycled pane id) must be skipped")
	assert.Equal(t, "Fix login", title)

	_, _, ok = PaneSession(home, "%6")
	assert.False(t, ok, "ended session")
	_, _, ok = PaneSession(home, "%7")
	assert.False(t, ok, "replaced session")
	_, _, ok = PaneSession(home, "")
	assert.False(t, ok)
	_, _, ok = PaneSession(t.TempDir(), "%5")
	assert.False(t, ok, "no db")
}

func TestLookup(t *testing.T) {
	home := setupTrailDB(t,
		trailRow{id: "a", title: "Live one", pane: "%1", tmuxSession: "dotfiles", pid: 1},
		trailRow{id: "b", title: "Ended one", pane: "%2", tmuxSession: "web", pid: 2, ended: true},
		trailRow{id: "c", title: "Crashed", pane: "%3", pid: 99},
		trailRow{id: "d", title: "Before clear", replacedBy: "e", ended: true},
		trailRow{id: "e", title: "After clear", pane: "%4", pid: 4},
		trailRow{id: "f", title: "Replacement gone", replacedBy: "nope", ended: true},
	)
	stubPIDs(t, 1, 2, 4)

	s, found, err := Lookup(home, "a")
	require.NoError(t, err)
	assert.True(t, found)
	assert.True(t, s.Live)
	assert.Equal(t, "%1", s.Pane)
	assert.Equal(t, "dotfiles", s.TmuxSession)

	s, found, _ = Lookup(home, "b")
	assert.True(t, found)
	assert.False(t, s.Live, "ended")

	s, _, _ = Lookup(home, "c")
	assert.False(t, s.Live, "a dead process without an end is resumable, not live")

	s, found, _ = Lookup(home, "d")
	assert.True(t, found)
	assert.Equal(t, "e", s.ID, "follows /clear replacements")
	assert.True(t, s.Live)

	s, found, _ = Lookup(home, "f")
	assert.True(t, found)
	assert.Equal(t, "f", s.ID, "a vanished replacement falls back to the row itself")

	_, found, err = Lookup(home, "zzz")
	assert.NoError(t, err)
	assert.False(t, found, "deleted row")

	_, found, err = Lookup(t.TempDir(), "a")
	assert.ErrorIs(t, err, ErrNoDB, "an unreadable db is never 'deleted'")
	assert.False(t, found)
}
