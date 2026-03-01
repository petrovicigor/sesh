package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
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

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)

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

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)

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

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)

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

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)

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

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)

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

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)

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

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)

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
		groups := buildWorktreeGroups(items, defaults, nil, GroupByPackage)

		if groups["repo"].defaultBranch != "main" {
			t.Errorf("expected defaultBranch 'main', got %q", groups["repo"].defaultBranch)
		}
	})

	t.Run("single worktree does not create group", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}},
		}

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)

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
		got := formatGroupDisplay("chase-monorepo", "develop", 3, false, GroupByPackage)
		if !strings.Contains(got, "chase-monorepo ⎇ develop") {
			t.Errorf("expected 'chase-monorepo ⎇ develop' in display, got %q", got)
		}
		if !strings.Contains(got, "(+)") {
			t.Errorf("expected (+) badge, got %q", got)
		}
	})

	t.Run("format without default branch", func(t *testing.T) {
		got := formatGroupDisplay("chase-monorepo", "", 4, false, GroupByPackage)
		if !strings.Contains(got, "chase-monorepo") {
			t.Errorf("expected repo name in display, got %q", got)
		}
		if !strings.Contains(got, "(+)") {
			t.Errorf("expected (+) badge, got %q", got)
		}
		if strings.Contains(got, "⎇") {
			t.Errorf("should not contain ⎇ when no default, got %q", got)
		}
	})

	t.Run("no badge when extra count is zero", func(t *testing.T) {
		got := formatGroupDisplay("repo", "main", 0, false, GroupByPackage)
		if strings.Contains(got, "(+") {
			t.Errorf("should not contain (+N) when count is 0, got %q", got)
		}
	})

	t.Run("workspace icon for workspace groups", func(t *testing.T) {
		got := formatGroupDisplay("mono/packages/auth", "develop", 0, true, GroupByPackage)
		if !strings.Contains(got, "📦") {
			t.Errorf("expected workspace icon 📦, got %q", got)
		}
		if !strings.Contains(got, "mono/packages/auth ⎇ develop") {
			t.Errorf("expected 'mono/packages/auth ⎇ develop', got %q", got)
		}
	})
}

