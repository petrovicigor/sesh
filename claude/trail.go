package claude

import (
	"database/sql"
	"errors"
	"syscall"
)

// Session is the part of a claude-sessions row `sesh last` needs to return
// to it.
type Session struct {
	ID          string
	Title       string
	Pane        string // tmux pane id (%N) it ran in
	TmuxSession string
	Transcript  string
	// Live: not ended and its process is still there. A row whose process
	// died without an end (a crash) counts as ended — it can be resumed.
	Live bool
}

// ErrNoDB means the database could not be read: callers must treat the
// session as unknown, never as deleted.
var ErrNoDB = errors.New("claude sessions db unavailable")

// pidAlive is the liveness probe; a var so tests can stub it.
var pidAlive = func(pid int) bool {
	return pid > 0 && syscall.Kill(pid, 0) == nil
}

// PaneSession returns the live Claude session running in pane, if any. Pane
// ids restart at %0 with every tmux server while rows persist, so a row only
// counts while its process is alive.
func PaneSession(homeDir, pane string) (id, title string, ok bool) {
	if pane == "" {
		return "", "", false
	}
	db, err := openReadOnly(homeDir)
	if err != nil {
		return "", "", false
	}
	defer db.Close()

	rows, err := db.Query(`SELECT session_id, COALESCE(custom_title, ''), COALESCE(pid, 0)
		FROM sessions
		WHERE tmux_pane = ?
		  AND ended_at IS NULL
		  AND replaced_by_session_id IS NULL
		ORDER BY last_activity DESC`, pane)
	if err != nil {
		return "", "", false
	}
	defer rows.Close()
	for rows.Next() {
		var pid int
		if rows.Scan(&id, &title, &pid) == nil && pidAlive(pid) {
			return id, title, true
		}
	}
	return "", "", false
}

// maxReplacementHops bounds the /clear replacement chain walk.
const maxReplacementHops = 10

// Lookup finds a session by id, following /clear replacements to the newest
// one. found is false only when the row is gone (deleted from the picker);
// an unreadable database returns ErrNoDB instead.
func Lookup(homeDir, id string) (s Session, found bool, err error) {
	db, err := openReadOnly(homeDir)
	if err != nil {
		return Session{}, false, ErrNoDB
	}
	defer db.Close()

	for hop := 0; hop <= maxReplacementHops; hop++ {
		var (
			ended      sql.NullString
			replacedBy sql.NullString
			pid        int
		)
		row := db.QueryRow(`SELECT session_id, COALESCE(custom_title, ''),
				COALESCE(tmux_pane, ''), COALESCE(tmux_session, ''),
				COALESCE(transcript_path, ''), ended_at, replaced_by_session_id,
				COALESCE(pid, 0)
			FROM sessions WHERE session_id = ?`, id)
		err := row.Scan(&s.ID, &s.Title, &s.Pane, &s.TmuxSession, &s.Transcript,
			&ended, &replacedBy, &pid)
		if errors.Is(err, sql.ErrNoRows) {
			if hop > 0 {
				// The replacement vanished: fall back to the last row found.
				return s, true, nil
			}
			return Session{}, false, nil
		}
		if err != nil {
			return Session{}, false, ErrNoDB
		}
		s.Live = !ended.Valid && pidAlive(pid)
		if !replacedBy.Valid || replacedBy.String == "" || replacedBy.String == id {
			return s, true, nil
		}
		id = replacedBy.String
	}
	return s, true, nil
}
