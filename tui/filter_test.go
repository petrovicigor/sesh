package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/joshmedeski/sesh/v2/model"
)

// TestTmuxFirstFilter validates that tmux sessions are always grouped first
func TestTmuxFirstFilter(t *testing.T) {
	// Create test sessions
	testSessions := []list.Item{
		sessionItem{session: model.SeshSession{Name: "chase-search", Src: "tmux"}, displayName: "chase-search"},
		sessionItem{session: model.SeshSession{Name: "chase-search/review", Src: "projects"}, displayName: "chase-search/review"},
		sessionItem{session: model.SeshSession{Name: "chase-search/develop", Src: "tmux"}, displayName: "chase-search/develop"},
		sessionItem{session: model.SeshSession{Name: "chase-search/feature-cdk", Src: "tmux"}, displayName: "chase-search/feature-cdk"},
		sessionItem{session: model.SeshSession{Name: "chase-search/regression-testing", Src: "projects"}, displayName: "chase-search/regression-testing"},
		sessionItem{session: model.SeshSession{Name: "dotfiles", Src: "config"}, displayName: "dotfiles"},
		sessionItem{session: model.SeshSession{Name: "sesh", Src: "tmux"}, displayName: "sesh"},
		sessionItem{session: model.SeshSession{Name: "projects/myapp", Src: "projects"}, displayName: "projects/myapp"},
	}

	testCases := []struct {
		name           string
		searchTerm     string
		expectedFirst  string // First result should be this
		expectedSource string // First result should be from this source
	}{
		{
			name:           "Search 'chase' - tmux results before projects",
			searchTerm:     "chase",
			expectedFirst:  "chase-search",
			expectedSource: "tmux",
		},
		{
			name:           "Search 'search' - tmux results before projects",
			searchTerm:     "search",
			expectedFirst:  "chase-search",
			expectedSource: "tmux",
		},
		{
			name:           "Search 'dev' - tmux develop before anything",
			searchTerm:     "dev",
			expectedFirst:  "chase-search/develop",
			expectedSource: "tmux",
		},
		{
			name:           "Search 'review' - only projects match, should return project",
			searchTerm:     "review",
			expectedFirst:  "chase-search/review",
			expectedSource: "projects",
		},
		{
			name:           "Search 'sesh' - tmux session first",
			searchTerm:     "sesh",
			expectedFirst:  "sesh",
			expectedSource: "tmux",
		},
		{
			name:           "Fuzzy search 'chssrch' - should still match and group tmux first",
			searchTerm:     "chssrch",
			expectedFirst:  "chase-search",
			expectedSource: "tmux",
		},
		{
			name:           "Search 'dot' - config session (no tmux match)",
			searchTerm:     "dot",
			expectedFirst:  "dotfiles",
			expectedSource: "config",
		},
		{
			name:           "Single character 'c' - should match chase tmux first",
			searchTerm:     "c",
			expectedFirst:  "chase-search",
			expectedSource: "tmux",
		},
		{
			name:           "Partial match 'feat' - should match feature-cdk tmux",
			searchTerm:     "feat",
			expectedFirst:  "chase-search/feature-cdk",
			expectedSource: "tmux",
		},
		{
			name:           "Case insensitive 'CHASE' - should work",
			searchTerm:     "CHASE",
			expectedFirst:  "chase-search",
			expectedSource: "tmux",
		},
		{
			name:           "Search with dash 'chase-search' - exact match",
			searchTerm:     "chase-search",
			expectedFirst:  "chase-search",
			expectedSource: "tmux",
		},
		{
			name:           "Search with slash 'chase/dev' - should fuzzy match",
			searchTerm:     "chase/dev",
			expectedFirst:  "chase-search/develop",
			expectedSource: "tmux",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Get filter values for each item
			targets := make([]string, len(testSessions))
			for i, item := range testSessions {
				if si, ok := item.(sessionItem); ok {
					targets[i] = si.FilterValue()
				}
			}

			// Create filter and run it
			filter := tmuxFirstFilter(testSessions)
			ranks := filter(tc.searchTerm, targets)

			if len(ranks) == 0 {
				t.Fatalf("Expected results but got none for search term '%s'", tc.searchTerm)
			}

			// Get first result
			firstRank := ranks[0]
			if firstRank.Index >= len(testSessions) {
				t.Fatalf("Invalid index %d for search term '%s'", firstRank.Index, tc.searchTerm)
			}

			firstItem := testSessions[firstRank.Index].(sessionItem)

			// Validate first result
			if firstItem.session.Name != tc.expectedFirst {
				t.Errorf("Expected first result '%s' but got '%s' for search term '%s'",
					tc.expectedFirst, firstItem.session.Name, tc.searchTerm)
			}

			if firstItem.session.Src != tc.expectedSource {
				t.Errorf("Expected first result from source '%s' but got '%s' for search term '%s'",
					tc.expectedSource, firstItem.session.Src, tc.searchTerm)
			}

			// Validate per-repo grouping: within each repo, tmux sessions come before non-tmux
			repoLastTmux := make(map[string]int)
			repoFirstNonTmux := make(map[string]int)
			for i, rank := range ranks {
				item := testSessions[rank.Index].(sessionItem)
				repo := item.session.Name
				if strings.Contains(repo, "/") {
					repo = strings.SplitN(repo, "/", 2)[0]
				}
				if item.session.Src == "tmux" {
					repoLastTmux[repo] = i
				} else {
					if _, exists := repoFirstNonTmux[repo]; !exists {
						repoFirstNonTmux[repo] = i
					}
				}
			}

			// Within each repo group, tmux must come before non-tmux
			for repo, lastTmux := range repoLastTmux {
				if firstNonTmux, exists := repoFirstNonTmux[repo]; exists {
					if lastTmux > firstNonTmux {
						t.Errorf("Repo %q: tmux at pos %d after non-tmux at pos %d for search term '%s'",
							repo, lastTmux, firstNonTmux, tc.searchTerm)
					}
				}
			}
		})
	}
}

