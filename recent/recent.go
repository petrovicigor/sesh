package recent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	maxRecentSessions = 50
	recentFileName    = "recent_sessions.json"
)

// SessionEntry tracks both the last-used time and the usage count for frecency scoring.
type SessionEntry struct {
	Time  time.Time `json:"t"`
	Count int       `json:"n"`
}

type RecentSessions struct {
	Sessions map[string]SessionEntry `json:"sessions"`
}

// UnmarshalJSON handles migration from the old format (plain timestamp strings)
// to the new format (SessionEntry objects).
func (rs *RecentSessions) UnmarshalJSON(data []byte) error {
	// Try new format first: {"sessions": {"name": {"t": "...", "n": 42}}}
	type newFormat struct {
		Sessions map[string]SessionEntry `json:"sessions"`
	}
	var nf newFormat
	if err := json.Unmarshal(data, &nf); err == nil && nf.Sessions != nil {
		// Verify this is actually the new format by checking we got valid entries.
		// The old format would also parse without error but all Times would be zero
		// and Counts would be 0 since a plain string doesn't match the struct fields.
		// We need to distinguish: if any entry has Count > 0, it's new format.
		isNew := false
		for _, entry := range nf.Sessions {
			if entry.Count > 0 {
				isNew = true
				break
			}
		}
		if isNew || len(nf.Sessions) == 0 {
			rs.Sessions = nf.Sessions
			return nil
		}
	}

	// Try old format: {"sessions": {"name": "2026-02-25T10:30:00Z"}}
	type oldFormat struct {
		Sessions map[string]string `json:"sessions"`
	}
	var of oldFormat
	if err := json.Unmarshal(data, &of); err == nil && of.Sessions != nil {
		rs.Sessions = make(map[string]SessionEntry, len(of.Sessions))
		for name, tsStr := range of.Sessions {
			parsed, err := time.Parse(time.RFC3339Nano, tsStr)
			if err != nil {
				// Try other common formats
				parsed, err = time.Parse(time.RFC3339, tsStr)
				if err != nil {
					// Skip entries we can't parse
					continue
				}
			}
			rs.Sessions[name] = SessionEntry{Time: parsed, Count: 1}
		}
		return nil
	}

	// If nothing works, start with empty map
	rs.Sessions = make(map[string]SessionEntry)
	return nil
}

type Recent interface {
	RecordSession(name string) error
	GetTimestamp(name string) (time.Time, bool)
	GetAll() map[string]time.Time
	GetFrecencyScores() map[string]float64
}

type RealRecent struct {
	configDir string
}

func NewRecent(configDir string) Recent {
	return &RealRecent{configDir: configDir}
}

// computeFrecency computes the frecency score for a single entry.
// Higher scores mean more frequently and recently used sessions.
func computeFrecency(entry SessionEntry, now time.Time) float64 {
	hours := now.Sub(entry.Time).Hours()
	if hours < 0 {
		hours = 0
	}
	return float64(entry.Count) / (hours + 1.0)
}

// RecordSession records a session as recently used, incrementing count and updating time.
func (r *RealRecent) RecordSession(name string) error {
	sessions, err := r.load()
	if err != nil {
		// If file doesn't exist or is corrupt, start fresh
		sessions = &RecentSessions{Sessions: make(map[string]SessionEntry)}
	}

	// Add/update entry
	now := time.Now()
	if existing, ok := sessions.Sessions[name]; ok {
		existing.Count++
		existing.Time = now
		sessions.Sessions[name] = existing
	} else {
		sessions.Sessions[name] = SessionEntry{Time: now, Count: 1}
	}

	// Prune to keep only the top N sessions by frecency score
	if len(sessions.Sessions) > maxRecentSessions {
		type scoredEntry struct {
			name  string
			score float64
		}
		var entries []scoredEntry
		for n, entry := range sessions.Sessions {
			entries = append(entries, scoredEntry{name: n, score: computeFrecency(entry, now)})
		}

		// Sort by score (lowest first) so we remove the least valuable entries
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].score < entries[j].score
		})

		// Remove lowest-scored entries
		toRemove := len(entries) - maxRecentSessions
		for i := 0; i < toRemove; i++ {
			delete(sessions.Sessions, entries[i].name)
		}
	}

	return r.save(sessions)
}

// GetTimestamp returns the timestamp for a session if it exists (backward compatible).
func (r *RealRecent) GetTimestamp(name string) (time.Time, bool) {
	sessions, err := r.load()
	if err != nil {
		return time.Time{}, false
	}

	entry, exists := sessions.Sessions[name]
	if !exists {
		return time.Time{}, false
	}
	return entry.Time, true
}

// GetAll returns all recent sessions as a map of name to timestamp (backward compatible).
func (r *RealRecent) GetAll() map[string]time.Time {
	sessions, err := r.load()
	if err != nil {
		return make(map[string]time.Time)
	}

	result := make(map[string]time.Time, len(sessions.Sessions))
	for name, entry := range sessions.Sessions {
		result[name] = entry.Time
	}
	return result
}

// GetFrecencyScores returns a map of session names to their frecency scores.
// Higher scores indicate sessions that are both frequently and recently used.
func (r *RealRecent) GetFrecencyScores() map[string]float64 {
	sessions, err := r.load()
	if err != nil {
		return make(map[string]float64)
	}

	now := time.Now()
	scores := make(map[string]float64, len(sessions.Sessions))
	for name, entry := range sessions.Sessions {
		scores[name] = computeFrecency(entry, now)
	}
	return scores
}

func (r *RealRecent) filePath() string {
	return filepath.Join(r.configDir, recentFileName)
}

func (r *RealRecent) load() (*RecentSessions, error) {
	path := r.filePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var sessions RecentSessions
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil, err
	}

	if sessions.Sessions == nil {
		sessions.Sessions = make(map[string]SessionEntry)
	}

	return &sessions, nil
}

func (r *RealRecent) save(sessions *RecentSessions) error {
	path := r.filePath()

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
