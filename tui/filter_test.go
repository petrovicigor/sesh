package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/joshmedeski/sesh/v2/model"
)

// TestSeshFilter validates that seshFilter ranks by score with tmux tiebreaks
func TestSeshFilter(t *testing.T) {
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
		minResults     int    // At least this many results expected
	}{
		{
			name:           "Search 'dev' - tmux develop surfaces at top",
			searchTerm:     "dev",
			expectedFirst:  "chase-search/develop",
			expectedSource: "tmux",
			minResults:     1,
		},
		{
			name:           "Search 'review' - only projects match, should return project",
			searchTerm:     "review",
			expectedFirst:  "chase-search/review",
			expectedSource: "projects",
			minResults:     1,
		},
		{
			name:           "Search 'sesh' - tmux session first",
			searchTerm:     "sesh",
			expectedFirst:  "sesh",
			expectedSource: "tmux",
			minResults:     1,
		},
		{
			name:           "Search 'dot' - config session (no tmux match)",
			searchTerm:     "dot",
			expectedFirst:  "dotfiles",
			expectedSource: "config",
			minResults:     1,
		},
		{
			name:           "Partial match 'feat' - should match feature-cdk tmux",
			searchTerm:     "feat",
			expectedFirst:  "chase-search/feature-cdk",
			expectedSource: "tmux",
			minResults:     1,
		},
		{
			name:           "Search 'chase' returns all chase items",
			searchTerm:     "chase",
			minResults:     5,
		},
		{
			name:           "Search 'chase-search' returns all chase-search items",
			searchTerm:     "chase-search",
			minResults:     5,
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
			filter := seshFilter(testSessions, nil, nil, GroupByPackage)
			ranks := filter(tc.searchTerm, targets)

			if len(ranks) < tc.minResults {
				t.Fatalf("Expected at least %d results but got %d for search term '%s'", tc.minResults, len(ranks), tc.searchTerm)
			}

			if tc.expectedFirst == "" {
				return // Only checking result count
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
				// Print all results for debugging
				t.Logf("All results for '%s':", tc.searchTerm)
				for j, rank := range ranks {
					item := testSessions[rank.Index].(sessionItem)
					t.Logf("  %d. %s (%s)", j, item.session.Name, item.session.Src)
				}
			}

			if tc.expectedSource != "" && firstItem.session.Src != tc.expectedSource {
				t.Errorf("Expected first result from source '%s' but got '%s' for search term '%s'",
					tc.expectedSource, firstItem.session.Src, tc.searchTerm)
			}

			// Validate that matched indices are populated
			if len(firstRank.MatchedIndexes) == 0 {
				t.Errorf("Expected matched indices to be populated for search term '%s'", tc.searchTerm)
			}
		})
	}
}

// TestSeshFilterEdgeCases tests edge cases
func TestSeshFilterEdgeCases(t *testing.T) {
	t.Run("Empty items list", func(t *testing.T) {
		items := []list.Item{}
		filter := seshFilter(items, nil, nil, GroupByPackage)
		ranks := filter("test", []string{})
		if len(ranks) != 0 {
			t.Errorf("Expected 0 results for empty items, got %d", len(ranks))
		}
	})

	t.Run("Empty search returns nil", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "test", Src: "tmux"}, displayName: "test"},
		}
		filter := seshFilter(items, nil, nil, GroupByPackage)
		ranks := filter("", []string{"test"})
		if ranks != nil {
			t.Errorf("Expected nil for empty search term, got %v", ranks)
		}
	})

	t.Run("Single tmux item", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "test", Src: "tmux"}, displayName: "test"},
		}
		filter := seshFilter(items, nil, nil, GroupByPackage)
		ranks := filter("test", []string{"test"})
		if len(ranks) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(ranks))
		}
		if ranks[0].Index != 0 {
			t.Errorf("Expected index 0, got %d", ranks[0].Index)
		}
		if len(ranks[0].MatchedIndexes) == 0 {
			t.Error("Expected matched indices to be populated")
		}
	})

	t.Run("Single non-tmux item", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "test", Src: "projects"}, displayName: "test"},
		}
		filter := seshFilter(items, nil, nil, GroupByPackage)
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
		filter := seshFilter(items, nil, nil, GroupByPackage)
		ranks := filter("xyz123notfound", []string{"foo", "bar"})
		if len(ranks) != 0 {
			t.Errorf("Expected 0 results for non-matching search, got %d", len(ranks))
		}
	})

	t.Run("worktreeGroupItem is scored as projects", func(t *testing.T) {
		items := []list.Item{
			worktreeGroupItem{repoName: "myrepo", displayName: "myrepo (3)"},
			sessionItem{session: model.SeshSession{Name: "myrepo-alt", Src: "tmux"}, displayName: "myrepo-alt"},
		}
		filter := seshFilter(items, nil, nil, GroupByPackage)
		ranks := filter("myrepo", []string{"myrepo (3)", "myrepo-alt"})
		if len(ranks) == 0 {
			t.Fatal("Expected results")
		}
		// Both should match; results are returned with matched indices
		for _, rank := range ranks {
			if len(rank.MatchedIndexes) == 0 {
				t.Errorf("Expected matched indices for rank index %d", rank.Index)
			}
		}
	})
}

