package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/joshmedeski/sesh/v2/model"
)

func TestBuildWorktreeGroups(t *testing.T) {
	t.Run("identifies worktree groups from projects", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "chase-monorepo", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "chase-monorepo/develop", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "chase-monorepo/feature-cdk", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "chase-monorepo/review", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "other-project", Src: "projects"}},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))

		if len(groups) != 1 {
			t.Fatalf("expected 1 group, got %d", len(groups))
		}
		group, ok := groups["chase-monorepo"]
		if !ok {
			t.Fatal("expected group 'chase-monorepo'")
		}
		if len(group.allItems) != 4 {
			t.Errorf("expected 4 allItems (3 with / + 1 bare tmux), got %d", len(group.allItems))
		}
	})

	t.Run("tracks active tmux sessions", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "geoip/develop", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "geoip/develop", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "geoip/feature-x", Src: "projects"}},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))

		group := groups["geoip"]
		if group == nil {
			t.Fatal("expected group 'geoip'")
		}
		if len(group.tmuxNames) != 1 {
			t.Errorf("expected 1 active tmux session, got %d", len(group.tmuxNames))
		}
		if !group.tmuxNames["geoip/develop"] {
			t.Error("expected geoip/develop to be active")
		}
		if len(group.allItems) != 3 {
			t.Errorf("expected 3 allItems (1 tmux + 2 projects), got %d", len(group.allItems))
		}
	})

	t.Run("no groups when no worktrees", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "sesh", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "dotfiles", Src: "config"}},
			sessionItem{session: model.SeshSession{Name: "myproject", Src: "projects"}},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))

		if len(groups) != 0 {
			t.Errorf("expected 0 groups, got %d", len(groups))
		}
	})

	t.Run("multiple repos each get their own group", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo-a/main", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "repo-a/develop", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "repo-b/main", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "repo-b/feature", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "repo-b/hotfix", Src: "projects"}},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))

		if len(groups) != 2 {
			t.Fatalf("expected 2 groups, got %d", len(groups))
		}
		if len(groups["repo-a"].allItems) != 2 {
			t.Errorf("repo-a: expected 2 allItems, got %d", len(groups["repo-a"].allItems))
		}
		if len(groups["repo-b"].allItems) != 3 {
			t.Errorf("repo-b: expected 3 allItems, got %d", len(groups["repo-b"].allItems))
		}
	})

	t.Run("ignores non-tmux-projects worktree-like names", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "geoip/develop", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "igorpetrovic/dotfiles", Src: "config"}},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))

		if len(groups) != 0 {
			t.Errorf("expected 0 groups (needs >= 2 unique names), got %d", len(groups))
		}
	})

	t.Run("tmux worktrees count toward grouping threshold", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "chase-monorepo/feature-cdk", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "chase-monorepo/develop", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "chase-monorepo/review", Src: "projects"}},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))

		if len(groups) != 1 {
			t.Fatalf("expected 1 group, got %d", len(groups))
		}
		group := groups["chase-monorepo"]
		if group == nil {
			t.Fatal("expected group 'chase-monorepo'")
		}
		if len(group.allItems) != 3 {
			t.Errorf("expected 3 allItems (2 tmux + 1 projects), got %d", len(group.allItems))
		}
		if len(group.tmuxNames) != 2 {
			t.Errorf("expected 2 tmux names, got %d", len(group.tmuxNames))
		}
	})

	t.Run("groups tmux-only worktrees when 2+ exist", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "tmux"}},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))

		if len(groups) != 1 {
			t.Fatalf("expected 1 group, got %d", len(groups))
		}
		group := groups["repo"]
		if group == nil {
			t.Fatal("expected group 'repo'")
		}
		if len(group.allItems) != 2 {
			t.Errorf("expected 2 allItems, got %d", len(group.allItems))
		}
		if len(group.tmuxNames) != 2 {
			t.Errorf("expected 2 tmux names, got %d", len(group.tmuxNames))
		}
	})

	t.Run("populates default branch from defaults map", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "projects"}},
		}

		defaults := map[string]string{"repo": "main"}
		groups := buildWorktreeGroups(items, defaults)

		if groups["repo"].defaultBranch != "main" {
			t.Errorf("expected defaultBranch 'main', got %q", groups["repo"].defaultBranch)
		}
	})

	t.Run("single worktree does not create group", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))

		if len(groups) != 0 {
			t.Errorf("expected 0 groups (single worktree = no group), got %d", len(groups))
		}
	})
}

