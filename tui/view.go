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
)

func (m Model) View() string {
	// Title at top
	title := titleStyle.Render(m.list.Title) + "\n"

	// Two columns: list on left, preview on right
	listView := listStyle.Render(m.list.View())
	previewView := previewStyle.Render(m.previewPort.View())

	// Place side by side
	columns := lipgloss.JoinHorizontal(lipgloss.Top, listView, previewView)

	return title + columns
}