func TestFormatDormantBadge(t *testing.T) {
	t.Run("shows badge", func(t *testing.T) {
		got := formatDormantBadge()
		if !strings.Contains(got, "(+)") {
			t.Errorf("expected (+) in badge, got %q", got)
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

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)
		display := buildDisplayItems(items, groups, "", nil, GroupByPackage)

		// sesh (tmux) + separator + chase-monorepo[group] + other-project = 4
		if len(display) != 4 {
			names := describeItems(display)
			t.Fatalf("expected 4 items, got %d: %v", len(display), names)
		}
		if si, ok := display[0].(sessionItem); !ok || si.session.Name != "sesh" {
			t.Errorf("expected first item to be 'sesh' sessionItem")
		}
		if _, ok := display[1].(separatorItem); !ok {
			t.Errorf("expected separator at index 1, got %T", display[1])
		}
		gi, ok := display[2].(worktreeGroupItem)
		if !ok {
			t.Fatalf("expected third item to be worktreeGroupItem, got %T", display[2])
		}
		if gi.repoName != "chase-monorepo" {
			t.Errorf("expected repoName 'chase-monorepo', got %q", gi.repoName)
		}
		if gi.totalCount != 3 {
			t.Errorf("expected totalCount 3, got %d", gi.totalCount)
		}
		if si, ok := display[3].(sessionItem); !ok || si.session.Name != "other-project" {
			t.Errorf("expected fourth item to be 'other-project' sessionItem")
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

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)
		display := buildDisplayItems(items, groups, "", nil, GroupByPackage)

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
		if !strings.Contains(si1.groupBadge, "(+)") {
			t.Errorf("expected (+) in badge, got %q", si1.groupBadge)
		}
	})

	t.Run("all active worktrees shown individually with no badge", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "projects"}},
		}

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)
		display := buildDisplayItems(items, groups, "", nil, GroupByPackage)

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

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)
		display := buildDisplayItems(items, groups, "", nil, GroupByPackage)

		// repo/main (tmux, with badge +2) = 1 item
		if len(display) != 1 {
			names := describeItems(display)
			t.Fatalf("expected 1 item, got %d: %v", len(display), names)
		}

		si := display[0].(sessionItem)
		if si.session.Src != "tmux" {
			t.Error("expected tmux sessionItem")
		}
		if !strings.Contains(si.groupBadge, "(+)") {
			t.Errorf("expected (+) badge, got %q", si.groupBadge)
		}
	})

	t.Run("expanded badge shows all worktrees below badged item", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "tmux"}, displayName: " repo/main"},
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}, displayName: " repo ⎇ main"},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "projects"}, displayName: " repo ⎇ develop"},
			sessionItem{session: model.SeshSession{Name: "repo/feature", Src: "projects"}, displayName: " repo ⎇ feature"},
		}

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)
		display := buildDisplayItems(items, groups, "repo", nil, GroupByPackage)

		// repo/main (tmux, badged) + repo/main (child) + repo/develop + repo/feature = 4
		if len(display) != 4 {
			names := describeItems(display)
			t.Fatalf("expected 4 items, got %d: %v", len(display), names)
		}

		// First is badged tmux item
		si0 := display[0].(sessionItem)
		if si0.session.Src != "tmux" || si0.groupRepo == "" {
			t.Error("expected badged tmux sessionItem first")
		}

		// All worktrees (including active ones) appear as children
		childNames := make([]string, 0)
		for _, item := range display[1:] {
			si := item.(sessionItem)
			childNames = append(childNames, si.session.Name)
			if !si.groupChild {
				t.Errorf("expected %s to be a group child", si.session.Name)
			}
		}
		// repo/main should now appear in expanded children (consistent with dormant expansion)
		found := false
		for _, name := range childNames {
			if name == "repo/main" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("repo/main should appear in expanded children, got: %v", childNames)
		}
	})

	t.Run("counts reflect active sessions", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "repo/feature", Src: "projects"}},
		}

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)
		display := buildDisplayItems(items, groups, "", nil, GroupByPackage)

		// The badged item should have (+) for dormant worktrees
		si := display[0].(sessionItem)
		if !strings.Contains(si.groupBadge, "(+)") {
			t.Errorf("expected (+) badge, got %q", si.groupBadge)
		}
	})
}

func TestBuildDisplayItemsEdgeCases(t *testing.T) {
	t.Run("single worktree shows normally (no group)", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}, displayName: " repo ⎇ main"},
		}

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)
		display := buildDisplayItems(items, groups, "", nil, GroupByPackage)

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

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)
		display := buildDisplayItems(items, groups, "", nil, GroupByPackage)

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

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)
		display := buildDisplayItems(items, groups, "", nil, GroupByPackage)

		// sesh (tmux) + separator + dotfiles + repo group + mydir = 5 items
		if len(display) != 5 {
			names := describeItems(display)
			t.Fatalf("expected 5 items, got %d: %v", len(display), names)
		}

		// Verify order including separator
		names := []string{}
		for _, item := range display {
			switch v := item.(type) {
			case sessionItem:
				names = append(names, v.session.Name)
			case worktreeGroupItem:
				names = append(names, v.repoName+"[group]")
			case separatorItem:
				names = append(names, "[separator]")
			}
		}

		expected := []string{"sesh", "[separator]", "dotfiles", "repo[group]", "mydir"}
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

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)
		display := buildDisplayItems(items, groups, "", nil, GroupByPackage)

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

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)
		display := buildDisplayItems(items, groups, "", nil, GroupByPackage)

		// repo/main, repo/develop(+), sesh = 3 (group items clustered at first encounter)
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
		if !strings.Contains(si1.groupBadge, "(+)") {
			t.Errorf("expected (+) badge on last active, got %q", si1.groupBadge)
		}
		si2 := display[2].(sessionItem)
		if si2.session.Name != "sesh" {
			t.Errorf("expected sesh at 2, got %s", si2.session.Name)
		}
	})
}

