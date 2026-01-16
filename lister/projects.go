package lister

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/joshmedeski/sesh/v2/model"
)

func ProjectKey(name string) string {
	return fmt.Sprintf("projects:%s", name)
}

// Simple cache for worktree results
var (
	worktreeCache      = make(map[string][]string)
	worktreeCacheTime  = make(map[string]time.Time)
	worktreeCacheMutex sync.RWMutex
	worktreeCacheTTL   = 10 * time.Second
)

// Cache for entire project sessions list
var (
	projectSessionsCache     model.SeshSessions
	projectSessionsCacheTime time.Time
	projectSessionsMutex     sync.RWMutex
	projectSessionsCacheTTL  = 10 * time.Second
)

func listProjects(l *RealLister) (model.SeshSessions, error) {
	// Check cache first
	now := time.Now()
	projectSessionsMutex.RLock()
	if !projectSessionsCacheTime.IsZero() && now.Sub(projectSessionsCacheTime) < projectSessionsCacheTTL {
		cached := projectSessionsCache
		projectSessionsMutex.RUnlock()
		return cached, nil
	}
	projectSessionsMutex.RUnlock()

	// If no project paths configured, return empty
	if len(l.config.ProjectsConfig.Paths) == 0 {
		return model.SeshSessions{
			Directory:    make(model.SeshSessionMap),
			OrderedIndex: []string{},
		}, nil
	}

	// Set defaults
	maxDepth := l.config.ProjectsConfig.MaxDepth
	if maxDepth == 0 {
		maxDepth = 1 // default to 1 level deep
	}

	// Collect all directories (sequential since usually just one root path)
	var allDirs []string
	for _, rootPath := range l.config.ProjectsConfig.Paths {
		// Expand home directory
		expandedPath := rootPath
		if strings.HasPrefix(rootPath, "~/") {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				continue
			}
			expandedPath = filepath.Join(homeDir, rootPath[2:])
		}

		// Check if root exists
		if _, err := os.Stat(expandedPath); os.IsNotExist(err) {
			continue
		}

		// Scan directories
		scannedDirs, err := scanDirectoryFast(expandedPath, maxDepth, l.config.ProjectsConfig.Exclude)
		if err != nil {
			continue
		}
		allDirs = append(allDirs, scannedDirs...)

		// If include_worktrees is enabled, detect git worktrees (parallel)
		if l.config.ProjectsConfig.IncludeWorktrees {
			worktrees, err := detectWorktreesFast(l.git, scannedDirs, l.config.ProjectsConfig.Exclude)
			if err == nil {
				allDirs = append(allDirs, worktrees...)
			}
		}
	}

	// Add saved project paths
	for _, saved := range l.config.ProjectsConfig.Saved {
		expandedPath := saved.Path
		if strings.HasPrefix(saved.Path, "~/") {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				continue
			}
			expandedPath = filepath.Join(homeDir, saved.Path[2:])
		}

		// Check if exists
		if _, err := os.Stat(expandedPath); err == nil {
			allDirs = append(allDirs, expandedPath)
		}
	}

	// Build sessions
	orderedIndex := make([]string, len(allDirs))
	directory := make(model.SeshSessionMap)

	for i, dirPath := range allDirs {
		// Use short name for display (basename or "repo/branch" for worktrees)
		name := getProjectDisplayName(dirPath, l.config.ProjectsConfig.Paths)

		key := ProjectKey(name)
		orderedIndex[i] = key
		directory[key] = model.SeshSession{
			Src:  "projects",
			Name: name, // Short name for display
			Path: dirPath, // Full path for connection
		}
	}

	result := model.SeshSessions{
		Directory:    directory,
		OrderedIndex: orderedIndex,
	}

	// Update cache
	projectSessionsMutex.Lock()
	projectSessionsCache = result
	projectSessionsCacheTime = now
	projectSessionsMutex.Unlock()

	return result, nil
}

// scanDirectoryFast scans a single level deep (optimized for depth=1)
func scanDirectoryFast(path string, maxDepth int, excludePatterns []string) ([]string, error) {
	var results []string

	entries, err := os.ReadDir(path)
	if err != nil {
		return results, err
	}

	// For depth=1 (most common), just scan this level
	if maxDepth == 1 {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if shouldExclude(name, excludePatterns) {
				continue
			}
			results = append(results, filepath.Join(path, name))
		}
		return results, nil
	}

	// For deeper scans, use recursive approach
	return scanDirectory(path, 0, maxDepth, excludePatterns), nil
}