// TestTmuxFirstFilterEdgeCases tests edge cases
func TestTmuxFirstFilterEdgeCases(t *testing.T) {
	t.Run("Empty items list", func(t *testing.T) {
		items := []list.Item{}
		filter := tmuxFirstFilter(items)
		ranks := filter("test", []string{})
		if len(ranks) != 0 {
			t.Errorf("Expected 0 results for empty items, got %d", len(ranks))
		}
	})

	t.Run("Single tmux item", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "test", Src: "tmux"}, displayName: "test"},
		}
		filter := tmuxFirstFilter(items)
		ranks := filter("test", []string{"test"})
		if len(ranks) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(ranks))
		}
		if ranks[0].Index != 0 {
			t.Errorf("Expected index 0, got %d", ranks[0].Index)
		}
	})

	t.Run("Single non-tmux item", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "test", Src: "projects"}, displayName: "test"},
		}
		filter := tmuxFirstFilter(items)
		ranks := filter("test", []string{"test"})
		if len(ranks) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(ranks))
		}
		if ranks[0].Index != 0 {
			t.Errorf("Expected index 0, got %d", ranks[0].Index)
		}
	})

	t.Run("No matches", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "foo", Src: "tmux"}, displayName: "foo"},
			sessionItem{session: model.SeshSession{Name: "bar", Src: "projects"}, displayName: "bar"},
		}
		filter := tmuxFirstFilter(items)
		ranks := filter("xyz123notfound", []string{"foo", "bar"})
		if len(ranks) != 0 {
			t.Errorf("Expected 0 results for non-matching search, got %d", len(ranks))
		}
	})

	t.Run("All sources mixed", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "app", Src: "projects"}, displayName: "app"},
			sessionItem{session: model.SeshSession{Name: "app-test", Src: "tmux"}, displayName: "app-test"},
			sessionItem{session: model.SeshSession{Name: "application", Src: "config"}, displayName: "application"},
			sessionItem{session: model.SeshSession{Name: "app-dev", Src: "tmux"}, displayName: "app-dev"},
			sessionItem{session: model.SeshSession{Name: "myapp", Src: "zoxide"}, displayName: "myapp"},
		}
		targets := []string{"app", "app-test", "application", "app-dev", "myapp"}
		filter := tmuxFirstFilter(items)
		ranks := filter("app", targets)

		// Should have results
		if len(ranks) == 0 {
			t.Fatal("Expected results but got none")
		}

		// Verify per-repo tmux-first: within each repo group, tmux items come before non-tmux
		tmuxFound := false
		repoLastTmux := make(map[string]int)    // last position of tmux item per repo
		repoFirstNonTmux := make(map[string]int) // first position of non-tmux item per repo
		for i, rank := range ranks {
			item := items[rank.Index].(sessionItem)
			repo := item.session.Name
			if strings.Contains(repo, "/") {
				repo = strings.SplitN(repo, "/", 2)[0]
			}
			if item.session.Src == "tmux" {
				tmuxFound = true
				repoLastTmux[repo] = i
			} else {
				if _, exists := repoFirstNonTmux[repo]; !exists {
					repoFirstNonTmux[repo] = i
				}
			}
		}

		// Within each repo, tmux should come before non-tmux
		for repo, lastTmux := range repoLastTmux {
			if firstNonTmux, exists := repoFirstNonTmux[repo]; exists {
				if lastTmux > firstNonTmux {
					t.Errorf("Repo %q: tmux session at pos %d after non-tmux at pos %d", repo, lastTmux, firstNonTmux)
				}
			}
		}

		if !tmuxFound {
			t.Error("Expected at least one tmux session in results")
		}
	})

	t.Run("Real-world scenario - many sessions mixed", func(t *testing.T) {
		items := []list.Item{
			// Projects
			sessionItem{session: model.SeshSession{Name: "api-gateway", Src: "projects"}, displayName: "api-gateway"},
			sessionItem{session: model.SeshSession{Name: "frontend-app", Src: "projects"}, displayName: "frontend-app"},
			// Tmux
			sessionItem{session: model.SeshSession{Name: "api-dev", Src: "tmux"}, displayName: "api-dev"},
			sessionItem{session: model.SeshSession{Name: "frontend", Src: "tmux"}, displayName: "frontend"},
			// Config
			sessionItem{session: model.SeshSession{Name: "dotfiles", Src: "config"}, displayName: "dotfiles"},
			// More projects
			sessionItem{session: model.SeshSession{Name: "backend-service", Src: "projects"}, displayName: "backend-service"},
			// More tmux
			sessionItem{session: model.SeshSession{Name: "backend-prod", Src: "tmux"}, displayName: "backend-prod"},
			// Zoxide
			sessionItem{session: model.SeshSession{Name: "Downloads", Src: "zoxide"}, displayName: "Downloads"},
		}
		targets := []string{"api-gateway", "frontend-app", "api-dev", "frontend", "dotfiles", "backend-service", "backend-prod", "Downloads"}

		testSearches := []struct {
			term string
		}{
			{"api"},
			{"front"},
			{"back"},
			{"end"},
			{"a"},
			{"dev"},
		}

		for _, search := range testSearches {
			filter := tmuxFirstFilter(items)
			ranks := filter(search.term, targets)

			if len(ranks) == 0 {
				continue // No matches is fine
			}

			// Verify per-repo tmux-first grouping
			repoLastTmux := make(map[string]int)
			repoFirstNonTmux := make(map[string]int)

			for i, rank := range ranks {
				item := items[rank.Index].(sessionItem)
				repo := item.session.Name
				if strings.Contains(repo, "/") {
					repo = strings.SplitN(repo, "/", 2)[0]
				}
				if item.session.Src == "tmux" {
					repoLastTmux[repo] = i
				} else {
					if _, exists := repoFirstNonTmux[repo]; !exists {
						repoFirstNonTmux[repo] = i
					}
				}
			}

			// Within each repo group, tmux must come before non-tmux
			for repo, lastTmux := range repoLastTmux {
				if firstNonTmux, exists := repoFirstNonTmux[repo]; exists {
					if lastTmux > firstNonTmux {
						t.Errorf("Search '%s': repo %q has tmux at pos %d after non-tmux at pos %d",
							search.term, repo, lastTmux, firstNonTmux)

						// Print the actual order for debugging
						t.Logf("Results for '%s':", search.term)
						for j, rank := range ranks {
							item := items[rank.Index].(sessionItem)
							t.Logf("  %d. %s (%s)", j, item.session.Name, item.session.Src)
						}
					}
				}
			}

			// Also verify repo grouping: items from same repo should be adjacent
			seen := make(map[string]bool)
			lastRepo := ""
			for _, rank := range ranks {
				item := items[rank.Index].(sessionItem)
				repo := item.session.Name
				if strings.Contains(repo, "/") {
					repo = strings.SplitN(repo, "/", 2)[0]
				}
				if repo != lastRepo {
					if seen[repo] {
						t.Errorf("Search '%s': repo %q appeared again after leaving — results not grouped", search.term, repo)
						break
					}
					seen[repo] = true
					lastRepo = repo
				}
			}
		}
	})
}