func TestDeduplicateWorktrees(t *testing.T) {
	t.Run("prefers tmux over projects for same name", func(t *testing.T) {
		group := &worktreeGroup{
			repoName:  "repo",
			tmuxNames: map[string]bool{"repo/main": true},
			allItems: []sessionItem{
				{session: model.SeshSession{Name: "repo/main", Src: "tmux"}},
				{session: model.SeshSession{Name: "repo/main", Src: "projects"}},
				{session: model.SeshSession{Name: "repo/develop", Src: "projects"}},
			},
		}

		result := deduplicateWorktrees(group)

		if len(result) != 2 {
			t.Fatalf("expected 2 unique items, got %d", len(result))
		}
		if result[0].session.Name != "repo/main" || result[0].session.Src != "tmux" {
			t.Errorf("expected tmux repo/main first, got %s/%s", result[0].session.Name, result[0].session.Src)
		}
		if result[1].session.Name != "repo/develop" || result[1].session.Src != "projects" {
			t.Errorf("expected projects repo/develop second, got %s/%s", result[1].session.Name, result[1].session.Src)
		}
	})

	t.Run("preserves unique items from both sources", func(t *testing.T) {
		group := &worktreeGroup{
			repoName:  "repo",
			tmuxNames: map[string]bool{"repo/main": true},
			allItems: []sessionItem{
				{session: model.SeshSession{Name: "repo/main", Src: "tmux"}},
				{session: model.SeshSession{Name: "repo/develop", Src: "projects"}},
				{session: model.SeshSession{Name: "repo/feature", Src: "projects"}},
			},
		}

		result := deduplicateWorktrees(group)

		if len(result) != 3 {
			t.Fatalf("expected 3 unique items, got %d", len(result))
		}
	})

	t.Run("projects item comes first when no tmux duplicate", func(t *testing.T) {
		group := &worktreeGroup{
			repoName:  "repo",
			tmuxNames: map[string]bool{},
			allItems: []sessionItem{
				{session: model.SeshSession{Name: "repo/main", Src: "projects"}},
				{session: model.SeshSession{Name: "repo/develop", Src: "projects"}},
			},
		}

		result := deduplicateWorktrees(group)

		if len(result) != 2 {
			t.Fatalf("expected 2 items, got %d", len(result))
		}
		if result[0].session.Src != "projects" {
			t.Errorf("expected projects source, got %s", result[0].session.Src)
		}
	})

	t.Run("replaces projects with tmux when tmux appears after", func(t *testing.T) {
		group := &worktreeGroup{
			repoName:  "repo",
			tmuxNames: map[string]bool{"repo/main": true},
			allItems: []sessionItem{
				{session: model.SeshSession{Name: "repo/main", Src: "projects"}},
				{session: model.SeshSession{Name: "repo/main", Src: "tmux"}},
			},
		}

		result := deduplicateWorktrees(group)

		if len(result) != 1 {
			t.Fatalf("expected 1 unique item, got %d", len(result))
		}
		if result[0].session.Src != "tmux" {
			t.Errorf("expected tmux source (preferred), got %s", result[0].session.Src)
		}
	})
}

