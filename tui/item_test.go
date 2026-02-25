package tui

import (
	"testing"

	"github.com/joshmedeski/sesh/v2/model"
)

func TestWorktreeGroupItemInterface(t *testing.T) {
	group := worktreeGroupItem{
		repoName:     "chase-monorepo",
		dormantCount: 2,
		totalCount:   4,
		displayName:  " chase-monorepo [2/4]",
	}

	t.Run("FilterValue returns repo name", func(t *testing.T) {
		if got := group.FilterValue(); got != "chase-monorepo" {
			t.Errorf("FilterValue() = %q, want %q", got, "chase-monorepo")
		}
	})

	t.Run("Title returns displayName", func(t *testing.T) {
		if got := group.Title(); got != " chase-monorepo [2/4]" {
			t.Errorf("Title() = %q, want %q", got, " chase-monorepo [2/4]")
		}
	})

	t.Run("Description returns empty", func(t *testing.T) {
		if got := group.Description(); got != "" {
			t.Errorf("Description() = %q, want %q", got, "")
		}
	})
}

func TestSeparatorItem(t *testing.T) {
	sep := separatorItem{}

	t.Run("FilterValue returns empty string", func(t *testing.T) {
		if sep.FilterValue() != "" {
			t.Errorf("expected empty FilterValue, got %q", sep.FilterValue())
		}
	})

	t.Run("Title returns empty string", func(t *testing.T) {
		if sep.Title() != "" {
			t.Errorf("expected empty Title, got %q", sep.Title())
		}
	})

	t.Run("Description returns empty string", func(t *testing.T) {
		if sep.Description() != "" {
			t.Errorf("expected empty Description, got %q", sep.Description())
		}
	})
}

func TestSessionItemInterface(t *testing.T) {
	item := sessionItem{
		session:     model.SeshSession{Name: "test", Src: "tmux"},
		displayName: " test",
	}

	if got := item.FilterValue(); got != "test" {
		t.Errorf("FilterValue() = %q, want %q", got, "test")
	}
	if got := item.Title(); got != " test" {
		t.Errorf("Title() = %q, want %q", got, " test")
	}
	if got := item.Description(); got != "" {
		t.Errorf("Description() = %q, want %q", got, "")
	}
}
