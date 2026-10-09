package trail

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/joshmedeski/sesh/v2/claude"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/recent"
)

// fakeRecent records departures and serves entries.
type fakeRecent struct {
	entries    map[string]recent.SessionEntry
	departures []string
}

func (f *fakeRecent) RecordSession(string) error { return nil }
func (f *fakeRecent) RecordDeparture(name, id, title string) error {
	f.departures = append(f.departures, name+"|"+id+"|"+title)
	return nil
}
func (f *fakeRecent) GetEntry(name string) (recent.SessionEntry, bool) {
	e, ok := f.entries[name]
	return e, ok
}
func (f *fakeRecent) GetTimestamp(string) (time.Time, bool)  { return time.Time{}, false }
func (f *fakeRecent) GetAll() map[string]time.Time           { return nil }
func (f *fakeRecent) GetFrecencyScores() map[string]float64 { return nil }

// fakeTmux answers by the joined argv and logs every call.
type fakeTmux struct {
	answers map[string]string
	calls   []string
}

func (f *fakeTmux) run(args ...string) (string, error) {
	key := strings.Join(args, " ")
	f.calls = append(f.calls, key)
	if out, ok := f.answers[key]; ok {
		return out, nil
	}
	return "", nil
}

const (
	spotCmd  = "display-message -p #{session_name}\t#{pane_id}\t#{pane_floating_flag}"
	panesCmd = "list-panes -F #{pane_id}\t#{pane_last}\t#{pane_floating_flag}"
)

func newTrail(r *fakeRecent, tm *fakeTmux) *Trail {
	return &Trail{
		Recent: r,
		Tmux:   tm.run,
		PaneSession: func(_, pane string) (string, string, bool) {
			if pane == "%1" {
				return "cs-1", "Fix login", true
			}
			return "", "", false
		},
		Lookup:     func(string, string) (claude.Session, bool, error) { return claude.Session{}, false, nil },
		Offer:      func(string, string) error { return nil },
		FileExists: func(string) bool { return true },
	}
}

func TestDepart(t *testing.T) {
	t.Run("notes the Claude session in the pane being left", func(t *testing.T) {
		r := &fakeRecent{}
		tm := &fakeTmux{answers: map[string]string{spotCmd: "web\t%1\t0"}}
		newTrail(r, tm).Depart("api")
		assert.Equal(t, []string{"web|cs-1|Fix login"}, r.departures)
	})

	t.Run("a plain pane clears the note", func(t *testing.T) {
		r := &fakeRecent{}
		tm := &fakeTmux{answers: map[string]string{spotCmd: "web\t%9\t0"}}
		newTrail(r, tm).Depart("api")
		assert.Equal(t, []string{"web||"}, r.departures)
	})

	t.Run("from a float, the pane under it", func(t *testing.T) {
		r := &fakeRecent{}
		tm := &fakeTmux{answers: map[string]string{
			spotCmd:  "web\t%50\t1",
			panesCmd: "%3\t0\t0\n%1\t1\t0\n%50\t0\t1",
		}}
		newTrail(r, tm).Depart("api")
		assert.Equal(t, []string{"web|cs-1|Fix login"}, r.departures)
	})

	t.Run("a switch within the session records nothing", func(t *testing.T) {
		r := &fakeRecent{}
		tm := &fakeTmux{answers: map[string]string{spotCmd: "web\t%1\t0"}}
		newTrail(r, tm).Depart("web")
		assert.Empty(t, r.departures)
	})

	t.Run("outside tmux records nothing", func(t *testing.T) {
		r := &fakeRecent{}
		newTrail(r, &fakeTmux{}).Depart("web")
		assert.Empty(t, r.departures)
	})
}

