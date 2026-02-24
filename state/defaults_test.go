package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Run("returns empty map when file does not exist", func(t *testing.T) {
		defaults, err := LoadDefaults("/nonexistent/path/defaults.json")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(defaults) != 0 {
			t.Errorf("expected empty map, got %d entries", len(defaults))
		}
	})

	t.Run("loads valid JSON file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "defaults.json")
		data := map[string]string{"chase-monorepo": "develop", "geoip": "main"}
		b, _ := json.Marshal(data)
		os.WriteFile(path, b, 0644)

		defaults, err := LoadDefaults(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if defaults["chase-monorepo"] != "develop" {
			t.Errorf("expected 'develop', got %q", defaults["chase-monorepo"])
		}
		if defaults["geoip"] != "main" {
			t.Errorf("expected 'main', got %q", defaults["geoip"])
		}
	})

	t.Run("returns empty map for corrupted file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "defaults.json")
		os.WriteFile(path, []byte("not json"), 0644)

		defaults, err := LoadDefaults(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(defaults) != 0 {
			t.Errorf("expected empty map for corrupted file, got %d entries", len(defaults))
		}
	})
}

func TestSaveDefaults(t *testing.T) {
	t.Run("writes JSON file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "defaults.json")
		defaults := map[string]string{"repo-a": "main", "repo-b": "dev"}

		err := SaveDefaults(path, defaults)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		b, _ := os.ReadFile(path)
		var loaded map[string]string
		json.Unmarshal(b, &loaded)
		if loaded["repo-a"] != "main" || loaded["repo-b"] != "dev" {
			t.Errorf("loaded data doesn't match: %v", loaded)
		}
	})

	t.Run("creates parent directories", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "nested", "deep", "defaults.json")

		err := SaveDefaults(path, map[string]string{"x": "y"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Error("expected file to exist")
		}
	})
}

func TestDefaultsPath(t *testing.T) {
	t.Run("uses XDG_STATE_HOME when set", func(t *testing.T) {
		path := DefaultsPath("/custom/state")
		expected := "/custom/state/sesh/worktree-defaults.json"
		if path != expected {
			t.Errorf("expected %q, got %q", expected, path)
		}
	})

	t.Run("falls back to home/.local/state", func(t *testing.T) {
		path := DefaultsPath("")
		if filepath.Base(path) != "worktree-defaults.json" {
			t.Errorf("expected filename 'worktree-defaults.json', got %q", filepath.Base(path))
		}
	})
}
