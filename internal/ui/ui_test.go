package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	cand "github.com/rwilgaard/thop/internal/candidates"
	"github.com/rwilgaard/thop/internal/tmux"
)

func TestNormalizeScores(t *testing.T) {
	tests := []struct {
		name   string
		input  map[string]float64
		expect map[string]float64
	}{
		{
			name:   "empty input",
			input:  map[string]float64{},
			expect: map[string]float64{},
		},
		{
			name:   "all zero returns empty (map miss = 0.0)",
			input:  map[string]float64{"a": 0, "b": 0},
			expect: map[string]float64{},
		},
		{
			name:  "normalizes to 0-1 range",
			input: map[string]float64{"a": 2.0, "b": 1.0, "c": 4.0},
			expect: map[string]float64{
				"a": 0.5,
				"b": 0.25,
				"c": 1.0,
			},
		},
		{
			name:   "single item becomes 1.0",
			input:  map[string]float64{"a": 5.0},
			expect: map[string]float64{"a": 1.0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeScores(tt.input)
			if len(got) != len(tt.expect) {
				t.Fatalf("len mismatch: got %d want %d", len(got), len(tt.expect))
			}
			for k, want := range tt.expect {
				if got[k] != want {
					t.Errorf("key %q: got %v want %v", k, got[k], want)
				}
			}
		})
	}
}

