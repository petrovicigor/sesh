package connector

import (
	"testing"

	"github.com/joshmedeski/sesh/v2/dir"
	"github.com/joshmedeski/sesh/v2/home"
	"github.com/joshmedeski/sesh/v2/lister"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/namer"
	"github.com/joshmedeski/sesh/v2/startup"
	"github.com/joshmedeski/sesh/v2/tmux"
	"github.com/joshmedeski/sesh/v2/tmuxinator"
	"github.com/joshmedeski/sesh/v2/zoxide"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
)

func TestEstablishTmuxConnection(t *testing.T) {
	mockDir := new(dir.MockDir)
	mockHome := new(home.MockHome)
	mockLister := new(lister.MockLister)
	mockNamer := new(namer.MockNamer)
	mockStartup := new(startup.MockStartup)
	mockTmux := new(tmux.MockTmux)
	mockZoxide := new(zoxide.MockZoxide)
	mockTmuxinator := new(tmuxinator.MockTmuxinator)

	c := &RealConnector{
		model.Config{},
		mockDir,
		mockHome,
		mockLister,
		mockNamer,
		mockStartup,
		mockTmux,
		mockZoxide,
		mockTmuxinator,
		nil, // recent
	}
	mockTmux.On("AttachSession", mock.Anything).Return("attaching", nil)
	mockZoxide.On("Add", mock.Anything).Return(nil)

	t.Run("should attach to tmux session", func(t *testing.T) {
		mockTmux.On("IsAttached").Return(false)
		mockLister.On("FindTmuxSession", "dotfiles").Return(model.SeshSession{
			Name: "dotfiles",
			Path: "/Users/joshmedeski/c/dotfiles",
		}, true)
		connection, err := tmuxStrategy(c, "dotfiles")
		assert.Nil(t, err)
		assert.Equal(t, "dotfiles", connection.Session.Name)
	})

	t.Run("should switch to tmux session", func(t *testing.T) {
		mockTmux.On("IsAttached").Return(true)
		mockLister.On("FindTmuxSession", "dotfiles").Return(model.SeshSession{
			Name: "dotfiles",
			Path: "/Users/joshmedeski/c/dotfiles",
		}, true)
		connection, err := tmuxStrategy(c, "dotfiles")
		assert.Nil(t, err)
		assert.Equal(t, "dotfiles", connection.Session.Name)
	})
}

func newTestConnector() (*RealConnector, *lister.MockLister, *namer.MockNamer, *startup.MockStartup, *tmux.MockTmux) {
	mockLister := new(lister.MockLister)
	mockNamer := new(namer.MockNamer)
	mockStartup := new(startup.MockStartup)
	mockTmux := new(tmux.MockTmux)
	c := &RealConnector{
		model.Config{},
		new(dir.MockDir),
		new(home.MockHome),
		mockLister,
		mockNamer,
		mockStartup,
		mockTmux,
		new(zoxide.MockZoxide),
		new(tmuxinator.MockTmuxinator),
		nil, // recent
	}
	return c, mockLister, mockNamer, mockStartup, mockTmux
}

func TestConnectToTmuxNewSession(t *testing.T) {
	t.Run("attaches without create or startup when session exists for the same path", func(t *testing.T) {
		c, mockLister, _, mockStartup, mockTmux := newTestConnector()
		existing := model.SeshSession{Name: "dotfiles", Path: "/Users/igor/dotfiles"}
		mockLister.On("FindTmuxSession", "dotfiles").Return(existing, true)
		mockTmux.On("StartServer").Return(nil)
		mockTmux.On("SwitchOrAttach", "dotfiles", mock.Anything).Return("switched", nil)

		connection := model.Connection{
			Found: true,
			New:   true,
			Session: model.SeshSession{
				Src:  "dir",
				Name: "dotfiles",
				Path: "/Users/igor/dotfiles/", // trailing slash must not defeat the match
			},
		}
		out, err := connectToTmux(c, connection, model.ConnectOpts{})
		assert.Nil(t, err)
		assert.Equal(t, "switched", out)
		mockTmux.AssertNotCalled(t, "NewSession", mock.Anything, mock.Anything)
		mockTmux.AssertNotCalled(t, "SendKeys", mock.Anything, mock.Anything)
		mockStartup.AssertNotCalled(t, "Exec", mock.Anything)
	})

	t.Run("disambiguates name when session exists for a different path", func(t *testing.T) {
		c, mockLister, mockNamer, mockStartup, mockTmux := newTestConnector()
		existing := model.SeshSession{Name: "develop", Path: "/repos/other-project/develop"}
		mockLister.On("FindTmuxSession", "develop").Return(existing, true)
		mockNamer.On("NameWithLength", "/repos/my-project/develop", 2).Return("my-project/develop", nil)
		mockLister.On("FindTmuxSession", "my-project/develop").Return(model.SeshSession{}, false)
		mockTmux.On("NewSession", "my-project/develop", "/repos/my-project/develop").Return("", nil)
		mockStartup.On("Exec", mock.Anything).Return("", nil)
		mockTmux.On("StartServer").Return(nil)
		mockTmux.On("SwitchOrAttach", "my-project/develop", mock.Anything).Return("switched", nil)

		connection := model.Connection{
			Found: true,
			New:   true,
			Session: model.SeshSession{
				Src:  "dir",
				Name: "develop",
				Path: "/repos/my-project/develop",
			},
		}
		_, err := connectToTmux(c, connection, model.ConnectOpts{})
		assert.Nil(t, err)
		mockTmux.AssertCalled(t, "NewSession", "my-project/develop", "/repos/my-project/develop")
	})

	t.Run("does not run startup command when session creation fails", func(t *testing.T) {
		c, mockLister, _, mockStartup, mockTmux := newTestConnector()
		mockLister.On("FindTmuxSession", "newproj").Return(model.SeshSession{}, false)
		mockTmux.On("NewSession", "newproj", "/repos/newproj").Return("duplicate session: newproj", assert.AnError)

		connection := model.Connection{
			Found: true,
			New:   true,
			Session: model.SeshSession{
				Src:  "dir",
				Name: "newproj",
				Path: "/repos/newproj",
			},
		}
		_, err := connectToTmux(c, connection, model.ConnectOpts{})
		assert.Error(t, err)
		mockTmux.AssertNotCalled(t, "SendKeys", mock.Anything, mock.Anything)
		mockStartup.AssertNotCalled(t, "Exec", mock.Anything)
		mockTmux.AssertNotCalled(t, "SwitchOrAttach", mock.Anything, mock.Anything)
	})
}