func TestBuildDisplayItemsDefaultBranchNoDuplication(t *testing.T) {
	t.Run("expanded dormant group with default shows default branch as child", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "chase-search/feature-cdk", Src: "projects"}, displayName: " chase-search ⎇ feature-cdk"},
			sessionItem{session: model.SeshSession{Name: "chase-search/review", Src: "projects"}, displayName: " chase-search ⎇ review"},
			sessionItem{session: model.SeshSession{Name: "chase-search/develop", Src: "projects"}, displayName: " chase-search ⎇ develop"},
			sessionItem{session: model.SeshSession{Name: "chase-search/regression-testing", Src: "projects"}, displayName: " chase-search ⎇ regression-testing"},
			sessionItem{session: model.SeshSession{Name: "chase-search", Src: "projects"}, displayName: " chase-search"},
		}

		defaults := map[string]string{"chase-search": "feature-cdk"}
		groups := buildWorktreeGroups(items, defaults, nil, GroupByPackage)
		display := buildDisplayItems(items, groups, "chase-search", nil, GroupByPackage) // expanded

		// Group header + 5 children (all worktrees including the default branch)
		if len(display) != 6 {
			names := describeItems(display)
			t.Fatalf("expected 6 items (1 group + 5 children), got %d: %v", len(display), names)
		}

		// First should be group item
		gi, ok := display[0].(worktreeGroupItem)
		if !ok {
			t.Fatalf("expected first item to be worktreeGroupItem, got %T", display[0])
		}
		if gi.defaultBranch != "feature-cdk" {
			t.Errorf("expected default 'feature-cdk', got %q", gi.defaultBranch)
		}

		// Default branch (feature-cdk) MUST appear as a selectable child
		found := false
		for _, item := range display[1:] {
			si := item.(sessionItem)
			if si.session.Name == "chase-search/feature-cdk" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("default branch feature-cdk should appear as a selectable child when expanded")
		}
	})

	t.Run("expanded dormant group without default shows all items", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}, displayName: " repo ⎇ main"},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "projects"}, displayName: " repo ⎇ develop"},
			sessionItem{session: model.SeshSession{Name: "repo/feature", Src: "projects"}, displayName: " repo ⎇ feature"},
		}

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage) // no defaults
		display := buildDisplayItems(items, groups, "repo", nil, GroupByPackage)           // expanded

		// Group header + 3 children (no default to skip)
		if len(display) != 4 {
			names := describeItems(display)
			t.Fatalf("expected 4 items (1 group + 3 children), got %d: %v", len(display), names)
		}
	})
}

func TestBuildDisplayItemsSeparator(t *testing.T) {
	t.Run("separator between tmux and non-tmux items", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "sesh", Src: "tmux"}, displayName: " sesh"},
			sessionItem{session: model.SeshSession{Name: "dotfiles", Src: "tmux"}, displayName: " dotfiles"},
			sessionItem{session: model.SeshSession{Name: "myproject", Src: "projects"}, displayName: " myproject"},
			sessionItem{session: model.SeshSession{Name: "mydir", Src: "zoxide"}, displayName: " mydir"},
		}
		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)
		display := buildDisplayItems(items, groups, "", nil, GroupByPackage)

		if len(display) != 5 {
			names := describeItems(display)
			t.Fatalf("expected 5 items (2 tmux + separator + 2 non-tmux), got %d: %v", len(display), names)
		}
		if _, ok := display[2].(separatorItem); !ok {
			t.Errorf("expected separatorItem at index 2, got %T", display[2])
		}
	})

	t.Run("no separator when no tmux sessions", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "myproject", Src: "projects"}, displayName: " myproject"},
			sessionItem{session: model.SeshSession{Name: "mydir", Src: "zoxide"}, displayName: " mydir"},
		}
		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)
		display := buildDisplayItems(items, groups, "", nil, GroupByPackage)

		for _, item := range display {
			if _, ok := item.(separatorItem); ok {
				t.Error("should not have separator when no tmux sessions exist")
			}
		}
	})

	t.Run("no separator when only tmux sessions", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "sesh", Src: "tmux"}, displayName: " sesh"},
			sessionItem{session: model.SeshSession{Name: "dotfiles", Src: "tmux"}, displayName: " dotfiles"},
		}
		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)
		display := buildDisplayItems(items, groups, "", nil, GroupByPackage)

		for _, item := range display {
			if _, ok := item.(separatorItem); ok {
				t.Error("should not have separator when only tmux sessions exist")
			}
		}
	})

	t.Run("separator with worktree groups", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "tmux"}, displayName: " repo/main"},
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}, displayName: " repo ⎇ main"},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "projects"}, displayName: " repo ⎇ develop"},
			sessionItem{session: model.SeshSession{Name: "other", Src: "zoxide"}, displayName: " other"},
		}
		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)
		display := buildDisplayItems(items, groups, "", nil, GroupByPackage)

		hasSeparator := false
		for _, item := range display {
			if _, ok := item.(separatorItem); ok {
				hasSeparator = true
			}
		}
		if !hasSeparator {
			names := describeItems(display)
			t.Errorf("expected separator between tmux group and non-tmux items: %v", names)
		}
	})
}

