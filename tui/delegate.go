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
	processInfo *map[string][]string // Pointer to model's processInfo map
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

	// Add process indicators on the right if detected (lookup from model's map)
	var processIcon string
	if d.processInfo != nil {
		if processes, ok := (*d.processInfo)[sessionItem.session.Name]; ok {
			// Map process names to Nerd Font icons and colors
			processIcons := map[string]struct {
				Icon  string
				Color string
			}{
				"node": {Icon: "\ue718", Color: "34"},  // green
				"npm":  {Icon: "\ue71e", Color: "196"}, // red
				"yarn": {Icon: "\ue6a7", Color: "39"},  // blue
			}

			// Render all detected processes
			for _, proc := range processes {
				if iconData, exists := processIcons[proc]; exists {
					processIcon += fmt.Sprintf(" \033[38;5;%sm%s\033[0m", iconData.Color, iconData.Icon)
				}
			}
		}
	}

	// Highlight selected item
	if index == m.Index() {
		str = lipgloss.NewStyle().
			Foreground(lipgloss.Color("170")).
			Render("❯ " + str + processIcon)
	} else {
		str = "  " + str + processIcon
	}

	fmt.Fprint(w, str)
}
