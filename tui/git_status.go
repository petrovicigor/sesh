package tui

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/list"
)

const (
	// gitCheckTimeout bounds the git commands for one path so a repo on a
	// hung mount can't hold the GitChangesMsg hostage.
	gitCheckTimeout = 3 * time.Second
	// gitCheckConcurrency caps in-flight git subprocesses during the scan.
	gitCheckConcurrency = 8
)

// gitChanges holds working-tree change counts for one repository: added and
// deleted lines across staged+unstaged tracked changes, plus untracked files.
type gitChanges struct {
	Added     int
	Deleted   int
	Untracked int
}

// badge returns the pre-rendered "+A -D ?U" suffix with zero-valued parts
// shed; "" when the tree is clean. Raw ANSI (not lipgloss Render) to avoid
// nested style conflicts with selectedItemStyle wrapping in bubbletea v2 —
// same reason as the ⟲ saved-state marker. Green 65 ≈ miasma #5f875f,
// red 131 a muted counterpart, gray 245 matches ⟲.
func (c gitChanges) badge() string {
	var parts []string
	if c.Added > 0 {
		parts = append(parts, "\033[38;5;65m+"+strconv.Itoa(c.Added)+"\033[0m")
	}
	if c.Deleted > 0 {
		parts = append(parts, "\033[38;5;131m-"+strconv.Itoa(c.Deleted)+"\033[0m")
	}
	if c.Untracked > 0 {
		parts = append(parts, "\033[38;5;245m?"+strconv.Itoa(c.Untracked)+"\033[0m")
	}
	return strings.Join(parts, " ")
}

// gitChangesForPaths runs the working-tree check for each path concurrently
// (capped at gitCheckConcurrency) and returns entries only for paths with
// changes. Non-repos, missing dirs, and errors all read as clean.
func gitChangesForPaths(paths []string) map[string]gitChanges {
	results := make(map[string]gitChanges, len(paths))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, gitCheckConcurrency)
	for _, p := range paths {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			c, ok := changesForPath(p)
			if !ok || (c.Added == 0 && c.Deleted == 0 && c.Untracked == 0) {
				return
			}
			mu.Lock()
			results[p] = c
			mu.Unlock()
		}()
	}
	wg.Wait()
	return results
}

// changesForPath collects change counts for one directory; ok is false when
// it is not a git repository.
func changesForPath(path string) (gitChanges, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), gitCheckTimeout)
	defer cancel()
	var c gitChanges
	// Untracked files via status --porcelain — this also proves the
	// directory is a repo.
	statusOut, err := gitOutput(ctx, path, "status", "--porcelain")
	if err != nil {
		return gitChanges{}, false
	}
	for _, line := range strings.Split(statusOut, "\n") {
		if strings.HasPrefix(line, "??") {
			c.Untracked++
		}
	}
	// Added/deleted lines across staged+unstaged tracked changes. Errors
	// (e.g. a fresh repo with no HEAD yet) leave the line counts at zero;
	// binary files report "-" and are skipped by the Atoi guards.
	if numstat, err := gitOutput(ctx, path, "diff", "--numstat", "HEAD"); err == nil {
		for _, line := range strings.Split(numstat, "\n") {
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}
			if a, err := strconv.Atoi(fields[0]); err == nil {
				c.Added += a
			}
			if d, err := strconv.Atoi(fields[1]); err == nil {
				c.Deleted += d
			}
		}
	}
	return c, true
}

// gitOutput runs one git command against dir. --no-optional-locks keeps the
// check read-only: plain status opportunistically rewrites the index and
// takes .git/index.lock, which would contend with a live lazygit.
func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	full := append([]string{"-C", dir, "--no-optional-locks"}, args...)
	out, err := exec.CommandContext(ctx, "git", full...).Output()
	return string(out), err
}

// tmuxSessionPaths returns the unique directory paths of tmux-backed rows —
// the scope of the git change badges.
func tmuxSessionPaths(items []list.Item) []string {
	seen := map[string]bool{}
	var paths []string
	for _, it := range items {
		si, ok := it.(sessionItem)
		if !ok || si.session.Src != "tmux" || si.session.Path == "" || seen[si.session.Path] {
			continue
		}
		seen[si.session.Path] = true
		paths = append(paths, si.session.Path)
	}
	return paths
}
