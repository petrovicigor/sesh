package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
)

// worktreeGroup holds metadata about a group of worktrees from the same repo.
// allItems contains ALL worktree items (both tmux and projects sources).
type worktreeGroup struct {
	repoName      string
	defaultBranch string          // from user defaults ("" = no default)
	allItems      []sessionItem   // ALL worktree items (tmux + projects)
	tmuxNames     map[string]bool // active tmux session names matching this repo's worktrees
}

// groupInfo holds pre-computed data for display building
type groupInfo struct {
	uniqueItems  []sessionItem
	activeCount  int
	dormantCount int
}

// buildWorktreeGroups identifies worktree clusters from a flat session list.
// Scans both tmux and projects sources to detect repos with 2+ unique worktree names.
func buildWorktreeGroups(items []list.Item, defaults map[string]string) map[string]*worktreeGroup {
	groups := make(map[string]*worktreeGroup)

	for _, item := range items {
		si, ok := item.(sessionItem)
		if !ok {
			continue
		}
		if !strings.Contains(si.session.Name, "/") {
			continue
		}
		if si.session.Src != "tmux" && si.session.Src != "projects" {
			continue
		}

		repoName := strings.SplitN(si.session.Name, "/", 2)[0]
		if groups[repoName] == nil {
			groups[repoName] = &worktreeGroup{
				repoName:      repoName,
				defaultBranch: defaults[repoName],
				tmuxNames:     make(map[string]bool),
			}
		}

		if si.session.Src == "tmux" {
			groups[repoName].tmuxNames[si.session.Name] = true
		}
		// Always add to allItems (both tmux and projects)
		groups[repoName].allItems = append(groups[repoName].allItems, si)
	}

	// Second pass: include bare repo items (no "/" but name matches a group's repoName)
	for _, item := range items {
		si, ok := item.(sessionItem)
		if !ok {
			continue
		}
		if strings.Contains(si.session.Name, "/") {
			continue // already handled in first pass
		}
		if si.session.Src != "tmux" && si.session.Src != "projects" {
			continue
		}
		if group, exists := groups[si.session.Name]; exists {
			group.allItems = append(group.allItems, si)
			if si.session.Src == "tmux" {
				group.tmuxNames[si.session.Name] = true
			}
		}
	}

	// Remove groups that don't qualify: must have >= 2 unique worktree names
	for name, group := range groups {
		uniqueNames := make(map[string]bool)
		for _, item := range group.allItems {
			uniqueNames[item.session.Name] = true
		}
		if len(uniqueNames) < 2 {
			delete(groups, name)
		}
	}

	return groups
}

// deduplicateWorktrees returns unique worktree items from a group,
// preferring the tmux version when both tmux and project items exist for the same name.
func deduplicateWorktrees(group *worktreeGroup) []sessionItem {
	seen := make(map[string]int) // name -> index in result
	result := make([]sessionItem, 0, len(group.allItems))

	for _, item := range group.allItems {
		if idx, exists := seen[item.session.Name]; exists {
			// Prefer tmux over projects for same name
			if item.session.Src == "tmux" && result[idx].session.Src != "tmux" {
				result[idx] = item
			}
			continue
		}
		seen[item.session.Name] = len(result)
		result = append(result, item)
	}

	return result
}

// dormantWorktrees returns only the dormant (non-tmux) worktrees from a deduplicated list.
func dormantWorktrees(uniqueItems []sessionItem, tmuxNames map[string]bool) []sessionItem {
	result := make([]sessionItem, 0)
	for _, wt := range uniqueItems {
		if !tmuxNames[wt.session.Name] {
			result = append(result, wt)
		}
	}
	return result
}

// formatGroupDisplay creates the display string for a collapsed worktree group (no active sessions).
// Uses folder icon (green).
func formatGroupDisplay(repoName string, defaultBranch string, extraCount int) string {
	icon := "\033[32m\uf114\033[39m" // green folder
	if defaultBranch != "" {
		name := repoName + " ⎇ " + defaultBranch
		if extraCount > 0 {
			badge := fmt.Sprintf("\033[240m(+%d)\033[39m", extraCount)
			return fmt.Sprintf("%s %s %s", icon, name, badge)
		}
		return fmt.Sprintf("%s %s", icon, name)
	}
	if extraCount > 0 {
		badge := fmt.Sprintf("\033[240m(+%d)\033[39m", extraCount)
		return fmt.Sprintf("%s %s %s", icon, repoName, badge)
	}
	return fmt.Sprintf("%s %s", icon, repoName)
}

// formatDormantBadge creates the ANSI badge string for dormant worktree count.
func formatDormantBadge(dormantCount int) string {
	return fmt.Sprintf(" \033[240m(+%d)\033[39m", dormantCount)
}

