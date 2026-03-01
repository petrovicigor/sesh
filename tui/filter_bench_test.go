package tui

import (
	"testing"

	"charm.land/bubbles/v2/list"
	"github.com/joshmedeski/sesh/v2/model"
)

// BenchmarkDefaultFilter benchmarks Bubble Tea's default filter
func BenchmarkDefaultFilter(b *testing.B) {
	items := make([]list.Item, 100)
	targets := make([]string, 100)

	// Create 100 sessions (mix of tmux and projects)
	for i := 0; i < 100; i++ {
		src := "projects"
		if i%3 == 0 {
			src = "tmux"
		}
		name := "session-" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26))
		items[i] = sessionItem{
			session:     model.SeshSession{Name: name, Src: src},
			displayName: name,
		}
		targets[i] = name
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = list.DefaultFilter("ses", targets)
	}
}

// BenchmarkSeshFilter benchmarks our custom filter
func BenchmarkSeshFilter(b *testing.B) {
	items := make([]list.Item, 100)
	targets := make([]string, 100)

	// Create 100 sessions (mix of tmux and projects)
	for i := 0; i < 100; i++ {
		src := "projects"
		if i%3 == 0 {
			src = "tmux"
		}
		name := "session-" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26))
		items[i] = sessionItem{
			session:     model.SeshSession{Name: name, Src: src},
			displayName: name,
		}
		targets[i] = name
	}

	filter := seshFilter(items, nil, nil, GroupByPackage)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter("ses", targets)
	}
}

// BenchmarkSeshFilterSmall benchmarks with realistic session count (20)
func BenchmarkSeshFilterSmall(b *testing.B) {
	items := make([]list.Item, 20)
	targets := make([]string, 20)

	// More realistic: 20 sessions
	for i := 0; i < 20; i++ {
		src := "projects"
		if i%2 == 0 {
			src = "tmux"
		}
		name := "session-" + string(rune('a'+i%26))
		items[i] = sessionItem{
			session:     model.SeshSession{Name: name, Src: src},
			displayName: name,
		}
		targets[i] = name
	}

	filter := seshFilter(items, nil, nil, GroupByPackage)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter("ses", targets)
	}
}

// BenchmarkDefaultFilterSmall benchmarks default filter with 20 sessions
func BenchmarkDefaultFilterSmall(b *testing.B) {
	targets := make([]string, 20)

	for i := 0; i < 20; i++ {
		targets[i] = "session-" + string(rune('a'+i%26))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = list.DefaultFilter("ses", targets)
	}
}

// BenchmarkFilterNoMatches benchmarks worst case (no matches)
func BenchmarkFilterNoMatches(b *testing.B) {
	items := make([]list.Item, 100)
	targets := make([]string, 100)

	for i := 0; i < 100; i++ {
		src := "projects"
		if i%3 == 0 {
			src = "tmux"
		}
		name := "session-" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26))
		items[i] = sessionItem{
			session:     model.SeshSession{Name: name, Src: src},
			displayName: name,
		}
		targets[i] = name
	}

	filter := seshFilter(items, nil, nil, GroupByPackage)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Search for something that doesn't exist
		_ = filter("xyz123notfound", targets)
	}
}

// BenchmarkFilterAllMatches benchmarks when everything matches
func BenchmarkFilterAllMatches(b *testing.B) {
	items := make([]list.Item, 50)
	targets := make([]string, 50)

	for i := 0; i < 50; i++ {
		src := "projects"
		if i%3 == 0 {
			src = "tmux"
		}
		name := "session-common-" + string(rune('a'+i%26))
		items[i] = sessionItem{
			session:     model.SeshSession{Name: name, Src: src},
			displayName: name,
		}
		targets[i] = name
	}

	filter := seshFilter(items, nil, nil, GroupByPackage)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Search for something that matches everything
		_ = filter("ses", targets)
	}
}

// BenchmarkFuzzyScore benchmarks the custom fuzzy scorer with realistic targets
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

// BenchmarkFuzzyScoreNoMatch benchmarks the scorer when nothing matches
func BenchmarkFuzzyScoreNoMatch(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fuzzyScore("xyz123", "chase-search/develop")
	}
}
