package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/joshmedeski/sesh/v2/model"
)

func TestPartitionItemsByTmux(t *testing.T) {
	tests := []struct {
		name     string
		input    []list.Item
		expected []list.Item
	}{
		{
			name:     "empty slice",
			input:    []list.Item{},
			expected: []list.Item{},
		},
		{
			name: "single tmux item",
			input: []list.Item{
				sessionItem{session: model.SeshSession{Src: "tmux", Name: "tmux1"}},
			},
			expected: []list.Item{
				sessionItem{session: model.SeshSession{Src: "tmux", Name: "tmux1"}},
			},
		},
		{
			name: "single non-tmux item",
			input: []list.Item{
				sessionItem{session: model.SeshSession{Src: "config", Name: "config1"}},
			},
			expected: []list.Item{
				sessionItem{session: model.SeshSession{Src: "config", Name: "config1"}},
			},
		},
		{
			name: "tmux items first, then others",
			input: []list.Item{
				sessionItem{session: model.SeshSession{Src: "tmux", Name: "tmux1"}},
				sessionItem{session: model.SeshSession{Src: "config", Name: "config1"}},
				sessionItem{session: model.SeshSession{Src: "tmux", Name: "tmux2"}},
				sessionItem{session: model.SeshSession{Src: "zoxide", Name: "zoxide1"}},
			},
			expected: []list.Item{
				sessionItem{session: model.SeshSession{Src: "tmux", Name: "tmux1"}},
				sessionItem{session: model.SeshSession{Src: "tmux", Name: "tmux2"}},
				sessionItem{session: model.SeshSession{Src: "config", Name: "config1"}},
				sessionItem{session: model.SeshSession{Src: "zoxide", Name: "zoxide1"}},
			},
		},
		{
			name: "all tmux items",
			input: []list.Item{
				sessionItem{session: model.SeshSession{Src: "tmux", Name: "tmux1"}},
				sessionItem{session: model.SeshSession{Src: "tmux", Name: "tmux2"}},
				sessionItem{session: model.SeshSession{Src: "tmux", Name: "tmux3"}},
			},
			expected: []list.Item{
				sessionItem{session: model.SeshSession{Src: "tmux", Name: "tmux1"}},
				sessionItem{session: model.SeshSession{Src: "tmux", Name: "tmux2"}},
				sessionItem{session: model.SeshSession{Src: "tmux", Name: "tmux3"}},
			},
		},
		{
			name: "no tmux items",
			input: []list.Item{
				sessionItem{session: model.SeshSession{Src: "config", Name: "config1"}},
				sessionItem{session: model.SeshSession{Src: "zoxide", Name: "zoxide1"}},
				sessionItem{session: model.SeshSession{Src: "projects", Name: "project1"}},
			},
			expected: []list.Item{
				sessionItem{session: model.SeshSession{Src: "config", Name: "config1"}},
				sessionItem{session: model.SeshSession{Src: "zoxide", Name: "zoxide1"}},
				sessionItem{session: model.SeshSession{Src: "projects", Name: "project1"}},
			},
		},
		{
			name: "preserves order within groups",
			input: []list.Item{
				sessionItem{session: model.SeshSession{Src: "config", Name: "config1"}},
				sessionItem{session: model.SeshSession{Src: "tmux", Name: "tmux1"}},
				sessionItem{session: model.SeshSession{Src: "zoxide", Name: "zoxide1"}},
				sessionItem{session: model.SeshSession{Src: "tmux", Name: "tmux2"}},
				sessionItem{session: model.SeshSession{Src: "config", Name: "config2"}},
				sessionItem{session: model.SeshSession{Src: "tmux", Name: "tmux3"}},
			},
			expected: []list.Item{
				sessionItem{session: model.SeshSession{Src: "tmux", Name: "tmux1"}},
				sessionItem{session: model.SeshSession{Src: "tmux", Name: "tmux2"}},
				sessionItem{session: model.SeshSession{Src: "tmux", Name: "tmux3"}},
				sessionItem{session: model.SeshSession{Src: "config", Name: "config1"}},
				sessionItem{session: model.SeshSession{Src: "zoxide", Name: "zoxide1"}},
				sessionItem{session: model.SeshSession{Src: "config", Name: "config2"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := partitionItemsByTmux(tt.input)

			if len(result) != len(tt.expected) {
				t.Fatalf("expected length %d, got %d", len(tt.expected), len(result))
			}

			for i := range result {
				resultItem, ok := result[i].(sessionItem)
				if !ok {
					t.Fatalf("result[%d] is not a sessionItem", i)
				}
				expectedItem, ok := tt.expected[i].(sessionItem)
				if !ok {
					t.Fatalf("expected[%d] is not a sessionItem", i)
				}

				if resultItem.session.Src != expectedItem.session.Src {
					t.Errorf("result[%d].Src = %s, expected %s", i, resultItem.session.Src, expectedItem.session.Src)
				}
				if resultItem.session.Name != expectedItem.session.Name {
					t.Errorf("result[%d].Name = %s, expected %s", i, resultItem.session.Name, expectedItem.session.Name)
				}
			}
		})
	}
}
