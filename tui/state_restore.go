package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Popup dimensions for tmux display-popup. Compact = preview off, wide = preview on.
// Hardcoded here so sesh can relaunch itself at the right size without env plumbing.
const (
	popupCompactWidth  = "45%"
	popupCompactHeight = "75%"
	popupWideWidth     = "80%"
	popupWideHeight    = "75%"

	// Env var the relaunched sesh process reads to rehydrate state.
	restoreStateEnvVar = "SESH_RESTORE_STATE"
)

// RestoreState is the TUI state persisted across a kill-and-relaunch toggle.
// Written to disk by ScheduleRelaunch and read by LoadRestoreState in the new process.
type RestoreState struct {
	Filter      string `json:"filter"`
	Cursor      int    `json:"cursor"`
	ShowPreview bool   `json:"show_preview"`
	// SessionName is the selected session at toggle time. Used to re-select it
	// after the list re-filters, since cursor index alone isn't stable across filters.
	SessionName string `json:"session_name,omitempty"`
}

func stateFilePath() string {
	// /tmp is always safe (no spaces, universally writable). A shared fixed
	// filename is fine because sesh TUI is interactive — only one at a time.
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = "/tmp"
	}
	return filepath.Join(dir, "sesh-tui-state.json")
}

// LoadRestoreState reads state from the path in SESH_RESTORE_STATE and deletes
// the file. Returns (nil, false) if the env var is unset or the file is invalid.
func LoadRestoreState() (*RestoreState, bool) {
	path := os.Getenv(restoreStateEnvVar)
	if path == "" {
		return nil, false
	}
	data, err := os.ReadFile(path)
	_ = os.Remove(path)
	if err != nil {
		return nil, false
	}
	var s RestoreState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, false
	}
	return &s, true
}

// ScheduleRelaunch writes state to disk and asks tmux to open a new popup
// at the size matching s.ShowPreview. The current sesh process should exit
// immediately after this call so the current popup closes.
//
// We use `tmux run-shell -b` which runs in tmux's server process, so the
// queued popup opens even after the current popup (and sesh process) die.
func ScheduleRelaunch(s RestoreState) error {
	path := stateFilePath()
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}

	width, height := popupCompactWidth, popupCompactHeight
	if s.ShowPreview {
		width, height = popupWideWidth, popupWideHeight
	}

	// The inner command is what tmux display-popup runs inside the popup.
	// Single-quoting is safe because path has no special chars (/tmp/...).
	popupCmd := fmt.Sprintf(
		"tmux display-popup -E -b none -w %s -h %s 'SESH_RESTORE_STATE=%s sesh tui'",
		width, height, path,
	)
	return exec.Command("tmux", "run-shell", "-b", popupCmd).Run()
}