func TestFormatGroupDisplay(t *testing.T) {
	t.Run("format with default branch", func(t *testing.T) {
		got := formatGroupDisplay("chase-monorepo", "develop", 3)
		if !strings.Contains(got, "chase-monorepo ⎇ develop") {
			t.Errorf("expected 'chase-monorepo ⎇ develop' in display, got %q", got)
		}
		if !strings.Contains(got, "(+3)") {
			t.Errorf("expected (+3) badge, got %q", got)
		}
	})

	t.Run("format without default branch", func(t *testing.T) {
		got := formatGroupDisplay("chase-monorepo", "", 4)
		if !strings.Contains(got, "chase-monorepo") {
			t.Errorf("expected repo name in display, got %q", got)
		}
		if !strings.Contains(got, "(+4)") {
			t.Errorf("expected (+4) badge, got %q", got)
		}
		if strings.Contains(got, "⎇") {
			t.Errorf("should not contain ⎇ when no default, got %q", got)
		}
	})

	t.Run("no badge when extra count is zero", func(t *testing.T) {
		got := formatGroupDisplay("repo", "main", 0)
		if strings.Contains(got, "(+") {
			t.Errorf("should not contain (+N) when count is 0, got %q", got)
		}
	})
}

func TestFormatDormantBadge(t *testing.T) {
	t.Run("shows count", func(t *testing.T) {
		got := formatDormantBadge(3)
		if !strings.Contains(got, "(+3)") {
			t.Errorf("expected (+3) in badge, got %q", got)
		}
	})
}

