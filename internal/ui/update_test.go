package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	cand "github.com/rwilgaard/thop/internal/candidates"
	"github.com/rwilgaard/thop/internal/config"
	"github.com/rwilgaard/thop/internal/tmux"
)

func TestCursorKeys(t *testing.T) {
	cs := []cand.Candidate{
		{AbsPath: "/p/a", RelPath: "a"},
		{AbsPath: "/p/b", RelPath: "b"},
		{AbsPath: "/p/c", RelPath: "c"},
	}
	tests := []struct {
		name   string
		layout string
		keys   []tea.Msg
		want   int
	}{
		{"down", "top", []tea.Msg{keyDown}, 1},
		{"up at the top wraps to the last row", "top", []tea.Msg{keyUp}, 2},
		{"down at the last row wraps to the top", "top", []tea.Msg{keyDown, keyDown, keyDown}, 0},
		{"ctrl+j and ctrl+k", "top", []tea.Msg{keyCtrl('j'), keyCtrl('j'), keyCtrl('k')}, 1},
		{"ctrl+p is unbound", "top", []tea.Msg{keyDown, keyCtrl('p')}, 1},
		// bottom layout lists best match last, so keys follow the screen
		{"bottom: up moves away from the search bar", "bottom", []tea.Msg{keyUp}, 1},
		{"bottom: down at the search bar wraps", "bottom", []tea.Msg{keyDown}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModel(cs, map[string]float64{}, tmux.State{}, false, config.Config{Layout: tt.layout}, false)
			m = press(m, tt.keys...)
			if m.cursor != tt.want || m.inputMode != modeNormal {
				t.Errorf("cursor = %d mode = %v, want cursor %d in modeNormal", m.cursor, m.inputMode, tt.want)
			}
		})
	}
}

func TestPaging(t *testing.T) {
	var cs []cand.Candidate
	for i := range 30 {
		name := fmt.Sprintf("p%02d", i)
		cs = append(cs, cand.Candidate{AbsPath: "/p/" + name, RelPath: name})
	}
	pgup := tea.KeyPressMsg{Code: tea.KeyPgUp}
	pgdn := tea.KeyPressMsg{Code: tea.KeyPgDown}
	ctrlU := keyCtrl('u')
	ctrlD := keyCtrl('d')

	tests := []struct {
		name   string
		layout string
		keys   []tea.KeyPressMsg
		want   int
	}{
		{"page down", "top", []tea.KeyPressMsg{pgdn}, 8},
		{"page down clamps at end", "top", []tea.KeyPressMsg{pgdn, pgdn, pgdn, pgdn, pgdn}, 29},
		{"page up clamps at start", "top", []tea.KeyPressMsg{pgdn, pgup, pgup}, 0},
		{"ctrl+d pages down", "top", []tea.KeyPressMsg{ctrlD, ctrlD}, 16},
		{"ctrl+u pages up", "top", []tea.KeyPressMsg{ctrlD, ctrlD, ctrlU}, 8},
		{"bottom layout: page up moves away from the search bar", "bottom", []tea.KeyPressMsg{pgup}, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModel(cs, map[string]float64{}, tmux.State{}, false, config.Config{Layout: tt.layout}, false)
			m.width, m.height, m.ready = 80, 12, true // 8 list rows
			for _, k := range tt.keys {
				m = press(m, k)
			}
			if m.cursor != tt.want {
				t.Errorf("cursor = %d, want %d", m.cursor, tt.want)
			}
		})
	}

	m := testModel(cs...)
	m = sized(m, 80, 12)
	if out := press(m, pgdn).View().Content; !strings.Contains(out, "9/30") {
		t.Errorf("status should show position 9/30: %q", out)
	}
}

func TestEnter_emptyListStays(t *testing.T) {
	tests := []struct {
		name string
		mode inputMode
	}{
		{"picker", modeNormal},
		{"dest picker", modeDestPicker},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel()
			m.inputMode = tt.mode
			updated, cmd := m.Update(keyEnter)
			if cmd != nil {
				t.Error("enter on an empty list should do nothing")
			}
			if got := updated.(model).inputMode; got != tt.mode {
				t.Errorf("mode changed to %v", got)
			}
		})
	}
}

func TestFilterCycle(t *testing.T) {
	cs := []cand.Candidate{
		{AbsPath: "/p/proj", RelPath: "proj"},
		{AbsPath: "/p/proj/repo", RelPath: "proj/repo", IsRepo: true},
		{AbsPath: "/t/scratch", RelPath: "scratch", IsTmp: true},
	}
	ts := tmux.State{Sessions: map[string]bool{"scratch": true}}
	m := newModel(cs, map[string]float64{}, ts, false, config.Config{}, false)

	tab := tea.KeyPressMsg{Code: tea.KeyTab}
	shiftTab := tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	steps := []struct {
		key   tea.KeyPressMsg
		view  viewMode
		count int
	}{
		{tab, viewProject, 1},
		{tab, viewRepo, 1},
		{tab, viewTmp, 1},
		{tab, viewOpen, 1},
		{tab, viewAll, 3},
		{shiftTab, viewOpen, 1},
		{shiftTab, viewTmp, 1},
	}
	for i, st := range steps {
		m = press(m, st.key)
		if m.view != st.view || len(m.filtered) != st.count {
			t.Errorf("step %d: view = %v with %d rows, want %v with %d", i, m.view, len(m.filtered), st.view, st.count)
		}
	}

	if got := newModel(cs, map[string]float64{}, ts, true, config.Config{}, false); got.view != viewOpen {
		t.Errorf("-s should start in the open filter, got %v", got.view)
	}
}

