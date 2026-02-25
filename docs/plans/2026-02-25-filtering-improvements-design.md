# TUI Filtering & Display Improvements Design

**Date**: 2026-02-25
**Branch**: feature/bubble-tea-tui
**Status**: Approved

## Overview

Four improvements to the TUI's filtering, display, and search experience:

1. Active/inactive separator in the session list
2. Source badges shown during filtered results
3. fzf-style fuzzy scoring replacing Bubble Tea's default filter
4. Bold match highlighting on filtered results

## Feature 1: Active/Inactive Separator

### Behavior

A dim, non-selectable divider line between live tmux sessions and everything else in the unfiltered list:

```
⚡ dotfiles
⚡ chase-search/develop
⚡ chase-search/feature-x
 ─── available ───────────────
📁 geoip (+3)
📁 sesh (+2)
📁 chase-cognito
```

### Implementation

- New `separatorItem` type implementing `list.Item` interface
- `FilterValue()` returns `""` (never matches any filter)
- `Title()` / `Description()` return empty strings
- Delegate renders it as a dim horizontal line with "available" label
- Inserted by `buildDisplayItems()` between last tmux and first non-tmux item
- Cursor navigation skips over it (intercept Up/Down keys, if current+1 is separator, move +2)
- Worktree grouping works independently within each section
- Separator disappears when filter text is non-empty (flat results mode)
- If no active tmux sessions exist, no separator is shown

## Feature 2: Source Badges During Filtering

### Behavior

When the user types a filter query, results flatten (as today) but each result shows an inline source badge:

```
[typing "dev"]
⚡ chase-search/develop        tmux
📁 geoip/develop               projects
📁 sesh/develop                projects
📁 dev-tools                   zoxide
```

### Implementation

- Badge rendered by delegate only when `filterState == Filtering && filterValue != ""`
- Badge text is `session.Src` value: `tmux`, `config`, `projects`, `zoxide`, `tmuxinator`
- Styled in a muted/dim color (e.g., lipgloss color "240") to not compete with session name
- Right-aligned within the available list width
- Name is truncated if needed to leave room for badge (badge gets priority since names are already recognizable from partial display)
- Badges disappear when filter is cleared and grouped view returns
- Active/inactive separator also disappears during filtering

## Feature 3: fzf-Style Fuzzy Scoring

### Behavior

Replace Bubble Tea's `list.DefaultFilter` with a custom scoring function that produces results matching fzf's intuitive behavior.

### Scoring Hierarchy

1. **Exact substring** (score 1.0) - typed string appears verbatim in name
2. **Prefix match** (score 0.9) - name starts with typed string
3. **Word-boundary match** (score 0.8) - match starts at `/`, `-`, `_`, or case transition
4. **Consecutive characters** (bonus multiplier) - each additional consecutive match char increases score
5. **Scattered single chars** (base score only) - effectively pushed to bottom of results

### Tiebreakers (when scores are equal)

1. Active tmux session wins over inactive
2. More recent usage wins (from recency data already available in lister)

### Implementation

- Custom `FilterFunc` signature: `func(term string, targets []string) []list.Rank`
- Returns `list.Rank` with `Index` (item index) and `MatchedIndexes` (character positions in name)
- `MatchedIndexes` feeds directly into highlighting (Feature 4)
- Word boundaries detected at: `/`, `-`, `_`, uppercase after lowercase
- Replaces current `tmuxFirstFilter` - the repo-grouping behavior of that function is no longer needed since filtered results are flat with badges
- Pure Go, no external dependencies
- Performance: operates on short strings (5-30 chars) with typically <100 items, so scoring is sub-millisecond

### Examples

| Query | Top results | Why |
|-------|-------------|-----|
| `ch` | chase-search, chase-cognito, chase-api | Prefix match |
| `dev` | */develop (all repos) | Exact substring in branch |
| `sd` | sesh/develop | Word-boundary: **s**esh/**d**evelop |
| `gf` | geoip/feature-x | Word-boundary: **g**eoip/**f**eature-x |
| `dot` | dotfiles | Prefix match |

## Feature 4: Bold Match Highlighting

### Behavior

Matched characters in the session name are highlighted with bold + a warm accent color:

```
[typing "dev"]
⚡ chase-search/develop        tmux
                    ^^^  bold + accent color
📁 geoip/develop               projects
         ^^^
```

### Implementation

- Replace current `filterMatchStyle` (underline only) with bold + foreground color
- Use a warm accent (e.g., color "214" orange/gold) that contrasts with both normal text and selection highlight
- Two style variants:
  - `filterMatchStyle`: bold + accent color (for unselected items)
  - `filterMatchSelectedStyle`: bold + brighter variant (for selected/highlighted item, to maintain contrast against selection background color "170")
- Match indices come from the scoring function (Feature 3) via `list.Rank.MatchedIndexes`
- Applied using existing `lipgloss.StyleRunes()` call in delegate
- Highlighting applies to session name only, not source badge or icon prefix

## Scope Boundaries (YAGNI)

Not building:
- Prefix-based project grouping (chase-*)
- User-defined groups in sesh.toml
- Source-type section headers (beyond the active/inactive separator)
- Tab-based view switching between sources
- Changes to default (unfiltered) sort order
- Scope-aware matching (searching by path or source type)
- Sticky/floating section headers

## Dependencies

```
Feature 3 (scoring) ──produces match indices──→ Feature 4 (highlighting)

Feature 1 (separator)  ← independent
Feature 2 (badges)     ← independent

Feature 1 visible only when NOT filtering
Feature 2 visible only when filtering
```

## Key Risks

- **Separator cursor skipping**: Bubble Tea's list doesn't natively support non-selectable items. Need to intercept key events and adjust cursor position. Similar pattern already exists for worktree group expand/collapse.
- **Custom scoring correctness**: Must handle edge cases (empty query, special characters, very short names). Mitigated by comprehensive tests modeled after existing `filter_test.go`.
- **Badge width calculation**: Must account for ANSI color codes in name when calculating available space. Existing `iconPrefix` handling provides a pattern for this.
