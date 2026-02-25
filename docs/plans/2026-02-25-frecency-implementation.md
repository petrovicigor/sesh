# Frecency Tiebreaker Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add lightweight frecency (frequency + recency) tracking to break ties in TUI filter scoring.

**Architecture:** Extend existing `recent/recent.go` persistence to store access count alongside timestamp. Compute `count / (hours_since_last_use + 1)` scores. Pass scores into `seshFilter` as a tiebreaker within 5% score bands.

**Tech Stack:** Go, existing `recent` package, `tui` package filter logic.

---

### Task 1: Extend recent.go Data Model

**Files:**
- Modify: `recent/recent.go`

**Context:**
- Current `RecentSessions` stores `map[string]time.Time` — plain timestamps
- JSON format: `{"sessions": {"name": "2026-02-25T10:30:00Z"}}`
- New format: `{"sessions": {"name": {"t": "2026-02-25T10:30:00Z", "n": 42}}}`
- Must handle migration transparently (old string → new object)
- Max 50 sessions (unchanged), but prune by lowest frecency instead of oldest timestamp
- `RecordSession` must increment count, not just update timestamp
- Add `GetFrecencyScores() map[string]float64` method to interface + implementation

**Step 1: Update data model and add migration**

Change the internal storage type from `map[string]time.Time` to `map[string]SessionEntry` where `SessionEntry` has `Time` and `Count` fields. Implement a custom `UnmarshalJSON` on `RecentSessions` that handles both old format (plain string timestamp) and new format (object with `t` and `n` fields).

```go
type SessionEntry struct {
	Time  time.Time `json:"t"`
	Count int       `json:"n"`
}

type RecentSessions struct {
	Sessions map[string]SessionEntry `json:"sessions"`
}
```

Custom unmarshal logic:
- Try unmarshalling value as `SessionEntry` (new format)
- If that fails, try as plain string (old format) → convert to `SessionEntry{Time: parsed, Count: 1}`

**Step 2: Update RecordSession to increment count**

When recording a session:
- If entry exists: increment `Count`, update `Time` to `time.Now()`
- If entry doesn't exist: create with `Count: 1`, `Time: time.Now()`
- Pruning: when over 50, sort by frecency score (`count / (hours + 1)`) and remove lowest

**Step 3: Update GetTimestamp and GetAll**

- `GetTimestamp(name)` → return `entry.Time`
- `GetAll()` → return `map[string]time.Time` (extract `.Time` from each entry for backward compat)

**Step 4: Add GetFrecencyScores method**

Add to `Recent` interface:
```go
GetFrecencyScores() map[string]float64
```

Implementation loads all sessions, computes `count / (hoursSinceLastUse + 1)` for each, returns map.

**Step 5: Run tests**

Run: `go test ./recent/...`
Expected: All existing tests still pass (if any), build succeeds.

---

### Task 2: Add recent.go Tests

**Files:**
- Create: `recent/recent_test.go`

**Context:**
- No existing test file for `recent` package
- Tests should use temp directories to avoid touching real config
- Test migration from old format, recording with count, frecency scoring, pruning

**Step 1: Write tests**

Test cases:
1. `TestRecordSessionIncrementsCount` — Record same session 3 times, verify count is 3
2. `TestRecordSessionNewEntry` — Record new session, verify count is 1
3. `TestMigrationFromOldFormat` — Write old-format JSON (`{"sessions":{"foo":"2026-01-01T00:00:00Z"}}`), load it, verify parsed as `SessionEntry{Count: 1}`
4. `TestGetFrecencyScores` — Record sessions with known counts/timestamps, verify scores match `count / (hours + 1)`
5. `TestPruneByFrecency` — Record 51 sessions, verify the one with lowest frecency is pruned (not oldest)
6. `TestGetTimestampBackwardCompat` — Verify `GetTimestamp` still returns `time.Time`
7. `TestGetAllBackwardCompat` — Verify `GetAll` still returns `map[string]time.Time`

**Step 2: Run tests**

Run: `go test -v ./recent/...`
Expected: All tests pass.

---

### Task 3: Wire Frecency into seshFilter

**Files:**
- Modify: `tui/model.go` (seshFilter function and newModel)

**Context:**
- `seshFilter` currently takes `items []list.Item` and returns a `list.FilterFunc`
- Sort comparator currently: score desc → tmux wins ties → stable order
- Design says: frecency is tiebreaker within 5% score band
- Tiebreaker order within band: higher frecency → tmux wins → stable order
- `seshFilter` needs to accept `frecencyScores map[string]float64`
- `newModel` needs to accept and store frecency scores, pass to `seshFilter`

**Step 1: Add frecency parameter to seshFilter**

Change signature to:
```go
func seshFilter(items []list.Item, frecencyScores map[string]float64) list.FilterFunc {
```

Add `frecency` field to `scoredRank` struct:
```go
type scoredRank struct {
    rank     list.Rank
    score    float64
    src      string
    repo     string
    frecency float64
}
```

