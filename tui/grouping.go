package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
)

// groupKeyForItem returns the worktree group key for a session item.
// For workspace items, the group key is everything before the last "/" (sub-project path).
// For regular projects/tmux, the group key is everything before the first "/" (repo name).
func groupKeyForItem(name, src string, workspacePrefixes []string) string {
	if src == "workspace" || isWorkspaceTmuxSession(name, workspacePrefixes) {
		lastSlash := strings.LastIndex(name, "/")
		if lastSlash > 0 {
			return name[:lastSlash]
		}
	}
	return strings.SplitN(name, "/", 2)[0]
}

// isWorkspaceTmuxSession checks if a tmux session name matches a workspace pattern.
// Workspace sessions have at least 2 slashes: {prefix}/{subproject}/{branch}.
func isWorkspaceTmuxSession(name string, workspacePrefixes []string) bool {
	for _, prefix := range workspacePrefixes {
		if strings.HasPrefix(name, prefix+"/") {
			rest := name[len(prefix)+1:]
			if strings.Contains(rest, "/") {
				return true
			}
		}
	}
	return false
}

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
// Scans tmux, projects, and workspace sources to detect repos with 2+ unique worktree names.
func buildWorktreeGroups(items []list.Item, defaults map[string]string, workspacePrefixes []string) map[string]*worktreeGroup {
	groups := make(map[string]*worktreeGroup)

	for _, item := range items {
		si, ok := item.(sessionItem)
		if !ok {
			continue
		}
		if !strings.Contains(si.session.Name, "/") {
			continue
		}
		if si.session.Src != "tmux" && si.session.Src != "projects" && si.session.Src != "workspace" {
			continue
		}

		repoName := groupKeyForItem(si.session.Name, si.session.Src, workspacePrefixes)
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
		if si.session.Src != "tmux" && si.session.Src != "projects" && si.session.Src != "workspace" {
			continue
		}
		if group, exists := groups[si.session.Name]; exists {
			group.allItems = append(group.allItems, si)
			if si.session.Src == "tmux" {
				group.tmuxNames[si.session.Name] = true
			}
		}
	}

	// Remove groups that don't qualify: must have >= 2 unique worktree names.
	// Exception: workspace groups always qualify (even with 1 worktree) so they
	// display in the same "repo ⎇ branch" format as multi-worktree projects.
	for name, group := range groups {
		uniqueNames := make(map[string]bool)
		hasWorkspace := false
		for _, item := range group.allItems {
			uniqueNames[item.session.Name] = true
			if item.session.Src == "workspace" || isWorkspaceTmuxSession(item.session.Name, workspacePrefixes) {
				hasWorkspace = true
			}
		}
		if len(uniqueNames) < 2 && !hasWorkspace {
			delete(groups, name)
			continue
		}
		// For workspace groups with a single worktree, auto-set default branch
		// so the display shows "repo ⎇ branch" instead of "repo (+)".
		if hasWorkspace && group.defaultBranch == "" && len(uniqueNames) == 1 {
			for n := range uniqueNames {
				lastSlash := strings.LastIndex(n, "/")
				if lastSlash > 0 {
					group.defaultBranch = n[lastSlash+1:]
				}
			}
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

// representativeSession returns the best session to use for previewing a collapsed group.
// Prefers the default branch worktree; falls back to the first worktree.
func representativeSession(group worktreeGroupItem) (sessionItem, bool) {
	if len(group.worktrees) == 0 {
		return sessionItem{}, false
	}
	if group.defaultBranch != "" {
		target := group.repoName + "/" + group.defaultBranch
		for _, wt := range group.worktrees {
			if wt.session.Name == target {
				return wt, true
			}
		}
	}
	return group.worktrees[0], true
}

// formatGroupDisplay creates the display string for a collapsed worktree group (no active sessions).
// Uses folder icon (green) for projects, 📦 icon (magenta) for workspace groups.
func formatGroupDisplay(repoName string, defaultBranch string, extraCount int, isWorkspace bool) string {
	icon := "\033[32m\uf114\033[39m" // green folder
	if isWorkspace {
		icon = "\033[35m📦\033[39m" // magenta workspace
	}
	badge := "\033[240m(+)\033[39m"
	if defaultBranch != "" {
		name := repoName + " ⎇ " + defaultBranch
		if extraCount > 0 {
			return fmt.Sprintf("%s %s %s", icon, name, badge)
		}
		return fmt.Sprintf("%s %s", icon, name)
	}
	if extraCount > 0 {
		return fmt.Sprintf("%s %s %s", icon, repoName, badge)
	}
	return fmt.Sprintf("%s %s", icon, repoName)
}

// formatDormantBadge creates the ANSI badge string for dormant worktrees.
func formatDormantBadge() string {
	return " \033[240m(+)\033[39m"
}

// sortBareRootFirst reorders children so the bare repo root (name without "/")
// appears first in the expanded list.
func sortBareRootFirst(children []sessionItem, repoName string) []sessionItem {
	for i, wt := range children {
		if !strings.Contains(wt.session.Name, "/") && wt.session.Name == repoName {
			if i == 0 {
				return children
			}
			// Move bare root to front
			result := make([]sessionItem, 0, len(children))
			result = append(result, wt)
			result = append(result, children[:i]...)
			result = append(result, children[i+1:]...)
			return result
		}
	}
	return children
}

// buildDisplayItems creates the list items for display, collapsing worktree groups.
// For groups with active tmux sessions: active worktrees show individually,
// the last active one gets a (+N) badge appended for dormant worktrees.
// For groups with no active sessions: collapsed group item as before.
// expandedGroup is the repo name of the currently expanded group ("" = all collapsed).
func buildDisplayItems(items []list.Item, groups map[string]*worktreeGroup, expandedGroup string, workspacePrefixes []string) []list.Item {
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

		// Check if this item belongs to any worktree group (tmux, projects, or workspace)
		// Items with "/" use repo prefix; bare items match by exact name
		if si.session.Src == "tmux" || si.session.Src == "projects" || si.session.Src == "workspace" {
			var repoName string
			if strings.Contains(si.session.Name, "/") {
				repoName = groupKeyForItem(si.session.Name, si.session.Src, workspacePrefixes)
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
						if !strings.Contains(wt.session.Name, "/") && wt.session.Name == repoName {
							wt.bareRoot = true
						}
						// Reformat workspace tmux sessions to use ⎇ branch format.
						// icon.go can't detect these as worktrees (sub-project path has no .git),
						// so we reformat here using the group key as the repo prefix.
						if strings.HasPrefix(wt.session.Name, repoName+"/") && len(wt.session.Name) > len(repoName)+1 {
							branch := wt.session.Name[len(repoName)+1:]
							if !strings.Contains(wt.displayName, "⎇") {
								wt.displayName = wt.iconPrefix + repoName + " ⎇ " + branch
							}
						}
						result = append(result, wt)
					}
				}

				// Attach badge to last active item
				lastIdx := len(result) - 1
				if gi.dormantCount > 0 {
					lastItem := result[lastIdx].(sessionItem)
					lastItem.groupBadge = formatDormantBadge()
					lastItem.groupRepo = repoName
					result[lastIdx] = lastItem
				}

				// If expanded, show all worktrees below (including active ones for consistency)
				if expandedGroup == repoName {
					children := sortBareRootFirst(gi.uniqueItems, repoName)
					for i, wt := range children {
						wt.groupChild = true
						wt.groupLastChild = (i == len(children)-1)
						if !strings.Contains(wt.session.Name, "/") && wt.session.Name == repoName {
							wt.bareRoot = true
						}
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

				// Check if this is a workspace group for icon selection
				groupIsWorkspace := false
				for _, wt := range gi.uniqueItems {
					if wt.session.Src == "workspace" || isWorkspaceTmuxSession(wt.session.Name, workspacePrefixes) {
						groupIsWorkspace = true
						break
					}
				}

				groupItem := worktreeGroupItem{
					repoName:      repoName,
					defaultBranch: group.defaultBranch,
					activeCount:   0,
					dormantCount:  gi.dormantCount,
					totalCount:    len(gi.uniqueItems),
					worktrees:     gi.uniqueItems,
					displayName:   formatGroupDisplay(repoName, group.defaultBranch, badgeCount, groupIsWorkspace),
				}
				result = append(result, groupItem)

				// If expanded, show all unique worktrees below (including default branch)
				if expandedGroup == repoName {
					children := make([]sessionItem, 0, len(gi.uniqueItems))
					for _, wt := range gi.uniqueItems {
						children = append(children, wt)
					}
					children = sortBareRootFirst(children, repoName)
					for i, wt := range children {
						wt.groupChild = true
						if !strings.Contains(wt.session.Name, "/") && wt.session.Name == repoName {
							wt.bareRoot = true
						}
						wt.groupLastChild = (i == len(children)-1)
						result = append(result, wt)
					}
				}
				continue
			}
		} else {
			// Non-tmux/non-projects/non-workspace items (e.g. zoxide): skip if they match an
			// already-inserted worktree group to avoid duplicates
			var matchesGroup bool
			if strings.Contains(si.session.Name, "/") {
				repo := groupKeyForItem(si.session.Name, si.session.Src, workspacePrefixes)
				matchesGroup = insertedGroups[repo]
			} else {
				matchesGroup = insertedGroups[si.session.Name]
			}
			if !matchesGroup {
				result = append(result, item)
			}
		}
	}

	// Insert separator between last active-tmux item and first non-tmux item.
	// "Active tmux" includes: tmux sessions, expanded children of active groups.
	lastTmuxIdx := -1
	firstNonTmuxIdx := -1
	for i, item := range result {
		switch v := item.(type) {
		case sessionItem:
			if v.session.Src == "tmux" {
				lastTmuxIdx = i
			} else if v.groupChild && lastTmuxIdx >= 0 && firstNonTmuxIdx == -1 {
				// Expanded children of active tmux groups stay with the tmux section,
				// but only if we haven't passed into the non-tmux section yet.
				lastTmuxIdx = i
			} else if firstNonTmuxIdx == -1 && lastTmuxIdx >= 0 {
				firstNonTmuxIdx = i
			}
		case worktreeGroupItem:
			if firstNonTmuxIdx == -1 && lastTmuxIdx >= 0 {
				firstNonTmuxIdx = i
			}
		}
	}

	if lastTmuxIdx >= 0 && firstNonTmuxIdx > lastTmuxIdx {
		newResult := make([]list.Item, 0, len(result)+1)
		newResult = append(newResult, result[:firstNonTmuxIdx]...)
		newResult = append(newResult, separatorItem{})
		newResult = append(newResult, result[firstNonTmuxIdx:]...)
		result = newResult
	}

	return result
}