func TestBuildDisplayItems(t *testing.T) {
	t.Run("collapses dormant-only worktree groups", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "sesh", Src: "tmux"}, displayName: " sesh"},
			sessionItem{session: model.SeshSession{Name: "chase-monorepo/develop", Src: "projects"}, displayName: " chase-monorepo ⎇ develop"},
			sessionItem{session: model.SeshSession{Name: "chase-monorepo/feature-cdk", Src: "projects"}, displayName: " chase-monorepo ⎇ feature-cdk"},
			sessionItem{session: model.SeshSession{Name: "chase-monorepo/review", Src: "projects"}, displayName: " chase-monorepo ⎇ review"},
			sessionItem{session: model.SeshSession{Name: "other-project", Src: "projects"}, displayName: " other-project"},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))
		display := buildDisplayItems(items, groups, "")

		if len(display) != 3 {
			t.Fatalf("expected 3 items, got %d", len(display))
		}
		if si, ok := display[0].(sessionItem); !ok || si.session.Name != "sesh" {
			t.Errorf("expected first item to be 'sesh' sessionItem")
		}
		gi, ok := display[1].(worktreeGroupItem)
		if !ok {
			t.Fatalf("expected second item to be worktreeGroupItem, got %T", display[1])
		}
		if gi.repoName != "chase-monorepo" {
			t.Errorf("expected repoName 'chase-monorepo', got %q", gi.repoName)
		}
		if gi.totalCount != 3 {
			t.Errorf("expected totalCount 3, got %d", gi.totalCount)
		}
		if si, ok := display[2].(sessionItem); !ok || si.session.Name != "other-project" {
			t.Errorf("expected third item to be 'other-project' sessionItem")
		}
	})

	t.Run("active worktrees shown individually with badge on last", func(t *testing.T) {
		// 2 active tmux + 1 dormant project
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "tmux"}, displayName: " repo/main"},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "tmux"}, displayName: " repo/develop"},
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}, displayName: " repo ⎇ main"},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "projects"}, displayName: " repo ⎇ develop"},
			sessionItem{session: model.SeshSession{Name: "repo/feature", Src: "projects"}, displayName: " repo ⎇ feature"},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))
		display := buildDisplayItems(items, groups, "")

		// repo/main + repo/develop(+1 badge) = 2 items
		if len(display) != 2 {
			names := describeItems(display)
			t.Fatalf("expected 2 items, got %d: %v", len(display), names)
		}

		// First should be plain tmux (no badge)
		si0 := display[0].(sessionItem)
		if si0.groupBadge != "" {
			t.Error("first item should not have badge")
		}

		// Second should be tmux with badge
		si1 := display[1].(sessionItem)
		if si1.session.Name != "repo/develop" || si1.session.Src != "tmux" {
			t.Errorf("expected tmux repo/develop, got %s/%s", si1.session.Name, si1.session.Src)
		}
		if si1.groupBadge == "" {
			t.Error("last active item should have dormant badge")
		}
		if si1.groupRepo != "repo" {
			t.Errorf("expected groupRepo 'repo', got %q", si1.groupRepo)
		}
		if !strings.Contains(si1.groupBadge, "(+1)") {
			t.Errorf("expected (+1) in badge, got %q", si1.groupBadge)
		}
	})

	t.Run("all active worktrees shown individually with no badge", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "projects"}},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))
		display := buildDisplayItems(items, groups, "")

		// 2 active tmux items, no dormant = no badge
		if len(display) != 2 {
			names := describeItems(display)
			t.Fatalf("expected 2 items (tmux only, no badge), got %d: %v", len(display), names)
		}
		for _, item := range display {
			si := item.(sessionItem)
			if si.session.Src != "tmux" {
				t.Errorf("expected tmux source, got %q", si.session.Src)
			}
			if si.groupBadge != "" {
				t.Error("no dormant worktrees, should not have badge")
			}
		}
	})

	t.Run("absorbs project worktrees from active group", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}, displayName: " repo ⎇ main"},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "projects"}, displayName: " repo ⎇ develop"},
			sessionItem{session: model.SeshSession{Name: "repo/feature", Src: "projects"}, displayName: " repo ⎇ feature"},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))
		display := buildDisplayItems(items, groups, "")

		// repo/main (tmux, with badge +2) = 1 item
		if len(display) != 1 {
			names := describeItems(display)
			t.Fatalf("expected 1 item, got %d: %v", len(display), names)
		}

		si := display[0].(sessionItem)
		if si.session.Src != "tmux" {
			t.Error("expected tmux sessionItem")
		}
		if !strings.Contains(si.groupBadge, "(+2)") {
			t.Errorf("expected (+2) badge, got %q", si.groupBadge)
		}
	})

	t.Run("expanded badge shows dormant worktrees below badged item", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "tmux"}, displayName: " repo/main"},
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}, displayName: " repo ⎇ main"},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "projects"}, displayName: " repo ⎇ develop"},
			sessionItem{session: model.SeshSession{Name: "repo/feature", Src: "projects"}, displayName: " repo ⎇ feature"},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))
		display := buildDisplayItems(items, groups, "repo")

		// repo/main (tmux, badged) + repo/develop + repo/feature = 3
		if len(display) != 3 {
			names := describeItems(display)
			t.Fatalf("expected 3 items, got %d: %v", len(display), names)
		}

		// First is badged tmux item
		si0 := display[0].(sessionItem)
		if si0.session.Src != "tmux" || si0.groupRepo == "" {
			t.Error("expected badged tmux sessionItem first")
		}

		// Expanded dormant items follow (not including repo/main which is active)
		for _, item := range display[1:] {
			si := item.(sessionItem)
			if si.session.Name == "repo/main" {
				t.Error("repo/main should not appear in expanded dormant list (it's active)")
			}
		}
	})

	t.Run("counts reflect active sessions", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "repo/feature", Src: "projects"}},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))
		display := buildDisplayItems(items, groups, "")

		// The badged item should have (+2) for 2 dormant
		si := display[0].(sessionItem)
		if !strings.Contains(si.groupBadge, "(+2)") {
			t.Errorf("expected (+2) badge, got %q", si.groupBadge)
		}
	})
}

