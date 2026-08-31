package scrim

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// stripRows splits a composed frame and strips escapes for text assertions.
func stripRows(frame string) []string {
	rows := strings.Split(frame, "\n")
	for i := range rows {
		rows[i] = ansi.Strip(rows[i])
	}
	return rows
}

func snap(t *testing.T, w, h int, lines []string) *Snapshot {
	t.Helper()
	return NewSnapshot([]paneCapture{{left: 0, top: 0, w: w, h: h, lines: lines}}, w, h)
}

func TestComposeCentersPanel(t *testing.T) {
	s := snap(t, 10, 5, []string{"aaaaaaaaaa", "bbbbbbbbbb", "cccccccccc", "dddddddddd", "eeeeeeeeee"})
	frame := s.Compose("XX\nYY", 10, 5)
	rows := stripRows(frame)
	if len(rows) != 5 {
		t.Fatalf("rows = %d, want 5", len(rows))
	}
	// box 2x2 on 10x5: x = 4, y = 1
	if rows[1] != "bbbbXXbbbb" || rows[2] != "ccccYYcccc" {
		t.Errorf("panel rows:\n%q\n%q", rows[1], rows[2])
	}
	if rows[0] != "aaaaaaaaaa" || rows[4] != "eeeeeeeeee" {
		t.Errorf("backdrop rows:\n%q\n%q", rows[0], rows[4])
	}
}

func TestComposeEveryRowFullWidth(t *testing.T) {
	s := snap(t, 8, 3, []string{"aaaaaaaa", "bbbbbbbb", "cccccccc"})
	for i, row := range stripRows(s.Compose("XX", 8, 3)) {
		if w := ansi.StringWidth(row); w != 8 {
			t.Errorf("row %d width = %d, want 8", i, w)
		}
	}
}

func TestComposeNilSnapshot(t *testing.T) {
	var s *Snapshot
	frame := s.Compose("XX\nYY", 6, 4)
	rows := stripRows(frame)
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4", len(rows))
	}
	if rows[1] != "  XX  " || rows[2] != "  YY  " {
		t.Errorf("nil-snapshot panel rows: %q, %q", rows[1], rows[2])
	}
	// Backdrop rows are scrim-base blanks, explicitly painted.
	if !strings.Contains(frame, "\x1b[0;38;2;") {
		t.Error("nil-snapshot backdrop carries no explicit style")
	}
}

func TestComposeShortPanelLinePadded(t *testing.T) {
	s := snap(t, 10, 3, []string{"aaaaaaaaaa", "bbbbbbbbbb", "cccccccccc"})
	rows := stripRows(s.Compose("WWWW\nY", 10, 3))
	// Box width 4 at x=3; short line "Y" padded to 4 with default-bg spaces.
	if rows[1] != "aaaWWWWaaa" && rows[0] != "aaaWWWWaaa" {
		t.Errorf("unexpected rows: %q %q %q", rows[0], rows[1], rows[2])
	}
	for _, row := range rows {
		if w := ansi.StringWidth(row); w != 10 {
			t.Errorf("row width = %d, want 10 (%q)", w, row)
		}
	}
}

func TestComposeOversizedPanelClamped(t *testing.T) {
	s := snap(t, 4, 2, []string{"aaaa", "bbbb"})
	frame := s.Compose("123456\n123456\n123456\n123456", 4, 2)
	rows := stripRows(frame)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	for _, row := range rows {
		if w := ansi.StringWidth(row); w != 4 {
			t.Errorf("row width = %d, want 4 (%q)", w, row)
		}
	}
}

func TestComposeWideRuneAtSpliceEdge(t *testing.T) {
	// 世 occupies columns 1-2; a panel starting at column 2 bisects it.
	s := snap(t, 6, 1, []string{"a世def"})
	frame := s.Compose("XX", 6, 1)
	row := stripRows(frame)[0]
	if ansi.StringWidth(row) != 6 {
		t.Fatalf("row width = %d, want 6 (%q)", ansi.StringWidth(row), row)
	}
	if !strings.Contains(row, "XX") {
		t.Fatalf("panel missing: %q", row)
	}
	if strings.ContainsRune(row, '世') {
		t.Errorf("bisected wide rune survived: %q", row)
	}
}

func TestComposeSnapshotSmallerThanScreen(t *testing.T) {
	s := snap(t, 4, 2, []string{"aaaa", "bbbb"})
	rows := stripRows(s.Compose("XX", 8, 4))
	if len(rows) != 4 {
		t.Fatalf("rows = %d", len(rows))
	}
	for i, row := range rows {
		if w := ansi.StringWidth(row); w != 8 {
			t.Errorf("row %d width = %d, want 8 (%q)", i, w, row)
		}
	}
}

func TestComposePanelKeepsOwnStyling(t *testing.T) {
	s := snap(t, 8, 1, []string{"aaaaaaaa"})
	panel := "\x1b[38;5;2mOK\x1b[0m"
	frame := s.Compose(panel, 8, 1)
	if !strings.Contains(frame, "\x1b[38;5;2mOK") {
		t.Error("panel escapes were rewritten")
	}
}