// scanDirectory recursively scans a directory up to maxDepth
func scanDirectory(path string, currentDepth int, maxDepth int, excludePatterns []string) []string {
	var results []string

	if currentDepth >= maxDepth {
		return results
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return results
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		if shouldExclude(name, excludePatterns) {
			continue
		}

		fullPath := filepath.Join(path, name)
		results = append(results, fullPath)

		if currentDepth+1 < maxDepth {
			subdirs := scanDirectory(fullPath, currentDepth+1, maxDepth, excludePatterns)
			results = append(results, subdirs...)
		}
	}

	return results
}

// shouldExclude checks if a directory name matches any exclude pattern
func shouldExclude(name string, patterns []string) bool {
	for _, pattern := range patterns {
		// Simple string matching for now (can enhance with glob later)
		if name == pattern || strings.HasPrefix(name, pattern) {
			return true
		}
	}
	return false
}

// getProjectDisplayName returns a short display name for a project path
// For worktrees: "repo/branch" (e.g., "geoip/develop")
// For regular projects: just the basename (e.g., "chase-cognito")
func getProjectDisplayName(fullPath string, projectRoots []string) string {
	// Check if this is a worktree by looking for .git file (not directory)
	gitPath := filepath.Join(fullPath, ".git")
	if info, err := os.Stat(gitPath); err == nil && !info.IsDir() {
		// This is a worktree - construct "repo/branch" name
		// The parent of the worktree path is usually the repo name
		parts := strings.Split(fullPath, string(filepath.Separator))
		if len(parts) >= 2 {
			// Get last two parts for "repo/branch" format
			return parts[len(parts)-2] + "/" + parts[len(parts)-1]
		}
	}

	// For regular projects, return basename
	return filepath.Base(fullPath)
}

// detectWorktreesFast scans directories in parallel for git worktrees with caching
func detectWorktreesFast(gitCmd interface{ WorktreeList(string) (bool, string, error) }, dirs []string, excludePatterns []string) ([]string, error) {
	// First, filter to only git repos (fast check)
	var gitRepos []string
	for _, dir := range dirs {
		gitPath := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			gitRepos = append(gitRepos, dir)
		}
	}

	// If no git repos, return early
	if len(gitRepos) == 0 {
		return []string{}, nil
	}

	now := time.Now()
	var allWorktrees []string
	var uncachedRepos []string

	// Check cache first
	worktreeCacheMutex.RLock()
	for _, repo := range gitRepos {
		if cached, exists := worktreeCache[repo]; exists {
			if cacheTime, ok := worktreeCacheTime[repo]; ok && now.Sub(cacheTime) < worktreeCacheTTL {
				// Cache hit
				allWorktrees = append(allWorktrees, cached...)
				continue
			}
		}
		// Cache miss or expired
		uncachedRepos = append(uncachedRepos, repo)
	}
	worktreeCacheMutex.RUnlock()

	// If everything was cached, return early
	if len(uncachedRepos) == 0 {
		return allWorktrees, nil
	}

	// Process uncached repos in parallel (but only a few at a time)
	worktreesChan := make(chan struct {
		repo      string
		worktrees []string
	}, len(uncachedRepos))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4) // Limit to 4 concurrent git commands

	for _, dir := range uncachedRepos {
		wg.Add(1)
		go func(fullPath string) {
			defer wg.Done()
			sem <- struct{}{}        // acquire semaphore
			defer func() { <-sem }() // release semaphore

			var worktrees []string

			// Try to get worktree list
			ok, output, err := gitCmd.WorktreeList(fullPath)
			if ok && err == nil {
				// Parse worktree list output
				// Format: /path/to/worktree HASH [branch]
				lines := strings.Split(strings.TrimSpace(output), "\n")
				for _, line := range lines {
					if line == "" {
						continue
					}
					// Extract path (first field)
					parts := strings.Fields(line)
					if len(parts) > 0 {
						worktreePath := parts[0]
						// Skip the bare repo itself
						if worktreePath != fullPath {
							worktrees = append(worktrees, worktreePath)
						}
					}
				}
			}

			worktreesChan <- struct {
				repo      string
				worktrees []string
			}{fullPath, worktrees}
		}(dir)
	}

	wg.Wait()
	close(worktreesChan)

	// Collect results and update cache
	worktreeCacheMutex.Lock()
	for result := range worktreesChan {
		worktreeCache[result.repo] = result.worktrees
		worktreeCacheTime[result.repo] = now
		allWorktrees = append(allWorktrees, result.worktrees...)
	}
	worktreeCacheMutex.Unlock()

	return allWorktrees, nil
}

func (l *RealLister) FindProjectSession(name string) (model.SeshSession, bool) {
	key := ProjectKey(name)
	sessions, _ := listProjects(l)
	if session, exists := sessions.Directory[key]; exists {
		return session, true
	}
	return model.SeshSession{}, false
}
