package seshcli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/joshmedeski/sesh/v2/connector"
	"github.com/joshmedeski/sesh/v2/home"
	"github.com/joshmedeski/sesh/v2/lister"
	"github.com/joshmedeski/sesh/v2/model"
)

func NewRootSessionCommand(l lister.Lister, c connector.Connector, config model.Config, h home.Home) *cobra.Command {
	return &cobra.Command{
		Use:     "root",
		Aliases: []string{"r"},
		Short:   "Connect to the worktree root of the current workspace session",
		RunE: func(cmd *cobra.Command, args []string) error {
			session, exists := l.GetAttachedTmuxSession()
			if !exists {
				return fmt.Errorf("no attached tmux session")
			}

			for _, wsCfg := range config.WorkspaceConfigs {
				if !strings.HasPrefix(session.Name, wsCfg.Name+"/") {
					continue
				}

				rootPath, err := h.ExpandHome(wsCfg.Path)
				if err != nil {
					return fmt.Errorf("failed to expand workspace path: %w", err)
				}

				// Session name format: {workspace}/{subproject}/{wtBranch}
				// Extract the worktree branch (last segment)
				wtBranch := session.Name[strings.LastIndex(session.Name, "/")+1:]

				// If wtBranch matches the repo basename, it's the main worktree (root itself)
				// Otherwise, the worktree is a subdirectory of the workspace root
				worktreePath := rootPath
				if wtBranch != filepath.Base(rootPath) {
					worktreePath = filepath.Join(rootPath, wtBranch)
				}

				// Already at the worktree root — nothing to do
				if session.Path == worktreePath {
					return nil
				}

				_, err = c.Connect(worktreePath, model.ConnectOpts{})
				return err
			}

			// Not a workspace sub-project session by name, but check if we're
			// already at a worktree root (session created by sesh connect <path>)
			for _, wsCfg := range config.WorkspaceConfigs {
				rootPath, err := h.ExpandHome(wsCfg.Path)
				if err != nil {
					continue
				}
				if session.Path == rootPath || filepath.Dir(session.Path) == rootPath {
					return nil
				}
			}

			return fmt.Errorf("current session '%s' is not a workspace session", session.Name)
		},
	}
}
