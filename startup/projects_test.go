package startup

import (
	"testing"

	"github.com/joshmedeski/sesh/v2/home"
	"github.com/joshmedeski/sesh/v2/lister"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/replacer"
	"github.com/joshmedeski/sesh/v2/tmux"
	"github.com/stretchr/testify/assert"
)

func TestProjectsStrategy(t *testing.T) {
	t.Run("returns empty when session source is not projects", func(t *testing.T) {
		mockLister := new(lister.MockLister)
		mockTmux := new(tmux.MockTmux)
		mockHome := new(home.MockHome)
		mockReplacer := new(replacer.MockReplacer)

		config := model.Config{
			ProjectsConfig: model.ProjectsConfig{
				StartupCommand: "echo hello",
			},
		}

		s := &RealStartup{
			lister:   mockLister,
			tmux:     mockTmux,
			config:   config,
			home:     mockHome,
			replacer: mockReplacer,
		}

		session := model.SeshSession{
			Src:  "tmux",
			Name: "test-session",
			Path: "/path/to/project",
		}

		result, err := projectsStrategy(s, session)

		assert.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("returns empty when startup command is not configured", func(t *testing.T) {
		mockLister := new(lister.MockLister)
		mockTmux := new(tmux.MockTmux)
		mockHome := new(home.MockHome)
		mockReplacer := new(replacer.MockReplacer)

		config := model.Config{
			ProjectsConfig: model.ProjectsConfig{
				StartupCommand: "",
			},
		}

		s := &RealStartup{
			lister:   mockLister,
			tmux:     mockTmux,
			config:   config,
			home:     mockHome,
			replacer: mockReplacer,
		}

		session := model.SeshSession{
			Src:  "projects",
			Name: "test-project",
			Path: "/path/to/project",
		}

		result, err := projectsStrategy(s, session)

		assert.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("returns startup command when session source is projects", func(t *testing.T) {
		mockLister := new(lister.MockLister)
		mockTmux := new(tmux.MockTmux)
		mockHome := new(home.MockHome)
		mockReplacer := new(replacer.MockReplacer)

		config := model.Config{
			ProjectsConfig: model.ProjectsConfig{
				StartupCommand: "clear && nvim",
			},
		}

		s := &RealStartup{
			lister:   mockLister,
			tmux:     mockTmux,
			config:   config,
			home:     mockHome,
			replacer: mockReplacer,
		}

		session := model.SeshSession{
			Src:  "projects",
			Name: "test-project",
			Path: "/path/to/project",
		}

		mockReplacer.On("Replace", "clear && nvim", map[string]string{"{}": "/path/to/project"}).Return("clear && nvim")

		result, err := projectsStrategy(s, session)

		assert.NoError(t, err)
		assert.Equal(t, "clear && nvim", result)
		mockReplacer.AssertExpectations(t)
	})

	t.Run("replaces {} placeholder with session path", func(t *testing.T) {
		mockLister := new(lister.MockLister)
		mockTmux := new(tmux.MockTmux)
		mockHome := new(home.MockHome)
		mockReplacer := new(replacer.MockReplacer)

		config := model.Config{
			ProjectsConfig: model.ProjectsConfig{
				StartupCommand: "cd {} && nvim",
			},
		}

		s := &RealStartup{
			lister:   mockLister,
			tmux:     mockTmux,
			config:   config,
			home:     mockHome,
			replacer: mockReplacer,
		}

		session := model.SeshSession{
			Src:  "projects",
			Name: "test-project",
			Path: "/home/user/projects/test-project",
		}

		mockReplacer.On("Replace", "cd {} && nvim", map[string]string{"{}": "/home/user/projects/test-project"}).Return("cd /home/user/projects/test-project && nvim")

		result, err := projectsStrategy(s, session)

		assert.NoError(t, err)
		assert.Equal(t, "cd /home/user/projects/test-project && nvim", result)
		mockReplacer.AssertExpectations(t)
	})
}
