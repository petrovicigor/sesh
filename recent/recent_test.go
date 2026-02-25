package recent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tempRecent(t *testing.T) *RealRecent {
	t.Helper()
	dir := t.TempDir()
	return &RealRecent{configDir: dir}
}

func TestRecordSessionNewEntry(t *testing.T) {
	r := tempRecent(t)
	if err := r.RecordSession("foo"); err != nil {
		t.Fatalf("RecordSession: %v", err)
	}

	sessions, err := r.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	entry, ok := sessions.Sessions["foo"]
	if !ok {
		t.Fatal("expected entry for 'foo'")
	}
	if entry.Count != 1 {
		t.Errorf("expected count 1, got %d", entry.Count)
	}
	if time.Since(entry.Time) > 5*time.Second {
		t.Error("expected timestamp to be recent")
	}
}

func TestRecordSessionIncrementsCount(t *testing.T) {
	r := tempRecent(t)
	for i := 0; i < 5; i++ {
		if err := r.RecordSession("bar"); err != nil {
			t.Fatalf("RecordSession %d: %v", i, err)
		}
	}

	sessions, err := r.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	entry := sessions.Sessions["bar"]
	if entry.Count != 5 {
		t.Errorf("expected count 5, got %d", entry.Count)
	}
}

func TestMigrationFromOldFormat(t *testing.T) {
	r := tempRecent(t)
	ts := "2026-01-15T10:30:00Z"

	// Write old format JSON directly
	oldData := `{"sessions":{"my-session":"` + ts + `"}}`
	path := filepath.Join(r.configDir, recentFileName)
	if err := os.MkdirAll(r.configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(oldData), 0644); err != nil {
		t.Fatal(err)
	}

	sessions, err := r.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	entry, ok := sessions.Sessions["my-session"]
	if !ok {
		t.Fatal("expected entry for 'my-session'")
	}
	if entry.Count != 1 {
		t.Errorf("expected migrated count 1, got %d", entry.Count)
	}

	expected, _ := time.Parse(time.RFC3339, ts)
	if !entry.Time.Equal(expected) {
		t.Errorf("expected time %v, got %v", expected, entry.Time)
	}
}

func TestMigrationFromOldFormatMultipleEntries(t *testing.T) {
	r := tempRecent(t)
	oldData := `{"sessions":{"a":"2026-01-01T00:00:00Z","b":"2026-02-01T00:00:00Z"}}`
	path := filepath.Join(r.configDir, recentFileName)
	if err := os.MkdirAll(r.configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(oldData), 0644); err != nil {
		t.Fatal(err)
	}

	sessions, err := r.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(sessions.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions.Sessions))
	}
	for name, entry := range sessions.Sessions {
		if entry.Count != 1 {
			t.Errorf("session %q: expected count 1, got %d", name, entry.Count)
		}
	}
}

func TestNewFormatRoundTrip(t *testing.T) {
	r := tempRecent(t)

	// Record a few sessions
	for i := 0; i < 3; i++ {
		if err := r.RecordSession("proj-a"); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.RecordSession("proj-b"); err != nil {
		t.Fatal(err)
	}

	// Reload and verify
	sessions, err := r.load()
	if err != nil {
		t.Fatal(err)
	}

	if sessions.Sessions["proj-a"].Count != 3 {
		t.Errorf("proj-a: expected count 3, got %d", sessions.Sessions["proj-a"].Count)
	}
	if sessions.Sessions["proj-b"].Count != 1 {
		t.Errorf("proj-b: expected count 1, got %d", sessions.Sessions["proj-b"].Count)
	}
}

func TestComputeFrecency(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		entry    SessionEntry
		expected float64
	}{
		{
			name:     "10 uses, just now",
			entry:    SessionEntry{Time: now, Count: 10},
			expected: 10.0, // 10 / (0 + 1)
		},
		{
			name:     "10 uses, 1 hour ago",
			entry:    SessionEntry{Time: now.Add(-1 * time.Hour), Count: 10},
			expected: 5.0, // 10 / (1 + 1)
		},
		{
			name:     "1 use, just now",
			entry:    SessionEntry{Time: now, Count: 1},
			expected: 1.0, // 1 / (0 + 1)
		},
		{
			name:     "50 uses, 1 week ago",
			entry:    SessionEntry{Time: now.Add(-168 * time.Hour), Count: 50},
			expected: 50.0 / 169.0, // ~0.296
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			score := computeFrecency(tc.entry, now)
			// Allow small floating-point tolerance
			diff := score - tc.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.01 {
				t.Errorf("expected ~%.3f, got %.3f", tc.expected, score)
			}
		})
	}
}

