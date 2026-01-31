package startup

import (
	"os"
	"path/filepath"
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

	t.Run("uses git_startup_command for git repo without worktrees", func(t *testing.T) {
		mockLister := new(lister.MockLister)
		mockTmux := new(tmux.MockTmux)
		mockHome := new(home.MockHome)
		mockReplacer := new(replacer.MockReplacer)

		// Create a temp git repo for testing
		tmpDir := t.TempDir()
		gitDir := filepath.Join(tmpDir, ".git")
		os.Mkdir(gitDir, 0755)

		config := model.Config{
			ProjectsConfig: model.ProjectsConfig{
				StartupCommand:    "echo default",
				GitStartupCommand: "git fetch && nvim",
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
			Path: tmpDir,
		}

		mockReplacer.On("Replace", "git fetch && nvim", map[string]string{"{}": tmpDir}).Return("git fetch && nvim")

		result, err := projectsStrategy(s, session)

		assert.NoError(t, err)
		assert.Equal(t, "git fetch && nvim", result)
		mockReplacer.AssertExpectations(t)
	})

	t.Run("skips git_startup_command for worktree parent", func(t *testing.T) {
		mockLister := new(lister.MockLister)
		mockTmux := new(tmux.MockTmux)
		mockHome := new(home.MockHome)
		mockReplacer := new(replacer.MockReplacer)

		// Create a temp git repo with worktrees for testing
		tmpDir := t.TempDir()
		gitDir := filepath.Join(tmpDir, ".git")
		os.Mkdir(gitDir, 0755)
		worktreesDir := filepath.Join(gitDir, "worktrees")
		os.Mkdir(worktreesDir, 0755)
		// Add a worktree entry
		os.Mkdir(filepath.Join(worktreesDir, "develop"), 0755)

		config := model.Config{
			ProjectsConfig: model.ProjectsConfig{
				StartupCommand:    "echo default",
				GitStartupCommand: "git fetch && nvim",
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
			Name: "chase-search",
			Path: tmpDir,
		}

		// Should fall back to startup_command, not git_startup_command
		mockReplacer.On("Replace", "echo default", map[string]string{"{}": tmpDir}).Return("echo default")

		result, err := projectsStrategy(s, session)

		assert.NoError(t, err)
		assert.Equal(t, "echo default", result)
		mockReplacer.AssertExpectations(t)
	})

	t.Run("uses git_startup_command for worktree (not parent)", func(t *testing.T) {
		mockLister := new(lister.MockLister)
		mockTmux := new(tmux.MockTmux)
		mockHome := new(home.MockHome)
		mockReplacer := new(replacer.MockReplacer)

		// Create a temp worktree (.git is a file, not directory)
		tmpDir := t.TempDir()
		gitFile := filepath.Join(tmpDir, ".git")
		os.WriteFile(gitFile, []byte("gitdir: /some/path/.git/worktrees/develop"), 0644)

		config := model.Config{
			ProjectsConfig: model.ProjectsConfig{
				StartupCommand:    "echo default",
				GitStartupCommand: "git fetch && nvim",
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
			Name: "chase-search/develop",
			Path: tmpDir,
		}

		mockReplacer.On("Replace", "git fetch && nvim", map[string]string{"{}": tmpDir}).Return("git fetch && nvim")

		result, err := projectsStrategy(s, session)

		assert.NoError(t, err)
		assert.Equal(t, "git fetch && nvim", result)
		mockReplacer.AssertExpectations(t)
	})

	t.Run("falls back to startup_command for non-git directory", func(t *testing.T) {
		mockLister := new(lister.MockLister)
		mockTmux := new(tmux.MockTmux)
		mockHome := new(home.MockHome)
		mockReplacer := new(replacer.MockReplacer)

		// Create a temp non-git directory
		tmpDir := t.TempDir()

		config := model.Config{
			ProjectsConfig: model.ProjectsConfig{
				StartupCommand:    "echo default",
				GitStartupCommand: "git fetch && nvim",
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
			Name: "non-git-project",
			Path: tmpDir,
		}

		mockReplacer.On("Replace", "echo default", map[string]string{"{}": tmpDir}).Return("echo default")

		result, err := projectsStrategy(s, session)

		assert.NoError(t, err)
		assert.Equal(t, "echo default", result)
		mockReplacer.AssertExpectations(t)
	})
}
