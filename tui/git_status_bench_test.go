package tui

import (
	"fmt"
	"io"
	"testing"

	"charm.land/bubbles/v2/list"
)

// Render-path cost of the git badge: these compare a tmux row render with a
// populated badge against one with an empty map — the delta is what every
// visible dirty row pays per frame.

func benchItems() (sessionItem, []list.Item) {
	it := mkGitTestItem("tmux", "alpha", "/proj/alpha")
	items := make([]list.Item, 0, 105)
	for i := 0; i < 105; i++ {
		src := "tmux"
		if i%3 == 0 {
			src = "config"
		}
		items = append(items, mkGitTestItem(src, fmt.Sprintf("s%d", i), fmt.Sprintf("/proj/p%d", i%40)))
	}
	return it, items
}

func BenchmarkDelegateRenderTmuxRow_WithBadge(b *testing.B) {
	it, _ := benchItems()
	changes := map[string]gitChanges{"/proj/alpha": {Added: 34, Deleted: 7, Untracked: 2}}
	d := compactDelegate{gitChanges: &changes}
	l := list.New([]list.Item{it}, d, 60, 10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Render(io.Discard, l, 1, it)
	}
}

func BenchmarkDelegateRenderTmuxRow_NoBadge(b *testing.B) {
	it, _ := benchItems()
	d := compactDelegate{gitChanges: &map[string]gitChanges{}}
	l := list.New([]list.Item{it}, d, 60, 10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Render(io.Discard, l, 1, it)
	}
}

func BenchmarkTmuxSessionPaths_105Items(b *testing.B) {
	_, items := benchItems()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tmuxSessionPaths(items)
	}
}