func TestCombineScore(t *testing.T) {
	tests := []struct {
		name         string
		normFuzzy    float64
		normFrecency float64
		want         float64
	}{
		{"both zero", 0.0, 0.0, 0.0},
		{"fuzzy only", 1.0, 0.0, 0.6},
		{"frecency only", 0.0, 1.0, 0.4},
		{"combined", 1.0, 1.0, 1.0},
		{"weighted mix", 0.5, 0.5, 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := combineScore(tt.normFuzzy, tt.normFrecency)
			// float64 comparison: allow epsilon
			const eps = 1e-9
			diff := got - tt.want
			if diff > eps || diff < -eps {
				t.Errorf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestRebuildFiltered(t *testing.T) {
	makeModel := func(items []baseItem, frecency map[string]float64) model {
		m := newModel(nil, frecency, tmux.TmuxState{}, false)
		m.all = items
		m.rebuildFiltered()
		return m
	}

	t.Run("empty query orders by frecency descending", func(t *testing.T) {
		items := []baseItem{
			{candidate: cand.Candidate{AbsPath: "/p/low", RelPath: "low"}},
			{candidate: cand.Candidate{AbsPath: "/p/high", RelPath: "high"}},
			{candidate: cand.Candidate{AbsPath: "/p/mid", RelPath: "mid"}},
		}
		m := makeModel(items, map[string]float64{
			"/p/low":  1.0,
			"/p/high": 3.0,
			"/p/mid":  2.0,
		})
		want := []string{"high", "mid", "low"}
		if len(m.filtered) != len(want) {
			t.Fatalf("got %d items, want %d", len(m.filtered), len(want))
		}
		for i, w := range want {
			if got := m.filtered[i].base.candidate.RelPath; got != w {
				t.Errorf("position %d: got %q, want %q", i, got, w)
			}
		}
	})

	t.Run("non-empty query: high-frecency item beats low-frecency with equal fuzzy match", func(t *testing.T) {
		// Both items share the same "abc" prefix so their fuzzy scores are
		// identical. After normalization both get normFuzzy=1.0, and the 40%
		// frecency weight determines order.
		items := []baseItem{
			{candidate: cand.Candidate{AbsPath: "/p/abc-alpha", RelPath: "abc-alpha"}},
			{candidate: cand.Candidate{AbsPath: "/p/abc-omega", RelPath: "abc-omega"}},
		}
		m := makeModel(items, map[string]float64{
			"/p/abc-alpha": 10.0, // high frecency
			"/p/abc-omega": 1.0,  // low frecency
		})
		m.tiQuery.SetValue("abc")
		m.rebuildFiltered()

		if len(m.filtered) < 2 {
			t.Fatalf("expected at least 2 results, got %d", len(m.filtered))
		}
		if got := m.filtered[0].base.candidate.RelPath; got != "abc-alpha" {
			t.Errorf("expected abc-alpha first (high frecency), got %q", got)
		}
	})

	t.Run("viewProject excludes repos", func(t *testing.T) {
		items := []baseItem{
			{candidate: cand.Candidate{RelPath: "myproject", IsRepo: false}},
			{candidate: cand.Candidate{RelPath: "myrepo", IsRepo: true}},
		}
		m := makeModel(items, map[string]float64{})
		m.view = viewProject
		m.rebuildFiltered()

		if len(m.filtered) != 1 {
			t.Fatalf("expected 1 item, got %d", len(m.filtered))
		}
		if m.filtered[0].base.candidate.IsRepo {
			t.Errorf("viewProject should exclude repos, got IsRepo=true item %q", m.filtered[0].base.candidate.RelPath)
		}
	})

	t.Run("viewRepo excludes non-repos", func(t *testing.T) {
		items := []baseItem{
			{candidate: cand.Candidate{RelPath: "myproject", IsRepo: false}},
			{candidate: cand.Candidate{RelPath: "myrepo", IsRepo: true}},
		}
		m := makeModel(items, map[string]float64{})
		m.view = viewRepo
		m.rebuildFiltered()

		if len(m.filtered) != 1 {
			t.Fatalf("expected 1 item, got %d", len(m.filtered))
		}
		if !m.filtered[0].base.candidate.IsRepo {
			t.Errorf("viewRepo should exclude non-repos, got IsRepo=false item %q", m.filtered[0].base.candidate.RelPath)
		}
	})

	t.Run("switchOnly excludes inactive items", func(t *testing.T) {
		items := []baseItem{
			{candidate: cand.Candidate{RelPath: "active-session", IsRepo: false}, active: true},
			{candidate: cand.Candidate{RelPath: "inactive-session", IsRepo: false}, active: false},
		}
		m := makeModel(items, map[string]float64{})
		m.switchOnly = true
		m.rebuildFiltered()

		if len(m.filtered) != 1 {
			t.Fatalf("expected 1 item with switchOnly, got %d", len(m.filtered))
		}
		if got := m.filtered[0].base.candidate.RelPath; got != "active-session" {
			t.Errorf("expected active-session, got %q", got)
		}
	})

	t.Run("switchOnly with query excludes inactive items", func(t *testing.T) {
		items := []baseItem{
			{candidate: cand.Candidate{RelPath: "proj-active", IsRepo: false}, active: true},
			{candidate: cand.Candidate{RelPath: "proj-inactive", IsRepo: false}, active: false},
		}
		m := makeModel(items, map[string]float64{})
		m.switchOnly = true
		m.tiQuery.SetValue("proj")
		m.rebuildFiltered()

		for _, item := range m.filtered {
			if !item.base.active {
				t.Errorf("switchOnly should exclude inactive items, got %q (active=false)", item.base.candidate.RelPath)
			}
		}
	})
}

func TestView(t *testing.T) {
	cs := []cand.Candidate{
		{AbsPath: "/p/golang/foo", RelPath: "golang/foo", IsRepo: true},
		{AbsPath: "/p/work", RelPath: "work", IsRepo: false},
	}
	scores := map[string]float64{"/p/golang/foo": 1.0}
	ts := tmux.TmuxState{
		Sessions: map[string]bool{"golang": true},
		Windows:  map[string]bool{"golang/foo": true},
	}
	m := newModel(cs, scores, ts, false)
	m.width = 80
	m.height = 24
	m.ready = true

	out := m.View().Content

	checks := []struct {
		want string
		desc string
	}{
		{"❯", "prompt glyph"},
		{"<ctrl-g> Clone", "clone hint"},
		{"<ctrl-a> All", "view hints"},
		{"golang/foo", "first item"},
		{"work", "second item"},
		{"● open", "active indicator"},
		{"● all", "status bar active view"},
		{"items", "item count"},
	}
	for _, c := range checks {
		if !strings.Contains(out, c.want) {
			t.Errorf("View() missing %s: %q not found", c.desc, c.want)
		}
	}
}

func TestUpdateURLInput(t *testing.T) {
	m := newModel(nil, map[string]float64{}, tmux.TmuxState{}, false)
	m.inputMode = modeURLInput
	_ = m.tiURL.Focus()

	// Type characters
	for _, ch := range []string{"h", "t", "t", "p"} {
		msg := tea.KeyPressMsg{Text: ch, Code: rune(ch[0])}
		updated, _ := m.Update(msg)
		m = updated.(model)
	}
	if m.tiURL.Value() != "http" {
		t.Errorf("tiURL = %q, want %q", m.tiURL.Value(), "http")
	}

	// Backspace
	bsp := tea.KeyPressMsg{Code: tea.KeyBackspace}
	updated, _ := m.Update(bsp)
	m = updated.(model)
	if m.tiURL.Value() != "htt" {
		t.Errorf("after backspace tiURL = %q, want %q", m.tiURL.Value(), "htt")
	}

	// Esc cancels
	esc := tea.KeyPressMsg{Code: tea.KeyEscape}
	updated, _ = m.Update(esc)
	m = updated.(model)
	if m.inputMode != modeNormal {
		t.Errorf("esc should return to modeNormal, got %v", m.inputMode)
	}
	if m.tiURL.Value() != "" {
		t.Errorf("esc should clear tiURL, got %q", m.tiURL.Value())
	}
	if !m.tiQuery.Focused() {
		t.Error("esc should focus tiQuery")
	}

	// Enter with non-empty URL advances to modeDestPicker
	m2 := newModel(nil, map[string]float64{}, tmux.TmuxState{}, false)
	m2.inputMode = modeURLInput
	_ = m2.tiURL.Focus()
	for _, ch := range []string{"h", "t", "t", "p"} {
		msg := tea.KeyPressMsg{Text: ch, Code: rune(ch[0])}
		updated2, _ := m2.Update(msg)
		m2 = updated2.(model)
	}
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	updated2, _ := m2.Update(enter)
	m2 = updated2.(model)
	if m2.inputMode != modeDestPicker {
		t.Errorf("enter should advance to modeDestPicker, got %v", m2.inputMode)
	}
}

func TestUpdateDestPicker_conflict(t *testing.T) {
	// Create a candidate dir and pre-create the expected repo subdir to trigger conflict.
	parentDir := t.TempDir()
	conflictDir := filepath.Join(parentDir, "myrepo")
	if err := os.MkdirAll(conflictDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cs := []cand.Candidate{
		{AbsPath: parentDir, Root: filepath.Dir(parentDir), RelPath: filepath.Base(parentDir), IsRepo: false},
	}
	m := newModel(cs, map[string]float64{}, tmux.TmuxState{}, false)
	m.inputMode = modeDestPicker
	m.tiURL.SetValue("https://github.com/user/myrepo")
	_ = m.tiDest.Focus()
	m.rebuildDestFiltered()

	// Select the candidate and press enter — should detect conflict.
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	updated, _ := m.Update(enter)
	m = updated.(model)
	if m.inputMode != modeCloneName {
		t.Errorf("conflict should advance to modeCloneName, got %v", m.inputMode)
	}
	if m.tiCloneName.Value() != "myrepo" {
		t.Errorf("tiCloneName = %q, want %q", m.tiCloneName.Value(), "myrepo")
	}

	// Type a new name and confirm.
	for _, ch := range []string{"2"} {
		msg := tea.KeyPressMsg{Text: ch, Code: rune(ch[0])}
		updated2, _ := m.Update(msg)
		m = updated2.(model)
	}
	enter2 := tea.KeyPressMsg{Code: tea.KeyEnter}
	updated3, _ := m.Update(enter2)
	m = updated3.(model)
	if m.result.Clone == nil {
		t.Fatal("result.Clone should be set after confirming name")
	}
	wantDest := filepath.Join(parentDir, "myrepo2")
	if m.result.Clone.Dest != wantDest {
		t.Errorf("Dest = %q, want %q", m.result.Clone.Dest, wantDest)
	}
}

func TestUpdateCloneName_esc(t *testing.T) {
	m := newModel(nil, map[string]float64{}, tmux.TmuxState{}, false)
	m.inputMode = modeCloneName
	m.tiCloneName.SetValue("myrepo")
	_ = m.tiCloneName.Focus()
	esc := tea.KeyPressMsg{Code: tea.KeyEscape}
	updated, _ := m.Update(esc)
	m = updated.(model)
	if m.inputMode != modeDestPicker {
		t.Errorf("esc should return to modeDestPicker, got %v", m.inputMode)
	}
}
