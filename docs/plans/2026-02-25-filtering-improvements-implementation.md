# Filtering & Display Improvements Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add active/inactive separator, source badges during filtering, fzf-style fuzzy scoring, and bold match highlighting to the TUI.

**Architecture:** Four independent features wired into the existing Bubble Tea list. The custom scorer (`scorer.go`) replaces `tmuxFirstFilter` and produces match indices consumed by the delegate for highlighting. The separator is a new non-selectable item type inserted by `buildDisplayItems`. Source badges are rendered by the delegate only during active filtering.

**Tech Stack:** Go, Bubble Tea (bubbles/list), lipgloss

---

### Task 1: Separator Item Type

Create the `separatorItem` type that renders as a dim divider line and is never matched by filtering.

**Files:**
- Modify: `tui/item.go:1-50`
- Modify: `tui/delegate.go:57-144`
- Test: `tui/item_test.go` (existing file)

**Step 1: Write tests for separatorItem**

Add to `tui/item_test.go`:

```go
func TestSeparatorItem(t *testing.T) {
	sep := separatorItem{}

	t.Run("FilterValue returns empty string", func(t *testing.T) {
		if sep.FilterValue() != "" {
			t.Errorf("expected empty FilterValue, got %q", sep.FilterValue())
		}
	})

	t.Run("Title returns empty string", func(t *testing.T) {
		if sep.Title() != "" {
			t.Errorf("expected empty Title, got %q", sep.Title())
		}
	})

	t.Run("Description returns empty string", func(t *testing.T) {
		if sep.Description() != "" {
			t.Errorf("expected empty Description, got %q", sep.Description())
		}
	})
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestSeparatorItem ./tui/...`
Expected: FAIL — `separatorItem` type does not exist

**Step 3: Implement separatorItem in item.go**

Add after the `worktreeGroupItem` block (after line 50) in `tui/item.go`:

```go
// separatorItem is a non-selectable visual divider between active and inactive sessions.
type separatorItem struct{}

func (s separatorItem) Title() string       { return "" }
func (s separatorItem) Description() string { return "" }
func (s separatorItem) FilterValue() string { return "" }
```

**Step 4: Run test to verify it passes**

Run: `go test -run TestSeparatorItem ./tui/...`
Expected: PASS

**Step 5: Add separator rendering to delegate**

In `tui/delegate.go`, add a cached style after the existing cached styles (after line 24):

```go
separatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
```

In the `Render` method, add a case for `separatorItem` in the switch at line 61, before the `default` case:

```go
case separatorItem:
	// Render a dim separator line — not selectable, cursor skips over it
	label := " ─── available "
	remaining := m.Width() - lipgloss.Width(label)
	if remaining > 0 {
		label += strings.Repeat("─", remaining)
	}
	fmt.Fprint(w, separatorStyle.Render(label))
	return
```

**Step 6: Run all existing tests to verify no regressions**

Run: `go test ./tui/...`
Expected: All PASS

---

### Task 2: Insert Separator in Display Items

Modify `buildDisplayItems` to insert a `separatorItem` between the last tmux session and the first non-tmux session.

**Files:**
- Modify: `tui/grouping.go:173-315`
- Test: `tui/grouping_test.go`

**Step 1: Write tests for separator insertion**

Add to `tui/grouping_test.go`:

```go
func TestBuildDisplayItemsSeparator(t *testing.T) {
	t.Run("separator between tmux and non-tmux items", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "sesh", Src: "tmux"}, displayName: " sesh"},
			sessionItem{session: model.SeshSession{Name: "dotfiles", Src: "tmux"}, displayName: " dotfiles"},
			sessionItem{session: model.SeshSession{Name: "myproject", Src: "projects"}, displayName: " myproject"},
			sessionItem{session: model.SeshSession{Name: "mydir", Src: "zoxide"}, displayName: " mydir"},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))
		display := buildDisplayItems(items, groups, "")

		// sesh, dotfiles, separator, myproject, mydir = 5
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

		groups := buildWorktreeGroups(items, make(map[string]string))
		display := buildDisplayItems(items, groups, "")

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

		groups := buildWorktreeGroups(items, make(map[string]string))
		display := buildDisplayItems(items, groups, "")

		for _, item := range display {
			if _, ok := item.(separatorItem); ok {
				t.Error("should not have separator when only tmux sessions exist")
			}
		}
	})

	t.Run("separator respects worktree groups", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "tmux"}, displayName: " repo/main"},
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}, displayName: " repo ⎇ main"},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "projects"}, displayName: " repo ⎇ develop"},
			sessionItem{session: model.SeshSession{Name: "other", Src: "zoxide"}, displayName: " other"},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))
		display := buildDisplayItems(items, groups, "")

		// repo/main(tmux, badged), separator, other = 3
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
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestBuildDisplayItemsSeparator ./tui/...`
Expected: FAIL — no separator inserted yet

