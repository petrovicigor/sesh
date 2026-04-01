package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var (
	listStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(1)

	previewStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(1)

	statusMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Italic(true)
)

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

	// Two columns: list on left (with built-in title), preview on right
	// Set explicit widths so boxes fill their allocated space exactly.
	// In lipgloss v2, Width() includes borders and padding, so pass box width.
	var columns string
	if m.showPreview {
		listBoxWidth := (m.width * 45) / 100
		previewBoxWidth := m.width - listBoxWidth
		styledListView := listStyle.Width(listBoxWidth).Render(m.list.View())
		previewView := previewStyle.Width(previewBoxWidth).Render(m.previewPort.View())
		columns = lipgloss.JoinHorizontal(lipgloss.Top, styledListView, previewView)
	} else {
		columns = listStyle.Width(m.width).Render(m.list.View())
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