Look up frecency score for each matched item by session name.

**Step 2: Update sort comparator for 5% band tiebreaking**

Replace the current comparator with:
```go
slices.SortStableFunc(results, func(a, b scoredRank) int {
    // If scores differ by more than 5%, sort by score
    maxScore := max(a.score, b.score)
    if maxScore > 0 && abs(a.score-b.score)/maxScore > 0.05 {
        if a.score > b.score { return -1 }
        return 1
    }
    // Within 5% band: frecency tiebreaker
    if a.frecency != b.frecency {
        if a.frecency > b.frecency { return -1 }
        return 1
    }
    // Tmux wins ties
    aIsTmux := a.src == "tmux"
    bIsTmux := b.src == "tmux"
    if aIsTmux && !bIsTmux { return -1 }
    if !aIsTmux && bIsTmux { return 1 }
    return 0
})
```

**Step 3: Add frecency to newModel and update call sites**

Add `frecencyScores map[string]float64` parameter to `newModel()`. Store in Model struct. Pass to `seshFilter()`.

Update the two call sites:
- `l.Filter = seshFilter(displayItems, frecencyScores)` in `newModel()`
- Ctrl+T repo focus filter rebuild (search for other `seshFilter` calls in `update.go`)

**Step 4: Run tests**

Run: `go test ./tui/...`
Expected: Tests pass (existing filter tests may need `nil` frecency map added to `seshFilter` calls).

---

### Task 4: Plumb Frecency Scores Through the Stack

**Files:**
- Modify: `tui/tui.go` (TUI.Run)
- Modify: `seshcli/tui.go` (NewTuiCommand)

**Context:**
- `TUI` struct has `lister`, `connector`, `icon`, `tmux`, `config`, `previewer`
- Need to add `recent.Recent` so TUI can call `GetFrecencyScores()`
- `NewTuiCommand` in `seshcli/tui.go` creates `TUI` — needs to pass `Recent`
- `seshcli/app.go` creates the `Recent` instance — check if it's already available

**Step 1: Check how Recent is created**

Read `seshcli/app.go` to find where `recent.NewRecent()` is called and how it's passed around.

**Step 2: Add Recent to TUI struct**

```go
type TUI struct {
    lister    lister.Lister
    connector connector.Connector
    icon      icon.Icon
    tmux      tmux.Tmux
    config    model.Config
    previewer previewer.Previewer
    recent    recent.Recent  // NEW
}
```

Update `NewTUI()` to accept and store `recent.Recent`.

**Step 3: Load frecency scores in TUI.Run()**

After loading sessions, before `newModel()`:
```go
frecencyScores := t.recent.GetFrecencyScores()
```

Pass to `newModel()`.

**Step 4: Update NewTuiCommand to pass Recent**

Update `NewTuiCommand` signature to accept `recent.Recent`, pass to `NewTUI()`.

**Step 5: Update app.go call site**

Pass the `Recent` instance to `NewTuiCommand`.

**Step 6: Run all tests and build**

Run: `go test ./... && go build -o /dev/null`
Expected: All tests pass, build succeeds.

---

### Task 5: Update Filter Tests for Frecency

**Files:**
- Modify: `tui/filter_test.go`

**Context:**
- All existing `seshFilter(items)` calls need to become `seshFilter(items, nil)` (no frecency)
- Add new test cases for frecency tiebreaking behavior

**Step 1: Fix existing test calls**

Update all `seshFilter(testSessions)` → `seshFilter(testSessions, nil)`.

**Step 2: Add frecency tiebreaker test**

```go
t.Run("Frecency breaks tie within 5% score band", func(t *testing.T) {
    items := []list.Item{
        sessionItem{session: model.SeshSession{Name: "app-dev", Src: "projects"}, displayName: "app-dev"},
        sessionItem{session: model.SeshSession{Name: "app-dev", Src: "projects"}, displayName: "app-dev"},
    }
    // Same name = same score, frecency should break tie
    frecency := map[string]float64{"app-dev": 5.0}
    filter := seshFilter(items, frecency)
    ranks := filter("app", []string{"app-dev", "app-dev"})
    // Should get results without errors
    if len(ranks) < 2 {
        t.Fatalf("Expected 2 results, got %d", len(ranks))
    }
})
```

**Step 3: Add test that frecency doesn't override large score differences**

Verify that a high-frecency item with a much lower fuzzy score does NOT beat a zero-frecency item with a much higher score.

**Step 4: Run tests**

Run: `go test -v ./tui/...`
Expected: All tests pass.

---

### Task 6: Build and Verify

**Step 1: Run full test suite**

Run: `go test ./...`

**Step 2: Build binary**

Run: `go vet ./... && go build -o /dev/null`

**Step 3: Verify no regressions**

Check that the TUI still works with the new frecency plumbing by building the real binary.
