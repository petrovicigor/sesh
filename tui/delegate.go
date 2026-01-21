package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// compactDelegate is a custom delegate with minimal spacing
type compactDelegate struct {
	processInfo *map[string]string
}

func (d compactDelegate) Height() int { return 1 } // Single line per item

func (d compactDelegate) Spacing() int { return 0 } // No spacing between items

func (d compactDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d compactDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	sessionItem, ok := item.(sessionItem)
	if !ok {
		return
	}

	str := sessionItem.displayName

	// Add process indicator if available
	nodeIndicator := ""
	if d.processInfo != nil {
		if process, ok := (*d.processInfo)[sessionItem.session.Name]; ok && process == "node" {
			// Green Node.js hexagon after the name (ANSI color 2 = green)
			nodeIndicator = lipgloss.NewStyle().
				Foreground(lipgloss.Color("2")).
				Render(" ⬢")
		}
	}

	// Highlight selected item
	if index == m.Index() {
		str = lipgloss.NewStyle().
			Foreground(lipgloss.Color("170")).
			Render("❯ " + str + nodeIndicator)
	} else {
		str = "  " + str + nodeIndicator
	}

	fmt.Fprint(w, str)
}
