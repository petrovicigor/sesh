// Package scrim draws an OpenCode-style dimmed backdrop behind a tmux popup.
//
// tmux cannot dim what lies behind a popup (popup-style styles the popup box
// only), so the popup owns the whole window area instead: Capture reads the
// panes beneath it straight from tmux's grid, blends every color toward a
// per-mode scrim base, and Compose splices the caller's panel — a borderless
// rectangle — centered over the pre-dimmed lines. The panel keeps the
// terminal's default background, one step above the scrim base, so it reads
// as a raised layer with no border chrome.
//
// The package is deliberately dependency-light (stdlib, x/ansi, go-runewidth
// — no repo-internal imports): the same files are duplicated byte-for-byte
// into the sesh fork, per the design decision to duplicate rather than share
// a module. Edit both copies together.
// Design: docs/superpowers/specs/2026-08-31-popup-scrim-overlay-design.md.
package scrim

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// keep is how much of a color survives the dim — the single overlay knob.
// Dimming is a straight multiply toward BLACK in both modes, exactly what a
// translucent black layer over the content would do (a browser modal scrim):
// hues survive, everything drops to half strength, and the undimmed box pops.
// Blending toward a theme-colored base was tried first and read as mud,
// especially in light mode.
const keepNum, keepDen = 50, 100

type rgb struct{ r, g, b uint8 }

// dim is the overlay: c as seen through the translucent black layer.
func dim(c rgb) rgb {
	ch := func(v uint8) uint8 {
		return uint8(int(v) * keepNum / keepDen)
	}
	return rgb{ch(c.r), ch(c.g), ch(c.b)}
}

// palette carries everything mode-dependent: the terminal defaults the
// Miasma themes define, the 16 ANSI colors as those themes paint them, the
// scrim base every backdrop color blends toward, and the pane-divider
// fallback for when MIASMA_BORDER_FG is unset.
type palette struct {
	fg, bg rgb
	base   rgb
	border rgb
	ansi   [16]rgb
}

// Palettes transcribed from .config/kitty/themes/miasma.conf and
// miasma-light.conf — the scrim must dim relative to what the terminal
// actually painted, and inside a tmux popup nothing can be queried (the
// OSC 11 trick gets no reply there), so the values are pinned.
//
// base is the theme background as seen through the overlay (bg through
// dim()) — what default-background cells and uncaptured area render as. The
// box keeps the real default background, well above it, so it pops without
// a border.
var darkPalette = palette{
	fg:     rgb{0xc2, 0xc2, 0xb0},
	bg:     rgb{0x22, 0x22, 0x22},
	base:   rgb{0x11, 0x11, 0x11}, // dim(#222222)
	border: rgb{0x3a, 0x33, 0x28},
	ansi: [16]rgb{
		{0x00, 0x00, 0x00}, {0x68, 0x57, 0x42}, {0x5f, 0x87, 0x5f}, {0xb3, 0x6d, 0x43},
		{0x78, 0x82, 0x4b}, {0xbb, 0x77, 0x44}, {0xc9, 0xa5, 0x54}, {0xd7, 0xc4, 0x83},
		{0x33, 0x33, 0x33}, {0x68, 0x57, 0x42}, {0x5f, 0x87, 0x5f}, {0xb3, 0x6d, 0x43},
		{0x78, 0x82, 0x4b}, {0xbb, 0x77, 0x44}, {0xc9, 0xa5, 0x54}, {0xd7, 0xc4, 0x83},
	},
}

var lightPalette = palette{
	fg:     rgb{0x6b, 0x62, 0x45},
	bg:     rgb{0xe8, 0xe0, 0xc8},
	base:   rgb{0x74, 0x70, 0x64}, // dim(#e8e0c8)
	border: rgb{0xc4, 0xb8, 0x90},
	ansi: [16]rgb{
		{0x8a, 0x80, 0x70}, {0x78, 0x56, 0x40}, {0x4a, 0x6a, 0x4a}, {0x8b, 0x5a, 0x2b},
		{0x5c, 0x68, 0x38}, {0x92, 0x5e, 0x2e}, {0x7d, 0x6b, 0x2e}, {0xd5, 0xcd, 0xae},
		{0xa0, 0x98, 0x80}, {0x96, 0x70, 0x4f}, {0x5d, 0x8a, 0x5d}, {0xa8, 0x70, 0x38},
		{0x74, 0x85, 0x48}, {0xb0, 0x76, 0x3a}, {0x9a, 0x86, 0x3a}, {0x6b, 0x62, 0x45},
	},
}

// currentPalette resolves the active mode from MIASMA_MODE (exported to the
// tmux global environment by miasma-theme.sh; unset — outside tmux or on a
// fresh machine — means dark).
func currentPalette() *palette {
	if os.Getenv("MIASMA_MODE") == "light" {
		return &lightPalette
	}
	return &darkPalette
}