func TestClose(t *testing.T) {
	cs := []cand.Candidate{
		{AbsPath: "/p/group", RelPath: "group"},
		{AbsPath: "/p/group/api", RelPath: "group/api", IsRepo: true},
		{AbsPath: "/p/idle", RelPath: "idle"},
	}
	open := tmux.State{
		Sessions: map[string]bool{"group": true},
		Windows:  map[string]bool{"group/api": true, "group/web": true},
	}
	ctrlQ := keyCtrl('q')
	y := keyRune('y')
	n := keyRune('n')

	tests := []struct {
		name        string
		row         string
		confirm     tea.KeyPressMsg
		wantTitle   string
		wantSession string
		wantWindow  string
	}{
		{"session", "group", y, "Close session group (2 windows)?", "group", ""},
		{"window", "group/api", y, "Close window group/api?", "group", "api"},
		{"cancel", "group", n, "Close session group (2 windows)?", "", ""},
		{"not open", "idle", y, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModel(cs, map[string]float64{}, open, false, config.Config{}, false)
			m = sized(m, 80, 24)
			var gotSession, gotWindow string
			m.killSession = func(_ tmux.State, s string) error {
				gotSession = s
				return nil
			}
			m.killWindow = func(s, w string) error {
				gotSession, gotWindow = s, w
				return nil
			}
			m.loadState = func() tmux.State { return tmux.State{} }
			for i, it := range m.filtered {
				if it.base.candidate.RelPath == tt.row {
					m.cursor = i
				}
			}

			m = press(m, ctrlQ)
			if tt.wantTitle == "" {
				if m.inputMode != modeNormal {
					t.Fatalf("close on a row that isn't open should do nothing, got mode %v", m.inputMode)
				}
				return
			}
			if m.inputMode != modeConfirmClose {
				t.Fatalf("mode = %v, want modeConfirmClose", m.inputMode)
			}
			if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, tt.wantTitle) {
				t.Errorf("missing %q in:\n%s", tt.wantTitle, plain)
			}

			m = press(m, tt.confirm)
			if m.inputMode != modeNormal {
				t.Errorf("mode after confirm = %v, want modeNormal", m.inputMode)
			}
			if gotSession != tt.wantSession || gotWindow != tt.wantWindow {
				t.Errorf("killed %q/%q, want %q/%q", gotSession, gotWindow, tt.wantSession, tt.wantWindow)
			}
			if tt.wantSession != "" {
				for _, it := range m.all {
					if it.active {
						t.Errorf("%s still marked open after state reload", it.candidate.RelPath)
					}
				}
			}
		})
	}
}

func TestLoadingSpinner(t *testing.T) {
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{TmpPath: t.TempDir()}, false)
	m = sized(m, 80, 24)
	m.inputMode = modeNameInput
	m.tmp.tiName.SetValue("foo")
	m.inTmux = true

	updated, cmd := m.Update(keyEnter)
	m = updated.(model)
	if m.inputMode != modeLoading {
		t.Fatalf("expected modeLoading, got %v", m.inputMode)
	}
	if cmd == nil {
		t.Fatal("entering loading should return a batched cmd (create + spinner tick)")
	}
	out := m.View().Content
	if !strings.Contains(out, m.spin.View()) {
		t.Errorf("loading view should contain spinner frame %q: %q", m.spin.View(), out)
	}
	if !strings.Contains(out, "Creating…") {
		t.Errorf("loading view should contain loading text: %q", out)
	}
}

func TestAddCandidates_reresolves(t *testing.T) {
	cs := cand.Resolve([]cand.Candidate{{AbsPath: "/code/alpha", Root: "/code", RelPath: "alpha"}})
	m := testModel(cs...)
	if got := m.sessionOf("/code/alpha"); got != "alpha" {
		t.Fatalf("session = %q, want alpha", got)
	}
	m.addCandidates(cand.Candidate{AbsPath: "/work/alpha", Root: "/work", RelPath: "alpha"})
	if a, b := m.sessionOf("/code/alpha"), m.sessionOf("/work/alpha"); a != "alpha@code" || b != "alpha@work" {
		t.Errorf("sessions = %q, %q; want alpha@code, alpha@work", a, b)
	}
}
