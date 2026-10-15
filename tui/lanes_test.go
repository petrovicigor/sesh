package tui

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// laneTestHome points HOME at a temp dir holding .claude/ and returns the
// sessions.db path (not created).
func laneTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, ".claude", "sessions.db")
}

func execTestSQL(t *testing.T, dbPath string, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func TestLoadLaneOwners_ReadsOwnedLanes(t *testing.T) {
	dbPath := laneTestHome(t)
	execTestSQL(t, dbPath,
		`CREATE TABLE lanes (path TEXT PRIMARY KEY, repo_dir TEXT NOT NULL, num INTEGER NOT NULL,
			owner_key TEXT, owner_label TEXT, route_id INTEGER, generation INTEGER NOT NULL DEFAULT 0,
			assigned_at TEXT, released_at TEXT, created_at TEXT NOT NULL)`,
		`INSERT INTO lanes (path, repo_dir, num, owner_label, created_at) VALUES
			('/p/usaa-customer/lane-1', '/r', 1, 'USAA-3351', 'x'),
			('/p/usaa-customer/lane-2', '/r', 2, NULL, 'x'),
			('/p/usaa-customer/lane-3', '/r', 3, '', 'x')`,
	)

	got := loadLaneOwners()
	if len(got) != 1 || got["/p/usaa-customer/lane-1"] != "USAA-3351" {
		t.Fatalf("loadLaneOwners() = %v, want only lane-1 -> USAA-3351", got)
	}
}

func TestLoadLaneOwners_NoLanesTable(t *testing.T) {
	dbPath := laneTestHome(t)
	execTestSQL(t, dbPath, `CREATE TABLE sessions (session_id TEXT PRIMARY KEY)`)

	got := loadLaneOwners()
	if got == nil || len(got) != 0 {
		t.Fatalf("loadLaneOwners() = %v, want empty non-nil map", got)
	}
}

func TestLoadLaneOwners_MissingFile(t *testing.T) {
	dbPath := laneTestHome(t)

	got := loadLaneOwners()
	if got == nil || len(got) != 0 {
		t.Fatalf("loadLaneOwners() = %v, want empty non-nil map", got)
	}
	// Loading must never create the database as a side effect.
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("loadLaneOwners() created %s (stat err = %v)", dbPath, err)
	}
}

func renderRow(d compactDelegate, width int, it sessionItem) string {
	l := list.New([]list.Item{it}, d, width, 10)
	var buf bytes.Buffer
	d.Render(&buf, l, 1, it) // index != cursor: unselected row
	return buf.String()
}

func laneDelegate(owners map[string]string, changes map[string]gitChanges) compactDelegate {
	return compactDelegate{laneOwners: &owners, gitChanges: &changes}
}

func TestDelegateRender_LaneOwnerAfterName(t *testing.T) {
	it := mkGitTestItem("tmux", "usaa-customer/lane-2", "/p/usaa-customer/lane-2")
	d := laneDelegate(map[string]string{"/p/usaa-customer/lane-2": "USAA-3351"}, nil)

	out := renderRow(d, 60, it)
	plain := ansi.Strip(out)
	if want := "  usaa-customer/lane-2  USAA-3351"; plain != want {
		t.Fatalf("row = %q, want %q", plain, want)
	}
	if !strings.Contains(out, "\033[38;5;245mUSAA-3351\033[0m") {
		t.Errorf("owner label must be drawn dim (gray 245), got %q", out)
	}
}

func TestDelegateRender_NoLaneOwner(t *testing.T) {
	it := mkGitTestItem("tmux", "usaa-customer/lane-2", "/p/usaa-customer/lane-2")

	// Path not in the map: name alone.
	d := laneDelegate(map[string]string{"/p/other/lane-1": "USAA-1"}, nil)
	if got := ansi.Strip(renderRow(d, 60, it)); got != "  usaa-customer/lane-2" {
		t.Errorf("unowned lane row = %q, want name alone", got)
	}

	// Empty map and zero-value delegate (no lanes table / load not landed).
	d = laneDelegate(map[string]string{}, nil)
	if got := ansi.Strip(renderRow(d, 60, it)); got != "  usaa-customer/lane-2" {
		t.Errorf("empty-map row = %q, want name alone", got)
	}
	if got := ansi.Strip(renderRow(compactDelegate{}, 60, it)); got != "  usaa-customer/lane-2" {
		t.Errorf("zero-value delegate row = %q, want name alone", got)
	}

	// A session with no path never matches, even against an empty-key entry.
	noPath := mkGitTestItem("tmux", "scratch", "")
	d = laneDelegate(map[string]string{"": "ghost"}, nil)
	if got := ansi.Strip(renderRow(d, 60, noPath)); got != "  scratch" {
		t.Errorf("pathless row = %q, want name alone", got)
	}
}

func TestDelegateRender_LaneOwnerTruncatesUnderNarrowWidth(t *testing.T) {
	it := mkGitTestItem("tmux", "usaa-customer/lane-2", "/p/usaa-customer/lane-2")
	d := laneDelegate(map[string]string{"/p/usaa-customer/lane-2": "USAA-3351 refund flow rewrite"}, nil)

	// "  usaa-customer/lane-2" is 22 cells; width 36 leaves 14 for gap + label.
	out := renderRow(d, 36, it)
	plain := ansi.Strip(out)
	if w := lipgloss.Width(out); w > 36 {
		t.Errorf("row is %d cells wide, must fit 36: %q", w, plain)
	}
	if !strings.HasPrefix(plain, "  usaa-customer/lane-2  USAA-3351") || !strings.HasSuffix(plain, "…") {
		t.Errorf("label must be cut with an ellipsis, got %q", plain)
	}
	if strings.Contains(plain, "refund") {
		t.Errorf("long label must not survive a narrow row, got %q", plain)
	}

	// Barely room: gap plus one cell cannot carry char + ellipsis, so the label
	// goes entirely and the name is untouched.
	plain = ansi.Strip(renderRow(d, 24, it))
	if plain != "  usaa-customer/lane-2" {
		t.Errorf("no-room row = %q, want name alone", plain)
	}
}