**Step 3: Implement separator insertion in buildDisplayItems**

In `tui/grouping.go`, modify `buildDisplayItems` to do a two-pass approach. After the existing loop builds `result`, insert a separator. Replace the `result` building logic: after the main loop completes (before the final `return result`), add separator insertion:

```go
// Insert separator between last tmux-sourced item and first non-tmux item
// Find the boundary: last index where item is tmux-sourced (or part of an active tmux group)
lastTmuxIdx := -1
firstNonTmuxIdx := -1
for i, item := range result {
	switch v := item.(type) {
	case sessionItem:
		if v.session.Src == "tmux" || v.groupChild {
			// tmux session or expanded child of a tmux group
			if !v.groupChild {
				lastTmuxIdx = i
			} else if lastTmuxIdx >= 0 {
				lastTmuxIdx = i // extended by group children
			}
		} else if firstNonTmuxIdx == -1 && lastTmuxIdx >= 0 {
			firstNonTmuxIdx = i
		}
	case worktreeGroupItem:
		if firstNonTmuxIdx == -1 && lastTmuxIdx >= 0 {
			firstNonTmuxIdx = i
		}
	}
}

// Insert separator if both tmux and non-tmux sections exist
if lastTmuxIdx >= 0 && firstNonTmuxIdx > lastTmuxIdx {
	newResult := make([]list.Item, 0, len(result)+1)
	newResult = append(newResult, result[:firstNonTmuxIdx]...)
	newResult = append(newResult, separatorItem{})
	newResult = append(newResult, result[firstNonTmuxIdx:]...)
	result = newResult
}

return result
```

**Step 4: Run tests to verify they pass**

Run: `go test -run TestBuildDisplayItemsSeparator ./tui/...`
Expected: PASS

**Step 5: Run all grouping tests to check for regressions**

Run: `go test ./tui/...`
Expected: All PASS. Some existing tests may need count adjustments if they have mixed tmux/non-tmux items — fix any that fail by incrementing expected counts by 1 for the new separator.

**Step 6: Fix any broken existing tests**

If existing `TestBuildDisplayItems` or `TestBuildDisplayItemsEdgeCases` tests fail due to new separator being counted, update expected item counts. For example, `TestBuildDisplayItemsEdgeCases/"mixed sources preserved around groups"` currently expects 4 items — with separator it'll be 5. Update the test to expect the separator at the correct position and adjust the expected names array.

Review each failing test, determine the correct new count and position, and update assertions accordingly.

---

### Task 3: Cursor Skip Logic for Separator

Make cursor navigation (up/down) skip over separator items.

**Files:**
- Modify: `tui/update.go:621-647` (arrow key handling in filter mode)

**Step 1: Implement cursor skip in update.go**

In `tui/update.go`, the arrow key handling block (lines 621-647) intercepts `up` and `down` keys. After `m.list.CursorUp()` or `m.list.CursorDown()`, check if the newly selected item is a `separatorItem` and skip over it:

Replace the `"up"` case (lines 625-635):

```go
case "up":
	m.list.CursorUp()
	// Skip separator items
	if _, ok := m.list.SelectedItem().(separatorItem); ok {
		m.list.CursorUp()
	}
	// Load preview for newly selected session, clear for group items
	switch item := m.list.SelectedItem().(type) {
	case sessionItem:
		return m.loadPreviewDebounced(item)
	case worktreeGroupItem:
		m.previewPort.SetContent("")
		m.previewContent = ""
	}
	return m, nil
```

Replace the `"down"` case (lines 636-647):

```go
case "down":
	m.list.CursorDown()
	// Skip separator items
	if _, ok := m.list.SelectedItem().(separatorItem); ok {
		m.list.CursorDown()
	}
	// Load preview for newly selected session, clear for group items
	switch item := m.list.SelectedItem().(type) {
	case sessionItem:
		return m.loadPreviewDebounced(item)
	case worktreeGroupItem:
		m.previewPort.SetContent("")
		m.previewContent = ""
	}
	return m, nil
```

**Step 2: Run all tests**

Run: `go test ./tui/...`
Expected: All PASS

**Step 3: Build and manually test**

Run: `rm ~/Dotfiles/bin/sesh && go build -ldflags "-X 'main.version=$(git describe --tags --abbrev=0)'" -o ~/Dotfiles/bin/sesh && chmod +x ~/Dotfiles/bin/sesh`

Manual test: Open sesh TUI, verify:
- Separator line appears between active tmux sessions and inactive items
- Arrow keys skip over the separator smoothly
- Separator disappears when typing filter text