func TestBuildDisplayItemsSeparatorWithGroups(t *testing.T) {
	t.Run("separator position with active and dormant groups", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "sesh", Src: "tmux"}, displayName: " sesh"},
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "tmux"}, displayName: " repo/main"},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "tmux"}, displayName: " repo/develop"},
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}, displayName: " repo ⎇ main"},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "projects"}, displayName: " repo ⎇ develop"},
			sessionItem{session: model.SeshSession{Name: "repo/feature", Src: "projects"}, displayName: " repo ⎇ feature"},
			sessionItem{session: model.SeshSession{Name: "other-project", Src: "projects"}, displayName: " other-project"},
			sessionItem{session: model.SeshSession{Name: "mydir", Src: "zoxide"}, displayName: " mydir"},
		}

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)
		display := buildDisplayItems(items, groups, "", nil, GroupByPackage)

		// Count separators — should be exactly 1
		sepCount := 0
		sepIdx := -1
		for i, item := range display {
			if _, ok := item.(separatorItem); ok {
				sepCount++
				sepIdx = i
			}
		}
		if sepCount != 1 {
			names := describeItems(display)
			t.Fatalf("expected exactly 1 separator, got %d in: %v", sepCount, names)
		}

		// Everything before separator should be tmux-sourced
		for i := 0; i < sepIdx; i++ {
			if si, ok := display[i].(sessionItem); ok {
				if si.session.Src != "tmux" {
					t.Errorf("item %d before separator is %s (expected tmux): %s", i, si.session.Src, si.session.Name)
				}
			}
		}

		// Everything after separator should be non-tmux
		for i := sepIdx + 1; i < len(display); i++ {
			if si, ok := display[i].(sessionItem); ok {
				if si.session.Src == "tmux" {
					t.Errorf("item %d after separator is tmux: %s", i, si.session.Name)
				}
			}
		}
	})

	t.Run("separator with expanded active group", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "tmux"}, displayName: " repo/main"},
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}, displayName: " repo ⎇ main"},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "projects"}, displayName: " repo ⎇ develop"},
			sessionItem{session: model.SeshSession{Name: "repo/feature", Src: "projects"}, displayName: " repo ⎇ feature"},
			sessionItem{session: model.SeshSession{Name: "other", Src: "zoxide"}, displayName: " other"},
		}

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)
		// Expanded: dormant children appear after the tmux item
		display := buildDisplayItems(items, groups, "repo", nil, GroupByPackage)

		// Separator should come after the expanded group (tmux + dormant children), before "other"
		hasSeparator := false
		for _, item := range display {
			if _, ok := item.(separatorItem); ok {
				hasSeparator = true
			}
		}
		if !hasSeparator {
			names := describeItems(display)
			t.Errorf("expected separator after expanded group, got: %v", names)
		}
	})

	t.Run("allItems never contains separator", func(t *testing.T) {
		// Simulate what newModel does: build items, partition, but DON'T call buildDisplayItems
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "sesh", Src: "tmux"}, displayName: " sesh"},
			sessionItem{session: model.SeshSession{Name: "myproject", Src: "projects"}, displayName: " myproject"},
		}
		// allItems is set from partitioned items, NOT from buildDisplayItems
		allItems := partitionItemsByTmux(items)

		for i, item := range allItems {
			if _, ok := item.(separatorItem); ok {
				t.Errorf("allItems[%d] is a separatorItem — allItems should never contain separators", i)
			}
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
		case separatorItem:
			names = append(names, "[separator]")
		}
	}
	return names
}

