package icon

import (
	"fmt"
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
)

func ansiString(code int, s string) string {
	return fmt.Sprintf("\033[%dm%s\033[39m", code, s)
}

func (i *RealIcon) AddIcon(s model.SeshSession) string {
	var icon string
	var colorCode int

	// Check if this is a worktree session (tmux session with "/" in name, not absolute path)
	isWorktree := s.Src == "tmux" && strings.Contains(s.Name, "/") && !strings.HasPrefix(s.Name, "/") && !strings.HasPrefix(s.Name, "~")

	if isWorktree {
		// For worktrees, use tmux icon + insert worktree separator between repo and branch
		// e.g., "geoip/develop" becomes " geoip ⎇ develop"
		icon = tmuxIcon
		colorCode = 34 // blue
		parts := strings.SplitN(s.Name, "/", 2)
		if len(parts) == 2 {
			return fmt.Sprintf("%s %s %s %s", ansiString(colorCode, icon), parts[0], worktreeIcon, parts[1])
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
		icon = configIcon
		colorCode = 90 // gray
	}

	if icon != "" {
		return fmt.Sprintf("%s %s", ansiString(colorCode, icon), s.Name)
	}
	return s.Name
}

func (i *RealIcon) RemoveIcon(name string) string {
	// Check if this is a worktree format: " geoip ⎇ develop"
	if strings.Contains(name, worktreeIcon) {
		// Strip icon prefix first
		stripped := name
		if strings.HasPrefix(name, tmuxIcon) {
			// Remove icon + space: assumes tmuxIcon is 3 bytes (UTF-8 ) + 1 space = 4 bytes total
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
	return name
}
