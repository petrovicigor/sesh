package lister

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/state"
)

func WorkspaceKey(name string) string {
	return fmt.Sprintf("workspace:%s", name)
}

// workspaceScanResult holds the intermediate scan result for a single workspace config.
// Shared by listWorkspace, FindWorkspaceSession, and ListWorkspaceSubProjects.
type workspaceScanResult struct {
	worktrees   []string
	subProjects []string
	rootPath    string
	config      model.WorkspaceConfig
}

// scanWorkspace performs the shared scanning logic for a single workspace config.
// Returns nil if the workspace config is invalid or the path doesn't exist.
func scanWorkspace(home interface{ ExpandHome(string) (string, error) }, wsCfg model.WorkspaceConfig) *workspaceScanResult {
	if home == nil || wsCfg.Name == "" || wsCfg.Path == "" {
		return nil
	}

	rootPath, err := home.ExpandHome(wsCfg.Path)
	if err != nil {
		return nil
	}
	if _, err := os.Stat(rootPath); os.IsNotExist(err) {
		return nil
	}

	worktrees := discoverWorktrees(rootPath)

	// Pick default worktree and get its sub-projects in one pass (avoids redundant glob eval).
	defaultWT, subProjects := pickDefaultWorktreeWithIncludes(worktrees, wsCfg.DefaultWorktree, wsCfg.Include)
	if defaultWT == "" {
		return nil
	}

	subProjects = applyExcludePatterns(subProjects, wsCfg.Exclude)

	return &workspaceScanResult{
		worktrees:   worktrees,
		subProjects: subProjects,
		rootPath:    rootPath,
		config:      wsCfg,
	}
}

// listWorkspace scans configured workspace paths and discovers sub-projects across worktrees.
// Each sub-project becomes an independent session with naming: {workspace}/{sub-path}/{worktree-branch}.
func listWorkspace(l *RealLister) (model.SeshSessions, error) {
	empty := model.SeshSessions{
		Directory:    make(model.SeshSessionMap),
		OrderedIndex: []string{},
	}

	if len(l.config.WorkspaceConfigs) == 0 {
		return empty, nil
	}

	// Load workspace excludes once per list call
	excludes, _ := state.LoadExcludes(l.excludesPath)

	orderedIndex := make([]string, 0)
	directory := make(model.SeshSessionMap)

	for _, wsCfg := range l.config.WorkspaceConfigs {
		scan := scanWorkspace(l.home, wsCfg)
		if scan == nil {
			continue
		}

		subProjects := scan.subProjects

		// Apply user excludes from state file (not applied in scanWorkspace)
		if wsExcludes, ok := excludes[wsCfg.Name]; ok {
			subProjects = applyExcludeList(subProjects, wsExcludes)
		}

		// For each sub-project, create sessions across all worktrees
		for _, subProject := range subProjects {
			for _, wt := range scan.worktrees {
				subPath := filepath.Join(wt, subProject)
				info, err := os.Stat(subPath)
				if err != nil || !info.IsDir() {
					continue
				}

				wtBranch := filepath.Base(wt)
				if wt == scan.rootPath {
					wtBranch = filepath.Base(scan.rootPath)
				}

				name := wsCfg.Name + "/" + subProject + "/" + wtBranch
				key := WorkspaceKey(name)

				orderedIndex = append(orderedIndex, key)
				directory[key] = model.SeshSession{
					Src:  "workspace",
					Name: name,
					Path: subPath,
				}
			}
		}
	}

	return model.SeshSessions{
		Directory:    directory,
		OrderedIndex: orderedIndex,
	}, nil
}

// discoverWorktrees finds all worktree directories under a root path.
// Returns a list of paths: the root itself (if it's a git repo) plus any worktree subdirectories.
func discoverWorktrees(rootPath string) []string {
	var worktrees []string

	gitPath := filepath.Join(rootPath, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		// Not a git repo — treat root itself as the only "worktree"
		return []string{rootPath}
	}

	if info.IsDir() {
		// Root is a main git repo
		worktrees = append(worktrees, rootPath)

		// Scan for nested worktrees (one level deep)
		entries, err := os.ReadDir(rootPath)
		if err != nil {
			return worktrees
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			subPath := filepath.Join(rootPath, entry.Name())
			subGitPath := filepath.Join(subPath, ".git")
			subInfo, err := os.Stat(subGitPath)
			if err == nil && !subInfo.IsDir() {
				// .git is a file = worktree
				worktrees = append(worktrees, subPath)
			}
		}
	} else {
		// Root itself is a worktree (.git is a file) — unusual but handle it
		worktrees = append(worktrees, rootPath)
	}

	return worktrees
}