func TestGetFrecencyScores(t *testing.T) {
	r := tempRecent(t)

	// Record sessions with different usage patterns
	for i := 0; i < 10; i++ {
		if err := r.RecordSession("heavy-use"); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.RecordSession("light-use"); err != nil {
		t.Fatal(err)
	}

	scores := r.GetFrecencyScores()

	heavyScore, ok := scores["heavy-use"]
	if !ok {
		t.Fatal("expected score for 'heavy-use'")
	}
	lightScore, ok := scores["light-use"]
	if !ok {
		t.Fatal("expected score for 'light-use'")
	}

	// Heavy use should score higher (both recorded just now, so time factor is same)
	if heavyScore <= lightScore {
		t.Errorf("expected heavy-use score (%.3f) > light-use score (%.3f)", heavyScore, lightScore)
	}
}

func TestPruneByFrecency(t *testing.T) {
	r := tempRecent(t)

	// Record 49 sessions with high frecency (many uses, recent)
	for i := 0; i < 49; i++ {
		name := "session-" + string(rune('A'+i%26)) + string(rune('a'+i/26))
		for j := 0; j < 5; j++ {
			if err := r.RecordSession(name); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Manually insert a stale entry (old timestamp, count=1) to reach 50
	sessions, err := r.load()
	if err != nil {
		t.Fatal(err)
	}
	sessions.Sessions["stale-session"] = SessionEntry{
		Time:  time.Now().Add(-720 * time.Hour), // 30 days ago
		Count: 1,
	}
	if err := r.save(sessions); err != nil {
		t.Fatal(err)
	}

	// Now we have 50 sessions. Recording one more triggers pruning.
	if err := r.RecordSession("new-hot-session"); err != nil {
		t.Fatal(err)
	}

	sessions, err = r.load()
	if err != nil {
		t.Fatal(err)
	}

	if len(sessions.Sessions) > maxRecentSessions {
		t.Errorf("expected at most %d sessions, got %d", maxRecentSessions, len(sessions.Sessions))
	}

	// The stale session should be pruned (lowest frecency)
	if _, ok := sessions.Sessions["stale-session"]; ok {
		t.Error("expected 'stale-session' to be pruned (lowest frecency)")
	}

	// The new session should survive (count=1, time=now, score=1.0 > stale's ~0.001)
	if _, ok := sessions.Sessions["new-hot-session"]; !ok {
		t.Error("expected 'new-hot-session' to survive pruning")
	}
}

func TestGetTimestampBackwardCompat(t *testing.T) {
	r := tempRecent(t)
	if err := r.RecordSession("compat-test"); err != nil {
		t.Fatal(err)
	}

	ts, ok := r.GetTimestamp("compat-test")
	if !ok {
		t.Fatal("expected to find timestamp")
	}
	if time.Since(ts) > 5*time.Second {
		t.Error("expected recent timestamp")
	}

	_, ok = r.GetTimestamp("nonexistent")
	if ok {
		t.Error("expected false for nonexistent session")
	}
}

func TestGetAllBackwardCompat(t *testing.T) {
	r := tempRecent(t)
	if err := r.RecordSession("a"); err != nil {
		t.Fatal(err)
	}
	if err := r.RecordSession("b"); err != nil {
		t.Fatal(err)
	}

	all := r.GetAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(all))
	}

	// Verify values are time.Time (not SessionEntry)
	for name, ts := range all {
		if time.Since(ts) > 5*time.Second {
			t.Errorf("session %q: expected recent timestamp, got %v", name, ts)
		}
	}
}

func TestEmptyFile(t *testing.T) {
	r := tempRecent(t)

	// GetAll on non-existent file should return empty map
	all := r.GetAll()
	if len(all) != 0 {
		t.Errorf("expected empty map, got %d entries", len(all))
	}

	// GetFrecencyScores on non-existent file
	scores := r.GetFrecencyScores()
	if len(scores) != 0 {
		t.Errorf("expected empty scores, got %d entries", len(scores))
	}
}

func TestCorruptFile(t *testing.T) {
	r := tempRecent(t)
	path := filepath.Join(r.configDir, recentFileName)
	if err := os.MkdirAll(r.configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	// RecordSession should start fresh on corrupt file
	if err := r.RecordSession("fresh"); err != nil {
		t.Fatalf("RecordSession: %v", err)
	}

	sessions, err := r.load()
	if err != nil {
		t.Fatal(err)
	}
	if sessions.Sessions["fresh"].Count != 1 {
		t.Errorf("expected count 1 after fresh start, got %d", sessions.Sessions["fresh"].Count)
	}
}

func TestNewFormatJSON(t *testing.T) {
	// Verify the JSON output format
	r := tempRecent(t)
	if err := r.RecordSession("test"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(r.configDir, recentFileName))
	if err != nil {
		t.Fatal(err)
	}

	// Verify it contains the new format fields
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}

	var sessions map[string]json.RawMessage
	if err := json.Unmarshal(raw["sessions"], &sessions); err != nil {
		t.Fatal(err)
	}

	// The value should be an object with "t" and "n" fields
	var entry struct {
		T string `json:"t"`
		N int    `json:"n"`
	}
	if err := json.Unmarshal(sessions["test"], &entry); err != nil {
		t.Fatalf("expected object format, got error: %v (raw: %s)", err, sessions["test"])
	}
	if entry.N != 1 {
		t.Errorf("expected n=1, got %d", entry.N)
	}
	if entry.T == "" {
		t.Error("expected non-empty timestamp")
	}
}
