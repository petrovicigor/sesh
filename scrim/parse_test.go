package scrim

import "testing"

func cellRunes(cells []cell) string {
	out := make([]rune, 0, len(cells))
	for _, c := range cells {
		if c.width == 0 {
			continue
		}
		out = append(out, c.r)
	}
	return string(out)
}

func TestParseANSI(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		text  string        // visible runes, continuations skipped
		cols  int           // expected column count (slice length)
		check func(*testing.T, []cell)
	}{
		{
			name: "plain text default style",
			in:   "abc",
			text: "abc",
			cols: 3,
			check: func(t *testing.T, cs []cell) {
				if cs[0].st != (style{}) {
					t.Errorf("style = %+v, want zero", cs[0].st)
				}
			},
		},
		{
			name: "256-color foreground",
			in:   "\x1b[38;5;241mdim",
			text: "dim",
			cols: 3,
			check: func(t *testing.T, cs []cell) {
				want := colorRef{kind: col256, n: 241}
				if cs[0].st.fg != want {
					t.Errorf("fg = %+v, want %+v", cs[0].st.fg, want)
				}
			},
		},
		{
			name: "truecolor bg and basic fg with bold",
			in:   "\x1b[1;31;48;2;10;20;30mX",
			text: "X",
			cols: 1,
			check: func(t *testing.T, cs []cell) {
				st := cs[0].st
				if !st.bold {
					t.Error("bold not set")
				}
				if st.fg != (colorRef{kind: colANSI, n: 1}) {
					t.Errorf("fg = %+v", st.fg)
				}
				if st.bg != (colorRef{kind: colRGB, r: 10, g: 20, b: 30}) {
					t.Errorf("bg = %+v", st.bg)
				}
			},
		},
		{
			name: "bright fg and reset mid-line",
			in:   "\x1b[97ma\x1b[0mb",
			text: "ab",
			cols: 2,
			check: func(t *testing.T, cs []cell) {
				if cs[0].st.fg != (colorRef{kind: colANSI, n: 15}) {
					t.Errorf("bright fg = %+v", cs[0].st.fg)
				}
				if cs[1].st != (style{}) {
					t.Errorf("after reset = %+v", cs[1].st)
				}
			},
		},
		{
			name: "bare ESC[m resets",
			in:   "\x1b[31ma\x1b[mb",
			text: "ab",
			cols: 2,
			check: func(t *testing.T, cs []cell) {
				if cs[1].st != (style{}) {
					t.Errorf("after bare reset = %+v", cs[1].st)
				}
			},
		},
		{
			name: "reverse video tracked",
			in:   "\x1b[7mR\x1b[27mn",
			text: "Rn",
			cols: 2,
			check: func(t *testing.T, cs []cell) {
				if !cs[0].st.reverse || cs[1].st.reverse {
					t.Errorf("reverse = %v,%v", cs[0].st.reverse, cs[1].st.reverse)
				}
			},
		},
		{
			name: "colon subparameter truecolor",
			in:   "\x1b[38:2:1:2:3mQ",
			text: "Q",
			cols: 1,
			check: func(t *testing.T, cs []cell) {
				if cs[0].st.fg != (colorRef{kind: colRGB, r: 1, g: 2, b: 3}) {
					t.Errorf("fg = %+v", cs[0].st.fg)
				}
			},
		},
		{
			name: "OSC hyperlink skipped",
			in:   "\x1b]8;;http://x\x07link\x1b]8;;\x07",
			text: "link",
			cols: 4,
		},
		{
			name: "unknown CSI skipped, text kept",
			in:   "\x1b[2Jab",
			text: "ab",
			cols: 2,
		},
		{
			name: "wide rune occupies two columns",
			in:   "a世b",
			text: "a世b",
			cols: 4,
			check: func(t *testing.T, cs []cell) {
				if cs[1].width != 2 || cs[2].width != 0 {
					t.Errorf("widths = %d,%d, want 2,0", cs[1].width, cs[2].width)
				}
			},
		},
		{
			name: "truncated escape at end of line",
			in:   "ab\x1b[38;5",
			text: "ab",
			cols: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := parseANSI(tt.in)
			if got := cellRunes(cs); got != tt.text {
				t.Fatalf("text = %q, want %q", got, tt.text)
			}
			if len(cs) != tt.cols {
				t.Fatalf("cols = %d, want %d", len(cs), tt.cols)
			}
			for _, c := range cs {
				if !c.covered {
					t.Fatal("parsed cell not marked covered")
				}
			}
			if tt.check != nil {
				tt.check(t, cs)
			}
		})
	}
}