// borderRGB is the pane-divider color before dimming: the live pane-border
// color when miasma-theme.sh has exported it, the palette fallback otherwise.
func borderRGB(p *palette) rgb {
	if c, ok := parseHexColor(os.Getenv("MIASMA_BORDER_FG")); ok {
		return c
	}
	return p.border
}

func parseHexColor(s string) (rgb, bool) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return rgb{}, false
	}
	var c rgb
	if _, err := fmt.Sscanf(s, "%02x%02x%02x", &c.r, &c.g, &c.b); err != nil {
		return rgb{}, false
	}
	return c, true
}

// colorRef is a color as SGR encodes it; resolution to rgb is mode-dependent
// and happens at dim time.
type colorKind uint8

const (
	colDefault colorKind = iota
	colANSI              // n in 0..15
	col256               // n in 0..255
	colRGB
)

type colorRef struct {
	kind    colorKind
	n       uint8
	r, g, b uint8
}

type style struct {
	fg, bg    colorRef
	bold      bool
	reverse   bool
	italic    bool
	underline bool
}

// cell is one terminal column. A wide rune owns its leading cell (width 2)
// and is followed by a width-0 continuation cell, so a row slice stays
// column-addressable and splicing can detect a bisected glyph.
type cell struct {
	r       rune
	width   int8
	st      style
	covered bool // false: no pane owns this cell (divider territory)
}

// resolve maps a colorRef to concrete rgb under palette p. isFg picks which
// terminal default an unset color falls back to.
func resolve(c colorRef, isFg bool, p *palette) rgb {
	switch c.kind {
	case colANSI:
		return p.ansi[c.n&0x0f]
	case col256:
		return xterm256(c.n, p)
	case colRGB:
		return rgb{c.r, c.g, c.b}
	default:
		if isFg {
			return p.fg
		}
		return p.bg
	}
}

// xterm256 maps a 256-color index: 0-15 through the Miasma palette, 16-231
// through the standard 6x6x6 cube, 232-255 through the grayscale ramp.
func xterm256(n uint8, p *palette) rgb {
	switch {
	case n < 16:
		return p.ansi[n]
	case n < 232:
		v := func(c uint8) uint8 {
			if c == 0 {
				return 0
			}
			return 55 + 40*c
		}
		i := n - 16
		return rgb{v(i / 36), v(i / 6 % 6), v(i % 6)}
	default:
		g := 8 + 10*(n-232)
		return rgb{g, g, g}
	}
}

// dimStyle turns a parsed style into the concrete dimmed truecolor pair the
// snapshot renders: resolve to RGB, then through the overlay. Bold survives
// (dimmed content keeps its texture); reverse is resolved here by swapping.
func dimStyle(st style, p *palette) style {
	fgRef, bgRef := st.fg, st.bg
	if st.reverse {
		fgRef, bgRef = bgRef, fgRef
	}
	fg := dim(resolve(fgRef, !st.reverse, p))
	bg := dim(resolve(bgRef, st.reverse, p))
	return style{
		fg:        colorRef{kind: colRGB, r: fg.r, g: fg.g, b: fg.b},
		bg:        colorRef{kind: colRGB, r: bg.r, g: bg.g, b: bg.b},
		bold:      st.bold,
		italic:    st.italic,
		underline: st.underline,
	}
}

// DimTerminalBG sets the terminal's own default background to the scrim base
// (OSC 11, wrapped in tmux passthrough — allow-passthrough is on). kitty
// paints its window padding with the default background, which popup cells
// can never cover, so without this the overlay stops at the cell grid and
// leaves bright strips at the window edges (ghostty extends edge-cell colors
// into its padding and doesn't need it, but it's harmless there). The box
// paints its background explicitly (see boxLine), so nothing on screen
// depends on the default while the popup is open. Also a free win: the whole
// terminal drops to the dim tone the instant the popup opens, before the
// first frame lands.
func DimTerminalBG(w io.Writer) {
	p := currentPalette()
	fmt.Fprintf(w, "\x1bPtmux;\x1b\x1b]11;#%02x%02x%02x\x07\x1b\\", p.base.r, p.base.g, p.base.b)
}

// RestoreTerminalBG puts the terminal background back to the THEME
// background, explicitly, on the way out. Not OSC 111: that resets to the
// terminal's config-file default, and kitty's config carries no background
// at all (the Miasma theme lands dynamically via the dark/light switcher) —
// the reset came out black and stuck until the next popup.
func RestoreTerminalBG(w io.Writer) {
	p := currentPalette()
	fmt.Fprintf(w, "\x1bPtmux;\x1b\x1b]11;#%02x%02x%02x\x07\x1b\\", p.bg.r, p.bg.g, p.bg.b)
}
