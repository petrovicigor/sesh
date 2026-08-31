package scrim

import "testing"

func gridRunes(grid [][]cell) []string {
	out := make([]string, len(grid))
	for r, row := range grid {
		s := make([]rune, 0, len(row))
		for _, c := range row {
			if !c.covered {
				s = append(s, '·') // uncovered sentinel for assertions
				continue
			}
			if c.width == 0 {
				continue
			}
			s = append(s, c.r)
		}
		out[r] = string(s)
	}
	return out
}

func TestBuildGridSideBySide(t *testing.T) {
	// Two panes with the classic one-column gap tmux draws its border in.
	panes := []paneCapture{
		{left: 0, top: 0, w: 3, h: 2, lines: []string{"abc", "def"}},
		{left: 4, top: 0, w: 3, h: 2, lines: []string{"ghi", "jkl"}},
	}
	grid := buildGrid(panes, 7, 2)
	got := gridRunes(grid)
	want := []string{"abc·ghi", "def·jkl"}
	for r := range want {
		if got[r] != want[r] {
			t.Errorf("row %d = %q, want %q", r, got[r], want[r])
		}
	}
}

func TestBuildGridPadsShortLines(t *testing.T) {
	panes := []paneCapture{{left: 0, top: 0, w: 4, h: 2, lines: []string{"ab"}}}
	grid := buildGrid(panes, 4, 2)
	got := gridRunes(grid)
	// Short line padded to pane width; missing second line padded too.
	want := []string{"ab  ", "    "}
	for r := range want {
		if got[r] != want[r] {
			t.Errorf("row %d = %q, want %q", r, got[r], want[r])
		}
	}
}

func TestBuildGridWideRuneAtPaneEdge(t *testing.T) {
	panes := []paneCapture{{left: 0, top: 0, w: 2, h: 1, lines: []string{"a世"}}}
	grid := buildGrid(panes, 2, 1)
	if grid[0][1].r != ' ' {
		t.Errorf("bisected wide rune = %q, want space", grid[0][1].r)
	}
}

func TestFillDividers(t *testing.T) {
	panes := []paneCapture{
		{left: 0, top: 0, w: 3, h: 2, lines: []string{"abc", "def"}},
		{left: 4, top: 0, w: 3, h: 2, lines: []string{"ghi", "jkl"}},
	}
	grid := buildGrid(panes, 7, 2)
	fillDividers(grid, &darkPalette)
	for r := 0; r < 2; r++ {
		if grid[r][3].r != '│' {
			t.Errorf("row %d divider = %q, want │", r, grid[r][3].r)
		}
		if !grid[r][3].covered {
			t.Error("divider not marked covered")
		}
	}
}

func TestFillDividersHorizontalAndJunction(t *testing.T) {
	// Left pane full height; right side split into two stacked panes:
	// vertical divider at col 3, horizontal divider at row 2 cols 4..6,
	// junction where they meet.
	panes := []paneCapture{
		{left: 0, top: 0, w: 3, h: 5, lines: []string{"aaa", "aaa", "aaa", "aaa", "aaa"}},
		{left: 4, top: 0, w: 3, h: 2, lines: []string{"bbb", "bbb"}},
		{left: 4, top: 3, w: 3, h: 2, lines: []string{"ccc", "ccc"}},
	}
	grid := buildGrid(panes, 7, 5)
	fillDividers(grid, &darkPalette)
	for _, r := range []int{0, 1, 3, 4} {
		if grid[r][3].r != '│' {
			t.Errorf("row %d col 3 = %q, want │", r, grid[r][3].r)
		}
	}
	for c := 4; c < 7; c++ {
		if grid[2][c].r != '─' {
			t.Errorf("row 2 col %d = %q, want ─", c, grid[2][c].r)
		}
	}
	// The junction cell picks up both directions from its drawn neighbors.
	if g := grid[2][3].r; g != '┼' && g != '│' {
		t.Errorf("junction = %q, want ┼ (or │)", g)
	}
}

func TestNewSnapshotZoomedEquivalent(t *testing.T) {
	// Zoom is resolved in listPanes (active pane only, full size); NewSnapshot
	// just has to handle a single full-size pane cleanly.
	panes := []paneCapture{{left: 0, top: 0, w: 4, h: 2, lines: []string{"full", "size"}}}
	s := NewSnapshot(panes, 4, 2)
	if s.w != 4 || s.h != 2 || len(s.lines) != 2 {
		t.Fatalf("snapshot dims = %dx%d, %d lines", s.w, s.h, len(s.lines))
	}
	for _, row := range s.cells {
		for _, c := range row {
			if c.st.fg.kind != colRGB || c.st.bg.kind != colRGB {
				t.Fatal("snapshot cell not concrete RGB after dimming")
			}
		}
	}
}