---

### Task 4: Custom Fuzzy Scorer

Implement fzf-style scoring in a new file. This is the core of the filtering improvement.

**Files:**
- Create: `tui/scorer.go`
- Create: `tui/scorer_test.go`

**Step 1: Write scorer tests**

Create `tui/scorer_test.go`:

```go
package tui

import (
	"testing"
)

func TestFuzzyScore(t *testing.T) {
	t.Run("exact substring gets highest score", func(t *testing.T) {
		score, indices := fuzzyScore("develop", "chase-search/develop")
		if score <= 0 {
			t.Fatal("expected positive score for exact substring match")
		}
		if len(indices) != 7 {
			t.Errorf("expected 7 matched indices, got %d", len(indices))
		}
		// Indices should be consecutive starting at position 13
		for i, idx := range indices {
			expected := 13 + i
			if idx != expected {
				t.Errorf("index %d: expected %d, got %d", i, expected, idx)
			}
		}
	})

	t.Run("prefix match scores high", func(t *testing.T) {
		prefixScore, _ := fuzzyScore("ch", "chase-search")
		midScore, _ := fuzzyScore("ch", "tech-archive")
		if prefixScore <= midScore {
			t.Errorf("prefix match (%f) should score higher than mid match (%f)", prefixScore, midScore)
		}
	})

	t.Run("consecutive chars score higher than scattered", func(t *testing.T) {
		consecScore, _ := fuzzyScore("sea", "chase-search")
		scatterScore, _ := fuzzyScore("sea", "s_e_a_other")
		if consecScore <= scatterScore {
			t.Errorf("consecutive (%f) should score higher than scattered (%f)", consecScore, scatterScore)
		}
	})

	t.Run("word boundary match scores high", func(t *testing.T) {
		boundaryScore, _ := fuzzyScore("sd", "sesh/develop")
		midScore, _ := fuzzyScore("sd", "abcsdxyz")
		if boundaryScore <= midScore {
			t.Errorf("word boundary (%f) should score higher than mid (%f)", boundaryScore, midScore)
		}
	})

	t.Run("case insensitive matching", func(t *testing.T) {
		score, indices := fuzzyScore("CHASE", "chase-search")
		if score <= 0 {
			t.Fatal("expected positive score for case-insensitive match")
		}
		if len(indices) != 5 {
			t.Errorf("expected 5 matched indices, got %d", len(indices))
		}
	})

	t.Run("no match returns zero score", func(t *testing.T) {
		score, indices := fuzzyScore("xyz", "chase-search")
		if score != 0 {
			t.Errorf("expected 0 score for no match, got %f", score)
		}
		if indices != nil {
			t.Errorf("expected nil indices for no match, got %v", indices)
		}
	})

	t.Run("empty query matches everything with zero score", func(t *testing.T) {
		score, _ := fuzzyScore("", "anything")
		if score != 0 {
			t.Errorf("expected 0 score for empty query, got %f", score)
		}
	})

	t.Run("query longer than target returns no match", func(t *testing.T) {
		score, indices := fuzzyScore("very-long-query", "short")
		if score != 0 {
			t.Errorf("expected 0 score, got %f", score)
		}
		if indices != nil {
			t.Errorf("expected nil indices, got %v", indices)
		}
	})

	t.Run("slash is a word boundary", func(t *testing.T) {
		score, indices := fuzzyScore("gd", "geoip/develop")
		if score <= 0 {
			t.Fatal("expected match across slash boundary")
		}
		if len(indices) != 2 {
			t.Errorf("expected 2 indices, got %d", len(indices))
		}
	})

	t.Run("dash is a word boundary", func(t *testing.T) {
		score, indices := fuzzyScore("cs", "chase-search")
		if score <= 0 {
			t.Fatal("expected match across dash boundary")
		}
		if len(indices) != 2 {
			t.Errorf("expected 2 indices, got %d", len(indices))
		}
	})

	t.Run("real world: dev matches develop branches", func(t *testing.T) {
		s1, _ := fuzzyScore("dev", "chase-search/develop")
		s2, _ := fuzzyScore("dev", "geoip/develop")
		s3, _ := fuzzyScore("dev", "dev-tools")
		// All should match
		if s1 <= 0 || s2 <= 0 || s3 <= 0 {
			t.Errorf("all should match: s1=%f s2=%f s3=%f", s1, s2, s3)
		}
	})

	t.Run("real world: exact prefix beats partial", func(t *testing.T) {
		dotScore, _ := fuzzyScore("dot", "dotfiles")
		otherScore, _ := fuzzyScore("dot", "god-other-thing")
		if dotScore <= otherScore {
			t.Errorf("prefix 'dot' in 'dotfiles' (%f) should beat scattered in other (%f)", dotScore, otherScore)
		}
	})
}

func TestFuzzyScoreMatchIndices(t *testing.T) {
	t.Run("indices are valid positions in target", func(t *testing.T) {
		_, indices := fuzzyScore("chase", "chase-search/develop")
		target := "chase-search/develop"
		for _, idx := range indices {
			if idx < 0 || idx >= len(target) {
				t.Errorf("index %d out of range for target len %d", idx, len(target))
			}
		}
	})

	t.Run("indices are in ascending order", func(t *testing.T) {
		_, indices := fuzzyScore("sd", "sesh/develop")
		for i := 1; i < len(indices); i++ {
			if indices[i] <= indices[i-1] {
				t.Errorf("indices not ascending: %v", indices)
			}
		}
	})
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -run TestFuzzyScore ./tui/...`
Expected: FAIL — `fuzzyScore` function does not exist

