package tui

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Cached styles to avoid per-item allocations during rendering
var (
	selectedItemStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#5f875f"))
	filterDimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	dimRepoStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	badgeStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	separatorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	// Pre-rendered strings (computed once at init, never re-rendered)
	nodeIndicatorStr = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(" ⬢")
	defaultStarStr   = lipgloss.NewStyle().Foreground(lipgloss.Color("#e4c47a")).Render("★")
	treeMidStr       = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("│")
	treeEndStr       = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("└")
	bareRootStr      = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("(bare root)")
	attentionIconStr = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Render("✋") + " "

	// Pre-rendered source badges for filtered view (avoids per-item lipgloss.Render)
	sourceBadges = map[string]string{
		"tmux":       badgeStyle.Render("tmux"),
		"zoxide":     badgeStyle.Render("zoxide"),
		"config":     badgeStyle.Render("config"),
		"projects":   badgeStyle.Render("projects"),
		"tmuxinator": badgeStyle.Render("tmuxinator"),
	}
)

// extractIconPrefix derives the ANSI icon prefix from a pre-computed displayName.
// It tries stripping the transformed name (with ⎇) first, then the raw session name.
func extractIconPrefix(displayName, sessionName string) string {
	if strings.Contains(sessionName, "/") {
		// Try first-slash split (regular projects: "geoip/develop" → "geoip ⎇ develop")
		parts := strings.SplitN(sessionName, "/", 2)
		transformed := parts[0] + " ⎇ " + parts[1]
		if prefix := strings.TrimSuffix(displayName, transformed); prefix != displayName {
			return prefix
		}
		// Try last-slash split (workspace: "mono/packages/box-api/develop" → "mono/packages/box-api ⎇ develop")
		lastSlash := strings.LastIndex(sessionName, "/")
		if lastSlash != strings.Index(sessionName, "/") { // only if multiple slashes
			subProject := sessionName[:lastSlash]
			branch := sessionName[lastSlash+1:]
			transformed2 := subProject + " ⎇ " + branch
			if prefix := strings.TrimSuffix(displayName, transformed2); prefix != displayName {
				return prefix
			}
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
	claudeAttention  *map[string]bool
	savedState       *map[string]bool
}


func (d compactDelegate) Height() int { return 1 } // Single line per item

func (d compactDelegate) Spacing() int { return 0 } // No spacing between items

func (d compactDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d compactDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	var str string
	var nodeIndicator string
	isFiltered := m.FilterState() == list.Filtering && m.FilterValue() != ""
	var matchedRunes []int
	if isFiltered {
		matchedRunes = m.MatchesForItem(index)
	}

	switch v := item.(type) {
	case sessionItem:
		// During active filtering with matches, highlight matched chars in raw session name
		if isFiltered && len(matchedRunes) > 0 {
			// Dim non-matched characters, matched chars stay normal
			textStyle := lipgloss.NewStyle()
			dimStyle := filterDimStyle
			if index == m.Index() {
				textStyle = selectedItemStyle
			}

			// For worktree items (repo/branch), dim the repo prefix and highlight in the branch
			if strings.Contains(v.session.Name, "/") {
				parts := strings.SplitN(v.session.Name, "/", 2)
				repoPrefix := parts[0] + "/"
				branchName := parts[1]
				repoPrefixLen := len(repoPrefix)

				// Split matched indices into repo part and branch part
				var repoIndices, branchIndices []int
				for _, idx := range matchedRunes {
					if idx < repoPrefixLen {
						repoIndices = append(repoIndices, idx)
					} else {
						branchIndices = append(branchIndices, idx-repoPrefixLen)
					}
				}

				// Render repo prefix: matched chars normal, rest dim
				var renderedRepo string
				if len(repoIndices) > 0 {
					renderedRepo = lipgloss.StyleRunes(repoPrefix, repoIndices, textStyle, dimRepoStyle)
				} else {
					renderedRepo = dimRepoStyle.Render(repoPrefix)
				}

				// Render branch name: matched chars normal, rest dim
				var renderedBranch string
				if len(branchIndices) > 0 {
					renderedBranch = lipgloss.StyleRunes(branchName, branchIndices, textStyle, dimStyle)
				} else {
					renderedBranch = dimStyle.Render(branchName)
				}

				str = v.iconPrefix + renderedRepo + renderedBranch
			} else {
				highlighted := lipgloss.StyleRunes(v.session.Name, matchedRunes, textStyle, dimStyle)
				str = v.iconPrefix + highlighted
			}
		} else {
			str = v.displayName
		}

		// Override tmux icon with attention indicator if CC session is awaiting
		if v.session.Src == "tmux" && d.claudeAttention != nil {
			if (*d.claudeAttention)[v.session.Name] {
				// Replace icon prefix: swap blue  with magenta 🖐️
				if v.iconPrefix != "" {
					str = attentionIconStr + strings.TrimPrefix(str, v.iconPrefix)
				}
			}
		}

		// Show restorable indicator for sessions with saved state.
		// Use raw ANSI (not lipgloss Render) to avoid nested style conflicts
		// with selectedItemStyle wrapping in bubbletea v2.
		if d.savedState != nil && (*d.savedState)[v.sanitizedName] {
			str = str + " \033[38;5;245m⟲\033[0m"
		}

		// Add process indicator if available
		if d.processInfo != nil {
			if process, ok := (*d.processInfo)[v.session.Name]; ok && process == "node" {
				nodeIndicator = nodeIndicatorStr
			}
		}

		// Bare repo root — always show as "name (bare root)"
		if v.bareRoot {
			str = v.iconPrefix + v.session.Name + " " + bareRootStr
		}

		// When group is expanded, show short branch names for dormant items
		// and default star indicator for the default branch
		if !v.bareRoot && d.expandedGroup != nil && *d.expandedGroup != "" &&
			strings.Contains(v.session.Name, "/") &&
			strings.HasPrefix(v.session.Name, *d.expandedGroup+"/") {
			repoName := *d.expandedGroup
			branchName := v.session.Name[len(repoName)+1:]
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

		// Append dormant badge inline if this is the last active tmux in a group
		if v.groupBadge != "" {
			str += v.groupBadge
		}

		// Show source badge during active filtering, truncating long names to fit
		if isFiltered && len(matchedRunes) > 0 {
			badge := sourceBadges[v.session.Src]
			if badge == "" {
				badge = badgeStyle.Render(v.session.Src) // fallback for unknown sources
			}
			badgeWidth := lipgloss.Width(badge)
			prefixWidth := 2 // "❯ " or "  "
			maxNameWidth := m.Width() - prefixWidth - badgeWidth - 1
			nameWidth := lipgloss.Width(str)
			if nameWidth > maxNameWidth && maxNameWidth > 3 {
				str = ansi.Truncate(str, maxNameWidth, "…")
				nameWidth = lipgloss.Width(str)
			}
			gap := m.Width() - prefixWidth - nameWidth - badgeWidth
			if gap > 0 {
				str = str + strings.Repeat(" ", gap) + badge
			}
		}

	case worktreeGroupItem:
		str = v.displayName
		// Add restorable indicator if any session in the group has saved state
		if d.savedState != nil {
			for _, wt := range v.worktrees {
				if (*d.savedState)[wt.sanitizedName] {
					str = str + " \033[38;5;245m⟲\033[0m"
					break
				}
			}
		}

	case workspaceToggleItem:
		check := "\033[32m[x]\033[39m" // green checkbox
		if v.excluded {
			check = "\033[240m[ ]\033[39m" // dim unchecked
		}
		str = check + " " + v.workspaceName + "/" + v.subProject

	case separatorItem:
		// Render a dim separator line — not selectable, cursor skips over it
		label := " " + strings.Repeat("─", m.Width()-1)
		fmt.Fprint(w, separatorStyle.Render(label))
		return

	default:
		return
	}

	// Add tree connector for expanded group children
	var treePrefix string
	if si, ok := item.(sessionItem); ok && si.groupChild {
		if si.groupLastChild {
			treePrefix = treeEndStr + " "
		} else {
			treePrefix = treeMidStr + " "
		}
	}

	// Highlight selected item
	if index == m.Index() {
		if isFiltered && len(matchedRunes) > 0 {
			// Already styled per-character above, just add cursor prefix
			str = selectedItemStyle.Render("❯ ") + treePrefix + str + nodeIndicator
		} else {
			str = selectedItemStyle.Render("❯ " + treePrefix + str + nodeIndicator)
		}
	} else {
		str = "  " + treePrefix + str + nodeIndicator
	}

	// Truncate to fit within list width — prevents overflow past border in bubbletea v2.
	// Fast path: if raw byte length fits, visual width must also fit
	// (ANSI escapes have zero visual width but nonzero byte length).
	if len(str) > m.Width() {
		if lineWidth := lipgloss.Width(str); lineWidth > m.Width() {
			str = ansi.Truncate(str, m.Width(), "")
		}
	}

	fmt.Fprint(w, str)
}
