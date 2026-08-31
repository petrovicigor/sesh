package scrim

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// paneCapture is one visible pane: its geometry within the window and its
// captured content, one string per row, SGR escapes included.
type paneCapture struct {
	left, top, w, h int
	lines           []string
}

// Snapshot is the dimmed backdrop: the window content that was beneath the
// popup, parsed to cells, dimmed, and pre-rendered. Compose is nil-safe, so
// callers hold a *Snapshot without branching on capture failure.
type Snapshot struct {
	cells [][]cell // every style already concrete colRGB
	lines []string // pre-rendered full rows, one per grid row
	w, h  int
}

// Capture reads the tmux window behind the popup and returns its dimmed
// snapshot. target is a pane or window id handed in by the launching bind
// (#{window_id}, expanded by run-shell) — inside a popup tmux's default
// target resolution points at the popup's own pane, so every call pins -t.
// Any failure returns nil and the error; the popup renders on a plain scrim
// with no backdrop, never breaks.
func Capture(target string) (*Snapshot, error) {
	if target == "" {
		return nil, fmt.Errorf("scrim: no capture target")
	}
	out, err := tmuxOut("display-message", "-p", "-t", target,
		"#{window_id} #{window_width} #{window_height} #{window_zoomed_flag}")
	if err != nil {
		return nil, err
	}
	f := strings.Fields(out)
	if len(f) != 4 {
		return nil, fmt.Errorf("scrim: unexpected window info %q", out)
	}
	winID := f[0]
	w, _ := strconv.Atoi(f[1])
	h, _ := strconv.Atoi(f[2])
	zoomed := f[3] == "1"
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("scrim: bad window size %dx%d", w, h)
	}

	panes, err := listPanes(winID, w, h, zoomed)
	if err != nil {
		return nil, err
	}
	for i := range panes {
		text, err := tmuxOut("capture-pane", "-epN", "-t", panes[i].id)
		if err != nil {
			return nil, err
		}
		panes[i].lines = strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	}

	caps := make([]paneCapture, len(panes))
	for i, p := range panes {
		caps[i] = p.paneCapture
	}
	return NewSnapshot(caps, w, h), nil
}

type paneGeom struct {
	id string
	paneCapture
}

// listPanes returns the panes to composite. A zoomed window is special: tmux
// reports every pane's unzoomed geometry, but only the active pane is on
// screen, at full window size.
func listPanes(winID string, w, h int, zoomed bool) ([]paneGeom, error) {
	out, err := tmuxOut("list-panes", "-t", winID, "-F",
		"#{pane_id} #{pane_left} #{pane_top} #{pane_width} #{pane_height} #{pane_active}")
	if err != nil {
		return nil, err
	}
	var panes []paneGeom
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Fields(line)
		if len(f) != 6 {
			continue
		}
		g := paneGeom{id: f[0]}
		g.left, _ = strconv.Atoi(f[1])
		g.top, _ = strconv.Atoi(f[2])
		g.w, _ = strconv.Atoi(f[3])
		g.h, _ = strconv.Atoi(f[4])
		if zoomed {
			if f[5] != "1" {
				continue
			}
			g.left, g.top, g.w, g.h = 0, 0, w, h
		}
		panes = append(panes, g)
	}
	if len(panes) == 0 {
		return nil, fmt.Errorf("scrim: no panes in %s", winID)
	}
	return panes, nil
}

func tmuxOut(args ...string) (string, error) {
	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		return "", fmt.Errorf("scrim: tmux %s: %w", args[0], err)
	}
	return string(out), nil
}

// NewSnapshot builds the dimmed snapshot from captured panes. Split from
// Capture so tests exercise everything below the exec boundary.
func NewSnapshot(panes []paneCapture, w, h int) *Snapshot {
	p := currentPalette()
	grid := buildGrid(panes, w, h)
	fillDividers(grid, p)
	for _, row := range grid {
		for i := range row {
			row[i].st = dimStyle(row[i].st, p)
		}
	}
	s := &Snapshot{cells: grid, w: w, h: h}
	s.lines = make([]string, h)
	for r := range grid {
		s.lines[r] = s.seg(r, 0, w, p)
	}
	return s
}