func TestTmuxFirstFilterGroupsByRepo(t *testing.T) {
	items := []list.Item{
		sessionItem{session: model.SeshSession{Name: "chase-monorepo", Src: "tmux"}, displayName: "chase-monorepo"},
		sessionItem{session: model.SeshSession{Name: "frontend-monorepo", Src: "projects"}, displayName: "frontend-monorepo"},
		sessionItem{session: model.SeshSession{Name: "frontend-monorepo/develop", Src: "projects"}, displayName: "frontend-monorepo/develop"},
		sessionItem{session: model.SeshSession{Name: "chase-monorepo/feature-cdk", Src: "tmux"}, displayName: "chase-monorepo/feature-cdk"},
		sessionItem{session: model.SeshSession{Name: "chase-monorepo/review", Src: "projects"}, displayName: "chase-monorepo/review"},
		sessionItem{session: model.SeshSession{Name: "chase-monorepo/develop", Src: "projects"}, displayName: "chase-monorepo/develop"},
	}

	targets := make([]string, len(items))
	for i, item := range items {
		targets[i] = item.(sessionItem).FilterValue()
	}

	filter := tmuxFirstFilter(items)
	ranks := filter("monorepo", targets)

	if len(ranks) == 0 {
		t.Fatal("expected results")
	}

	// Verify grouping: items from same repo should be adjacent
	type result struct {
		name string
		repo string
	}
	results := make([]result, 0, len(ranks))
	for _, rank := range ranks {
		si := items[rank.Index].(sessionItem)
		repo := si.session.Name
		if strings.Contains(repo, "/") {
			repo = strings.SplitN(repo, "/", 2)[0]
		}
		results = append(results, result{name: si.session.Name, repo: repo})
	}

	// Check that once we leave a repo group, we don't come back to it
	seen := make(map[string]bool)
	lastRepo := ""
	for _, r := range results {
		if r.repo != lastRepo {
			if seen[r.repo] {
				t.Errorf("repo %q appeared again after leaving — results not grouped. Order: %v", r.repo, results)
				break
			}
			seen[r.repo] = true
			lastRepo = r.repo
		}
	}

	// Within chase-monorepo group, tmux items should come first
	chaseResults := []result{}
	for _, r := range results {
		if r.repo == "chase-monorepo" {
			chaseResults = append(chaseResults, r)
		}
	}
	if len(chaseResults) >= 2 {
		// First chase result should be tmux
		firstChaseName := chaseResults[0].name
		foundFirstInTmux := false
		for _, item := range items {
			if si, ok := item.(sessionItem); ok && si.session.Name == firstChaseName && si.session.Src == "tmux" {
				foundFirstInTmux = true
			}
		}
		if !foundFirstInTmux {
			t.Errorf("first chase-monorepo result should be tmux, got %q", firstChaseName)
		}
	}
}
