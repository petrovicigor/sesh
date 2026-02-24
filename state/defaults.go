package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// DefaultsPath returns the path to the worktree defaults JSON file.
// Uses xdgStateHome if non-empty, otherwise falls back to ~/.local/state.
func DefaultsPath(xdgStateHome string) string {
	if xdgStateHome != "" {
		return filepath.Join(xdgStateHome, "sesh", "worktree-defaults.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".local", "state", "sesh", "worktree-defaults.json")
}

// LoadDefaults reads the worktree defaults from the JSON file.
// Returns an empty map if the file doesn't exist or is invalid.
func LoadDefaults(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return make(map[string]string), nil
	}

	var defaults map[string]string
	if err := json.Unmarshal(data, &defaults); err != nil {
		return make(map[string]string), nil
	}

	return defaults, nil
}

// SaveDefaults writes the worktree defaults to the JSON file.
// Creates parent directories if they don't exist.
func SaveDefaults(path string, defaults map[string]string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(defaults, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
