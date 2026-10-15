package claude

import "strings"

// LaneOwners returns, for each claude-sessions lane (a git worktree reserved
// for one piece of work, `~/projects/<repo>/lane-N`), the label of the work
// that holds it: lane path -> owner_label. Lanes with no owner are absent.
//
// Best effort by design: a missing sessions.db, a database from before lanes
// existed (no `lanes` table), or any query failure reads as "no owners" — an
// empty map, never an error — so a row simply shows its name alone.
func LaneOwners(homeDir string) map[string]string {
	owners := make(map[string]string)

	db, err := openReadOnly(homeDir)
	if err != nil {
		return owners
	}
	defer db.Close()

	rows, err := db.Query(`SELECT path, owner_label FROM lanes
		WHERE owner_label IS NOT NULL AND TRIM(owner_label) != ''`)
	if err != nil {
		return owners
	}
	defer rows.Close()

	for rows.Next() {
		var path, label string
		if err := rows.Scan(&path, &label); err != nil {
			continue
		}
		if label = cleanLabel(label); label != "" {
			owners[path] = label
		}
	}
	return owners
}

// cleanLabel makes a database string safe to draw in a terminal row: control
// characters (newlines, escape sequences) become spaces and runs of whitespace
// collapse, so a label can never move the cursor or break the row.
func cleanLabel(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}
