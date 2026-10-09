package tui

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"github.com/joshmedeski/sesh/v2/model"
)

// initTestRepo creates a temp git repo with tracked.txt ("a\nb\nc\n") committed.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runTestGit(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, dir, "add", ".")
	runTestGit(t, dir, "-c", "user.email=test@test", "-c", "user.name=test",
		"commit", "-q", "-m", "init")
	return dir
}

func runTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestGitChangesBadge(t *testing.T) {
	tests := []struct {
		name    string
		c       gitChanges
		want    []string // substrings that must appear, in this order
		wantNot []string
	}{
		{"clean is empty", gitChanges{}, nil, []string{"+", "-", "?"}},
		{"added and deleted", gitChanges{Added: 12, Deleted: 5}, []string{"+12", "-5"}, []string{"?"}},
		{"deleted only", gitChanges{Deleted: 3}, []string{"-3"}, []string{"+", "?"}},
		{"untracked only", gitChanges{Untracked: 2}, []string{"?2"}, []string{"+", "-"}},
		{"all parts in order", gitChanges{Added: 1, Deleted: 2, Untracked: 3}, []string{"+1", "-2", "?3"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.c.badge()
			if tt.want == nil && got != "" {
				t.Fatalf("badge() = %q, want empty", got)
			}
			pos := 0
			for _, sub := range tt.want {
				idx := strings.Index(got[pos:], sub)
				if idx < 0 {
					t.Fatalf("badge() = %q, missing %q (in order)", got, sub)
				}
				pos += idx + len(sub)
			}
			for _, sub := range tt.wantNot {
				if strings.Contains(got, sub) {
					t.Errorf("badge() = %q, must not contain %q", got, sub)
				}
			}
		})
	}
}

func TestGitChangesForPaths_ModifiedTracked(t *testing.T) {
	repo := initTestRepo(t)
	// b→X is 1 add + 1 delete; appending d is 1 more add.
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("a\nX\nc\nd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := gitChangesForPaths([]string{repo})[repo]
	if !ok {
		t.Fatal("dirty repo absent from result")
	}
	if got.Added != 2 || got.Deleted != 1 || got.Untracked != 0 {
		t.Errorf("changes = %+v, want {Added:2 Deleted:1 Untracked:0}", got)
	}
}

func TestGitChangesForPaths_UntrackedOnly(t *testing.T) {
	repo := initTestRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := gitChangesForPaths([]string{repo})[repo]
	if !ok {
		t.Fatal("repo with untracked file absent from result")
	}
	if got.Added != 0 || got.Deleted != 0 || got.Untracked != 1 {
		t.Errorf("changes = %+v, want {Added:0 Deleted:0 Untracked:1}", got)
	}
}

func TestGitChangesForPaths_StagedCounts(t *testing.T) {
	repo := initTestRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "staged.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repo, "add", "staged.txt")
	got, ok := gitChangesForPaths([]string{repo})[repo]
	if !ok {
		t.Fatal("repo with staged file absent from result")
	}
	if got.Added != 2 || got.Untracked != 0 {
		t.Errorf("changes = %+v, want Added:2 Untracked:0 (staged counts as tracked)", got)
	}
}

func TestGitChangesForPaths_CleanRepoAbsent(t *testing.T) {
	repo := initTestRepo(t)
	if got, ok := gitChangesForPaths([]string{repo})[repo]; ok {
		t.Errorf("clean repo present in result: %+v", got)
	}
}

func TestGitChangesForPaths_NonRepoAbsent(t *testing.T) {
	dir := t.TempDir()
	if got, ok := gitChangesForPaths([]string{dir})[dir]; ok {
		t.Errorf("non-repo present in result: %+v", got)
	}
}

func TestGitChangesForPaths_NoCommitsCountsUntracked(t *testing.T) {
	dir := t.TempDir()
	runTestGit(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := gitChangesForPaths([]string{dir})[dir]
	if !ok {
		t.Fatal("fresh repo (no HEAD) with untracked file absent from result")
	}
	if got.Untracked != 1 {
		t.Errorf("changes = %+v, want Untracked:1", got)
	}
}

func TestGitChangesForPaths_EmptyInput(t *testing.T) {
	if got := gitChangesForPaths(nil); len(got) != 0 {
		t.Errorf("gitChangesForPaths(nil) = %v, want empty", got)
	}
}

func mkGitTestItem(src, name, path string) sessionItem {
	return sessionItem{
		session:     model.SeshSession{Src: src, Name: name, Path: path},
		displayName: name,
	}
}

func TestTmuxSessionPaths(t *testing.T) {
	items := []list.Item{
		mkGitTestItem("tmux", "alpha", "/proj/alpha"),
		mkGitTestItem("tmux", "alpha2", "/proj/alpha"), // dup path
		mkGitTestItem("tmux", "beta", "/proj/beta"),
		mkGitTestItem("config", "gamma", "/proj/gamma"), // not tmux
		mkGitTestItem("tmux", "nopath", ""),             // empty path
		worktreeGroupItem{repoName: "delta"},            // not a sessionItem
	}
	got := tmuxSessionPaths(items)
	want := []string{"/proj/alpha", "/proj/beta"}
	if len(got) != len(want) {
		t.Fatalf("tmuxSessionPaths = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tmuxSessionPaths = %v, want %v", got, want)
		}
	}
}

func TestDelegateRender_GitBadgeOnTmuxRows(t *testing.T) {
	changes := map[string]gitChanges{"/proj/alpha": {Added: 3, Deleted: 1}}
	d := compactDelegate{gitChanges: &changes}
	tmuxItem := mkGitTestItem("tmux", "alpha", "/proj/alpha")
	l := list.New([]list.Item{tmuxItem}, d, 60, 10)

	var buf bytes.Buffer
	d.Render(&buf, l, 1, tmuxItem) // index != cursor: unselected row
	out := buf.String()
	if !strings.Contains(out, "+3") || !strings.Contains(out, "-1") {
		t.Errorf("tmux row must render change counts, got %q", out)
	}

	// Same path but not tmux-backed: out of scope, no badge.
	buf.Reset()
	d.Render(&buf, l, 1, mkGitTestItem("config", "alpha", "/proj/alpha"))
	if strings.Contains(buf.String(), "+3") {
		t.Errorf("non-tmux row must not render counts, got %q", buf.String())
	}

	// Zero-value delegate (check not landed): no badge, no panic.
	buf.Reset()
	compactDelegate{}.Render(&buf, l, 1, tmuxItem)
	if strings.Contains(buf.String(), "+3") {
		t.Errorf("nil map must not render counts, got %q", buf.String())
	}
}

func TestUpdateGitChangesMsg_FillsSharedMap(t *testing.T) {
	shared := &map[string]gitChanges{"/stale": {Added: 9}}
	m := Model{gitChanges: shared}
	d := compactDelegate{gitChanges: shared} // delegate holds the same pointer

	nm, _ := m.Update(GitChangesMsg{Changes: map[string]gitChanges{
		"/proj/alpha": {Added: 3, Deleted: 1},
	}})
	_ = nm

	got := *d.gitChanges
	if _, stale := got["/stale"]; stale {
		t.Error("stale entries must be cleared on update")
	}
	if c := got["/proj/alpha"]; c.Added != 3 || c.Deleted != 1 {
		t.Errorf("shared map = %v, want /proj/alpha {Added:3 Deleted:1}", got)
	}
}