func TestBuildDisplayItemsEdgeCases(t *testing.T) {
	t.Run("single worktree shows normally (no group)", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}, displayName: " repo ⎇ main"},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))
		display := buildDisplayItems(items, groups, "")

		if len(display) != 1 {
			t.Fatalf("expected 1 item, got %d", len(display))
		}
		if _, ok := display[0].(sessionItem); !ok {
			t.Error("single worktree should show as sessionItem, not group")
		}
	})

	t.Run("non-worktree projects pass through unchanged", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "myproject", Src: "projects"}, displayName: " myproject"},
			sessionItem{session: model.SeshSession{Name: "another", Src: "config"}, displayName: " another"},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))
		display := buildDisplayItems(items, groups, "")

		if len(display) != 2 {
			t.Fatalf("expected 2 items, got %d", len(display))
		}
	})

	t.Run("mixed sources preserved around groups", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "sesh", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "dotfiles", Src: "config"}},
			sessionItem{session: model.SeshSession{Name: "repo/a", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "repo/b", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "mydir", Src: "zoxide"}},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))
		display := buildDisplayItems(items, groups, "")

		// sesh, dotfiles, repo group, mydir = 4 items
		if len(display) != 4 {
			t.Fatalf("expected 4 items, got %d", len(display))
		}

		// Verify order
		names := []string{}
		for _, item := range display {
			switch v := item.(type) {
			case sessionItem:
				names = append(names, v.session.Name)
			case worktreeGroupItem:
				names = append(names, v.repoName+"[group]")
			}
		}

		expected := []string{"sesh", "dotfiles", "repo[group]", "mydir"}
		for i, name := range names {
			if name != expected[i] {
				t.Errorf("position %d: expected %q, got %q", i, expected[i], name)
			}
		}
	})

	t.Run("tmux-only worktrees shown individually with no badge", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "sesh", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "tmux"}},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))
		display := buildDisplayItems(items, groups, "")

		// sesh + repo/main + repo/develop = 3 (all active, no badge needed)
		if len(display) != 3 {
			names := describeItems(display)
			t.Fatalf("expected 3 items, got %d: %v", len(display), names)
		}
		for _, item := range display {
			si := item.(sessionItem)
			if si.groupBadge != "" {
				t.Errorf("all active, should not have badge, got %q", si.groupBadge)
			}
		}
	})

	t.Run("non-adjacent tmux items grouped together at first position", func(t *testing.T) {
		// Tmux items from same group with another tmux item between them
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "tmux"}, displayName: " repo/main"},
			sessionItem{session: model.SeshSession{Name: "sesh", Src: "tmux"}, displayName: " sesh"},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "tmux"}, displayName: " repo/develop"},
			sessionItem{session: model.SeshSession{Name: "repo/feature", Src: "projects"}, displayName: " repo ⎇ feature"},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))
		display := buildDisplayItems(items, groups, "")

		// repo/main, repo/develop(+1), sesh = 3 (group items clustered at first encounter)
		if len(display) != 3 {
			names := describeItems(display)
			t.Fatalf("expected 3 items, got %d: %v", len(display), names)
		}

		// Both repo items grouped together starting at position 0
		si0 := display[0].(sessionItem)
		if si0.session.Name != "repo/main" || si0.groupBadge != "" {
			t.Errorf("repo/main should be first without badge")
		}
		si1 := display[1].(sessionItem)
		if si1.session.Name != "repo/develop" {
			t.Errorf("expected repo/develop at 1, got %s", si1.session.Name)
		}
		if !strings.Contains(si1.groupBadge, "(+1)") {
			t.Errorf("expected (+1) badge on last active, got %q", si1.groupBadge)
		}
		si2 := display[2].(sessionItem)
		if si2.session.Name != "sesh" {
			t.Errorf("expected sesh at 2, got %s", si2.session.Name)
		}
	})
}

// describeItems is a test helper that returns a human-readable list of items
func describeItems(items []list.Item) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case sessionItem:
			desc := v.session.Name + "(" + v.session.Src + ")"
			if v.groupBadge != "" {
				desc += "[badged]"
			}
			names = append(names, desc)
		case worktreeGroupItem:
			names = append(names, v.repoName+"[group]")
		}
	}
	return names
}
