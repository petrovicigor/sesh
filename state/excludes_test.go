package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExcludes(t *testing.T) {
	t.Run("returns empty map when file does not exist", func(t *testing.T) {
		excludes, err := LoadExcludes("/nonexistent/path/excludes.json")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(excludes) != 0 {
			t.Errorf("expected empty map, got %d entries", len(excludes))
		}
	})

	t.Run("loads valid JSON file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "excludes.json")
		data := map[string][]string{
			"mono":  {"packages/config-eslint", "packages/config-ts"},
			"other": {"apps/legacy"},
		}
		b, _ := json.Marshal(data)
		os.WriteFile(path, b, 0644)

		excludes, err := LoadExcludes(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(excludes["mono"]) != 2 {
			t.Errorf("expected 2 excludes for 'mono', got %d", len(excludes["mono"]))
		}
		if excludes["mono"][0] != "packages/config-eslint" {
			t.Errorf("expected 'packages/config-eslint', got %q", excludes["mono"][0])
		}
		if len(excludes["other"]) != 1 {
			t.Errorf("expected 1 exclude for 'other', got %d", len(excludes["other"]))
		}
	})

	t.Run("returns empty map for corrupted file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "excludes.json")
		os.WriteFile(path, []byte("not json"), 0644)

		excludes, err := LoadExcludes(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(excludes) != 0 {
			t.Errorf("expected empty map for corrupted file, got %d entries", len(excludes))
		}
	})
}

func TestSaveExcludes(t *testing.T) {
	t.Run("writes JSON file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "excludes.json")
		excludes := map[string][]string{
			"mono": {"packages/config-eslint"},
		}

		err := SaveExcludes(path, excludes)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		b, _ := os.ReadFile(path)
		var loaded map[string][]string
		json.Unmarshal(b, &loaded)
		if len(loaded["mono"]) != 1 || loaded["mono"][0] != "packages/config-eslint" {
			t.Errorf("loaded data doesn't match: %v", loaded)
		}
	})

	t.Run("creates parent directories", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "nested", "deep", "excludes.json")

		err := SaveExcludes(path, map[string][]string{"x": {"y"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Error("expected file to exist")
		}
	})
}

func TestExcludesPath(t *testing.T) {
	t.Run("uses XDG_STATE_HOME when set", func(t *testing.T) {
		path := ExcludesPath("/custom/state")
		expected := "/custom/state/sesh/workspace-excludes.json"
		if path != expected {
			t.Errorf("expected %q, got %q", expected, path)
		}
	})

	t.Run("falls back to home/.local/state", func(t *testing.T) {
		path := ExcludesPath("")
		if filepath.Base(path) != "workspace-excludes.json" {
			t.Errorf("expected filename 'workspace-excludes.json', got %q", filepath.Base(path))
		}
	})
}
