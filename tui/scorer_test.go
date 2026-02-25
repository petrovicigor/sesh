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

	t.Run("prefix match scores higher than mid match", func(t *testing.T) {
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
		if s1 <= 0 || s2 <= 0 || s3 <= 0 {
			t.Errorf("all should match: s1=%f s2=%f s3=%f", s1, s2, s3)
		}
	})

	t.Run("real world: exact prefix beats scattered", func(t *testing.T) {
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
