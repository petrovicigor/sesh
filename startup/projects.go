package startup

import (
	"os"
	"path/filepath"

	"github.com/joshmedeski/sesh/v2/model"
)

func projectsStrategy(s *RealStartup, session model.SeshSession) (string, error) {
	if session.Src != "projects" {
		return "", nil
	}

	replacements := map[string]string{"{}": session.Path}

	// If git_startup_command is set, check if this is a git repo (not a worktree parent)
	gitStartupCommand := s.config.ProjectsConfig.GitStartupCommand
	if gitStartupCommand != "" {
		isGitRepo, hasWorktrees := getGitStatus(session.Path)
		if isGitRepo && !hasWorktrees {
			return s.replacer.Replace(gitStartupCommand, replacements), nil
		}
	}

	// Fall back to regular startup command
	startupCommand := s.config.ProjectsConfig.StartupCommand
	if startupCommand == "" {
		return "", nil
	}

	return s.replacer.Replace(startupCommand, replacements), nil
}

// getGitStatus checks if a directory is a git repo and if it has worktrees inside
// Returns (isGitRepo, hasWorktrees)
// Only called at connection time, not during listing
func getGitStatus(dirPath string) (bool, bool) {
	gitPath := filepath.Join(dirPath, ".git")

	info, err := os.Stat(gitPath)
	if err != nil {
		// No .git = not a git repo
		return false, false
	}

	// .git exists, so it's a git repo
	isGitRepo := true

	// If .git is a file (not directory), it's a worktree itself, not a worktree parent
	if !info.IsDir() {
		return isGitRepo, false
	}

	// .git is a directory - check if it has worktrees inside
	worktreesPath := filepath.Join(gitPath, "worktrees")
	entries, err := os.ReadDir(worktreesPath)
	if err != nil {
		// No worktrees directory or can't read it
		return isGitRepo, false
	}

	// Has worktrees if there are any entries in .git/worktrees/
	hasWorktrees := len(entries) > 0
	return isGitRepo, hasWorktrees
}