func TestReturn(t *testing.T) {
	noted := &fakeRecent{entries: map[string]recent.SessionEntry{
		"web": {Claude: "cs-1", ClaudeTitle: "Fix login"},
	}}

	t.Run("no note does nothing", func(t *testing.T) {
		tm := &fakeTmux{}
		tr := newTrail(&fakeRecent{}, tm)
		tr.Lookup = func(string, string) (claude.Session, bool, error) {
			t.Fatal("no lookup without a note")
			return claude.Session{}, false, nil
		}
		tr.Return("web")()
		assert.Empty(t, tm.calls)
	})

	t.Run("live: selects its pane before the switch, nothing after", func(t *testing.T) {
		tm := &fakeTmux{answers: map[string]string{"display-message -p -t %1 #{session_name}": "web"}}
		tr := newTrail(noted, tm)
		tr.Lookup = func(string, string) (claude.Session, bool, error) {
			return claude.Session{ID: "cs-1", Pane: "%1", Live: true}, true, nil
		}
		after := tr.Return("web")
		assert.Contains(t, tm.calls, "select-window -t %1 ; select-pane -t %1")
		n := len(tm.calls)
		after()
		assert.Len(t, tm.calls, n)
	})

	t.Run("live in another session: no pane select", func(t *testing.T) {
		tm := &fakeTmux{answers: map[string]string{"display-message -p -t %1 #{session_name}": "elsewhere"}}
		tr := newTrail(noted, tm)
		tr.Lookup = func(string, string) (claude.Session, bool, error) {
			return claude.Session{ID: "cs-1", Pane: "%1", Live: true}, true, nil
		}
		tr.Return("web")()
		for _, c := range tm.calls {
			assert.NotContains(t, c, "select-pane")
		}
	})

	t.Run("ended: offers resume after the switch", func(t *testing.T) {
		tm := &fakeTmux{}
		tr := newTrail(noted, tm)
		tr.Lookup = func(string, string) (claude.Session, bool, error) {
			return claude.Session{ID: "cs-2", Title: "Fix login (resumed)", Transcript: "/t.jsonl"}, true, nil
		}
		var offered []string
		tr.Offer = func(id, session string) error {
			offered = append(offered, id+"@"+session)
			return nil
		}
		after := tr.Return("web")
		assert.Empty(t, offered, "the offer waits for the switch")
		after()
		assert.Equal(t, []string{"cs-2@web"}, offered)
	})

	t.Run("ended but claude-sessions missing: a message", func(t *testing.T) {
		tm := &fakeTmux{}
		tr := newTrail(noted, tm)
		tr.Lookup = func(string, string) (claude.Session, bool, error) {
			return claude.Session{ID: "cs-1", Title: "Fix #12"}, true, nil
		}
		tr.Offer = func(string, string) error { return errors.New("not found") }
		tr.Return("web")()
		assert.Equal(t, []string{"display-message -d 4000 Claude session “Fix ##12” ended — resume it from the picker"}, tm.calls)
	})

	t.Run("deleted: a message", func(t *testing.T) {
		tm := &fakeTmux{}
		tr := newTrail(noted, tm)
		tr.Return("web")()
		assert.Equal(t, []string{"display-message -d 4000 Claude session “Fix login” was deleted"}, tm.calls)
	})

	t.Run("transcript gone: a message, no offer", func(t *testing.T) {
		tm := &fakeTmux{}
		tr := newTrail(noted, tm)
		tr.Lookup = func(string, string) (claude.Session, bool, error) {
			return claude.Session{ID: "cs-1", Title: "Fix login", Transcript: "/gone.jsonl"}, true, nil
		}
		tr.FileExists = func(string) bool { return false }
		tr.Offer = func(string, string) error {
			t.Fatal("no offer without a transcript")
			return nil
		}
		tr.Return("web")()
		assert.Equal(t, []string{"display-message -d 4000 Claude session “Fix login” is gone (no transcript)"}, tm.calls)
	})

	t.Run("unreadable db: nothing, never 'deleted'", func(t *testing.T) {
		tm := &fakeTmux{}
		tr := newTrail(noted, tm)
		tr.Lookup = func(string, string) (claude.Session, bool, error) {
			return claude.Session{}, false, claude.ErrNoDB
		}
		tr.Return("web")()
		assert.Empty(t, tm.calls)
	})
}

// stubConnector records what reached the wrapped connector.
type stubConnector struct{ got []string }

func (s *stubConnector) Connect(name string, opts model.ConnectOpts) (string, error) {
	s.got = append(s.got, name)
	return "", nil
}

func TestConnector(t *testing.T) {
	t.Run("departs, selects the pane, then connects", func(t *testing.T) {
		r := &fakeRecent{}
		tm := &fakeTmux{answers: map[string]string{spotCmd: "web\t%1\t0"}}
		inner := &stubConnector{}
		_, err := Connector{Connector: inner, T: newTrail(r, tm)}.Connect("api", model.ConnectOpts{Pane: "%7"})
		assert.NoError(t, err)
		assert.Equal(t, []string{"web|cs-1|Fix login"}, r.departures)
		assert.Contains(t, tm.calls, "select-window -t %7 ; select-pane -t %7")
		assert.Equal(t, []string{"api"}, inner.got)
	})

	t.Run("SkipRecent skips the departure", func(t *testing.T) {
		r := &fakeRecent{}
		tm := &fakeTmux{answers: map[string]string{spotCmd: "web\t%1\t0"}}
		_, _ = Connector{Connector: &stubConnector{}, T: newTrail(r, tm)}.Connect("api", model.ConnectOpts{SkipRecent: true})
		assert.Empty(t, r.departures)
	})
}