func TestGroupKeyForItem(t *testing.T) {
	t.Run("regular project uses first slash", func(t *testing.T) {
		key := groupKeyForItem("geoip/develop", "projects", nil, GroupByPackage)
		if key != "geoip" {
			t.Errorf("expected 'geoip', got %q", key)
		}
	})

	t.Run("workspace item uses last slash", func(t *testing.T) {
		key := groupKeyForItem("mono/packages/auth/develop", "workspace", nil, GroupByPackage)
		if key != "mono/packages/auth" {
			t.Errorf("expected 'mono/packages/auth', got %q", key)
		}
	})

	t.Run("tmux session matching workspace prefix uses last slash", func(t *testing.T) {
		key := groupKeyForItem("mono/packages/auth/develop", "tmux", []string{"mono"}, GroupByPackage)
		if key != "mono/packages/auth" {
			t.Errorf("expected 'mono/packages/auth', got %q", key)
		}
	})

	t.Run("tmux session not matching workspace prefix uses first slash", func(t *testing.T) {
		key := groupKeyForItem("geoip/develop", "tmux", []string{"mono"}, GroupByPackage)
		if key != "geoip" {
			t.Errorf("expected 'geoip', got %q", key)
		}
	})

	t.Run("workspace item with single slash uses first slash fallback", func(t *testing.T) {
		key := groupKeyForItem("mono/develop", "workspace", nil, GroupByPackage)
		if key != "mono" {
			t.Errorf("expected 'mono', got %q", key)
		}
	})
}

func TestIsWorkspaceTmuxSession(t *testing.T) {
	t.Run("matches workspace prefix with multi-segment path", func(t *testing.T) {
		if !isWorkspaceTmuxSession("mono/packages/auth/develop", []string{"mono"}) {
			t.Error("expected true for workspace-like tmux session")
		}
	})

	t.Run("does not match regular worktree", func(t *testing.T) {
		if isWorkspaceTmuxSession("geoip/develop", []string{"mono"}) {
			t.Error("expected false for regular worktree")
		}
	})

	t.Run("does not match with empty prefixes", func(t *testing.T) {
		if isWorkspaceTmuxSession("mono/packages/auth/develop", nil) {
			t.Error("expected false with nil prefixes")
		}
	})

	t.Run("matches correct prefix among multiple", func(t *testing.T) {
		if !isWorkspaceTmuxSession("mono/apps/portal/feature-x", []string{"other", "mono"}) {
			t.Error("expected true for matching prefix")
		}
	})
}