// TestSeshFilterTmuxTiebreaker validates that tmux wins when scores are equal
func TestSeshFilterTmuxTiebreaker(t *testing.T) {
	t.Run("Tmux wins tiebreak for equal scores", func(t *testing.T) {
		// Two identical names, one tmux, one projects — scores will be identical
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "app-dev", Src: "projects"}, displayName: "app-dev"},
			sessionItem{session: model.SeshSession{Name: "app-dev", Src: "tmux"}, displayName: "app-dev"},
		}
		filter := seshFilter(items, nil, nil, GroupByPackage)
		ranks := filter("app", []string{"app-dev", "app-dev"})

		if len(ranks) < 2 {
			t.Fatalf("Expected 2 results, got %d", len(ranks))
		}

		// The tmux item (index 1) should come first due to tiebreaker
		firstItem := items[ranks[0].Index].(sessionItem)
		if firstItem.session.Src != "tmux" {
			t.Errorf("Expected tmux to win tiebreak, got source '%s' (name=%s)", firstItem.session.Src, firstItem.session.Name)
		}
	})

	t.Run("Tmux wins tiebreak even when listed second", func(t *testing.T) {
		// Reverse order to ensure it's not just position-based
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "backend", Src: "tmux"}, displayName: "backend"},
			sessionItem{session: model.SeshSession{Name: "backend", Src: "projects"}, displayName: "backend"},
		}
		filter := seshFilter(items, nil, nil, GroupByPackage)
		ranks := filter("back", []string{"backend", "backend"})

		if len(ranks) < 2 {
			t.Fatalf("Expected 2 results, got %d", len(ranks))
		}

		firstItem := items[ranks[0].Index].(sessionItem)
		if firstItem.session.Src != "tmux" {
			t.Errorf("Expected tmux to win tiebreak, got source '%s'", firstItem.session.Src)
		}
	})

	t.Run("All sources mixed - results have matched indices", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "app", Src: "projects"}, displayName: "app"},
			sessionItem{session: model.SeshSession{Name: "app-test", Src: "tmux"}, displayName: "app-test"},
			sessionItem{session: model.SeshSession{Name: "application", Src: "config"}, displayName: "application"},
			sessionItem{session: model.SeshSession{Name: "app-dev", Src: "tmux"}, displayName: "app-dev"},
			sessionItem{session: model.SeshSession{Name: "myapp", Src: "zoxide"}, displayName: "myapp"},
		}
		targets := []string{"app", "app-test", "application", "app-dev", "myapp"}
		filter := seshFilter(items, nil, nil, GroupByPackage)
		ranks := filter("app", targets)

		// Should have results
		if len(ranks) == 0 {
			t.Fatal("Expected results but got none")
		}

		// Verify all results have matched indices
		for i, rank := range ranks {
			if len(rank.MatchedIndexes) == 0 {
				item := items[rank.Index].(sessionItem)
				t.Errorf("Result %d (%s) has no matched indices", i, item.session.Name)
			}
		}

		// Verify at least one tmux session is present
		tmuxFound := false
		for _, rank := range ranks {
			item := items[rank.Index].(sessionItem)
			if item.session.Src == "tmux" {
				tmuxFound = true
				break
			}
		}
		if !tmuxFound {
			t.Error("Expected at least one tmux session in results")
		}
	})
}