**Step 3: Implement the scorer**

Create `tui/scorer.go`:

```go
package tui

import (
	"strings"
	"unicode"
)

// fuzzyScore computes a match score and character indices for a query against a target string.
// Returns (0, nil) for no match. Higher scores = better match.
//
// Scoring priorities:
//  1. Exact substring match (highest)
//  2. Prefix match
//  3. Word-boundary aligned matches
//  4. Consecutive character bonus
//  5. Scattered character matches (lowest)
func fuzzyScore(query, target string) (float64, []int) {
	if query == "" {
		return 0, nil
	}

	qLower := strings.ToLower(query)
	tLower := strings.ToLower(target)

	if len(qLower) > len(tLower) {
		return 0, nil
	}

	// Try exact substring first (highest score)
	if idx := strings.Index(tLower, qLower); idx >= 0 {
		indices := make([]int, len(qLower))
		for i := range qLower {
			indices[i] = idx + i
		}
		score := 1.0
		if idx == 0 {
			score += 0.2 // prefix bonus
		}
		if idx == 0 || isWordBoundary(tLower, idx) {
			score += 0.1 // word boundary bonus
		}
		return score, indices
	}

	// Fuzzy match: find best alignment of query chars in target
	indices := bestFuzzyMatch(qLower, tLower)
	if indices == nil {
		return 0, nil
	}

	// Score based on match quality
	score := computeScore(qLower, tLower, indices)
	return score, indices
}

// bestFuzzyMatch finds the best positions for query characters in target.
// Uses a greedy approach that prefers word boundaries and consecutive matches.
func bestFuzzyMatch(query, target string) []int {
	qRunes := []rune(query)
	tRunes := []rune(target)

	if len(qRunes) > len(tRunes) {
		return nil
	}

	// First try: prefer word-boundary-aligned matches
	indices := matchPreferBoundaries(qRunes, tRunes)
	if indices != nil {
		return indices
	}

	// Fallback: simple left-to-right greedy match
	indices = make([]int, 0, len(qRunes))
	ti := 0
	for _, qr := range qRunes {
		found := false
		for ti < len(tRunes) {
			if tRunes[ti] == qr {
				indices = append(indices, ti)
				ti++
				found = true
				break
			}
			ti++
		}
		if !found {
			return nil
		}
	}
	return indices
}

// matchPreferBoundaries tries to align query chars at word boundaries in the target.
func matchPreferBoundaries(query, target []rune) []int {
	indices := make([]int, 0, len(query))
	qi := 0
	lastMatchIdx := -1

	for ti := 0; ti < len(target) && qi < len(query); ti++ {
		if target[ti] != query[qi] {
			continue
		}

		// Prefer this position if it's:
		// 1. A word boundary
		// 2. Consecutive with previous match
		// 3. First character
		isBoundary := ti == 0 || isWordBoundaryRune(target, ti)
		isConsecutive := lastMatchIdx >= 0 && ti == lastMatchIdx+1

		if isBoundary || isConsecutive || len(indices) == 0 {
			indices = append(indices, ti)
			lastMatchIdx = ti
			qi++
		}
	}

	if qi < len(query) {
		return nil // couldn't match all query chars at boundaries
	}
	return indices
}

// computeScore calculates score based on match quality.
func computeScore(query, target string, indices []int) float64 {
	if len(indices) == 0 {
		return 0
	}

	score := 0.3 // base score for any match

	// Prefix bonus: first match is at position 0
	if indices[0] == 0 {
		score += 0.3
	}

	// Word boundary bonus: count matches at word boundaries
	tRunes := []rune(target)
	boundaryMatches := 0
	for _, idx := range indices {
		if idx == 0 || isWordBoundaryRune(tRunes, idx) {
			boundaryMatches++
		}
	}
	score += float64(boundaryMatches) * 0.1

	// Consecutive bonus: count consecutive match pairs
	consecutive := 0
	for i := 1; i < len(indices); i++ {
		if indices[i] == indices[i-1]+1 {
			consecutive++
		}
	}
	if len(indices) > 1 {
		score += float64(consecutive) / float64(len(indices)-1) * 0.4
	}

	// Compactness: prefer matches closer together
	spread := indices[len(indices)-1] - indices[0] + 1
	if spread > 0 {
		compactness := float64(len(indices)) / float64(spread)
		score += compactness * 0.1
	}

	// Length ratio: prefer shorter targets (more specific matches)
	score += 0.05 * (1.0 - float64(len(target)-len(query))/float64(len(target)+1))

	return score
}

// isWordBoundary checks if position idx in target is at a word boundary.
func isWordBoundary(target string, idx int) bool {
	if idx == 0 {
		return true
	}
	return isWordBoundaryRune([]rune(target), idx)
}

// isWordBoundaryRune checks if the rune at idx is at a word boundary.
func isWordBoundaryRune(runes []rune, idx int) bool {
	if idx == 0 || idx >= len(runes) {
		return idx == 0
	}
	prev := runes[idx-1]
	curr := runes[idx]

	// After separator characters
	if prev == '/' || prev == '-' || prev == '_' || prev == '.' || prev == ' ' {
		return true
	}

	// camelCase boundary
	if unicode.IsLower(prev) && unicode.IsUpper(curr) {
		return true
	}

	return false
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -run TestFuzzyScore ./tui/...`
Expected: All PASS

