package scrim

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func stripRows(frame string) []string {
	rows := strings.Split(frame, "\n")
	for i := range rows {
		rows[i] = ansi.Strip(rows[i])
	}
	return rows
}

func TestFillCentersPanel(t *testing.T) {
	rows := stripRows(Fill("XX\nYY", 10, 5))
	if len(rows) != 5 {
		t.Fatalf("rows = %d, want 5", len(rows))
	}
	// box 2x2 on 10x5: x = 4, y = 1
	if rows[1] != "    XX    " || rows[2] != "    YY    " {
		t.Errorf("panel rows:\n%q\n%q", rows[1], rows[2])
	}
}

func TestFillEveryRowFullWidthAndPainted(t *testing.T) {
	frame := Fill("XX", 8, 3)
	for i, row := range strings.Split(frame, "\n") {
		if w := ansi.StringWidth(row); w != 8 {
			t.Errorf("row %d width = %d, want 8", i, w)
		}
	}
	if !strings.Contains(frame, "\x1b[48;2;") {
		t.Error("backdrop carries no explicit scrim background")
	}
}

func TestFillLightMode(t *testing.T) {
	t.Setenv("MIASMA_MODE", "light")
	if !strings.Contains(Fill("X", 4, 2), baseLight) {
		t.Error("light mode did not paint the light scrim base")
	}
	t.Setenv("MIASMA_MODE", "")
	if !strings.Contains(Fill("X", 4, 2), baseDark) {
		t.Error("unset mode should paint the dark scrim base")
	}
}

func TestFillShortPanelLinePadded(t *testing.T) {
	rows := stripRows(Fill("WWWW\nY", 10, 3))
	for i, row := range rows {
		if w := ansi.StringWidth(row); w != 10 {
			t.Errorf("row %d width = %d, want 10 (%q)", i, w, row)
		}
	}
	if rows[1] != "   WWWW   " && rows[0] != "   WWWW   " {
		t.Errorf("unexpected rows: %q %q %q", rows[0], rows[1], rows[2])
	}
}

func TestFillOversizedPanelClamped(t *testing.T) {
	rows := stripRows(Fill("123456\n123456\n123456\n123456", 4, 2))
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	for _, row := range rows {
		if w := ansi.StringWidth(row); w != 4 {
			t.Errorf("row width = %d, want 4 (%q)", w, row)
		}
	}
}

func TestFillPanelKeepsOwnStyling(t *testing.T) {
	panel := "\x1b[38;5;2mOK\x1b[0m"
	if !strings.Contains(Fill(panel, 8, 1), "\x1b[38;5;2mOK") {
		t.Error("panel escapes were rewritten")
	}
}

func TestFillDegenerateFrame(t *testing.T) {
	if got := Fill("panel", 0, 0); got != "panel" {
		t.Errorf("zero frame = %q, want the panel unchanged", got)
	}
}
