// Package scrim paints the backdrop for a full-client tmux popup: a solid
// per-mode scrim color edge to edge, with the caller's borderless panel
// spliced centered on top.
//
// tmux cannot dim what lies behind a popup (popup-style styles the popup box
// only), so the popup covers the whole client and this package paints the
// overlay surface. The backdrop is a plain color by design — no capture, no
// dimmed ghost of the content beneath (evaluated and rejected as visual
// noise). The panel keeps the terminal's default background, which sits above
// the scrim base, so it reads as a raised layer with no border chrome.
//
// The package is deliberately dependency-light (stdlib + x/ansi): the same
// files are duplicated byte-for-byte into the sesh fork, per the design
// decision to duplicate rather than share a module. Edit both copies
// together.
// Design: docs/superpowers/specs/2026-08-31-popup-scrim-overlay-design.md.
package scrim

import (
	"os"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Scrim base per mode, resolved from MIASMA_MODE (exported to the tmux global
// environment by miasma-theme.sh; unset — outside tmux or on a fresh machine
// — means dark). Both sit well below the theme background (dark `#222222`,
// light `#e8e0c8`), so the panel — which keeps the terminal default bg —
// pops without a border. The light base is roughly a 30% black scrim over
// the cream background, the classical modal treatment.
const (
	baseDark  = "\x1b[48;2;15;15;15m"    // #0f0f0f
	baseLight = "\x1b[48;2;160;152;128m" // #a09880
)

func base() string {
	if os.Getenv("MIASMA_MODE") == "light" {
		return baseLight
	}
	return baseDark
}

// Fill returns the full w×h popup frame: panel centered over a solid
// scrim-base backdrop, every cell painted (no bleed-through under diff-based
// renderers). Panel lines pass through untouched — their default-background
// cells render as the terminal background, which IS the raised-panel color —
// padded defensively to the panel's width; lines wider or taller than the
// frame are clamped.
func Fill(panel string, w, h int) string {
	if w <= 0 || h <= 0 {
		return panel
	}
	bg := base()
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

	blank := func(n int) string {
		if n <= 0 {
			return ""
		}
		return "\x1b[0m" + bg + strings.Repeat(" ", n) + "\x1b[0m"
	}
	full := blank(w)

	var b strings.Builder
	for row := 0; row < h; row++ {
		if row > 0 {
			b.WriteByte('\n')
		}
		if row < y || row >= y+len(lines) {
			b.WriteString(full)
			continue
		}
		line := lines[row-y]
		b.WriteString(blank(x))
		b.WriteString("\x1b[0m")
		b.WriteString(line)
		if pad := boxW - ansi.StringWidth(line); pad > 0 {
			b.WriteString("\x1b[0m")
			b.WriteString(strings.Repeat(" ", pad))
		}
		b.WriteString(blank(w - x - boxW))
	}
	return b.String()
}