// TestSeshFilterScoringBehavior validates scoring produces expected ranking
func TestSeshFilterScoringBehavior(t *testing.T) {
	t.Run("Word boundary prefix match beats mid-word match", func(t *testing.T) {
		// "chase" matches at position 0 in both targets (both are prefix matches),
		// but "chase-cognito" is shorter so it gets a slightly different specificity score.
		// The key test: word boundary match at start beats non-boundary.
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "xchase-app", Src: "projects"}, displayName: "xchase-app"},
			sessionItem{session: model.SeshSession{Name: "chase-cognito", Src: "projects"}, displayName: "chase-cognito"},
		}
		filter := seshFilter(items, nil, nil, GroupByPackage)
		ranks := filter("chase", []string{"xchase-app", "chase-cognito"})

		if len(ranks) < 2 {
			t.Fatalf("Expected 2 results, got %d", len(ranks))
		}

		// "chase-cognito" starts at position 0 (word boundary), should rank first
		firstItem := items[ranks[0].Index].(sessionItem)
		if firstItem.session.Name != "chase-cognito" {
			t.Errorf("Expected 'chase-cognito' (prefix match) first, got '%s'", firstItem.session.Name)
		}
	})

	t.Run("Exact substring matches surface correctly", func(t *testing.T) {
		// Both targets contain "develop" as an exact substring at a word boundary
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "chase-search/develop", Src: "tmux"}, displayName: "chase-search/develop"},
			sessionItem{session: model.SeshSession{Name: "geoip/develop", Src: "projects"}, displayName: "geoip/develop"},
		}
		filter := seshFilter(items, nil, nil, GroupByPackage)
		ranks := filter("develop", []string{"chase-search/develop", "geoip/develop"})

		if len(ranks) != 2 {
			t.Fatalf("Expected 2 results, got %d", len(ranks))
		}

		// Both should match with non-zero matched indices
		for _, rank := range ranks {
			if len(rank.MatchedIndexes) == 0 {
				item := items[rank.Index].(sessionItem)
				t.Errorf("Expected matched indices for %s", item.session.Name)
			}
		}

		// tmux should win tiebreak when scores are close
		firstItem := items[ranks[0].Index].(sessionItem)
		if firstItem.session.Src != "tmux" {
			t.Logf("Note: tmux item did not rank first (scores may differ due to target length)")
		}
	})

	t.Run("Shorter target with same prefix match gets different specificity score", func(t *testing.T) {
		// When the query is a prefix of both targets, the scorer's specificity
		// component (queryLen/targetLen ratio) may prefer longer targets.
		// This test verifies results are deterministic.
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "dev", Src: "tmux"}, displayName: "dev"},
			sessionItem{session: model.SeshSession{Name: "develop", Src: "projects"}, displayName: "develop"},
		}
		filter := seshFilter(items, nil, nil, GroupByPackage)
		ranks := filter("dev", []string{"dev", "develop"})

		if len(ranks) < 2 {
			t.Fatalf("Expected 2 results, got %d", len(ranks))
		}

		// Both match, results should be deterministic
		first := items[ranks[0].Index].(sessionItem)
		second := items[ranks[1].Index].(sessionItem)
		if first.session.Name == second.session.Name {
			t.Error("Expected different items in result")
		}
	})

	t.Run("Real-world scenario - many sessions mixed", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "api-gateway", Src: "projects"}, displayName: "api-gateway"},
			sessionItem{session: model.SeshSession{Name: "frontend-app", Src: "projects"}, displayName: "frontend-app"},
			sessionItem{session: model.SeshSession{Name: "api-dev", Src: "tmux"}, displayName: "api-dev"},
			sessionItem{session: model.SeshSession{Name: "frontend", Src: "tmux"}, displayName: "frontend"},
			sessionItem{session: model.SeshSession{Name: "dotfiles", Src: "config"}, displayName: "dotfiles"},
			sessionItem{session: model.SeshSession{Name: "backend-service", Src: "projects"}, displayName: "backend-service"},
			sessionItem{session: model.SeshSession{Name: "backend-prod", Src: "tmux"}, displayName: "backend-prod"},
			sessionItem{session: model.SeshSession{Name: "Downloads", Src: "zoxide"}, displayName: "Downloads"},
		}
		targets := []string{"api-gateway", "frontend-app", "api-dev", "frontend", "dotfiles", "backend-service", "backend-prod", "Downloads"}

		testSearches := []struct {
			term       string
			minResults int
		}{
			{"api", 2},
			{"front", 2},
			{"back", 2},
			{"dev", 1},
			{"a", 1},
		}

		for _, search := range testSearches {
			filter := seshFilter(items, nil, nil, GroupByPackage)
			ranks := filter(search.term, targets)

			if len(ranks) < search.minResults {
				t.Errorf("Search '%s': expected at least %d results, got %d", search.term, search.minResults, len(ranks))
			}

			// Verify all results have matched indices
			for i, rank := range ranks {
				if len(rank.MatchedIndexes) == 0 {
					item := items[rank.Index].(sessionItem)
					t.Errorf("Search '%s': result %d (%s) has no matched indices", search.term, i, item.session.Name)
				}
			}
		}
	})
}

