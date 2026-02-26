package model

type (
	Config struct {
		StrictMode           bool                 `toml:"strict_mode"`
		ImportPaths          []string             `toml:"import"`
		DefaultSessionConfig DefaultSessionConfig `toml:"default_session"`
		Blacklist            []string             `toml:"blacklist"`
		SessionConfigs       []SessionConfig      `toml:"session"`
		SortOrder            []string             `toml:"sort_order"`
		WindowConfigs        []WindowConfig       `toml:"window"`
		DirLength            int                  `toml:"dir_length"`
		ProjectsConfig       ProjectsConfig       `toml:"projects"`
		ZoxideConfig         ZoxideConfig         `toml:"zoxide"`
		WorkspaceConfigs     []WorkspaceConfig    `toml:"workspace"`
	}
	Evaluation struct {
		StrictMode bool `toml:"strict_mode"`
	}

	DefaultSessionConfig struct {
		// TODO: mention breaking change in v2 release notes
		// StartupScript  string `toml:"startup_script"`
		StartupCommand string   `toml:"startup_command"`
		Tmuxp          string   `toml:"tmuxp"`
		Tmuxinator     string   `toml:"tmuxinator"`
		PreviewCommand string   `toml:"preview_command"`
		Windows        []string `toml:"windows"`
	}

	SessionConfig struct {
		Name                string `toml:"name"`
		Path                string `toml:"path"`
		DisableStartCommand bool   `toml:"disable_startup_command"`
		DefaultSessionConfig
	}

	WindowConfig struct {
		Name          string `toml:"name"`
		StartupScript string `toml:"startup_script"`
		Path          string `toml:"path"`
	}

	ProjectsConfig struct {
		Paths             []string       `toml:"paths"`
		MaxDepth          int            `toml:"max_depth"`
		IncludeWorktrees  bool           `toml:"include_worktrees"`
		Exclude           []string       `toml:"exclude"`
		Saved             []SavedProject `toml:"saved"`
		StartupCommand    string         `toml:"startup_command"`
		GitStartupCommand string         `toml:"git_startup_command"`
	}

	SavedProject struct {
		Path string `toml:"path"`
	}

	ZoxideConfig struct {
		Exclude []string `toml:"exclude"`
	}

	WorkspaceConfig struct {
		Name              string   `toml:"name"`
		Path              string   `toml:"path"`
		Include           []string `toml:"include"`
		Exclude           []string `toml:"exclude"`
		DefaultWorktree   string   `toml:"default_worktree"`
		StartupCommand    string   `toml:"startup_command"`
		GitStartupCommand string   `toml:"git_startup_command"`
	}
)
