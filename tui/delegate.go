package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Cached styles to avoid per-item allocations during rendering
var (
	nodeIndicatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	selectedItemStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("170"))
	defaultStarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	nodeIndicatorStr = nodeIndicatorStyle.Render(" ⬢")  // Pre-rendered
	filterMatchStyle = lipgloss.NewStyle().Underline(true)
	defaultStarStr   = defaultStarStyle.Render("★")      // Pre-rendered gold star
)

// extractIconPrefix derives the ANSI icon prefix from a pre-computed displayName.
// It tries stripping the transformed name (with ⎇) first, then the raw session name.
func extractIconPrefix(displayName, sessionName string) string {
	if strings.Contains(sessionName, "/") {
		parts := strings.SplitN(sessionName, "/", 2)
		transformed := parts[0] + " ⎇ " + parts[1]
		if prefix := strings.TrimSuffix(displayName, transformed); prefix != displayName {
			return prefix
		}
	}
	if prefix := strings.TrimSuffix(displayName, sessionName); prefix != displayName {
		return prefix
	}
	return ""
}

// compactDelegate is a custom delegate with minimal spacing
type compactDelegate struct {
	processInfo      *map[string]string
	expandedGroup    *string
	worktreeDefaults *map[string]string
}


func (d compactDelegate) Height() int { return 1 } // Single line per item

func (d compactDelegate) Spacing() int { return 0 } // No spacing between items

func (d compactDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d compactDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	var str string
	var nodeIndicator string

	switch v := item.(type) {
	case sessionItem:
		// During active filtering with matches, highlight matched chars in raw session name
		isFiltered := m.FilterState() == list.Filtering && m.FilterValue() != ""
		matchedRunes := m.MatchesForItem(index)
		if isFiltered && len(matchedRunes) > 0 {
			highlighted := lipgloss.StyleRunes(v.session.Name, matchedRunes, filterMatchStyle, lipgloss.NewStyle())
			str = v.iconPrefix + highlighted
		} else {
			str = v.displayName
		}

		// Add process indicator if available
		if d.processInfo != nil {
			if process, ok := (*d.processInfo)[v.session.Name]; ok && process == "node" {
				nodeIndicator = nodeIndicatorStr
			}
		}

		// When group is expanded, show short branch names for dormant items
		// and default star indicator for the default branch
		if d.expandedGroup != nil && *d.expandedGroup != "" &&
			strings.Contains(v.session.Name, "/") {
			repoName := strings.SplitN(v.session.Name, "/", 2)[0]
			if repoName == *d.expandedGroup && v.groupRepo == "" {
				branchName := strings.SplitN(v.session.Name, "/", 2)[1]
				isDefault := false
				if d.worktreeDefaults != nil {
					if defaultBranch, ok := (*d.worktreeDefaults)[repoName]; ok && defaultBranch == branchName {
						isDefault = true
					}
				}

				// Dormant items (not active tmux): show short branch name only
				if v.session.Src != "tmux" {
					if isDefault {
						str = v.iconPrefix + defaultStarStr + " ⎇ " + branchName
					} else {
						str = v.iconPrefix + "⎇ " + branchName
					}
				} else if isDefault {
					// Active tmux: keep full name but add star
					namePart := strings.TrimPrefix(str, v.iconPrefix)
					str = v.iconPrefix + defaultStarStr + " " + namePart
				}
			}
		}

		// Append dormant badge inline if this is the last active tmux in a group
		if v.groupBadge != "" {
			str += v.groupBadge
		}

	case worktreeGroupItem:
		str = v.displayName

	default:
		return
	}

	// Highlight selected item
	if index == m.Index() {
		str = selectedItemStyle.Render("❯ " + str + nodeIndicator)
	} else {
		str = "  " + str + nodeIndicator
	}

	fmt.Fprint(w, str)
}
