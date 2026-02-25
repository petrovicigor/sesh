package icon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joshmedeski/sesh/v2/model"
)

type Icon interface {
	AddIcon(session model.SeshSession) string
	RemoveIcon(name string) string
}

type RealIcon struct {
	config model.Config
}

func NewIcon(config model.Config) Icon {
	return &RealIcon{config}
}

var (
	zoxideIcon     string = ""
	tmuxIcon       string = ""
	worktreeIcon   string = "⎇"
	configIcon     string = ""
	tmuxinatorIcon string = ""
	workspaceIcon  string = "📦"
)

func ansiString(code int, s string) string {
	return fmt.Sprintf("\033[%dm%s\033[39m", code, s)
}

// isGitWorktree checks if a path is a git worktree by checking if .git is a file (not a directory)
func isGitWorktree(path string) bool {
	if path == "" {
		return false
	}
	gitPath := filepath.Join(path, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return false
	}
	// In worktrees, .git is a file, not a directory
	return !info.IsDir()
}

func (i *RealIcon) AddIcon(s model.SeshSession) string {
	var icon string
	var colorCode int

	// Check if this is a worktree session by:
	// 1. Name has "/" format (repo/branch)
	// 2. Path is actually a git worktree (.git is a file, not directory)
	// This distinguishes real worktrees from disambiguated names like "igorpetrovic/test-zoxide"
	isWorktree := (s.Src == "tmux" || s.Src == "projects" || s.Src == "workspace") &&
		strings.Contains(s.Name, "/") &&
		(s.Src == "workspace" || isGitWorktree(s.Path))

	if isWorktree {
		// For worktrees, insert worktree separator between repo and branch
		// e.g., "geoip/develop" becomes " geoip ⎇ develop" (tmux) or " geoip ⎇ develop" (projects)
		switch s.Src {
		case "tmux":
			icon = tmuxIcon
			colorCode = 34 // blue
		case "projects":
			icon = zoxideIcon // folder icon
			colorCode = 32    // green
		case "workspace":
			icon = workspaceIcon
			colorCode = 35 // magenta
		}
		if s.Src == "workspace" {
			// Workspace names: "mono/packages/box-api/develop" → split at LAST slash
			// to get "mono/packages/box-api ⎇ develop"
			lastSlash := strings.LastIndex(s.Name, "/")
			if lastSlash > 0 {
				subProject := s.Name[:lastSlash]
				branch := s.Name[lastSlash+1:]
				return fmt.Sprintf("%s %s %s %s", ansiString(colorCode, icon), subProject, worktreeIcon, branch)
			}
		} else {
			// Regular projects/tmux: "geoip/develop" → split at first slash
			parts := strings.SplitN(s.Name, "/", 2)
			if len(parts) == 2 {
				return fmt.Sprintf("%s %s %s %s", ansiString(colorCode, icon), parts[0], worktreeIcon, parts[1])
			}
		}
	}

	switch s.Src {
	case "tmux":
		icon = tmuxIcon
		colorCode = 34 // blue
	case "tmuxinator":
		icon = tmuxinatorIcon
		colorCode = 33 // yellow
	case "zoxide":
		icon = zoxideIcon
		colorCode = 36 // cyan
	case "projects":
		icon = zoxideIcon // use folder icon
		colorCode = 32     // green (different from zoxide)
	case "config":
		icon = zoxideIcon
		colorCode = 90 // gray
	case "workspace":
		icon = workspaceIcon
		colorCode = 35 // magenta
	}

	if icon != "" {
		return fmt.Sprintf("%s %s", ansiString(colorCode, icon), s.Name)
	}
	return s.Name
}

func (i *RealIcon) RemoveIcon(name string) string {
	// Check if this is a worktree format: " geoip ⎇ develop" or " geoip ⎇ develop"
	if strings.Contains(name, worktreeIcon) {
		// Strip icon prefix first
		stripped := name
		if strings.HasPrefix(name, tmuxIcon) || strings.HasPrefix(name, zoxideIcon) {
			// Remove icon + space: assumes icon is 3 bytes (UTF-8  or ) + 1 space = 4 bytes total
			stripped = name[4:]
		}
		// Remove worktree separator and rejoin with "/"
		parts := strings.Split(stripped, " "+worktreeIcon+" ")
		if len(parts) == 2 {
			return strings.TrimSpace(parts[0]) + "/" + strings.TrimSpace(parts[1])
		}
	}

	// Regular icon stripping
	if strings.HasPrefix(name, tmuxIcon) || strings.HasPrefix(name, zoxideIcon) || strings.HasPrefix(name, configIcon) || strings.HasPrefix(name, tmuxinatorIcon) {
		return name[4:]
	}
	// workspaceIcon (📦) is 4 bytes UTF-8 + 1 space = 5 bytes
	if strings.HasPrefix(name, workspaceIcon) {
		return name[len(workspaceIcon)+1:]
	}
	return name
}
