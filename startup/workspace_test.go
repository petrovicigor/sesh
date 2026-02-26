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

func TestWorkspaceStrategy(t *testing.T) {
	t.Run("returns empty when session source is not workspace", func(t *testing.T) {
		s := &RealStartup{
			lister:   new(lister.MockLister),
			tmux:     new(tmux.MockTmux),
			config: model.Config{
				WorkspaceConfigs: []model.WorkspaceConfig{
					{Name: "sesh", GitStartupCommand: "nvim"},
				},
			},
			home:     new(home.MockHome),
			replacer: new(replacer.MockReplacer),
		}

		session := model.SeshSession{
			Src:  "projects",
			Name: "sesh/tui/main",
			Path: "/path/to/sesh",
		}

		result, err := workspaceStrategy(s, session)
		assert.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("returns empty when no matching workspace config", func(t *testing.T) {
		s := &RealStartup{
			lister:   new(lister.MockLister),
			tmux:     new(tmux.MockTmux),
			config: model.Config{
				WorkspaceConfigs: []model.WorkspaceConfig{
					{Name: "other", GitStartupCommand: "nvim"},
				},
			},
			home:     new(home.MockHome),
			replacer: new(replacer.MockReplacer),
		}

		session := model.SeshSession{
			Src:  "workspace",
			Name: "sesh/tui/main",
			Path: "/path/to/sesh",
		}

		result, err := workspaceStrategy(s, session)
		assert.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("uses git_startup_command for sub-project path", func(t *testing.T) {
		// Workspace sub-projects don't have their own .git — they're inside a git repo.
		// git_startup_command should be used directly without checking for .git.
		tmpDir := t.TempDir()

		mockReplacer := new(replacer.MockReplacer)
		mockReplacer.On("Replace", "clear && nvim +GoToFile", map[string]string{"{}": tmpDir}).Return("clear && nvim +GoToFile")

		s := &RealStartup{
			lister: new(lister.MockLister),
			tmux:   new(tmux.MockTmux),
			config: model.Config{
				WorkspaceConfigs: []model.WorkspaceConfig{
					{Name: "sesh", GitStartupCommand: "clear && nvim +GoToFile"},
				},
			},
			home:     new(home.MockHome),
			replacer: mockReplacer,
		}

		session := model.SeshSession{
			Src:  "workspace",
			Name: "sesh/tui/main",
			Path: tmpDir,
		}

		result, err := workspaceStrategy(s, session)
		assert.NoError(t, err)
		assert.Equal(t, "clear && nvim +GoToFile", result)
		mockReplacer.AssertExpectations(t)
	})

	t.Run("git_startup_command takes priority over startup_command", func(t *testing.T) {
		tmpDir := t.TempDir()

		mockReplacer := new(replacer.MockReplacer)
		mockReplacer.On("Replace", "nvim", map[string]string{"{}": tmpDir}).Return("nvim")

		s := &RealStartup{
			lister: new(lister.MockLister),
			tmux:   new(tmux.MockTmux),
			config: model.Config{
				WorkspaceConfigs: []model.WorkspaceConfig{
					{
						Name:              "sesh",
						StartupCommand:    "echo default",
						GitStartupCommand: "nvim",
					},
				},
			},
			home:     new(home.MockHome),
			replacer: mockReplacer,
		}

		session := model.SeshSession{
			Src:  "workspace",
			Name: "sesh/tui/main",
			Path: tmpDir,
		}

		result, err := workspaceStrategy(s, session)
		assert.NoError(t, err)
		assert.Equal(t, "nvim", result)
		mockReplacer.AssertExpectations(t)
	})

	t.Run("falls back to startup_command when no git_startup_command", func(t *testing.T) {
		tmpDir := t.TempDir()

		mockReplacer := new(replacer.MockReplacer)
		mockReplacer.On("Replace", "echo hello", map[string]string{"{}": tmpDir}).Return("echo hello")

		s := &RealStartup{
			lister: new(lister.MockLister),
			tmux:   new(tmux.MockTmux),
			config: model.Config{
				WorkspaceConfigs: []model.WorkspaceConfig{
					{
						Name:           "sesh",
						StartupCommand: "echo hello",
					},
				},
			},
			home:     new(home.MockHome),
			replacer: mockReplacer,
		}

		session := model.SeshSession{
			Src:  "workspace",
			Name: "sesh/tui/main",
			Path: tmpDir,
		}

		result, err := workspaceStrategy(s, session)
		assert.NoError(t, err)
		assert.Equal(t, "echo hello", result)
		mockReplacer.AssertExpectations(t)
	})

	t.Run("returns empty when no commands configured", func(t *testing.T) {
		s := &RealStartup{
			lister: new(lister.MockLister),
			tmux:   new(tmux.MockTmux),
			config: model.Config{
				WorkspaceConfigs: []model.WorkspaceConfig{
					{Name: "sesh"},
				},
			},
			home:     new(home.MockHome),
			replacer: new(replacer.MockReplacer),
		}

		session := model.SeshSession{
			Src:  "workspace",
			Name: "sesh/tui/main",
			Path: "/path/to/sesh",
		}

		result, err := workspaceStrategy(s, session)
		assert.NoError(t, err)
		assert.Equal(t, "", result)
	})
}