func TestBuildWorktreeGroupsWorkspace(t *testing.T) {
	t.Run("workspace items grouped by sub-project not workspace name", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "mono/packages/auth/develop", Src: "workspace"}},
			sessionItem{session: model.SeshSession{Name: "mono/packages/auth/feature-x", Src: "workspace"}},
			sessionItem{session: model.SeshSession{Name: "mono/apps/portal/develop", Src: "workspace"}},
			sessionItem{session: model.SeshSession{Name: "mono/apps/portal/feature-x", Src: "workspace"}},
		}

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)

		if len(groups) != 2 {
			t.Fatalf("expected 2 groups (one per sub-project), got %d", len(groups))
		}
		if groups["mono/packages/auth"] == nil {
			t.Error("expected group 'mono/packages/auth'")
		}
		if groups["mono/apps/portal"] == nil {
			t.Error("expected group 'mono/apps/portal'")
		}
	})

	t.Run("active tmux workspace sessions grouped with workspace items", func(t *testing.T) {
		prefixes := []string{"mono"}
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "mono/packages/auth/develop", Src: "tmux"}},
			sessionItem{session: model.SeshSession{Name: "mono/packages/auth/develop", Src: "workspace"}},
			sessionItem{session: model.SeshSession{Name: "mono/packages/auth/feature-x", Src: "workspace"}},
		}

		groups := buildWorktreeGroups(items, make(map[string]string), prefixes, GroupByPackage)

		if len(groups) != 1 {
			t.Fatalf("expected 1 group, got %d", len(groups))
		}
		group := groups["mono/packages/auth"]
		if group == nil {
			t.Fatal("expected group 'mono/packages/auth'")
		}
		if len(group.tmuxNames) != 1 {
			t.Errorf("expected 1 active tmux, got %d", len(group.tmuxNames))
		}
	})

	t.Run("mixed workspace and regular projects coexist", func(t *testing.T) {
		prefixes := []string{"mono"}
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "geoip/develop", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "geoip/feature-x", Src: "projects"}},
			sessionItem{session: model.SeshSession{Name: "mono/packages/auth/develop", Src: "workspace"}},
			sessionItem{session: model.SeshSession{Name: "mono/packages/auth/feature-x", Src: "workspace"}},
		}

		groups := buildWorktreeGroups(items, make(map[string]string), prefixes, GroupByPackage)

		if len(groups) != 2 {
			t.Fatalf("expected 2 groups, got %d", len(groups))
		}
		if groups["geoip"] == nil {
			t.Error("expected group 'geoip'")
		}
		if groups["mono/packages/auth"] == nil {
			t.Error("expected group 'mono/packages/auth'")
		}
	})
}

func TestBuildDisplayItemsWorkspace(t *testing.T) {
	t.Run("workspace groups collapse correctly", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "sesh", Src: "tmux"}, displayName: " sesh"},
			sessionItem{session: model.SeshSession{Name: "mono/packages/auth/develop", Src: "workspace"}, displayName: "📦 mono/packages/auth ⎇ develop"},
			sessionItem{session: model.SeshSession{Name: "mono/packages/auth/feature-x", Src: "workspace"}, displayName: "📦 mono/packages/auth ⎇ feature-x"},
		}

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)
		display := buildDisplayItems(items, groups, "", nil, GroupByPackage)

		// sesh (tmux) + separator + mono/packages/auth[group] = 3
		if len(display) != 3 {
			names := describeItems(display)
			t.Fatalf("expected 3 items, got %d: %v", len(display), names)
		}

		gi, ok := display[2].(worktreeGroupItem)
		if !ok {
			t.Fatalf("expected worktreeGroupItem at index 2, got %T", display[2])
		}
		if gi.repoName != "mono/packages/auth" {
			t.Errorf("expected repoName 'mono/packages/auth', got %q", gi.repoName)
		}
	})

	t.Run("workspace groups expand correctly", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "mono/packages/auth/develop", Src: "workspace"}, displayName: "📦 mono/packages/auth ⎇ develop"},
			sessionItem{session: model.SeshSession{Name: "mono/packages/auth/feature-x", Src: "workspace"}, displayName: "📦 mono/packages/auth ⎇ feature-x"},
			sessionItem{session: model.SeshSession{Name: "mono/packages/auth/hotfix", Src: "workspace"}, displayName: "📦 mono/packages/auth ⎇ hotfix"},
		}

		groups := buildWorktreeGroups(items, make(map[string]string), nil, GroupByPackage)
		display := buildDisplayItems(items, groups, "mono/packages/auth", nil, GroupByPackage)

		// group header + 3 children = 4
		if len(display) != 4 {
			names := describeItems(display)
			t.Fatalf("expected 4 items, got %d: %v", len(display), names)
		}

		if _, ok := display[0].(worktreeGroupItem); !ok {
			t.Error("expected worktreeGroupItem first")
		}
		for _, item := range display[1:] {
			si, ok := item.(sessionItem)
			if !ok {
				t.Error("expected sessionItem child")
			}
			if !si.groupChild {
				t.Error("expected groupChild=true")
			}
		}
	})
}
