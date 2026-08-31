package tui

import (
	"image/color"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/joshmedeski/sesh/v2/scrim"
)

// miasmaBorder reads MIASMA_BORDER_FG (set by the tmux miasma theme) so popup
// borders match the active pane-border color in both light and dark modes.
func miasmaBorder() color.Color {
	if c := os.Getenv("MIASMA_BORDER_FG"); c != "" {
		return lipgloss.Color(c)
	}
	return lipgloss.Color("#3a3328")
}

var (
	listStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(miasmaBorder()).
			Padding(1)

	previewStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(miasmaBorder()).
			Padding(1)

	// Scrim-mode column boxes: borderless (the raised panel against the
	// dimmed backdrop replaces the border — OpenCode-style), horizontal
	// chrome kept at 4 like the bordered pair so the width arithmetic in
	// applyLayout holds for both; vertical chrome is boxChromeV's business.
	scrimListStyle    = lipgloss.NewStyle().Padding(1, 2)
	scrimPreviewStyle = lipgloss.NewStyle().Padding(1, 2)

	statusMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Italic(true)

	hintKeyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#e4c47a"))
	hintDescStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	hintSepStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// trimTrailingBlankLines drops the padding rows the list appends to fill its
// nominal height, so the content-sized scrim panel ends where content does.
func trimTrailingBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	end := len(lines)
	for end > 1 && strings.TrimSpace(ansi.Strip(lines[end-1])) == "" {
		end--
	}
	return strings.Join(lines[:end], "\n")
}

// renderHint formats key/description pairs as a status hint line.
func renderHint(pairs ...[2]string) string {
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, hintKeyStyle.Render(p[0])+" "+hintDescStyle.Render(p[1]))
	}
	return strings.Join(parts, hintSepStyle.Render("  ·  "))
}

// viewCount tracks render calls for startup timing (reset via resetStartTime → init)
var viewCount int

func (m Model) View() tea.View {
	// Don't render until we know the real terminal size.
	// Avoids a wasted render at wrong default dimensions (80x24).
	if m.width == 0 || m.height == 0 {
		v := tea.NewView("")
		v.AltScreen = true
		return v
	}

	viewCount++
	if viewCount <= 3 {
		logDebug("View() call #%d", viewCount)
	}

	// Build inner list content, optionally with a hint line stitched inside
	// the bordered area so it can't be clipped by lipgloss.Place.
	listInner := m.list.View()
	if m.expandedGroup != nil && *m.expandedGroup != "" {
		// Indent matches list-item prefix ("  " or "❯ ") so hint aligns with item text.
		hintLine := "  " + renderHint(
			[2]string{"ctrl+f", "set default"},
			[2]string{"tab", "collapse"},
		)
		listInner = lipgloss.JoinVertical(lipgloss.Left, listInner, "", hintLine)
	}

	// Two columns: list on left (with built-in title), preview on right
	// Set explicit widths so boxes fill their allocated space exactly.
	// In lipgloss v2, Width() includes borders and padding, so pass box width.
	listBox, previewBox := listStyle, previewStyle
	if m.scrimMode {
		listBox, previewBox = scrimListStyle, scrimPreviewStyle
	}
	var columns string
	if m.showPreview {
		listBoxWidth := (m.width * 45) / 100
		previewBoxWidth := m.width - listBoxWidth
		styledListView := listBox.Width(listBoxWidth).Render(listInner)
		previewView := previewBox.Width(previewBoxWidth).Render(m.previewPort.View())
		columns = lipgloss.JoinHorizontal(lipgloss.Top, styledListView, previewView)
	} else {
		columns = listBox.Width(m.width).Render(listInner)
	}

	// The panel is content-sized in compact scrim mode: the list pads itself
	// to the height it was given, and boxing those trailing blanks into a
	// 75%-tall panel read as a huge empty slab on the scrim. Preview mode
	// keeps the full height (the preview pane wants it), and the classic
	// full-screen path keeps filling the terminal.
	frameH := m.height
	if m.scrimMode && !m.showPreview {
		columns = trimTrailingBlankLines(columns)
		if h := lipgloss.Height(columns); h < frameH {
			frameH = h
		}
	}

	// Place within a fixed-size frame so every line is padded to full width
	// and empty lines are filled with spaces. Prevents underlying content
	// bleed-through in bubbletea v2's diff-based renderer.
	constrained := lipgloss.Place(m.width, frameH, lipgloss.Left, lipgloss.Top, columns)

	// Scrim mode: the panel rect sits centered on a solid scrim backdrop
	// filling the whole client popup.
	content := constrained
	if m.scrimMode {
		content = scrim.Fill(constrained, m.screenW, m.screenH)
	}

	if viewCount <= 3 {
		logDebug("View() call #%d rendered", viewCount)
	}

	v := tea.NewView(content)
	v.AltScreen = true
	v.WindowTitle = "sesh"
	return v
}
