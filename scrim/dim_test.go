package scrim

import "testing"

func TestDim(t *testing.T) {
	tests := []struct {
		name string
		in   rgb
		want rgb
	}{
		// c * 50/100 per channel — the translucent-black-layer model.
		{"miasma fg", rgb{0xc2, 0xc2, 0xb0}, rgb{0x61, 0x61, 0x58}},
		{"dark bg", rgb{0x22, 0x22, 0x22}, rgb{0x11, 0x11, 0x11}},
		{"black stays black", rgb{0, 0, 0}, rgb{0, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dim(tt.in); got != tt.want {
				t.Errorf("dim(%+v) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	p := &darkPalette
	tests := []struct {
		name string
		c    colorRef
		isFg bool
		want rgb
	}{
		{"default fg", colorRef{}, true, p.fg},
		{"default bg", colorRef{}, false, p.bg},
		{"ansi green through palette", colorRef{kind: colANSI, n: 2}, true, rgb{0x5f, 0x87, 0x5f}},
		{"256 low index through palette", colorRef{kind: col256, n: 2}, true, rgb{0x5f, 0x87, 0x5f}},
		{"256 cube 16 is black", colorRef{kind: col256, n: 16}, true, rgb{0, 0, 0}},
		{"256 cube 231 is white", colorRef{kind: col256, n: 231}, true, rgb{0xff, 0xff, 0xff}},
		{"256 cube 196 is red", colorRef{kind: col256, n: 196}, true, rgb{0xff, 0, 0}},
		{"256 grayscale 232", colorRef{kind: col256, n: 232}, true, rgb{8, 8, 8}},
		{"256 grayscale 255", colorRef{kind: col256, n: 255}, true, rgb{238, 238, 238}},
		{"rgb passthrough", colorRef{kind: colRGB, r: 1, g: 2, b: 3}, true, rgb{1, 2, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolve(tt.c, tt.isFg, p); got != tt.want {
				t.Errorf("resolve = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDimStyle(t *testing.T) {
	p := &darkPalette
	t.Run("default bg lands exactly on the dimmed theme bg", func(t *testing.T) {
		d := dimStyle(style{}, p)
		want := dim(p.bg)
		if d.bg != (colorRef{kind: colRGB, r: want.r, g: want.g, b: want.b}) {
			t.Errorf("bg = %+v, want dimmed theme bg", d.bg)
		}
		if want != p.base {
			t.Errorf("palette base %+v out of sync with dim(bg) %+v", p.base, want)
		}
		wantFg := dim(p.fg)
		if d.fg != (colorRef{kind: colRGB, r: wantFg.r, g: wantFg.g, b: wantFg.b}) {
			t.Errorf("fg = %+v, want dimmed default fg", d.fg)
		}
	})
	t.Run("explicit bg dims through the overlay", func(t *testing.T) {
		d := dimStyle(style{bg: colorRef{kind: colRGB, r: 0xff, g: 0, b: 0}}, p)
		want := dim(rgb{0xff, 0, 0})
		if d.bg != (colorRef{kind: colRGB, r: want.r, g: want.g, b: want.b}) {
			t.Errorf("bg = %+v, want %+v", d.bg, want)
		}
	})
	t.Run("reverse swaps before dimming", func(t *testing.T) {
		d := dimStyle(style{fg: colorRef{kind: colRGB, r: 0xff, g: 0, b: 0}, reverse: true}, p)
		wantBg := dim(rgb{0xff, 0, 0})
		if d.bg != (colorRef{kind: colRGB, r: wantBg.r, g: wantBg.g, b: wantBg.b}) {
			t.Errorf("reversed bg = %+v, want dimmed red", d.bg)
		}
		wantFg := dim(p.bg) // original default bg becomes the glyph color
		if d.fg != (colorRef{kind: colRGB, r: wantFg.r, g: wantFg.g, b: wantFg.b}) {
			t.Errorf("reversed fg = %+v, want dimmed default bg", d.fg)
		}
	})
	t.Run("bold survives dimming", func(t *testing.T) {
		if d := dimStyle(style{bold: true}, p); !d.bold {
			t.Error("bold dropped")
		}
	})
}

func TestCurrentPaletteAndBorder(t *testing.T) {
	t.Setenv("MIASMA_MODE", "light")
	if currentPalette() != &lightPalette {
		t.Error("light mode not honored")
	}
	t.Setenv("MIASMA_MODE", "")
	if currentPalette() != &darkPalette {
		t.Error("unset mode should be dark")
	}
	t.Setenv("MIASMA_BORDER_FG", "#8a4b3a")
	if got := borderRGB(&darkPalette); got != (rgb{0x8a, 0x4b, 0x3a}) {
		t.Errorf("borderRGB env = %+v", got)
	}
	t.Setenv("MIASMA_BORDER_FG", "nonsense")
	if got := borderRGB(&darkPalette); got != darkPalette.border {
		t.Errorf("borderRGB fallback = %+v", got)
	}
}
