# Frecency Tiebreaker for TUI Filter Scoring

**Date**: 2026-02-25
**Branch**: feature/bubble-tea-tui
**Status**: Approved

## Overview

Add lightweight frecency (frequency + recency) tracking to break ties in the TUI's fuzzy filter results. Sessions used more often and more recently rank higher when fuzzy scores are within 5% of each other.

## Data Model

Extend the existing `recent/recent.go` persistence (`~/.config/sesh/recent_sessions.json`) to store access count alongside timestamp.

**Current format:**
```json
{"chase-search/develop": "2026-02-25T10:30:00Z"}
```

**New format:**
```json
{"chase-search/develop": {"t": "2026-02-25T10:30:00Z", "n": 42}}
```

**Migration:** On read, if value is a plain string (old format), convert to `{"t": <string>, "n": 1}`. Transparent, no user action.

**Limit:** 50 sessions max (unchanged). Prune by lowest frecency score (not oldest timestamp).

## Scoring Formula

```
frecencyScore = count / (hours_since_last_use + 1)
```

Behavior:
- 10 uses, 1 hour ago: `10/2 = 5.0`
- 10 uses, 24 hours ago: `10/25 = 0.4`
- 2 uses, 5 min ago: `2/1.08 = 1.85`
- 50 uses, 1 week ago: `50/169 = 0.3`

Recency-dominant: recent usage always wins over stale frequency.

## Integration with Filter Scoring

Frecency is a **light tiebreaker only** — it only matters when two results have fuzzy scores within 5% of each other.

**Tiebreaker order (within 5% score band):**
1. Higher frecency score
2. Tmux source wins
3. Stable original order

**Example:**
```
Query: "dev"
dev-tools             → fuzzy: 0.98, frecency: 0.0  → rank 1 (score gap > 5%)
chase-search/develop  → fuzzy: 0.95, frecency: 5.0  → rank 2 (frecency wins tie)
geoip/develop         → fuzzy: 0.96, frecency: 0.3  → rank 3
```

## Recording

`RecordSession(name)` in `recent/recent.go` already called by `connector/connect.go` on every session connection. Extended to also increment count.

## Plumbing

1. `recent/recent.go` — New `GetFrecencyScores() map[string]float64` method
2. `seshcli/tui.go` — Calls `GetFrecencyScores()`, passes to TUI model
3. `tui/model.go` — `newModel()` accepts frecency map, passes to `seshFilter`
4. `seshFilter` — Uses frecency in sort comparator for tiebreaking

## Scope Boundaries

Not building:
- Separate frecency database (reuse existing file)
- Config options for decay tuning
- Frecency in unfiltered list ordering
- Frecency in CLI `sesh list`
- TUI-side recording (connector already handles it)
- Background tracking or daemons
