package tui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170")).
			MarginBottom(1)

	listStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(1)

	previewStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(1)

	constrainedStyle = lipgloss.NewStyle()
)

// viewCount tracks render calls for startup timing (reset via resetStartTime → init)
var viewCount int

func (m Model) View() string {
	// Don't render until we know the real terminal size.
	// Avoids a wasted render at wrong default dimensions (80x24).
	if m.width == 0 || m.height == 0 {
		return ""
	}

	viewCount++
	if viewCount <= 3 {
		logDebug("View() call #%d", viewCount)
	}

	// Two columns: list on left (with built-in title), preview on right
	listView := m.list.View()
	styledListView := listStyle.Render(listView)
	previewView := previewStyle.Render(m.previewPort.View())

	// Place side by side
	columns := lipgloss.JoinHorizontal(lipgloss.Top, styledListView, previewView)

	// Constrain total width to terminal width
	constrained := constrainedStyle.MaxWidth(m.width).Render(columns)

	if viewCount <= 3 {
		logDebug("View() call #%d rendered", viewCount)
	}

	return constrained
}
