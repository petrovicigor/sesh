package recent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	maxRecentSessions = 50
	recentFileName    = "recent_sessions.json"
)

type RecentSessions struct {
	Sessions map[string]time.Time `json:"sessions"`
}

type Recent interface {
	RecordSession(name string) error
	GetTimestamp(name string) (time.Time, bool)
	GetAll() map[string]time.Time
}

type RealRecent struct {
	configDir string
}

func NewRecent(configDir string) Recent {
	return &RealRecent{configDir: configDir}
}

// RecordSession records a session as recently used
func (r *RealRecent) RecordSession(name string) error {
	sessions, err := r.load()
	if err != nil {
		// If file doesn't exist or is corrupt, start fresh
		sessions = &RecentSessions{Sessions: make(map[string]time.Time)}
	}

	// Add/update timestamp
	sessions.Sessions[name] = time.Now()

	// Prune to keep only the most recent N sessions
	if len(sessions.Sessions) > maxRecentSessions {
		// Find oldest entries to remove
		type entry struct {
			name string
			time time.Time
		}
		var entries []entry
		for name, ts := range sessions.Sessions {
			entries = append(entries, entry{name, ts})
		}

		// Sort by time (oldest first)
		for i := 0; i < len(entries)-1; i++ {
			for j := i + 1; j < len(entries); j++ {
				if entries[i].time.After(entries[j].time) {
					entries[i], entries[j] = entries[j], entries[i]
				}
			}
		}

		// Remove oldest entries
		toRemove := len(entries) - maxRecentSessions
		for i := 0; i < toRemove; i++ {
			delete(sessions.Sessions, entries[i].name)
		}
	}

	return r.save(sessions)
}

// GetTimestamp returns the timestamp for a session if it exists
func (r *RealRecent) GetTimestamp(name string) (time.Time, bool) {
	sessions, err := r.load()
	if err != nil {
		return time.Time{}, false
	}

	ts, exists := sessions.Sessions[name]
	return ts, exists
}

// GetAll returns all recent sessions
func (r *RealRecent) GetAll() map[string]time.Time {
	sessions, err := r.load()
	if err != nil {
		return make(map[string]time.Time)
	}
	return sessions.Sessions
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
		sessions.Sessions = make(map[string]time.Time)
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
