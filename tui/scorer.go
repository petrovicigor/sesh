package tui

import "strings"

// fuzzyScore scores how well query matches target using fzf-style fuzzy matching.
// Returns (score, matchedIndices) where higher score = better match.
// Returns (0, nil) for no match.
// Matching is case-insensitive. Indices are byte positions in the original target.
func fuzzyScore(query, target string) (float64, []int) {
	if len(query) == 0 {
		return 0, nil
	}
	if len(query) > len(target) {
		return 0, nil
	}

	lowerQuery := strings.ToLower(query)
	lowerTarget := strings.ToLower(target)

	// Try both exact substring and fuzzy match, pick the best score.
	var bestScore float64
	var bestIndices []int

	// --- Candidate 1: Exact substring match ---
	exactPos := strings.Index(lowerTarget, lowerQuery)
	if exactPos >= 0 {
		indices := make([]int, len(query))
		for i := range query {
			indices[i] = exactPos + i
		}
		score := computeScore(indices, target)
		bestScore = score
		bestIndices = indices
	}

	// --- Candidate 2: Fuzzy match with word-boundary preference ---
	if fuzzyIndices := fuzzyMatch(lowerQuery, lowerTarget, target); fuzzyIndices != nil {
		fuzzyS := computeScore(fuzzyIndices, target)
		if fuzzyS > bestScore {
			bestScore = fuzzyS
			bestIndices = fuzzyIndices
		}
	}

	if bestIndices == nil {
		return 0, nil
	}
	return bestScore, bestIndices
}

// isWordBoundary returns true if position pos in target starts a new "word".
// A character is at a word boundary if:
//   - It is the first character (pos == 0)
//   - The previous character is a separator: / - _ . or space
//   - There is a lowercase-to-uppercase transition (camelCase)
func isWordBoundary(target string, pos int) bool {
	if pos == 0 {
		return true
	}
	prev := target[pos-1]
	switch prev {
	case '/', '-', '_', '.', ' ':
		return true
	}
	// camelCase transition: previous is lowercase, current is uppercase
	cur := target[pos]
	if prev >= 'a' && prev <= 'z' && cur >= 'A' && cur <= 'Z' {
		return true
	}
	return false
}

// fuzzyMatch finds the best fuzzy match positions for query in target.
// It uses a two-pass approach:
//  1. Try to match preferring word boundaries and consecutive characters.
//  2. Fall back to greedy left-to-right match.
func fuzzyMatch(lowerQuery, lowerTarget, originalTarget string) []int {
	// First pass: word-boundary-aware matching
	if indices := boundaryAwareMatch(lowerQuery, lowerTarget, originalTarget); indices != nil {
		return indices
	}

	// Second pass: simple greedy left-to-right
	return greedyMatch(lowerQuery, lowerTarget)
}

// boundaryAwareMatch tries to align query characters to word boundaries when possible.
// For each query character, it looks ahead for a word-boundary position that matches,
// preferring boundaries over arbitrary positions.
func boundaryAwareMatch(lowerQuery, lowerTarget, originalTarget string) []int {
	indices := make([]int, 0, len(lowerQuery))
	qi := 0 // query index
	ti := 0 // target index

	for qi < len(lowerQuery) && ti < len(lowerTarget) {
		qc := lowerQuery[qi]

		// Look for the best match position starting from ti.
		// Strategy: scan forward for a word-boundary match first.
		bestPos := -1

		// Check if there is a word-boundary match within a reasonable window.
		for j := ti; j < len(lowerTarget); j++ {
			if lowerTarget[j] == qc {
				if isWordBoundary(originalTarget, j) {
					bestPos = j
					break
				}
				// Record the first non-boundary match as fallback.
				if bestPos == -1 {
					bestPos = j
				}
				// If we already have a non-boundary match and the last matched
				// character was at bestPos-1, prefer consecutive.
				if len(indices) > 0 && j == indices[len(indices)-1]+1 && bestPos != indices[len(indices)-1]+1 {
					bestPos = j
					break
				}
			}
		}

		if bestPos == -1 {
			return nil // query char not found
		}

		indices = append(indices, bestPos)
		qi++
		ti = bestPos + 1
	}

	if qi < len(lowerQuery) {
		return nil
	}
	return indices
}

// greedyMatch does a simple left-to-right scan matching query chars in order.
func greedyMatch(lowerQuery, lowerTarget string) []int {
	indices := make([]int, 0, len(lowerQuery))
	ti := 0
	for qi := 0; qi < len(lowerQuery); qi++ {
		found := false
		for ; ti < len(lowerTarget); ti++ {
			if lowerTarget[ti] == lowerQuery[qi] {
				indices = append(indices, ti)
				ti++
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return indices
}

// computeScore calculates the final score from matched indices.
//
// Scoring model (all bonuses are additive on a base of 0.3):
//
//	Base:         0.30  (any match)
//	Consecutive:  up to 0.40  (ratio of consecutive pairs)
//	Exact substr: 0.15  (all chars consecutive)
//	Prefix:       0.25  (match starts at position 0)
//	Boundary:     first hit 0.25, additional hits 0.12 each (diminishing)
//	Compactness:  up to 0.05  (tightness of match span)
//	Specificity:  up to 0.05  (shorter targets slightly preferred)
//
// The diminishing boundary bonus ensures that:
//   - A 2-char query with both chars on boundaries beats a mid-word exact substring
//     (e.g., "sd" in "sesh/develop" > "sd" in "abcsdxyz")
//   - But a 3-char consecutive match beats 3 scattered boundary hits
//     (e.g., "sea" in "chase-search" > "sea" in "s_e_a_other")
func computeScore(indices []int, target string) float64 {
	if len(indices) == 0 {
		return 0
	}

	queryLen := len(indices)
	targetLen := len(target)

	// Count consecutive pairs and word-boundary hits
	consecutiveCount := 0
	boundaryHits := 0
	for i, idx := range indices {
		if i > 0 && idx == indices[i-1]+1 {
			consecutiveCount++
		}
		if isWordBoundary(target, idx) {
			boundaryHits++
		}
	}

	allConsecutive := queryLen <= 1 || consecutiveCount == queryLen-1

	// Base score
	score := 0.3

	// Consecutive bonus: strong reward for consecutive character runs
	if queryLen > 1 {
		ratio := float64(consecutiveCount) / float64(queryLen-1)
		score += ratio * 0.40
	}

	// Exact substring bonus (all consecutive)
	if allConsecutive && queryLen > 1 {
		score += 0.15
	}

	// Prefix bonus
	if indices[0] == 0 {
		score += 0.25
	}

	// Word-boundary bonus: first boundary hit is strong, additional hits diminish.
	// This prevents many scattered boundary hits from overwhelming consecutive matches,
	// while still ensuring that short queries with all chars on boundaries beat
	// mid-word exact substrings.
	if boundaryHits > 0 {
		score += 0.25 // first boundary hit
		if boundaryHits > 1 {
			score += float64(boundaryHits-1) * 0.12 // additional hits (diminishing)
		}
	}

	// Compactness bonus
	if queryLen > 1 {
		span := indices[queryLen-1] - indices[0]
		if span > 0 {
			score += (float64(queryLen-1) / float64(span)) * 0.05
		} else {
			score += 0.05
		}
	}

	// Specificity bonus: prefer shorter targets
	if targetLen > 0 {
		score += 0.05 * (1.0 - float64(queryLen)/float64(targetLen))
	}

	return score
}
