package startup

import (
	"strings"

	"github.com/joshmedeski/sesh/v2/model"
)

func workspaceStrategy(s *RealStartup, session model.SeshSession) (string, error) {
	if session.Src != "workspace" {
		return "", nil
	}

	// Find the matching workspace config by name prefix
	var wsCfg *model.WorkspaceConfig
	for i := range s.config.WorkspaceConfigs {
		if strings.HasPrefix(session.Name, s.config.WorkspaceConfigs[i].Name+"/") {
			wsCfg = &s.config.WorkspaceConfigs[i]
			break
		}
	}
	if wsCfg == nil {
		return "", nil
	}

	replacements := map[string]string{"{}": session.Path}

	// Workspace sessions are always inside a git repo (workspaces scan worktrees),
	// but sub-project paths don't have their own .git. Use git_startup_command directly.
	if wsCfg.GitStartupCommand != "" {
		return s.replacer.Replace(wsCfg.GitStartupCommand, replacements), nil
	}

	// Fall back to regular startup command
	if wsCfg.StartupCommand == "" {
		return "", nil
	}

	return s.replacer.Replace(wsCfg.StartupCommand, replacements), nil
}
