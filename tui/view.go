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

func (m Model) View() string {
	// Title at top
	title := titleStyle.Render(m.list.Title) + "\n"

	// Two columns: list on left, preview on right
	// Don't set explicit widths - let content size determine box size
	listView := listStyle.Render(m.list.View())
	previewView := previewStyle.Render(m.previewPort.View())

	// Place side by side
	columns := lipgloss.JoinHorizontal(lipgloss.Top, listView, previewView)

	// Constrain total width to terminal width
	constrained := constrainedStyle.MaxWidth(m.width).Render(columns)

	return title + constrained
}