// pickDefaultWorktreeWithIncludes selects the worktree for glob evaluation AND returns
// the evaluated sub-projects, avoiding redundant glob evaluation.
func pickDefaultWorktreeWithIncludes(worktrees []string, preferred string, includes []string) (string, []string) {
	if len(worktrees) == 0 {
		return "", nil
	}

	// Try preferred worktree first
	if preferred != "" {
		for _, wt := range worktrees {
			if filepath.Base(wt) == preferred {
				return wt, evaluateIncludes(wt, includes)
			}
		}
	}

	// Try each worktree and return the first where includes match something.
	// This handles bare repos where the root has no working tree content.
	for _, wt := range worktrees {
		subs := evaluateIncludes(wt, includes)
		if len(subs) > 0 {
			return wt, subs
		}
	}

	return worktrees[0], nil
}

// evaluateIncludes evaluates include glob patterns against a directory,
// returning relative paths of matching directories.
func evaluateIncludes(baseDir string, includes []string) []string {
	if len(includes) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var result []string

	for _, pattern := range includes {
		fullPattern := filepath.Join(baseDir, pattern)
		matches, err := filepath.Glob(fullPattern)
		if err != nil {
			continue
		}

		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil || !info.IsDir() {
				continue
			}

			rel, err := filepath.Rel(baseDir, match)
			if err != nil {
				continue
			}

			if !seen[rel] {
				seen[rel] = true
				result = append(result, rel)
			}
		}
	}

	return result
}

// applyExcludePatterns filters out sub-projects matching glob-style exclude patterns.
func applyExcludePatterns(subProjects []string, patterns []string) []string {
	if len(patterns) == 0 {
		return subProjects
	}

	result := make([]string, 0, len(subProjects))
	for _, sp := range subProjects {
		excluded := false
		for _, pattern := range patterns {
			matched, err := filepath.Match(pattern, sp)
			if err == nil && matched {
				excluded = true
				break
			}
			// Also try matching against the basename for nested paths
			matched, err = filepath.Match(pattern, filepath.Base(sp))
			if err == nil && matched {
				excluded = true
				break
			}
		}
		if !excluded {
			result = append(result, sp)
		}
	}
	return result
}

// applyExcludeList filters out sub-projects that are in the exclude list.
func applyExcludeList(subProjects []string, excludeList []string) []string {
	if len(excludeList) == 0 {
		return subProjects
	}

	excludeSet := make(map[string]bool, len(excludeList))
	for _, ex := range excludeList {
		excludeSet[ex] = true
	}

	result := make([]string, 0, len(subProjects))
	for _, sp := range subProjects {
		if !excludeSet[sp] {
			result = append(result, sp)
		}
	}
	return result
}

// FindWorkspaceSession looks up a workspace session by name.
// Uses scanWorkspace to avoid full session building — only scans the matching workspace.
func (l *RealLister) FindWorkspaceSession(name string) (model.SeshSession, bool) {
	// Load excludes once, outside the loop
	excludes, _ := state.LoadExcludes(l.excludesPath)

	for _, wsCfg := range l.config.WorkspaceConfigs {
		if !strings.HasPrefix(name, wsCfg.Name+"/") {
			continue
		}

		scan := scanWorkspace(l.home, wsCfg)
		if scan == nil {
			continue
		}
		subProjects := scan.subProjects
		if wsExcludes, ok := excludes[wsCfg.Name]; ok {
			subProjects = applyExcludeList(subProjects, wsExcludes)
		}

		// Look for the matching session
		for _, subProject := range subProjects {
			for _, wt := range scan.worktrees {
				wtBranch := filepath.Base(wt)
				if wt == scan.rootPath {
					wtBranch = filepath.Base(scan.rootPath)
				}

				sessionName := wsCfg.Name + "/" + subProject + "/" + wtBranch
				if sessionName == name {
					subPath := filepath.Join(wt, subProject)
					info, err := os.Stat(subPath)
					if err != nil || !info.IsDir() {
						continue
					}
					return model.SeshSession{
						Src:  "workspace",
						Name: name,
						Path: subPath,
					}, true
				}
			}
		}
	}

	return model.SeshSession{}, false
}

// ListWorkspaceSubProjects returns all discovered sub-projects per workspace.
// Applies config exclude patterns but NOT state excludes.
func (l *RealLister) ListWorkspaceSubProjects() map[string][]string {
	result := make(map[string][]string)

	for _, wsCfg := range l.config.WorkspaceConfigs {
		scan := scanWorkspace(l.home, wsCfg)
		if scan == nil {
			continue
		}
		result[wsCfg.Name] = scan.subProjects
	}

	return result
}
