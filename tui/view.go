package tui

import (
	"image/color"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

	statusMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Italic(true)

	hintKeyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#e4c47a"))
	hintDescStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	hintSepStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

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
	var columns string
	if m.showPreview {
		listBoxWidth := (m.width * 45) / 100
		previewBoxWidth := m.width - listBoxWidth
		styledListView := listStyle.Width(listBoxWidth).Render(listInner)
		previewView := previewStyle.Width(previewBoxWidth).Render(m.previewPort.View())
		columns = lipgloss.JoinHorizontal(lipgloss.Top, styledListView, previewView)
	} else {
		columns = listStyle.Width(m.width).Render(listInner)
	}

	// Add status message below columns if present
	if m.restorePreviewMode {
		if m.restoreAllSessions != nil {
			columns += "\n " + statusMsgStyle.Render("restoring all sessions...")
		} else {
			columns += "\n " + statusMsgStyle.Render("Enter restore | Ctrl+A restore all | Esc cancel")
		}
	} else if m.savePreviewMode {
		if m.saveAllSessions != nil {
			columns += "\n " + statusMsgStyle.Render("saving all sessions...")
		} else {
			columns += "\n " + statusMsgStyle.Render("Enter save | Ctrl+A save all | Esc cancel")
		}
	} else if m.statusMessage != "" {
		columns += "\n " + statusMsgStyle.Render(m.statusMessage)
	}

	// Place within a fixed-size frame so every line is padded to full terminal width
	// and empty lines are filled with spaces. Prevents underlying content bleed-through
	// in bubbletea v2's diff-based renderer.
	constrained := lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, columns)

	if viewCount <= 3 {
		logDebug("View() call #%d rendered", viewCount)
	}

	v := tea.NewView(constrained)
	v.AltScreen = true
	v.WindowTitle = "sesh"
	return v
}