func TestDelegateRender_LaneOwnerYieldsBeforeGitBadge(t *testing.T) {
	it := mkGitTestItem("tmux", "usaa-customer/lane-2", "/p/usaa-customer/lane-2")
	d := laneDelegate(
		map[string]string{"/p/usaa-customer/lane-2": "USAA-3351 refund flow rewrite"},
		map[string]gitChanges{"/p/usaa-customer/lane-2": {Added: 12, Deleted: 3}},
	)

	// Width 44: name row 22 + badge " +12 -3" (7) leaves 15 for gap + label.
	out := renderRow(d, 44, it)
	plain := ansi.Strip(out)
	if w := lipgloss.Width(out); w > 44 {
		t.Errorf("row is %d cells wide, must fit 44: %q", w, plain)
	}
	if !strings.HasSuffix(plain, " +12 -3") {
		t.Errorf("git badge must survive the label being cut, got %q", plain)
	}
	if !strings.Contains(plain, "USAA-3351") || !strings.Contains(plain, "…") {
		t.Errorf("label must be truncated, not dropped, got %q", plain)
	}
	if !strings.Contains(plain, "lane-2  USAA") {
		t.Errorf("label must sit between name and badge, got %q", plain)
	}
}

func TestDelegateRender_LaneOwnerFitsWhenRoomy(t *testing.T) {
	// Exactly enough room: no ellipsis, no loss.
	it := mkGitTestItem("tmux", "usaa-customer/lane-2", "/p/usaa-customer/lane-2")
	d := laneDelegate(map[string]string{"/p/usaa-customer/lane-2": "USAA-3351"}, nil)

	// 22 (row) + 2 (gap) + 9 (label) = 33
	plain := ansi.Strip(renderRow(d, 33, it))
	if plain != "  usaa-customer/lane-2  USAA-3351" {
		t.Errorf("exact-fit row = %q, want the full label", plain)
	}
}

func TestLaneOwnerSuffix(t *testing.T) {
	tests := []struct {
		name   string
		label  string
		budget int
		want   string // ANSI-stripped
	}{
		{"fits", "USAA-1", 20, "  USAA-1"},
		{"exact fit", "USAA-1", 8, "  USAA-1"},
		{"one short truncates", "USAA-1", 7, "  USAA…"},
		{"two cells", "USAA-1", 4, "  U…"},
		{"gap only", "USAA-1", 3, ""},
		{"no budget", "USAA-1", 0, ""},
		{"negative budget", "USAA-1", -5, ""},
		{"empty label", "", 20, ""},
		{"wide runes count by cells", "改善改善", 7, "  改善…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ansi.Strip(laneOwnerSuffix(tt.label, tt.budget))
			if got != tt.want {
				t.Errorf("laneOwnerSuffix(%q, %d) = %q, want %q", tt.label, tt.budget, got, tt.want)
			}
			if tt.budget > 0 && lipgloss.Width(got) > tt.budget {
				t.Errorf("suffix is %d cells, budget %d", lipgloss.Width(got), tt.budget)
			}
		})
	}
}

func TestUpdateLaneOwnersMsg_FillsSharedMap(t *testing.T) {
	shared := &map[string]string{"/stale": "OLD-1"}
	m := Model{laneOwners: shared}
	d := compactDelegate{laneOwners: shared} // delegate holds the same pointer

	m.Update(LaneOwnersMsg{Owners: map[string]string{"/p/r/lane-1": "USAA-3351"}})

	got := *d.laneOwners
	if _, stale := got["/stale"]; stale {
		t.Error("stale entries must be cleared on update")
	}
	if got["/p/r/lane-1"] != "USAA-3351" {
		t.Errorf("shared map = %v, want /p/r/lane-1 -> USAA-3351", got)
	}
}

func TestDelegateRender_LaneOwnerWhileFiltering(t *testing.T) {
	it := mkGitTestItem("tmux", "usaa-customer/lane-2", "/p/usaa-customer/lane-2")
	d := laneDelegate(map[string]string{"/p/usaa-customer/lane-2": "USAA-3351 refund flow rewrite"}, nil)
	l := list.New([]list.Item{it}, d, 50, 10)
	l.SetFilterText("lane")
	l.SetFilterState(list.Filtering)
	if len(l.MatchesForItem(0)) == 0 {
		t.Fatal("harness: filter produced no matches for the item")
	}

	var buf bytes.Buffer
	d.Render(&buf, l, 0, it) // the selected, filter-matched row
	out := buf.String()
	plain := ansi.Strip(out)
	if w := lipgloss.Width(out); w > 50 {
		t.Errorf("row is %d cells wide, must fit 50: %q", w, plain)
	}
	if !strings.HasSuffix(plain, "tmux") {
		t.Errorf("source badge must stay right-aligned, got %q", plain)
	}
	if !strings.Contains(plain, "lane-2  USAA-3351") || !strings.Contains(plain, "…") {
		t.Errorf("label must be shown and cut to the room left of the badge, got %q", plain)
	}
}
