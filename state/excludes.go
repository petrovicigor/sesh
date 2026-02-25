package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ExcludesPath returns the path to the workspace-excludes JSON file.
// Uses xdgStateHome if non-empty, otherwise falls back to ~/.local/state.
func ExcludesPath(xdgStateHome string) string {
	if xdgStateHome != "" {
		return filepath.Join(xdgStateHome, "sesh", "workspace-excludes.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".local", "state", "sesh", "workspace-excludes.json")
}

// LoadExcludes reads the workspace excludes from the JSON file.
// Returns a map of workspace name → excluded sub-project paths.
// Returns an empty map if the file doesn't exist or is invalid.
func LoadExcludes(path string) (map[string][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return make(map[string][]string), nil
	}

	var excludes map[string][]string
	if err := json.Unmarshal(data, &excludes); err != nil {
		return make(map[string][]string), nil
	}

	return excludes, nil
}

// SaveExcludes writes the workspace excludes to the JSON file.
// Creates parent directories if they don't exist.
func SaveExcludes(path string, excludes map[string][]string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(excludes, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
