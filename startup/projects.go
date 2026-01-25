package startup

import "github.com/joshmedeski/sesh/v2/model"

func projectsStrategy(s *RealStartup, session model.SeshSession) (string, error) {
	if session.Src != "projects" {
		return "", nil
	}

	startupCommand := s.config.ProjectsConfig.StartupCommand
	if startupCommand == "" {
		return "", nil
	}

	replacements := map[string]string{"{}": session.Path}
	return s.replacer.Replace(startupCommand, replacements), nil
}