**Step 5: Add benchmark tests**

Add to `tui/filter_bench_test.go`:

```go
func BenchmarkFuzzyScore(b *testing.B) {
	targets := []string{
		"chase-search/develop",
		"geoip/feature-x",
		"dotfiles",
		"sesh",
		"chase-cognito",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, t := range targets {
			fuzzyScore("dev", t)
		}
	}
}

func BenchmarkFuzzyScoreNoMatch(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fuzzyScore("xyz123", "chase-search/develop")
	}
}
```

**Step 6: Run benchmarks to verify performance**

Run: `go test -bench=BenchmarkFuzzyScore -benchmem ./tui/...`
Expected: Sub-microsecond per call (these are short strings)

---

### Task 5: Replace Filter Function with Custom Scorer

Replace `tmuxFirstFilter` with a new `seshFilter` that uses `fuzzyScore` and applies tmux-first tiebreaking.

**Files:**
- Modify: `tui/model.go:18-87` (replace `tmuxFirstFilter`)
- Modify: `tui/filter_test.go` (update tests)
- Modify: `tui/filter_bench_test.go` (update benchmarks)

**Step 1: Write tests for the new filter function**

Update tests in `tui/filter_test.go`. The new function `seshFilter` should satisfy the same invariants: tmux sessions ranked higher than non-tmux for equal scores. Replace `tmuxFirstFilter` references with `seshFilter` and add new scoring-focused tests:

