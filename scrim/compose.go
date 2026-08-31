package scrim

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Compose splices panel — a rectangular ANSI-styled block — centered over
// the snapshot and returns the full w×h frame, every cell painted (no
// bleed-through under diff-based renderers). A nil receiver, or a snapshot
// smaller than the screen, fills the difference with plain scrim base, so
// capture failure degrades to a dimmed-but-blank backdrop.
//
// Panel cells keep their styling, except that default BACKGROUNDS are made
// explicit (the theme background): while the popup is open the terminal's
// default background is set to the scrim base (DimTerminalBG, so kitty's
// window padding dims too), and a box cell left on the default would dim
// with it. That explicit theme bg, well above the scrim base, is what makes
// the borderless panel read as a raised layer. Panel lines shorter than the
// widest are padded defensively; lines wider than the screen are truncated.
func (s *Snapshot) Compose(panel string, w, h int) string {
	p := currentPalette()
	lines := strings.Split(panel, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	boxW := 0
	for _, l := range lines {
		if lw := ansi.StringWidth(l); lw > boxW {
			boxW = lw
		}
	}
	if boxW > w {
		boxW = w
		for i := range lines {
			lines[i] = ansi.Truncate(lines[i], w, "")
		}
	}
	x := (w - boxW) / 2
	y := (h - len(lines)) / 2

	var b strings.Builder
	for row := 0; row < h; row++ {
		if row > 0 {
			b.WriteByte('\n')
		}
		if row < y || row >= y+len(lines) {
			b.WriteString(s.bgLine(row, w, p))
			continue
		}
		line := lines[row-y]
		b.WriteString(s.seg(row, 0, x, p))
		b.WriteString(boxLine(line, boxW, p))
		b.WriteString(s.seg(row, x+boxW, w, p))
	}
	return b.String()
}

// boxLine re-renders one panel row with every default background rewritten
// to the explicit theme background (see Compose's doc), padded to width.
// Foregrounds and attributes pass through as the panel styled them —
// default foregrounds stay default, since only the background is hijacked
// while the popup is open.
func boxLine(line string, width int, p *palette) string {
	cells := parseANSI(line)
	var b strings.Builder
	var cur style
	curSet := false
	emit := func(c cell) {
		if !curSet || c.st != cur {
			writeBoxSGR(&b, c.st, p)
			cur, curSet = c.st, true
		}
		b.WriteRune(c.r)
	}
	col := 0
	for _, c := range cells {
		if col >= width {
			break
		}
		if c.width == 0 {
			continue // continuation cell of a wide rune already emitted
		}
		if c.width == 2 && col+1 >= width {
			// Wide rune bisected by the panel edge: a space keeps the
			// column count honest.
			emit(cell{r: ' ', width: 1, st: c.st})
			col++
			continue
		}
		emit(c)
		col += int(c.width)
	}
	for ; col < width; col++ {
		emit(cell{r: ' ', width: 1})
	}
	b.WriteString("\x1b[0m")
	return b.String()
}

// writeBoxSGR serializes a panel style, leading 0 so nothing leaks between
// runs: attributes as-is, foreground as the panel encoded it, background
// made explicit when it was the default.
func writeBoxSGR(b *strings.Builder, st style, p *palette) {
	b.WriteString("\x1b[0")
	if st.bold {
		b.WriteString(";1")
	}
	if st.italic {
		b.WriteString(";3")
	}
	if st.underline {
		b.WriteString(";4")
	}
	if st.reverse {
		b.WriteString(";7")
	}
	switch st.fg.kind {
	case colANSI:
		if st.fg.n < 8 {
			b.WriteString(";3")
			b.WriteString(itoa(st.fg.n))
		} else {
			b.WriteString(";9")
			b.WriteString(itoa(st.fg.n - 8))
		}
	case col256:
		b.WriteString(";38;5;")
		b.WriteString(itoa(st.fg.n))
	case colRGB:
		writeColor(b, ";38;2;", rgb{st.fg.r, st.fg.g, st.fg.b})
	}
	switch st.bg.kind {
	case colDefault:
		writeColor(b, ";48;2;", p.bg)
	case colANSI:
		if st.bg.n < 8 {
			b.WriteString(";4")
			b.WriteString(itoa(st.bg.n))
		} else {
			b.WriteString(";10")
			b.WriteString(itoa(st.bg.n - 8))
		}
	case col256:
		b.WriteString(";48;5;")
		b.WriteString(itoa(st.bg.n))
	case colRGB:
		writeColor(b, ";48;2;", rgb{st.bg.r, st.bg.g, st.bg.b})
	}
	b.WriteByte('m')
}

// bgLine is one full backdrop row: the pre-rendered snapshot line when it
// matches the frame width, otherwise re-rendered (or base-filled) to fit.
func (s *Snapshot) bgLine(row, w int, p *palette) string {
	if s != nil && row < s.h && s.w == w {
		return s.lines[row]
	}
	return s.seg(row, 0, w, p)
}

// seg renders backdrop columns [from, to) of one row. Columns outside the
// snapshot (or all of them, on a nil snapshot) are scrim-base blanks. A wide
// rune bisected by either boundary becomes a base-background space so the
// panel edge never shears.
func (s *Snapshot) seg(row, from, to int, p *palette) string {
	if to <= from {
		return ""
	}
	baseBlank := cell{r: ' ', width: 1}
	var b strings.Builder
	var cur style
	curSet := false
	emit := func(c cell) {
		if !curSet || c.st != cur {
			writeSGR(&b, c.st, p)
			cur, curSet = c.st, true
		}
		b.WriteRune(c.r)
	}
	col := from
	for col < to {
		if s == nil || row >= s.h || col >= s.w {
			emit(baseBlank)
			col++
			continue
		}
		c := s.cells[row][col]
		switch {
		case c.width == 0:
			// Continuation of a wide rune cut off by the left boundary.
			emit(cell{r: ' ', width: 1, st: c.st})
			col++
		case c.width == 2 && col+1 < to && col+1 < s.w:
			emit(c)
			col += 2
		case c.width == 2:
			// Wide rune bisected by the right boundary.
			emit(cell{r: ' ', width: 1, st: c.st})
			col++
		default:
			emit(c)
			col++
		}
	}
	b.WriteString("\x1b[0m")
	return b.String()
}

// writeSGR emits the full style in one sequence, leading 0 so no attribute
// leaks between runs. Styles here are always concrete colRGB (dimStyle's
// output); a zero style — nil-snapshot blanks — paints scrim base.
func writeSGR(b *strings.Builder, st style, p *palette) {
	fg := resolve(st.fg, true, p)
	bg := resolve(st.bg, false, p)
	if st.fg.kind == colDefault {
		fg = dim(p.fg)
	}
	if st.bg.kind == colDefault {
		bg = p.base
	}
	b.WriteString("\x1b[0")
	if st.bold {
		b.WriteString(";1")
	}
	writeColor(b, ";38;2;", fg)
	writeColor(b, ";48;2;", bg)
	b.WriteByte('m')
}

func writeColor(b *strings.Builder, prefix string, c rgb) {
	b.WriteString(prefix)
	b.WriteString(itoa(c.r))
	b.WriteByte(';')
	b.WriteString(itoa(c.g))
	b.WriteByte(';')
	b.WriteString(itoa(c.b))
}

func itoa(v uint8) string {
	// strconv.Itoa without the import churn in the hot render path.
	if v == 0 {
		return "0"
	}
	var buf [3]byte
	i := 3
	for v > 0 {
		i--
		buf[i] = '0' + v%10
		v /= 10
	}
	return string(buf[i:])
}
