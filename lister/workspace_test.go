package lister

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/joshmedeski/sesh/v2/model"
)

// passthroughHome is a minimal home.Home implementation for tests.
// It returns paths unchanged since test paths are absolute.
type passthroughHome struct{}

func (p passthroughHome) ShortenHome(path string) (string, error) { return path, nil }
func (p passthroughHome) ExpandHome(path string) (string, error)  { return path, nil }

// setupWorkspace creates a temporary workspace directory with the given structure.
// subProjects is a map of relative path -> true (directories to create).
func setupWorkspace(t *testing.T, subProjects map[string]bool) string {
	t.Helper()
	root := t.TempDir()

	// Create .git directory (main repo)
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	for sp := range subProjects {
		if err := os.MkdirAll(filepath.Join(root, sp), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// setupWorkspaceWithWorktrees creates a workspace with a main repo and worktree branches.
// worktreeBranches are subdirectory names that will have .git files (worktrees).
func setupWorkspaceWithWorktrees(t *testing.T, subProjects map[string]bool, worktreeBranches []string) string {
	t.Helper()
	root := t.TempDir()

	// Create .git directory in root (main repo)
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create sub-projects in root
	for sp := range subProjects {
		if err := os.MkdirAll(filepath.Join(root, sp), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Create worktree branches
	for _, branch := range worktreeBranches {
		branchDir := filepath.Join(root, branch)
		if err := os.MkdirAll(branchDir, 0755); err != nil {
			t.Fatal(err)
		}
		// .git as a file = worktree
		if err := os.WriteFile(filepath.Join(branchDir, ".git"), []byte("gitdir: ../main/.git/worktrees/"+branch), 0644); err != nil {
			t.Fatal(err)
		}

		// Create same sub-projects in worktree
		for sp := range subProjects {
			if err := os.MkdirAll(filepath.Join(branchDir, sp), 0755); err != nil {
				t.Fatal(err)
			}
		}
	}

	return root
}

func TestEvaluateIncludes(t *testing.T) {
	t.Run("matches glob patterns", func(t *testing.T) {
		root := t.TempDir()
		os.MkdirAll(filepath.Join(root, "apps", "portal"), 0755)
		os.MkdirAll(filepath.Join(root, "apps", "admin"), 0755)
		os.MkdirAll(filepath.Join(root, "packages", "api"), 0755)
		os.MkdirAll(filepath.Join(root, "packages", "config-eslint"), 0755)
		os.MkdirAll(filepath.Join(root, "infra"), 0755)

		result := evaluateIncludes(root, []string{"apps/*", "packages/*", "infra"})

		if len(result) != 5 {
			t.Fatalf("expected 5 matches, got %d: %v", len(result), result)
		}

		expected := map[string]bool{
			"apps/portal":            true,
			"apps/admin":             true,
			"packages/api":           true,
			"packages/config-eslint": true,
			"infra":                  true,
		}
		for _, r := range result {
			if !expected[r] {
				t.Errorf("unexpected match: %q", r)
			}
		}
	})

	t.Run("returns nil for empty includes", func(t *testing.T) {
		result := evaluateIncludes("/tmp", nil)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("skips files (only directories)", func(t *testing.T) {
		root := t.TempDir()
		os.MkdirAll(filepath.Join(root, "apps", "portal"), 0755)
		os.WriteFile(filepath.Join(root, "apps", "README.md"), []byte("hi"), 0644)

		result := evaluateIncludes(root, []string{"apps/*"})
		if len(result) != 1 {
			t.Fatalf("expected 1 match (directory only), got %d: %v", len(result), result)
		}
	})

	t.Run("deduplicates matches", func(t *testing.T) {
		root := t.TempDir()
		os.MkdirAll(filepath.Join(root, "apps", "portal"), 0755)

		result := evaluateIncludes(root, []string{"apps/*", "apps/portal"})
		if len(result) != 1 {
			t.Fatalf("expected 1 deduplicated match, got %d: %v", len(result), result)
		}
	})
}

func TestApplyExcludePatterns(t *testing.T) {
	t.Run("filters matching patterns", func(t *testing.T) {
		subs := []string{"packages/api", "packages/config-eslint", "packages/config-ts", "apps/portal"}
		result := applyExcludePatterns(subs, []string{"packages/config-*"})

		if len(result) != 2 {
			t.Fatalf("expected 2 after exclude, got %d: %v", len(result), result)
		}
		for _, r := range result {
			if r == "packages/config-eslint" || r == "packages/config-ts" {
				t.Errorf("should have been excluded: %q", r)
			}
		}
	})

	t.Run("no-op with empty patterns", func(t *testing.T) {
		subs := []string{"a", "b", "c"}
		result := applyExcludePatterns(subs, nil)
		if len(result) != 3 {
			t.Errorf("expected 3, got %d", len(result))
		}
	})
}

func TestApplyExcludeList(t *testing.T) {
	t.Run("filters exact matches", func(t *testing.T) {
		subs := []string{"apps/portal", "apps/admin", "packages/api"}
		result := applyExcludeList(subs, []string{"apps/admin"})

		if len(result) != 2 {
			t.Fatalf("expected 2, got %d: %v", len(result), result)
		}
		for _, r := range result {
			if r == "apps/admin" {
				t.Error("apps/admin should have been excluded")
			}
		}
	})
}

func TestDiscoverWorktrees(t *testing.T) {
	t.Run("discovers main repo only", func(t *testing.T) {
		root := setupWorkspace(t, map[string]bool{"apps/portal": true})
		wts := discoverWorktrees(root)
		if len(wts) != 1 {
			t.Fatalf("expected 1 worktree, got %d: %v", len(wts), wts)
		}
		if wts[0] != root {
			t.Errorf("expected root path, got %q", wts[0])
		}
	})

	t.Run("discovers worktree branches", func(t *testing.T) {
		root := setupWorkspaceWithWorktrees(t, map[string]bool{"apps/portal": true}, []string{"develop", "feature-x"})
		wts := discoverWorktrees(root)

		if len(wts) != 3 {
			t.Fatalf("expected 3 worktrees (root + 2 branches), got %d: %v", len(wts), wts)
		}
	})

	t.Run("non-git directory returns self", func(t *testing.T) {
		root := t.TempDir()
		wts := discoverWorktrees(root)
		if len(wts) != 1 || wts[0] != root {
			t.Errorf("expected [root], got %v", wts)
		}
	})
}

func TestPickDefaultWorktreeWithIncludes(t *testing.T) {
	t.Run("prefers configured default", func(t *testing.T) {
		wts := []string{"/repo", "/repo/develop", "/repo/main"}
		result, _ := pickDefaultWorktreeWithIncludes(wts, "develop", nil)
		if filepath.Base(result) != "develop" {
			t.Errorf("expected 'develop', got %q", result)
		}
	})

	t.Run("falls back to first with matching includes", func(t *testing.T) {
		root := t.TempDir()
		wt := filepath.Join(root, "develop")
		os.MkdirAll(filepath.Join(wt, "apps/portal"), 0755)

		wts := []string{root, wt}
		result, subs := pickDefaultWorktreeWithIncludes(wts, "nonexistent", []string{"apps/*"})
		// Root has no apps/, so should pick develop
		if result != wt {
			t.Errorf("expected %q (worktree with matching includes), got %q", wt, result)
		}
		if len(subs) != 1 || subs[0] != "apps/portal" {
			t.Errorf("expected [apps/portal], got %v", subs)
		}
	})

	t.Run("falls back to first when no includes", func(t *testing.T) {
		wts := []string{"/repo", "/repo/develop"}
		result, _ := pickDefaultWorktreeWithIncludes(wts, "nonexistent", nil)
		if result != "/repo" {
			t.Errorf("expected '/repo', got %q", result)
		}
	})

	t.Run("empty returns empty", func(t *testing.T) {
		result, _ := pickDefaultWorktreeWithIncludes(nil, "", nil)
		if result != "" {
			t.Errorf("expected empty, got %q", result)
		}
	})
}

func newTestLister(config model.Config, excludesPath string) *RealLister {
	return &RealLister{
		config:       config,
		home:         passthroughHome{},
		excludesPath: excludesPath,
	}
}

func TestListWorkspace(t *testing.T) {
	t.Run("no config returns empty", func(t *testing.T) {
		l := newTestLister(model.Config{}, "")
		sessions, err := listWorkspace(l)
		if err != nil {
			t.Fatal(err)
		}
		if len(sessions.OrderedIndex) != 0 {
			t.Errorf("expected 0 sessions, got %d", len(sessions.OrderedIndex))
		}
	})

	t.Run("scans workspace without worktrees", func(t *testing.T) {
		root := setupWorkspace(t, map[string]bool{
			"apps/portal":  true,
			"apps/admin":   true,
			"packages/api": true,
		})

		l := newTestLister(model.Config{
			WorkspaceConfigs: []model.WorkspaceConfig{
				{
					Name:    "mono",
					Path:    root,
					Include: []string{"apps/*", "packages/*"},
				},
			},
		}, "")

		sessions, err := listWorkspace(l)
		if err != nil {
			t.Fatal(err)
		}

		// root is the only "worktree", so 3 sub-projects × 1 worktree = 3 sessions
		rootBase := filepath.Base(root)
		if len(sessions.OrderedIndex) != 3 {
			t.Fatalf("expected 3 sessions, got %d", len(sessions.OrderedIndex))
		}

		// Verify naming: "mono/apps/portal/{rootBase}"
		found := false
		for _, key := range sessions.OrderedIndex {
			s := sessions.Directory[key]
			if s.Name == "mono/apps/portal/"+rootBase {
				found = true
				if s.Src != "workspace" {
					t.Errorf("expected src 'workspace', got %q", s.Src)
				}
				if s.Path != filepath.Join(root, "apps/portal") {
					t.Errorf("expected path %q, got %q", filepath.Join(root, "apps/portal"), s.Path)
				}
			}
		}
		if !found {
			t.Error("expected to find session 'mono/apps/portal/" + rootBase + "'")
		}
	})

	t.Run("scans workspace with worktrees", func(t *testing.T) {
		root := setupWorkspaceWithWorktrees(t,
			map[string]bool{
				"apps/portal":  true,
				"packages/api": true,
			},
			[]string{"develop", "feature-x"},
		)

		l := newTestLister(model.Config{
			WorkspaceConfigs: []model.WorkspaceConfig{
				{
					Name:    "mono",
					Path:    root,
					Include: []string{"apps/*", "packages/*"},
				},
			},
		}, "")

		sessions, err := listWorkspace(l)
		if err != nil {
			t.Fatal(err)
		}

		// 2 sub-projects × 3 worktrees (root + develop + feature-x) = 6 sessions
		if len(sessions.OrderedIndex) != 6 {
			names := make([]string, 0)
			for _, key := range sessions.OrderedIndex {
				names = append(names, sessions.Directory[key].Name)
			}
			t.Fatalf("expected 6 sessions, got %d: %v", len(sessions.OrderedIndex), names)
		}
	})

	t.Run("applies config excludes", func(t *testing.T) {
		root := setupWorkspace(t, map[string]bool{
			"apps/portal":          true,
			"packages/api":         true,
			"packages/config-lint": true,
		})

		l := newTestLister(model.Config{
			WorkspaceConfigs: []model.WorkspaceConfig{
				{
					Name:    "mono",
					Path:    root,
					Include: []string{"apps/*", "packages/*"},
					Exclude: []string{"packages/config-*"},
				},
			},
		}, "")

		sessions, err := listWorkspace(l)
		if err != nil {
			t.Fatal(err)
		}

		// packages/config-lint excluded, so 2 sessions (portal + api)
		if len(sessions.OrderedIndex) != 2 {
			names := make([]string, 0)
			for _, key := range sessions.OrderedIndex {
				names = append(names, sessions.Directory[key].Name)
			}
			t.Fatalf("expected 2 sessions, got %d: %v", len(sessions.OrderedIndex), names)
		}
	})

	t.Run("applies user excludes from state file", func(t *testing.T) {
		root := setupWorkspace(t, map[string]bool{
			"apps/portal": true,
			"apps/admin":  true,
			"apps/legacy": true,
		})

		// Write excludes state file
		stateDir := t.TempDir()
		excludesPath := filepath.Join(stateDir, "workspace-excludes.json")
		os.WriteFile(excludesPath, []byte(`{"mono":["apps/legacy"]}`), 0644)

		l := newTestLister(model.Config{
			WorkspaceConfigs: []model.WorkspaceConfig{
				{
					Name:    "mono",
					Path:    root,
					Include: []string{"apps/*"},
				},
			},
		}, excludesPath)

		sessions, err := listWorkspace(l)
		if err != nil {
			t.Fatal(err)
		}

		// apps/legacy excluded by state, so 2 sessions (portal + admin)
		if len(sessions.OrderedIndex) != 2 {
			names := make([]string, 0)
			for _, key := range sessions.OrderedIndex {
				names = append(names, sessions.Directory[key].Name)
			}
			t.Fatalf("expected 2 sessions, got %d: %v", len(sessions.OrderedIndex), names)
		}

		for _, key := range sessions.OrderedIndex {
			s := sessions.Directory[key]
			if filepath.Base(filepath.Dir(s.Path)) == "legacy" {
				t.Error("apps/legacy should have been excluded")
			}
		}
	})

	t.Run("skips nonexistent workspace path", func(t *testing.T) {
		l := newTestLister(model.Config{
			WorkspaceConfigs: []model.WorkspaceConfig{
				{
					Name:    "ghost",
					Path:    "/nonexistent/path/mono",
					Include: []string{"apps/*"},
				},
			},
		}, "")

		sessions, err := listWorkspace(l)
		if err != nil {
			t.Fatal(err)
		}
		if len(sessions.OrderedIndex) != 0 {
			t.Errorf("expected 0 sessions for nonexistent path, got %d", len(sessions.OrderedIndex))
		}
	})
}

func TestFindWorkspaceSession(t *testing.T) {
	root := setupWorkspace(t, map[string]bool{
		"apps/portal": true,
	})
	rootBase := filepath.Base(root)

	l := newTestLister(model.Config{
		WorkspaceConfigs: []model.WorkspaceConfig{
			{
				Name:    "mono",
				Path:    root,
				Include: []string{"apps/*"},
			},
		},
	}, "")

	t.Run("finds existing session", func(t *testing.T) {
		session, found := l.FindWorkspaceSession("mono/apps/portal/" + rootBase)
		if !found {
			t.Fatal("expected to find session")
		}
		if session.Path != filepath.Join(root, "apps/portal") {
			t.Errorf("expected path %q, got %q", filepath.Join(root, "apps/portal"), session.Path)
		}
	})

	t.Run("returns false for unknown session", func(t *testing.T) {
		_, found := l.FindWorkspaceSession("mono/apps/nonexistent/main")
		if found {
			t.Error("should not find nonexistent session")
		}
	})
}
