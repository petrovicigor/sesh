package lister

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joshmedeski/sesh/v2/model"
)

func listZoxide(l *RealLister) (model.SeshSessions, error) {
	zoxideResults, err := l.zoxide.ListResults()
	if err != nil {
		return model.SeshSessions{}, fmt.Errorf("couldn't list zoxide sessions: %q", err)
	}

	orderedIndex := []string{}
	directory := make(model.SeshSessionMap)

	for _, r := range zoxideResults {
		// Skip if path matches any exclude pattern
		if shouldExcludeZoxidePath(r.Path, l.config.ZoxideConfig.Exclude) {
			continue
		}

		name, err := l.home.ShortenHome(r.Path)
		if err != nil {
			return model.SeshSessions{}, fmt.Errorf("couldn't shorten path: %q", err)
		}
		key := fmt.Sprintf("zoxide:%s", name)
		orderedIndex = append(orderedIndex, key)
		directory[key] = model.SeshSession{
			Src:   "zoxide",
			Name:  name,
			Path:  r.Path,
			Score: r.Score,
		}
	}
	return model.SeshSessions{
		Directory:    directory,
		OrderedIndex: orderedIndex,
	}, nil
}

// shouldExcludeZoxidePath checks if a path matches any exclude pattern
func shouldExcludeZoxidePath(path string, patterns []string) bool {
	for _, pattern := range patterns {
		// Expand ~ in pattern
		expandedPattern := pattern
		if strings.HasPrefix(pattern, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				expandedPattern = filepath.Join(home, pattern[2:])
			}
		}
		// Check if path starts with pattern (prefix match)
		if strings.HasPrefix(path, expandedPattern) {
			return true
		}
	}
	return false
}

func (l *RealLister) FindZoxideSession(path string) (model.SeshSession, bool) {
	result, err := l.zoxide.Query(path)
	if err != nil {
		return model.SeshSession{}, false
	}
	return model.SeshSession{
		Src:   "zoxide",
		Name:  result.Path,
		Path:  result.Path,
		Score: result.Score,
	}, true
}