```go
func TestSeshFilterScoring(t *testing.T) {
	items := []list.Item{
		sessionItem{session: model.SeshSession{Name: "chase-search/develop", Src: "tmux"}, displayName: "chase-search/develop"},
		sessionItem{session: model.SeshSession{Name: "chase-cognito", Src: "projects"}, displayName: "chase-cognito"},
		sessionItem{session: model.SeshSession{Name: "tech-archive", Src: "zoxide"}, displayName: "tech-archive"},
		sessionItem{session: model.SeshSession{Name: "dotfiles", Src: "config"}, displayName: "dotfiles"},
		sessionItem{session: model.SeshSession{Name: "geoip/develop", Src: "projects"}, displayName: "geoip/develop"},
		sessionItem{session: model.SeshSession{Name: "dev-tools", Src: "zoxide"}, displayName: "dev-tools"},
	}

	t.Run("prefix match ranks first", func(t *testing.T) {
		targets := extractTargets(items)
		filter := seshFilter(items)
		ranks := filter("ch", targets)

		if len(ranks) == 0 {
			t.Fatal("expected results")
		}
		// First result should start with "ch"
		first := items[ranks[0].Index].(sessionItem)
		if first.session.Name != "chase-search/develop" && first.session.Name != "chase-cognito" {
			t.Errorf("expected chase-* first, got %s", first.session.Name)
		}
	})

	t.Run("exact substring ranks high", func(t *testing.T) {
		targets := extractTargets(items)
		filter := seshFilter(items)
		ranks := filter("develop", targets)

		if len(ranks) == 0 {
			t.Fatal("expected results")
		}
		// Both develop matches should be at top
		topNames := make([]string, 0)
		for i := 0; i < len(ranks) && i < 2; i++ {
			topNames = append(topNames, items[ranks[i].Index].(sessionItem).session.Name)
		}
		for _, name := range topNames {
			if !strings.Contains(name, "develop") {
				t.Errorf("expected 'develop' in top results, got %s", name)
			}
		}
	})

	t.Run("tmux wins tiebreak for equal scores", func(t *testing.T) {
		tiedItems := []list.Item{
			sessionItem{session: model.SeshSession{Name: "foo/develop", Src: "projects"}, displayName: "foo/develop"},
			sessionItem{session: model.SeshSession{Name: "foo/develop", Src: "tmux"}, displayName: "foo/develop"},
		}
		targets := extractTargets(tiedItems)
		filter := seshFilter(tiedItems)
		ranks := filter("develop", targets)

		if len(ranks) < 2 {
			t.Fatalf("expected 2 results, got %d", len(ranks))
		}
		first := tiedItems[ranks[0].Index].(sessionItem)
		if first.session.Src != "tmux" {
			t.Errorf("expected tmux to win tiebreak, got %s", first.session.Src)
		}
	})

	t.Run("matched indices are populated", func(t *testing.T) {
		targets := extractTargets(items)
		filter := seshFilter(items)
		ranks := filter("dev", targets)

		for _, rank := range ranks {
			if len(rank.MatchedIndexes) == 0 {
				name := items[rank.Index].(sessionItem).session.Name
				t.Errorf("expected MatchedIndexes for %s, got empty", name)
			}
		}
	})
}

func extractTargets(items []list.Item) []string {
	targets := make([]string, len(items))
	for i, item := range items {
		switch v := item.(type) {
		case sessionItem:
			targets[i] = v.FilterValue()
		case worktreeGroupItem:
			targets[i] = v.FilterValue()
		}
	}
	return targets
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestSeshFilterScoring ./tui/...`
Expected: FAIL — `seshFilter` does not exist

**Step 3: Implement seshFilter in model.go**

Replace `tmuxFirstFilter` (lines 18-87) in `tui/model.go` with:

```go
// seshFilter returns a custom FilterFunc with fzf-style fuzzy scoring.
// Ranks results by match quality, with tmux sessions winning tiebreaks.
func seshFilter(items []list.Item) list.FilterFunc {
	return func(term string, targets []string) []list.Rank {
		if term == "" {
			return nil
		}

		type scoredRank struct {
			rank  list.Rank
			score float64
			src   string
		}

		results := make([]scoredRank, 0, len(targets))
		for i, target := range targets {
			score, indices := fuzzyScore(term, target)
			if score <= 0 {
				continue
			}

			var src string
			if i < len(items) {
				switch v := items[i].(type) {
				case sessionItem:
					src = v.session.Src
				case worktreeGroupItem:
					src = "projects"
				}
			}

			results = append(results, scoredRank{
				rank: list.Rank{
					Index:          i,
					MatchedIndexes: indices,
				},
				score: score,
				src:   src,
			})
		}

		// Sort: higher score first, tmux wins tiebreaks
		slices.SortStableFunc(results, func(a, b scoredRank) int {
			// Higher score first
			if a.score != b.score {
				if a.score > b.score {
					return -1
				}
				return 1
			}
			// Tiebreak: tmux first
			aIsTmux := a.src == "tmux"
			bIsTmux := b.src == "tmux"
			if aIsTmux && !bIsTmux {
				return -1
			}
			if !aIsTmux && bIsTmux {
				return 1
			}
			return 0
		})

		ranks := make([]list.Rank, len(results))
		for i, r := range results {
			ranks[i] = r.rank
		}
		return ranks
	}
}
```

Add `"slices"` to the imports at the top of `model.go`.

**Step 4: Update all references from tmuxFirstFilter to seshFilter**

In `tui/model.go`:
- Line 193: `l.Filter = tmuxFirstFilter(displayItems)` → `l.Filter = seshFilter(displayItems)`

In `tui/update.go` (search for all `tmuxFirstFilter`):
- Line 50: `m.list.Filter = tmuxFirstFilter(displayItems)` → `m.list.Filter = seshFilter(displayItems)`
- Line 133: `m.list.Filter = tmuxFirstFilter(displayItems)` → `m.list.Filter = seshFilter(displayItems)`
- Line 222: `m.list.Filter = tmuxFirstFilter(displayItems)` → `m.list.Filter = seshFilter(displayItems)`
- Line 527: `m.list.Filter = tmuxFirstFilter(displayItems)` → `m.list.Filter = seshFilter(displayItems)`
- Line 601: `m.list.Filter = tmuxFirstFilter(displayItems)` → `m.list.Filter = seshFilter(displayItems)`
- Line 665: `m.list.Filter = tmuxFirstFilter(m.allItems)` → `m.list.Filter = seshFilter(m.allItems)`
- Line 678: `m.list.Filter = tmuxFirstFilter(displayItems)` → `m.list.Filter = seshFilter(displayItems)`

