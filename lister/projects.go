package lister

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joshmedeski/sesh/v2/model"
)

func ProjectKey(name string) string {
	return fmt.Sprintf("projects:%s", name)
}

func listProjects(l *RealLister) (model.SeshSessions, error) {

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

	// Sort by recency - recently used sessions first
	sortProjectsByRecency(orderedIndex, directory, l.recent)

	return model.SeshSessions{
		Directory:    directory,
		OrderedIndex: orderedIndex,
	}, nil
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
// Returns the relative path from the project root
// Examples:
//   - /Users/user/Projects/geoip/feature/cdk -> "geoip/feature/cdk"
//   - /Users/user/Projects/chase-cognito -> "chase-cognito"
//   - /Users/user/Projects/geoip/develop -> "geoip/develop"
func getProjectDisplayName(fullPath string, projectRoots []string) string {
	// Find which project root this path belongs to
	for _, rootPath := range projectRoots {
		// Expand home directory
		expandedRoot := rootPath
		if strings.HasPrefix(rootPath, "~/") {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				continue
			}
			expandedRoot = filepath.Join(homeDir, rootPath[2:])
		}

		// Check if fullPath is under this root
		if strings.HasPrefix(fullPath, expandedRoot+string(filepath.Separator)) {
			// Return relative path from root
			relPath, err := filepath.Rel(expandedRoot, fullPath)
			if err == nil {
				return relPath
			}
		}
	}

	// Fallback to basename if no matching root found
	return filepath.Base(fullPath)
}

// detectWorktreesFast scans directories for nested git worktrees using filesystem checks
func detectWorktreesFast(gitCmd interface{ WorktreeList(string) (bool, string, error) }, dirs []string, excludePatterns []string) ([]string, error) {
	var allWorktrees []string

	// For each directory, check if it's a git repo and scan for nested worktrees
	for _, dir := range dirs {
		gitPath := filepath.Join(dir, ".git")

		// Check if .git exists
		info, err := os.Stat(gitPath)
		if err != nil {
			continue // Not a git repo
		}

		// Only scan for nested worktrees if .git is a directory (main repo)
		// If .git is a file, this directory itself is already a worktree
		if !info.IsDir() {
			continue
		}

		// Scan one level deep inside this git repo for worktrees
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			name := entry.Name()
			if shouldExclude(name, excludePatterns) {
				continue
			}

			subPath := filepath.Join(dir, name)
			subGitPath := filepath.Join(subPath, ".git")

			// Check if .git exists and is a file (= worktree)
			subInfo, err := os.Stat(subGitPath)
			if err == nil && !subInfo.IsDir() {
				allWorktrees = append(allWorktrees, subPath)
			}
		}
	}

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

// sortProjectsByRecency sorts the ordered index by recency
// Recently used sessions appear first, followed by others in original order
func sortProjectsByRecency(orderedIndex []string, directory model.SeshSessionMap, recentTracker interface{ GetAll() map[string]time.Time }) {
	if recentTracker == nil {
		return
	}

	recentSessions := recentTracker.GetAll()
	if len(recentSessions) == 0 {
		return
	}

	// Separate into recent and non-recent
	type sessionWithTime struct {
		key       string
		timestamp time.Time
		hasTime   bool
	}

	sessions := make([]sessionWithTime, len(orderedIndex))
	for i, key := range orderedIndex {
		session := directory[key]
		if ts, exists := recentSessions[session.Name]; exists {
			sessions[i] = sessionWithTime{key, ts, true}
		} else {
			sessions[i] = sessionWithTime{key, time.Time{}, false}
		}
	}

	// Sort: recent sessions first (by timestamp desc), then non-recent in original order
	for i := 0; i < len(sessions)-1; i++ {
		for j := i + 1; j < len(sessions); j++ {
			shouldSwap := false

			if sessions[i].hasTime && sessions[j].hasTime {
				// Both have timestamps - sort by timestamp (most recent first)
				shouldSwap = sessions[i].timestamp.Before(sessions[j].timestamp)
			} else if !sessions[i].hasTime && sessions[j].hasTime {
				// j has timestamp, i doesn't - j should come first
				shouldSwap = true
			}
			// If i has timestamp and j doesn't, or neither have timestamps, keep original order

			if shouldSwap {
				sessions[i], sessions[j] = sessions[j], sessions[i]
			}
		}
	}

	// Update orderedIndex with sorted keys
	for i, session := range sessions {
		orderedIndex[i] = session.key
	}
}
