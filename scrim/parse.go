package scrim

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// parseANSI parses one line of `capture-pane -e` output into cells, one per
// terminal column. The stream is SGR-only in practice (tmux reconstructs the
// grid without cursor motion), but unknown CSI/OSC sequences are skipped
// rather than failed — a stray escape must never take the backdrop down.
func parseANSI(s string) []cell {
	var cells []cell
	var st style
	i := 0
	for i < len(s) {
		if s[i] == 0x1b {
			i = handleEscape(s, i, &st)
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		if r == '\r' || r == '\n' {
			continue
		}
		switch runewidth.RuneWidth(r) {
		case 0:
			// Combining marks and other zero-width runes: dropping them
			// loses a diacritic on a dimmed backdrop — invisible in practice
			// — and keeps every slice entry exactly one column.
			continue
		case 2:
			cells = append(cells,
				cell{r: r, width: 2, st: st, covered: true},
				cell{width: 0, st: st, covered: true})
		default:
			cells = append(cells, cell{r: r, width: 1, st: st, covered: true})
		}
	}
	return cells
}

// handleEscape advances past the escape sequence starting at s[i] (an ESC),
// applying it to st when it is SGR. Returns the index of the first byte
// after the sequence.
func handleEscape(s string, i int, st *style) int {
	if i+1 >= len(s) {
		return len(s)
	}
	switch s[i+1] {
	case '[': // CSI
		j := i + 2
		for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
			j++
		}
		if j >= len(s) {
			return len(s)
		}
		if s[j] == 'm' {
			applySGR(s[i+2:j], st)
		}
		return j + 1
	case ']': // OSC: runs to BEL or ST (ESC \)
		j := i + 2
		for j < len(s) {
			if s[j] == 0x07 {
				return j + 1
			}
			if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
				return j + 2
			}
			j++
		}
		return len(s)
	default:
		return i + 2
	}
}

// applySGR mutates st per one SGR parameter string ("0;1;38;5;241"-style;
// colon subparameters like "38:2:R:G:B" are handled too). Attributes the
// dimmer cannot honor (underline, italic, faint, …) are dropped silently.
func applySGR(seq string, st *style) {
	if seq == "" {
		*st = style{}
		return
	}
	parts := strings.Split(seq, ";")
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if strings.Contains(p, ":") {
			sub := strings.Split(p, ":")
			if n, err := strconv.Atoi(sub[0]); err == nil && (n == 38 || n == 48) {
				if c, _, ok := parseExtColor(sub[1:]); ok {
					if n == 38 {
						st.fg = c
					} else {
						st.bg = c
					}
				}
			}
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			continue
		}
		switch {
		case n == 0:
			*st = style{}
		case n == 1:
			st.bold = true
		case n == 22:
			st.bold = false
		case n == 7:
			st.reverse = true
		case n == 27:
			st.reverse = false
		case n == 39:
			st.fg = colorRef{}
		case n == 49:
			st.bg = colorRef{}
		case n >= 30 && n <= 37:
			st.fg = colorRef{kind: colANSI, n: uint8(n - 30)}
		case n >= 40 && n <= 47:
			st.bg = colorRef{kind: colANSI, n: uint8(n - 40)}
		case n >= 90 && n <= 97:
			st.fg = colorRef{kind: colANSI, n: uint8(n - 90 + 8)}
		case n >= 100 && n <= 107:
			st.bg = colorRef{kind: colANSI, n: uint8(n - 100 + 8)}
		case n == 38 || n == 48:
			c, used, ok := parseExtColor(parts[i+1:])
			i += used
			if !ok {
				continue
			}
			if n == 38 {
				st.fg = c
			} else {
				st.bg = c
			}
		}
	}
}

// parseExtColor reads the arguments of an extended color (after 38/48):
// "5;n" or "2;r;g;b" (already split). Returns how many elements it consumed
// — discriminator included — so semicolon-form parsing can skip them.
func parseExtColor(rest []string) (colorRef, int, bool) {
	if len(rest) == 0 {
		return colorRef{}, 0, false
	}
	switch rest[0] {
	case "5":
		if len(rest) < 2 {
			return colorRef{}, len(rest), false
		}
		n, err := strconv.Atoi(rest[1])
		if err != nil || n < 0 || n > 255 {
			return colorRef{}, 2, false
		}
		return colorRef{kind: col256, n: uint8(n)}, 2, true
	case "2":
		if len(rest) < 4 {
			return colorRef{}, len(rest), false
		}
		var v [3]uint8
		for i := 0; i < 3; i++ {
			n, err := strconv.Atoi(rest[1+i])
			if err != nil || n < 0 || n > 255 {
				return colorRef{}, 4, false
			}
			v[i] = uint8(n)
		}
		return colorRef{kind: colRGB, r: v[0], g: v[1], b: v[2]}, 4, true
	}
	return colorRef{}, 0, false
}