**Step 5: Update existing filter tests**

In `tui/filter_test.go`, replace all occurrences of `tmuxFirstFilter` with `seshFilter`. The test names (`TestTmuxFirstFilter` etc.) should also be renamed to `TestSeshFilter` etc. The invariant tests (tmux before non-tmux within groups) may need adjustment since seshFilter uses score-based ordering rather than repo-grouping — the key invariant now is: tmux wins tiebreaks, not that repos are grouped together. Remove the "repo adjacency" assertions since the new filter doesn't group by repo (flat results with badges instead).

**Step 6: Update benchmarks**

In `tui/filter_bench_test.go`, rename `BenchmarkTmuxFirstFilter*` to `BenchmarkSeshFilter*` and use `seshFilter`.

**Step 7: Run all tests**

Run: `go test ./tui/...`
Expected: All PASS

---

### Task 6: Bold Match Highlighting

Replace the invisible underline highlighting with bold + warm accent color.

**Files:**
- Modify: `tui/delegate.go:14-25` (cached styles)
- Modify: `tui/delegate.go:63-68` (highlight rendering)

**Step 1: Update styles in delegate.go**

Replace `filterMatchStyle` (line 20) and add a selected variant:

```go
filterMatchStyle         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))  // Bold orange/gold
filterMatchSelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("228"))  // Bold bright yellow (visible on selection bg)
```

**Step 2: Update highlight rendering**

In the `Render` method, update the highlight block (lines 64-68) to use the appropriate style based on whether the item is selected:

```go
if isFiltered && len(matchedRunes) > 0 {
	matchStyle := filterMatchStyle
	baseStyle := lipgloss.NewStyle()
	if index == m.Index() {
		matchStyle = filterMatchSelectedStyle
		baseStyle = selectedItemStyle
	}
	highlighted := lipgloss.StyleRunes(v.session.Name, matchedRunes, matchStyle, baseStyle)
	str = v.iconPrefix + highlighted
}
```

Note: The selection highlight at line 137-141 still wraps the whole line, so we need to ensure the per-character highlighting doesn't conflict. Since `selectedItemStyle` applies color 170 to the whole string, and `filterMatchSelectedStyle` applies bold + color 228 to matched chars, the `StyleRunes` approach will override matched characters with the brighter style. We need to NOT apply `selectedItemStyle.Render()` wrapping when we've already done per-character styling for the selected item. Adjust the selection block (lines 137-141):

```go
// Highlight selected item
if index == m.Index() {
	if isFiltered && len(matchedRunes) > 0 {
		// Already styled per-character above, just add cursor
		str = selectedItemStyle.Render("❯ ") + treePrefix + str + nodeIndicator
	} else {
		str = selectedItemStyle.Render("❯ " + treePrefix + str + nodeIndicator)
	}
} else {
	str = "  " + treePrefix + str + nodeIndicator
}
```

This requires `isFiltered` and `matchedRunes` to be accessible outside the `sessionItem` case. Move them to variables declared at the top of `Render`, before the switch:

```go
func (d compactDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	var str string
	var nodeIndicator string
	isFiltered := m.FilterState() == list.Filtering && m.FilterValue() != ""
	matchedRunes := m.MatchesForItem(index)

	switch v := item.(type) {
	case sessionItem:
		// ...existing code using isFiltered and matchedRunes...
```

**Step 3: Run all tests**

Run: `go test ./tui/...`
Expected: All PASS

**Step 4: Build and manually test**

Run: `rm ~/Dotfiles/bin/sesh && go build -ldflags "-X 'main.version=$(git describe --tags --abbrev=0)'" -o ~/Dotfiles/bin/sesh && chmod +x ~/Dotfiles/bin/sesh`

Manual test: Open sesh TUI, type a filter query. Matched characters should appear in bold orange/gold. Selected item should have bright yellow matched characters.

---

### Task 7: Source Badges During Filtering

Show a dim source badge (tmux, projects, config, zoxide) next to each result when actively filtering.

**Files:**
- Modify: `tui/delegate.go:57-144`

**Step 1: Add badge style**

In `tui/delegate.go`, add a cached style (after the existing ones around line 24):

```go
badgeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
```

**Step 2: Add badge rendering in delegate**

In the `Render` method's `sessionItem` case, after all the existing name rendering (after the groupBadge append around line 117), add badge rendering when filtered:

```go
// Show source badge during active filtering
if isFiltered && len(matchedRunes) > 0 {
	badge := badgeStyle.Render(v.session.Src)
	// Calculate available space: list width - visible name length - prefix (2 for "❯ " or "  ") - badge length - 1 gap
	nameWidth := lipgloss.Width(str)
	badgeWidth := lipgloss.Width(badge)
	prefixWidth := 2 // "❯ " or "  "
	gap := m.Width() - prefixWidth - nameWidth - badgeWidth
	if gap > 0 {
		str = str + strings.Repeat(" ", gap) + badge
	}
}
```

**Step 3: Run all tests**

Run: `go test ./tui/...`
Expected: All PASS

**Step 4: Build and manually test**

Run: `rm ~/Dotfiles/bin/sesh && go build -ldflags "-X 'main.version=$(git describe --tags --abbrev=0)'" -o ~/Dotfiles/bin/sesh && chmod +x ~/Dotfiles/bin/sesh`

Manual test: Open sesh TUI, type a filter query. Each result should show a dim badge on the right (e.g., "tmux", "projects", "zoxide").

---

### Task 8: Integration & Edge Cases

Ensure all features work together: separator hides during filtering, badges only appear during filtering, filtering doesn't show separator items, expand/collapse still works with separator present.

**Files:**
- Modify: `tui/update.go:654-705` (filter transition logic)
- Modify: `tui/model.go:118-238` (newModel — pass allItems without separator to filter)

**Step 1: Ensure allItems does NOT contain separators**

The `allItems` field is used as the flat item list for filtering. It should never contain `separatorItem` entries. Verify that `allItems` is set from `items` (pre-separator) and `buildDisplayItems` adds separators only to display items. Based on the current code, this is already correct — `allItems` is set from `items` at `model.go:169` before `buildDisplayItems` is called at line 154. No change needed here.

**Step 2: Ensure filter transition doesn't leak separators**

In `tui/update.go`, the transition from empty→non-empty filter (line 662-670) swaps to `m.allItems`. Since `allItems` has no separators, this is correct. The transition from non-empty→empty (line 675-681) calls `buildDisplayItems` which will re-insert the separator. This is also correct.

Verify by checking: when `seshFilter` processes targets, `separatorItem`'s `FilterValue()` returns `""`, so it will never match any query. Even if separator is in the display items when filter starts, it won't appear in results.

**Step 3: Test the full flow**

Add an integration-style test to `tui/grouping_test.go`:

```go
func TestBuildDisplayItemsSeparatorWithGroups(t *testing.T) {
	t.Run("separator position with active and dormant groups", func(t *testing.T) {
		items := []list.Item{
			// Active tmux sessions
			sessionItem{session: model.SeshSession{Name: "sesh", Src: "tmux"}, displayName: " sesh"},
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "tmux"}, displayName: " repo/main"},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "tmux"}, displayName: " repo/develop"},
			// Inactive projects
			sessionItem{session: model.SeshSession{Name: "repo/main", Src: "projects"}, displayName: " repo ⎇ main"},
			sessionItem{session: model.SeshSession{Name: "repo/develop", Src: "projects"}, displayName: " repo ⎇ develop"},
			sessionItem{session: model.SeshSession{Name: "repo/feature", Src: "projects"}, displayName: " repo ⎇ feature"},
			sessionItem{session: model.SeshSession{Name: "other-project", Src: "projects"}, displayName: " other-project"},
			sessionItem{session: model.SeshSession{Name: "mydir", Src: "zoxide"}, displayName: " mydir"},
		}

		groups := buildWorktreeGroups(items, make(map[string]string))
		display := buildDisplayItems(items, groups, "")

		// Count separators
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
	})
}
```

**Step 4: Run all tests**

Run: `go test ./tui/...`
Expected: All PASS

**Step 5: Run full test suite**

Run: `make test`
Expected: All PASS

**Step 6: Final build and manual testing**

Run: `rm ~/Dotfiles/bin/sesh && go build -ldflags "-X 'main.version=$(git describe --tags --abbrev=0)'" -o ~/Dotfiles/bin/sesh && chmod +x ~/Dotfiles/bin/sesh`

Manual test checklist:
- [ ] Separator visible between active tmux and inactive items
- [ ] Cursor skips over separator with arrow keys
- [ ] Typing filter text: separator disappears, flat results shown
- [ ] Matched characters highlighted in bold orange/gold
- [ ] Source badges visible next to each result during filtering
- [ ] Clearing filter: separator reappears, grouped view restored
- [ ] Tab expand/collapse still works with separator present
- [ ] Ctrl+T repo focus still works
- [ ] Enter on a session still connects
- [ ] Ctrl+D still deletes tmux sessions