// buildGrid composites the panes' cells into one w×h grid at their window
// offsets. Cells no pane wrote keep covered=false — the divider gaps tmux
// draws its pane borders in, which capture-pane never returns.
func buildGrid(panes []paneCapture, w, h int) [][]cell {
	grid := make([][]cell, h)
	for r := range grid {
		grid[r] = make([]cell, w)
	}
	for _, p := range panes {
		for row := 0; row < p.h && p.top+row < h; row++ {
			var cells []cell
			if row < len(p.lines) {
				cells = parseANSI(p.lines[row])
			}
			gr := grid[p.top+row]
			col := 0
			for _, c := range cells {
				if col >= p.w || p.left+col >= w {
					break
				}
				if c.width == 2 && (col+1 >= p.w || p.left+col+1 >= w) {
					// Wide rune bisected by the pane edge: a space keeps the
					// column count honest.
					c = cell{r: ' ', width: 1, st: c.st, covered: true}
				}
				gr[p.left+col] = c
				col++
			}
			// capture-pane -N keeps trailing spaces, but a line can still
			// come back short; pad to the pane width with default-style
			// blanks so the pane area never leaks divider glyphs.
			for ; col < p.w && p.left+col < w; col++ {
				gr[p.left+col] = cell{r: ' ', width: 1, covered: true}
			}
		}
	}
	return grid
}

// fillDividers turns uncovered cells into dimmed border glyphs. Two passes:
// cells pinched between covered neighbors pick their obvious direction, and
// the leftovers (junctions, corners) follow whatever lines reach them.
func fillDividers(grid [][]cell, p *palette) {
	h := len(grid)
	if h == 0 {
		return
	}
	w := len(grid[0])
	// Snapshot coverage before drawing: a divider cell placed by this pass
	// must not make its uncovered neighbor look pinched — that turned every
	// second cell of a straight divider into a ┼.
	cov := make([][]bool, h)
	for r := range grid {
		cov[r] = make([]bool, w)
		for c := range grid[r] {
			cov[r][c] = grid[r][c].covered
		}
	}
	covered := func(r, c int) bool {
		if r < 0 || r >= h || c < 0 || c >= w {
			// Outside the window counts as covered: panes tile the full
			// area, so an edge gap is still a divider.
			return true
		}
		return cov[r][c]
	}
	bc := borderRGB(p)
	border := style{fg: colorRef{kind: colRGB, r: bc.r, g: bc.g, b: bc.b}}

	type pos struct{ r, c int }
	var unknown []pos
	for r := 0; r < h; r++ {
		for c := 0; c < w; c++ {
			if grid[r][c].covered {
				continue
			}
			hPinch := covered(r, c-1) && covered(r, c+1)
			vPinch := covered(r-1, c) && covered(r+1, c)
			var g rune
			switch {
			case hPinch && vPinch:
				g = '┼'
			case hPinch:
				g = '│'
			case vPinch:
				g = '─'
			default:
				unknown = append(unknown, pos{r, c})
				continue
			}
			grid[r][c] = cell{r: g, width: 1, st: border, covered: true}
		}
	}
	// Junctions: follow the lines already drawn around them.
	for _, u := range unknown {
		vert := (u.r > 0 && grid[u.r-1][u.c].r == '│') || (u.r+1 < h && grid[u.r+1][u.c].r == '│')
		horiz := (u.c > 0 && grid[u.r][u.c-1].r == '─') || (u.c+1 < w && grid[u.r][u.c+1].r == '─')
		g := ' '
		switch {
		case vert && horiz:
			g = '┼'
		case vert:
			g = '│'
		case horiz:
			g = '─'
		}
		grid[u.r][u.c] = cell{r: g, width: 1, st: border, covered: true}
	}
}