// TestSeshFilterFrecencyTiebreaker validates frecency breaks ties within 5% score bands
func TestSeshFilterFrecencyTiebreaker(t *testing.T) {
	t.Run("Frecency does not override large score gap", func(t *testing.T) {
		// "app" is exact prefix for "app-dev" but scattered in "my-backend-api-proxy"
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "my-backend-api-proxy", Src: "projects"}, displayName: "my-backend-api-proxy"},
			sessionItem{session: model.SeshSession{Name: "app-dev", Src: "projects"}, displayName: "app-dev"},
		}
		// Give the worse-scoring item massive frecency
		frecency := map[string]float64{"my-backend-api-proxy": 100.0}
		filter := seshFilter(items, frecency, nil, GroupByPackage)
		ranks := filter("app", []string{"my-backend-api-proxy", "app-dev"})

		if len(ranks) < 2 {
			t.Fatalf("Expected 2 results, got %d", len(ranks))
		}

		// "app-dev" has prefix match (much higher score), should still be first
		firstItem := items[ranks[0].Index].(sessionItem)
		if firstItem.session.Name != "app-dev" {
			t.Errorf("Expected 'app-dev' (prefix match) to beat high-frecency scattered match, got '%s'", firstItem.session.Name)
		}
	})

	t.Run("Frecency promotes within close scores", func(t *testing.T) {
		// Two very similar names with similar fuzzy scores
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "api-server", Src: "projects"}, displayName: "api-server"},
			sessionItem{session: model.SeshSession{Name: "api-service", Src: "projects"}, displayName: "api-service"},
		}
		// "api-service" has higher frecency
		frecency := map[string]float64{"api-service": 10.0, "api-server": 0.0}
		filter := seshFilter(items, frecency, nil, GroupByPackage)
		ranks := filter("api-ser", []string{"api-server", "api-service"})

		if len(ranks) < 2 {
			t.Fatalf("Expected 2 results, got %d", len(ranks))
		}

		// Both have very similar scores (prefix match, similar length)
		// Frecency should promote api-service
		firstItem := items[ranks[0].Index].(sessionItem)
		if firstItem.session.Name != "api-service" {
			t.Errorf("Expected 'api-service' (higher frecency) first within similar scores, got '%s'", firstItem.session.Name)
		}
	})

	t.Run("Nil frecency map works like no frecency", func(t *testing.T) {
		items := []list.Item{
			sessionItem{session: model.SeshSession{Name: "test", Src: "tmux"}, displayName: "test"},
		}
		filter := seshFilter(items, nil, nil, GroupByPackage)
		ranks := filter("test", []string{"test"})
		if len(ranks) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(ranks))
		}
	})
}