// buildDisplayItems creates the list items for display, collapsing worktree groups.
// For groups with active tmux sessions: active worktrees show individually,
// the last active one gets a (+N) badge appended for dormant worktrees.
// For groups with no active sessions: collapsed group item as before.
// expandedGroup is the repo name of the currently expanded group ("" = all collapsed).
func buildDisplayItems(items []list.Item, groups map[string]*worktreeGroup, expandedGroup string) []list.Item {
	// Pre-compute per-group data
	info := make(map[string]*groupInfo)
	for name, group := range groups {
		unique := deduplicateWorktrees(group)
		active := 0
		for _, wt := range unique {
			if group.tmuxNames[wt.session.Name] {
				active++
			}
		}
		info[name] = &groupInfo{
			uniqueItems:  unique,
			activeCount:  active,
			dormantCount: len(unique) - active,
		}
	}

	result := make([]list.Item, 0, len(items))
	insertedGroups := make(map[string]bool)

	for _, item := range items {
		si, ok := item.(sessionItem)
		if !ok {
			result = append(result, item)
			continue
		}

		// Check if this item belongs to any worktree group (tmux OR projects)
		// Items with "/" use repo prefix; bare items match by exact name
		if si.session.Src == "tmux" || si.session.Src == "projects" {
			var repoName string
			if strings.Contains(si.session.Name, "/") {
				repoName = strings.SplitN(si.session.Name, "/", 2)[0]
			} else {
				repoName = si.session.Name
			}
			group, isGrouped := groups[repoName]
			if !isGrouped {
				// Not grouped — show normally
				result = append(result, item)
				continue
			}

			gi := info[repoName]

			if gi.activeCount > 0 {
				// Group has active sessions — insert all active items together at first encounter
				if insertedGroups[repoName] {
					continue // already inserted all items for this group
				}
				insertedGroups[repoName] = true

				// Add all active tmux items together
				for _, wt := range gi.uniqueItems {
					if group.tmuxNames[wt.session.Name] {
						result = append(result, wt)
					}
				}

				// Attach badge to last active item
				lastIdx := len(result) - 1
				if gi.dormantCount > 0 {
					lastItem := result[lastIdx].(sessionItem)
					lastItem.groupBadge = formatDormantBadge(gi.dormantCount)
					lastItem.groupRepo = repoName
					result[lastIdx] = lastItem
				}

				// If expanded, show dormant worktrees below
				if expandedGroup == repoName {
					dormant := dormantWorktrees(gi.uniqueItems, group.tmuxNames)
					for i, wt := range dormant {
						wt.groupChild = true
						wt.groupLastChild = (i == len(dormant)-1)
						result = append(result, wt)
					}
				}
			} else {
				// No active sessions — show collapsed group item
				if insertedGroups[repoName] {
					continue
				}
				insertedGroups[repoName] = true

				badgeCount := len(gi.uniqueItems)
				if group.defaultBranch != "" {
					badgeCount = len(gi.uniqueItems) - 1
				}

				groupItem := worktreeGroupItem{
					repoName:      repoName,
					defaultBranch: group.defaultBranch,
					activeCount:   0,
					dormantCount:  gi.dormantCount,
					totalCount:    len(gi.uniqueItems),
					worktrees:     gi.uniqueItems,
					displayName:   formatGroupDisplay(repoName, group.defaultBranch, badgeCount),
				}
				result = append(result, groupItem)

				// If expanded, show all unique worktrees below
				// Skip the default branch — it's already represented in the group header
				if expandedGroup == repoName {
					children := make([]sessionItem, 0, len(gi.uniqueItems))
					for _, wt := range gi.uniqueItems {
						if group.defaultBranch != "" && strings.Contains(wt.session.Name, "/") {
							branch := strings.SplitN(wt.session.Name, "/", 2)[1]
							if branch == group.defaultBranch {
								continue
							}
						}
						children = append(children, wt)
					}
					for i, wt := range children {
						wt.groupChild = true
						wt.groupLastChild = (i == len(children)-1)
						result = append(result, wt)
					}
				}
				continue
			}
		} else {
			// Non-tmux/non-projects items (e.g. zoxide): skip if they match an
			// already-inserted worktree group to avoid duplicates
			var matchesGroup bool
			if strings.Contains(si.session.Name, "/") {
				repo := strings.SplitN(si.session.Name, "/", 2)[0]
				matchesGroup = insertedGroups[repo]
			} else {
				matchesGroup = insertedGroups[si.session.Name]
			}
			if !matchesGroup {
				result = append(result, item)
			}
		}
	}

	return result
}
