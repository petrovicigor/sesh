package lister

import (
	"fmt"

	"github.com/joshmedeski/sesh/v2/model"
)

func tmuxKey(name string) string {
	return fmt.Sprintf("tmux:%s", name)
}

func listTmux(l *RealLister) (model.SeshSessions, error) {
	// Return cached if available
	if l.tmuxCacheLoaded {
		return l.tmuxCache, nil
	}

	tmuxSessions, err := l.tmux.ListSessions()
	if err != nil {
		return model.SeshSessions{}, fmt.Errorf("couldn't list tmux sessions: %q", err)
	}

	directory := make(map[string]model.SeshSession)
	orderedIndex := []string{}

	for _, session := range tmuxSessions {
		key := tmuxKey(session.Name)
		orderedIndex = append(orderedIndex, key)
		directory[key] = model.SeshSession{
			Src:      "tmux",
			Name:     session.Name,
			Path:     session.Path,
			Attached: session.Attached,
			Windows:  session.Windows,
		}
	}

	result := model.SeshSessions{
		Directory:    directory,
		OrderedIndex: orderedIndex,
	}

	// Cache the result
	l.tmuxCache = result
	l.tmuxCacheLoaded = true

	return result, nil
}

func (l *RealLister) FindTmuxSession(name string) (model.SeshSession, bool) {
	sessions, err := listTmux(l)
	if err != nil {
		return model.SeshSession{}, false
	}
	key := tmuxKey(name)
	if session, exists := sessions.Directory[key]; exists {
		return session, exists
	} else {
		return model.SeshSession{}, false
	}
}

func (l *RealLister) GetLastTmuxSession() (model.SeshSession, bool) {
	sessions, err := listTmux(l)
	if err != nil {
		return model.SeshSession{}, false
	}
	if len(sessions.OrderedIndex) < 2 {
		return model.SeshSession{}, false
	}
	secondSessionIndex := sessions.OrderedIndex[1]
	return sessions.Directory[secondSessionIndex], true
}

func (l *RealLister) ListTmuxSessions() model.SeshSessions {
	sessions, _ := listTmux(l)
	return sessions
}

func (l *RealLister) GetAttachedTmuxSession() (model.SeshSession, bool) {
	return GetAttachedTmuxSession(l)
}

func GetAttachedTmuxSession(l *RealLister) (model.SeshSession, bool) {
	sessions, err := listTmux(l)
	if err != nil {
		return model.SeshSession{}, false
	}
	for _, key := range sessions.OrderedIndex {
		session := sessions.Directory[key]
		if session.Attached != 0 {
			return session, true
		}
	}
	return model.SeshSession{}, false
}

func (l *RealLister) FindTmuxSessionByPath(path string) (model.SeshSession, bool) {
	// Normalize path by trimming trailing slashes
	normalizedPath := path
	if len(normalizedPath) > 1 && normalizedPath[len(normalizedPath)-1] == '/' {
		normalizedPath = normalizedPath[:len(normalizedPath)-1]
	}

	sessions, err := listTmux(l)
	if err != nil {
		return model.SeshSession{}, false
	}
	for _, key := range sessions.OrderedIndex {
		session := sessions.Directory[key]
		// Normalize session path as well
		sessionPath := session.Path
		if len(sessionPath) > 1 && sessionPath[len(sessionPath)-1] == '/' {
			sessionPath = sessionPath[:len(sessionPath)-1]
		}
		if sessionPath == normalizedPath {
			return session, true
		}
	}
	return model.SeshSession{}, false
}
