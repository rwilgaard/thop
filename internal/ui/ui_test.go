package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	cand "github.com/rwilgaard/thop/internal/candidates"
	"github.com/rwilgaard/thop/internal/config"
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

func TestRebuildFiltered(t *testing.T) {
	makeModel := func(items []baseItem, frecency map[string]float64) model {
		m := newModel(nil, frecency, tmux.State{}, false, config.Config{}, false)
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

	t.Run("viewTmp excludes non-tmp", func(t *testing.T) {
		items := []baseItem{
			{candidate: cand.Candidate{RelPath: "proj"}},
			{candidate: cand.Candidate{RelPath: "scratch", IsTmp: true}},
		}
		m := makeModel(items, map[string]float64{})
		m.view = viewTmp
		m.rebuildFiltered()

		if len(m.filtered) != 1 || !m.filtered[0].base.candidate.IsTmp {
			t.Errorf("viewTmp should show only tmp items, got %v", m.filtered)
		}
	})

	t.Run("viewProject excludes tmp", func(t *testing.T) {
		items := []baseItem{
			{candidate: cand.Candidate{RelPath: "proj"}},
			{candidate: cand.Candidate{RelPath: "scratch", IsTmp: true}},
			{candidate: cand.Candidate{RelPath: "repo", IsRepo: true}},
		}
		m := makeModel(items, map[string]float64{})
		m.view = viewProject
		m.rebuildFiltered()

		if len(m.filtered) != 1 || m.filtered[0].base.candidate.RelPath != "proj" {
			t.Errorf("viewProject should show only non-repo non-tmp, got %v", m.filtered)
		}
	})

	t.Run("switchOnly excludes inactive items", func(t *testing.T) {
		items := []baseItem{
			{candidate: cand.Candidate{RelPath: "active-session", IsRepo: false}, active: true},
			{candidate: cand.Candidate{RelPath: "inactive-session", IsRepo: false}, active: false},
		}
		m := makeModel(items, map[string]float64{})
		m.view = viewOpen
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
		m.view = viewOpen
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
	ts := tmux.State{
		Sessions: map[string]bool{"golang": true},
		Windows:  map[string]bool{"golang/foo": true},
	}
	m := newModel(cs, scores, ts, false, config.Config{}, false)
	m.width = 80
	m.height = 24
	m.ready = true

	out := m.View().Content

	checks := []struct {
		want string
		desc string
	}{
		{"❯", "prompt glyph"},
		{"Help", "help hint"},
		{"Open", "open hint"},
		// full placeholder can't match: textinput renders the first rune as a
		// separate cursor-styled ANSI run
		{"earch projects…", "search placeholder"},
		{"golang/foo", "first item"},
		{"work", "second item"},
		{"", "repo icon"},
		{"󰉋", "project icon"},
		{"● open", "active indicator"},
		{"Filter", "status bar filter heading"},
		{"All", "status bar active view"},
		{"1/2", "cursor position"},
	}
	for _, c := range checks {
		if !strings.Contains(out, c.want) {
			t.Errorf("View() missing %s: %q not found", c.desc, c.want)
		}
	}
	if strings.Contains(out, "Clone repository") {
		t.Error("help content should be hidden by default")
	}

	m.inputMode = modeHelp
	outHelp := m.View().Content
	if !strings.Contains(outHelp, "Clone repository") {
		t.Error("help overlay should show clone binding")
	}
}

func TestUpdateURLInput(t *testing.T) {
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	m.inputMode = modeURLInput
	_ = m.clone.tiURL.Focus()

	// Type characters
	for _, ch := range []string{"h", "t", "t", "p"} {
		msg := tea.KeyPressMsg{Text: ch, Code: rune(ch[0])}
		updated, _ := m.Update(msg)
		m = updated.(model)
	}
	if m.clone.tiURL.Value() != "http" {
		t.Errorf("clone.tiURL = %q, want %q", m.clone.tiURL.Value(), "http")
	}

	// Backspace
	bsp := tea.KeyPressMsg{Code: tea.KeyBackspace}
	updated, _ := m.Update(bsp)
	m = updated.(model)
	if m.clone.tiURL.Value() != "htt" {
		t.Errorf("after backspace tiURL = %q, want %q", m.clone.tiURL.Value(), "htt")
	}

	// Esc cancels
	esc := tea.KeyPressMsg{Code: tea.KeyEscape}
	updated, _ = m.Update(esc)
	m = updated.(model)
	if m.inputMode != modeNormal {
		t.Errorf("esc should return to modeNormal, got %v", m.inputMode)
	}
	if m.clone.tiURL.Value() != "" {
		t.Errorf("esc should clear tiURL, got %q", m.clone.tiURL.Value())
	}
	if !m.tiQuery.Focused() {
		t.Error("esc should focus tiQuery")
	}

	// Enter with non-empty URL advances to modeDestPicker
	m2 := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	m2.inputMode = modeURLInput
	_ = m2.clone.tiURL.Focus()
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
	m := newModel(cs, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	m.inputMode = modeDestPicker
	m.clone.tiURL.SetValue("https://github.com/user/myrepo")
	_ = m.clone.tiDest.Focus()
	m.rebuildDestFiltered()

	// Select the candidate and press enter — should detect conflict.
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	updated, _ := m.Update(enter)
	m = updated.(model)
	if m.inputMode != modeCloneName {
		t.Errorf("conflict should advance to modeCloneName, got %v", m.inputMode)
	}
	if m.clone.tiName.Value() != "myrepo" {
		t.Errorf("clone.tiName = %q, want %q", m.clone.tiName.Value(), "myrepo")
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
	if m.inputMode != modeCloning {
		t.Errorf("should be modeCloning after confirming clone name, got %v", m.inputMode)
	}
}

func TestUpdateCloneName_esc(t *testing.T) {
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	m.inputMode = modeCloneName
	m.clone.tiName.SetValue("myrepo")
	_ = m.clone.tiName.Focus()
	esc := tea.KeyPressMsg{Code: tea.KeyEscape}
	updated, _ := m.Update(esc)
	m = updated.(model)
	if m.inputMode != modeDestPicker {
		t.Errorf("esc should return to modeDestPicker, got %v", m.inputMode)
	}
}

func TestUpdateNameInput(t *testing.T) {
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	m.inputMode = modeNameInput
	_ = m.tmp.tiName.Focus()

	for _, ch := range []string{"f", "o", "o"} {
		msg := tea.KeyPressMsg{Text: ch, Code: rune(ch[0])}
		updated, _ := m.Update(msg)
		m = updated.(model)
	}
	if m.tmp.tiName.Value() != "foo" {
		t.Errorf("tmp.tiName = %q, want %q", m.tmp.tiName.Value(), "foo")
	}

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	updated, _ := m.Update(enter)
	m = updated.(model)
	if m.result.Tmp == nil || m.result.Tmp.Name != "foo" {
		t.Errorf("enter should set Tmp.Name=%q, got %v", "foo", m.result.Tmp)
	}
	if m.inputMode != modeLoading {
		t.Errorf("enter should switch to modeLoading, got %v", m.inputMode)
	}
}

func TestUpdateNameInput_conflict(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "existing"), 0o755); err != nil {
		t.Fatal(err)
	}

	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{TmpPath: tmpDir}, false)
	m.inputMode = modeNameInput
	_ = m.tmp.tiName.Focus()
	m.tmp.tiName.SetValue("existing")

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}

	// first enter: should flag conflict, not quit
	updated, _ := m.Update(enter)
	m = updated.(model)
	if !m.tmp.conflict {
		t.Error("first enter on existing name should set nameConflict")
	}
	if m.result.Tmp != nil {
		t.Error("should not quit on first conflict")
	}

	// second enter: should proceed despite conflict (switches to modeLoading)
	updated, _ = m.Update(enter)
	m = updated.(model)
	if m.result.Tmp == nil || m.result.Tmp.Name != "existing" {
		t.Errorf("second enter should set Tmp.Name=%q, got %v", "existing", m.result.Tmp)
	}
	if m.inputMode != modeLoading {
		t.Errorf("second enter should switch to modeLoading, got %v", m.inputMode)
	}

	// typing resets conflict flag
	m2 := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{TmpPath: tmpDir}, false)
	m2.inputMode = modeNameInput
	m2.tmp.conflict = true
	_ = m2.tmp.tiName.Focus()
	m2.tmp.tiName.SetValue("exist")
	ch := tea.KeyPressMsg{Text: "x", Code: 'x'}
	updated2, _ := m2.Update(ch)
	m2 = updated2.(model)
	if m2.tmp.conflict {
		t.Error("typing should reset nameConflict")
	}
}

func TestUpdateConfirmClean(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "scratch"), 0o755); err != nil {
		t.Fatal(err)
	}
	scratch := cand.Candidate{RelPath: "scratch", IsTmp: true, AbsPath: filepath.Join(tmpDir, "scratch")}
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{TmpPath: tmpDir}, false)
	m.inputMode = modeCleanTmp
	m.all = []baseItem{{candidate: scratch}}
	m.rebuildCleanFiltered()
	m.rebuildFiltered()

	// enter in clean mode → confirm prompt
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	updated, _ := m.Update(enter)
	m = updated.(model)
	if m.inputMode != modeConfirmClean {
		t.Errorf("enter should go to modeConfirmClean, got %v", m.inputMode)
	}

	// y confirms delete
	y := tea.KeyPressMsg{Text: "y", Code: 'y'}
	updated, _ = m.Update(y)
	m = updated.(model)

	if m.inputMode != modeNormal {
		t.Errorf("after y should be modeNormal, got %v", m.inputMode)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "scratch")); !os.IsNotExist(err) {
		t.Errorf("scratch dir should be deleted after confirm clean")
	}
	for _, item := range m.all {
		if item.candidate.IsTmp {
			t.Errorf("tmp candidate still in m.all after clean")
		}
	}
}

func TestSelectToggle(t *testing.T) {
	tmpDir := t.TempDir()
	scratch := cand.Candidate{RelPath: "scratch", IsTmp: true, AbsPath: filepath.Join(tmpDir, "scratch")}

	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{TmpPath: tmpDir}, false)
	m.all = []baseItem{{candidate: scratch}}
	m.rebuildCleanFiltered()
	m.clean.cursor = 0
	m.inputMode = modeCleanTmp

	space := tea.KeyPressMsg{Text: " ", Code: ' '}

	updated, _ := m.Update(space)
	m = updated.(model)
	if !m.clean.selected[scratch.AbsPath] {
		t.Error("space should select item in clean mode")
	}

	updated, _ = m.Update(space)
	m = updated.(model)
	if m.clean.selected[scratch.AbsPath] {
		t.Error("second space should deselect")
	}
}

func TestConfirmClean_selective(t *testing.T) {
	tmpDir := t.TempDir()
	for _, name := range []string{"keep", "delete"} {
		if err := os.MkdirAll(filepath.Join(tmpDir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	keep := cand.Candidate{RelPath: "keep", IsTmp: true, AbsPath: filepath.Join(tmpDir, "keep")}
	del := cand.Candidate{RelPath: "delete", IsTmp: true, AbsPath: filepath.Join(tmpDir, "delete")}

	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{TmpPath: tmpDir}, false)
	m.all = []baseItem{{candidate: keep}, {candidate: del}}
	m.rebuildCleanFiltered()
	m.clean.selected = map[string]bool{del.AbsPath: true}
	m.inputMode = modeConfirmClean
	m.rebuildFiltered()

	y := tea.KeyPressMsg{Text: "y", Code: 'y'}
	updated, _ := m.Update(y)
	m = updated.(model)

	if _, err := os.Stat(del.AbsPath); !os.IsNotExist(err) {
		t.Error("selected dir should be deleted")
	}
	if _, err := os.Stat(keep.AbsPath); err != nil {
		t.Error("unselected dir should be kept")
	}
	if len(m.clean.selected) != 0 {
		t.Error("selected should be cleared after delete")
	}
	found := false
	for _, item := range m.all {
		if item.candidate.AbsPath == keep.AbsPath {
			found = true
		}
	}
	if !found {
		t.Error("kept candidate missing from m.all")
	}
}

func TestHelpOverlay(t *testing.T) {
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	m.width, m.height, m.ready = 100, 24, true

	if m.inputMode == modeHelp {
		t.Fatal("help should start hidden")
	}

	q := tea.KeyPressMsg{Text: "?", Code: '?'}
	updated, _ := m.Update(q)
	m = updated.(model)
	if m.inputMode != modeHelp {
		t.Error("? should show help")
	}

	out := ansi.Strip(m.View().Content)
	for _, w := range []string{"Navigate", "Actions", "Filters", "Clone repository", "Move up", "Next filter", "Close session", "Page down"} {
		if !strings.Contains(out, w) {
			t.Errorf("help overlay missing %q", w)
		}
	}

	updated, _ = m.Update(q)
	m = updated.(model)
	if m.inputMode != modeNormal {
		t.Error("second ? should hide help")
	}

	m.inputMode = modeHelp
	esc := tea.KeyPressMsg{Code: tea.KeyEscape}
	updated, _ = m.Update(esc)
	m = updated.(model)
	if m.inputMode != modeNormal {
		t.Error("esc should hide help")
	}
}

func TestRenderName_highlights(t *testing.T) {
	base := lipgloss.NewStyle()
	match := lipgloss.NewStyle().Bold(true)
	out := renderName("abc", []int{0, 2}, base, match)
	// matched runes wrapped in bold, unmatched not. lipgloss v2 resets with
	// "\x1b[m" rather than "\x1b[0m", so match on the bold-open + reset pair.
	if !strings.Contains(out, "\x1b[1ma\x1b[m") || !strings.Contains(out, "\x1b[1mc\x1b[m") {
		t.Errorf("matched runes not bold: %q", out)
	}
	if strings.Contains(out, "\x1b[1mb") {
		t.Errorf("unmatched rune styled: %q", out)
	}
}

func TestRebuildFiltered_storesMatches(t *testing.T) {
	items := []baseItem{{candidate: cand.Candidate{AbsPath: "/p/abc", RelPath: "abc"}}}
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	m.all = items
	m.tiQuery.SetValue("ac")
	m.rebuildFiltered()
	if len(m.filtered) != 1 {
		t.Fatalf("got %d items, want 1", len(m.filtered))
	}
	if len(m.filtered[0].matches) == 0 {
		t.Error("matches not stored on scoredItem")
	}
}

func TestView_emptyState(t *testing.T) {
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	m.width, m.height, m.ready = 80, 24, true

	out := m.View().Content
	if !strings.Contains(out, "Nothing here") {
		t.Errorf("empty pool should say 'Nothing here': %q", out)
	}

	m.all = []baseItem{{candidate: cand.Candidate{AbsPath: "/p/foo", RelPath: "foo"}}}
	m.tiQuery.SetValue("zzz")
	m.rebuildFiltered()
	out = m.View().Content
	if !strings.Contains(out, "No matches") {
		t.Errorf("query without hits should say 'No matches': %q", out)
	}
}

func TestErrorRecovery(t *testing.T) {
	tests := []struct {
		name     string
		msg      tea.Msg
		wantMode inputMode
	}{
		{"clone fail returns to URL input", cloneDoneMsg{err: os.ErrPermission}, modeURLInput},
		{"open fail returns to picker", selectionDoneMsg{err: os.ErrPermission}, modeNormal},
		{"tmp create fail returns to name input", tmpCreatedMsg{err: os.ErrPermission}, modeNameInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
			m.clone.tiURL.SetValue("https://x/y.git")
			m.result.Clone = &CloneRequest{}
			m.result.Tmp = &TmpRequest{}
			m.inputMode = modeLoading

			updated, _ := m.Update(tt.msg)
			m = updated.(model)
			if m.inputMode != modeError {
				t.Fatalf("expected modeError, got %v", m.inputMode)
			}

			// any key dismisses back to origin
			updated, _ = m.Update(tea.KeyPressMsg{Text: "x", Code: 'x'})
			m = updated.(model)
			if m.inputMode != tt.wantMode {
				t.Errorf("expected recovery to %v, got %v", tt.wantMode, m.inputMode)
			}
			if m.errMsg != "" {
				t.Error("errMsg should be cleared on dismiss")
			}
			if tt.wantMode == modeURLInput && m.clone.tiURL.Value() != "https://x/y.git" {
				t.Error("URL should be preserved for retry")
			}
		})
	}

	// ctrl+c still quits
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	m.inputMode = modeError
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Error("ctrl+c in error mode should quit")
	}
}

func TestLoadingSpinner(t *testing.T) {
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{TmpPath: t.TempDir()}, false)
	m.width, m.height, m.ready = 80, 24, true
	m.inputMode = modeNameInput
	m.tmp.tiName.SetValue("foo")
	m.inTmux = true

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
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

func TestTextInputPrompts(t *testing.T) {
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)

	// Verify all textinputs have cleared default prompt (no stray "> " after "❯")
	promptChecks := []struct {
		got  string
		name string
	}{
		{m.tiQuery.Prompt, "tiQuery.Prompt"},
		{m.clone.tiURL.Prompt, "clone.tiURL.Prompt"},
		{m.clone.tiDest.Prompt, "clone.tiDest.Prompt"},
		{m.clone.tiName.Prompt, "clone.tiName.Prompt"},
		{m.clean.tiQuery.Prompt, "clean.tiQuery.Prompt"},
		{m.tmp.tiName.Prompt, "tmp.tiName.Prompt"},
	}
	for _, c := range promptChecks {
		if c.got != "" {
			t.Errorf("%s = %q, want empty string", c.name, c.got)
		}
	}

	// Render normal mode and verify no stray "❯ >" appears
	m.width, m.height, m.ready = 80, 24, true
	out := m.View().Content
	if strings.Contains(out, "❯ >") {
		t.Error("normal mode view should not contain stray '❯ >' (textinput default prompt leak)")
	}
}

func TestPrompts_modes(t *testing.T) {
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	m.width, m.height, m.ready = 100, 24, true

	tests := []struct {
		mode inputMode
		want []string
	}{
		{modeURLInput, []string{"Clone repository", "Next", "Cancel"}},
		{modeDestPicker, []string{"Clone › Destination ❯", "Select", "Back"}},
		{modeCloneName, []string{"Name conflict", "Clone as", "Back"}},
		{modeNameInput, []string{"New tmp project", "Create", "Cancel"}},
		{modeCleanTmp, []string{"Delete tmp projects ❯", "Select", "Delete", "Cancel"}},
	}
	for _, tt := range tests {
		m.inputMode = tt.mode
		out := m.View().Content
		for _, w := range tt.want {
			if !strings.Contains(out, w) {
				t.Errorf("mode %v: missing %q", tt.mode, w)
			}
		}
	}
}

func TestConfirmClean_pluralization(t *testing.T) {
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	m.width, m.height, m.ready = 100, 24, true
	m.inputMode = modeConfirmClean
	m.all = []baseItem{
		{candidate: cand.Candidate{AbsPath: "/t/a", RelPath: "a", IsTmp: true}},
		{candidate: cand.Candidate{AbsPath: "/t/b", RelPath: "b", IsTmp: true}, active: true},
	}

	m.clean.selected = map[string]bool{"/t/a": true}
	if out := m.View().Content; !strings.Contains(out, "Delete 1 tmp project?") {
		t.Errorf("singular form missing: %q", out)
	}
	m.clean.selected = map[string]bool{"/t/a": true, "/t/b": true}
	if out := m.View().Content; !strings.Contains(out, "Delete 2 tmp projects (1 open)?") {
		t.Errorf("plural form with open count missing: %q", out)
	}
}

func TestStatusBar_modes(t *testing.T) {
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	m.width, m.height, m.ready = 100, 24, true

	// normal, wide: spelled+bracketed keys with dot separators, matching the
	// other hint rows
	out := m.View().Content
	for _, w := range []string{"Filter", "All", "Projects", "Repos", "Tmp", "Open", "•", "0 items"} {
		if !strings.Contains(out, w) {
			t.Errorf("wide normal status missing %q: %q", w, out)
		}
	}
	if status := strings.Split(out, "\n")[m.maxRows()+3]; strings.Contains(status, "<") {
		t.Errorf("unbound filter keys should not render: %q", status)
	}

	// bound direct-jump keys show up; narrow falls back to compact carets, no
	// dots, so it fits
	m.keys = buildKeyMap(config.Config{Keymap: map[string][]string{"all": {"ctrl+a"}}})
	if wide := m.View().Content; !strings.Contains(wide, "<ctrl-a>") {
		t.Errorf("bound filter key should render: %q", wide)
	}
	m.width = 60
	narrow := m.View().Content
	if !strings.Contains(narrow, "^A") || strings.Contains(narrow, "<ctrl-a>") || strings.Contains(narrow, "•") {
		t.Errorf("narrow normal status should use compact caret fallback: %q", narrow)
	}
	for _, line := range strings.Split(narrow, "\n") {
		if lipgloss.Width(line) > 60 {
			t.Errorf("narrow status line exceeds width 60 (got %d): %q", lipgloss.Width(line), line)
		}
	}
	m.width = 100

	// the active filter must render distinctly from inactive ones: switching
	// the active view changes the bar. Guards against every label rendering
	// the same (dropping the m.view==t.mode branch).
	m.view = viewAll
	allActive := m.View().Content
	m.view = viewProject
	projActive := m.View().Content
	if allActive == projActive {
		t.Error("active filter not styled distinctly: switching active view produced identical output")
	}
	m.view = viewAll

	// dest picker: clone URL + own count, no tabs
	m.inputMode = modeDestPicker
	m.clone.tiURL.SetValue("https://x/y.git")
	m.rebuildDestFiltered()
	out = m.View().Content
	if !strings.Contains(out, "https://x/y.git") {
		t.Errorf("dest picker should show clone URL: %q", out)
	}
	if strings.Contains(out, "Projects") {
		t.Errorf("dest picker should not show view tabs: %q", out)
	}

	// clean tmp: N selected + own count
	m.inputMode = modeCleanTmp
	m.all = []baseItem{{candidate: cand.Candidate{AbsPath: "/t/a", RelPath: "a", IsTmp: true}}}
	m.rebuildCleanFiltered()
	m.clean.selected = map[string]bool{"/t/a": true}
	out = m.View().Content
	if !strings.Contains(out, "1 selected") || !strings.Contains(out, "1/1") {
		t.Errorf("clean mode should show selection and count: %q", out)
	}
}

func TestNewModel_layout(t *testing.T) {
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{Layout: "bottom"}, false)
	if !m.layoutBottom {
		t.Error("layout: bottom should set layoutBottom")
	}
	m = newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	if m.layoutBottom {
		t.Error("empty layout should default to top")
	}
	m = newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{Layout: "sideways"}, false)
	if m.layoutBottom {
		t.Error("unknown layout should default to top")
	}
}

func TestLayoutBottom_frame(t *testing.T) {
	cs := []cand.Candidate{{AbsPath: "/p/foo", RelPath: "foo"}}
	m := newModel(cs, map[string]float64{}, tmux.State{}, false, config.Config{Layout: "bottom"}, false)
	m.width, m.height, m.ready = 80, 24, true

	lines := strings.Split(m.View().Content, "\n")
	if !strings.Contains(lines[0], "Filter") {
		t.Errorf("bottom layout: first line should be status bar, got %q", lines[0])
	}
	if !strings.Contains(lines[len(lines)-1], "❯") {
		t.Errorf("bottom layout: last line should be search bar, got %q", lines[len(lines)-1])
	}
}

func TestLayoutBottom_reversedList(t *testing.T) {
	cs := []cand.Candidate{
		{AbsPath: "/p/best", RelPath: "best"},
		{AbsPath: "/p/worse", RelPath: "worse"},
	}
	scores := map[string]float64{"/p/best": 5.0, "/p/worse": 1.0}
	m := newModel(cs, scores, tmux.State{}, false, config.Config{Layout: "bottom"}, false)
	m.width, m.height, m.ready = 80, 24, true

	lines := strings.Split(m.View().Content, "\n")
	bestIdx, worseIdx := -1, -1
	for i, l := range lines {
		if strings.Contains(l, "best") {
			bestIdx = i
		}
		if strings.Contains(l, "worse") {
			worseIdx = i
		}
	}
	if bestIdx == -1 || worseIdx == -1 {
		t.Fatalf("items missing: best=%d worse=%d", bestIdx, worseIdx)
	}
	if bestIdx < worseIdx {
		t.Errorf("bottom layout: best match should render below worse match (best=%d worse=%d)", bestIdx, worseIdx)
	}
}

func TestLayoutBottom_emptyStateAnchored(t *testing.T) {
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{Layout: "bottom"}, false)
	m.width, m.height, m.ready = 80, 24, true

	lines := strings.Split(m.View().Content, "\n")
	idx := -1
	for i, l := range lines {
		if strings.Contains(l, "Nothing here") {
			idx = i
		}
	}
	if idx == -1 {
		t.Fatal("empty-state message missing")
	}
	// frame is 24 lines; message should hug the bottom separator (line len-3)
	if idx != len(lines)-3 {
		t.Errorf("bottom layout: empty state at line %d, want %d (adjacent to search bar)", idx, len(lines)-3)
	}
}

func TestLayoutBottom_visualCursor(t *testing.T) {
	cs := []cand.Candidate{
		{AbsPath: "/p/best", RelPath: "best"},
		{AbsPath: "/p/worse", RelPath: "worse"},
	}
	scores := map[string]float64{"/p/best": 5.0, "/p/worse": 1.0}
	m := newModel(cs, scores, tmux.State{}, false, config.Config{Layout: "bottom"}, false)
	m.width, m.height, m.ready = 80, 24, true

	up := tea.KeyPressMsg{Code: tea.KeyUp}
	down := tea.KeyPressMsg{Code: tea.KeyDown}

	// bottom layout: best match (index 0) is visually at the bottom.
	// Up must move visually up = index 1.
	updated, _ := m.Update(up)
	m = updated.(model)
	if m.cursor != 1 {
		t.Errorf("bottom layout: up from index 0 should reach index 1, got %d", m.cursor)
	}
	updated, _ = m.Update(down)
	m = updated.(model)
	if m.cursor != 0 {
		t.Errorf("bottom layout: down should return to index 0, got %d", m.cursor)
	}
	// down at index 0 (visual bottom) wraps to the far end
	updated, _ = m.Update(down)
	m = updated.(model)
	if m.cursor != len(m.filtered)-1 {
		t.Errorf("bottom layout: down at bottom should wrap to %d, got %d", len(m.filtered)-1, m.cursor)
	}
}

func TestTopLayout_cursor(t *testing.T) {
	cs := []cand.Candidate{
		{AbsPath: "/p/best", RelPath: "best"},
		{AbsPath: "/p/worse", RelPath: "worse"},
	}
	scores := map[string]float64{"/p/best": 5.0, "/p/worse": 1.0}
	m := newModel(cs, scores, tmux.State{}, false, config.Config{}, false)
	m.width, m.height, m.ready = 80, 24, true

	up := tea.KeyPressMsg{Code: tea.KeyUp}
	down := tea.KeyPressMsg{Code: tea.KeyDown}

	steps := []struct {
		key  tea.KeyPressMsg
		want int
		desc string
	}{
		{up, 1, "up at the top wraps to the last row"},
		{down, 0, "down at the last row wraps to the top"},
		{down, 1, "down moves to index 1"},
		{up, 0, "up returns to index 0"},
	}
	for _, st := range steps {
		updated, _ := m.Update(st.key)
		m = updated.(model)
		if m.cursor != st.want {
			t.Errorf("%s: cursor = %d, want %d", st.desc, m.cursor, st.want)
		}
	}
}

func TestEnter_emptyListStays(t *testing.T) {
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	tests := []struct {
		name string
		mode inputMode
	}{
		{"picker", modeNormal},
		{"dest picker", modeDestPicker},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
			m.inputMode = tt.mode
			updated, cmd := m.Update(enter)
			if cmd != nil {
				t.Error("enter on an empty list should do nothing")
			}
			if got := updated.(model).inputMode; got != tt.mode {
				t.Errorf("mode changed to %v", got)
			}
		})
	}
}

func TestTruncateName(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		matches     []int
		maxW        int
		want        string
		wantMatches []int
	}{
		{"fits", "abc", []int{0, 2}, 3, "abc", []int{0, 2}},
		{"cut", "abcdef", []int{0, 4}, 4, "abc…", []int{0}},
		{"multibyte", "æøåabc", []int{0, 2}, 3, "æø…", []int{0, 2}},
		{"no room", "abc", nil, 0, "…", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotMatches := truncateName(tt.in, tt.matches, tt.maxW)
			if got != tt.want {
				t.Errorf("name = %q, want %q", got, tt.want)
			}
			if len(gotMatches) != len(tt.wantMatches) {
				t.Fatalf("matches = %v, want %v", gotMatches, tt.wantMatches)
			}
			for i := range gotMatches {
				if gotMatches[i] != tt.wantMatches[i] {
					t.Errorf("matches = %v, want %v", gotMatches, tt.wantMatches)
				}
			}
		})
	}
}

func TestView_longRowsFitWidth(t *testing.T) {
	long := strings.Repeat("very-long-name-", 10)
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	m.all = []baseItem{
		{candidate: cand.Candidate{AbsPath: "/p/a", RelPath: long}, active: true, current: true},
		{candidate: cand.Candidate{AbsPath: "/p/b", RelPath: long + "b"}},
	}
	m.rebuildFiltered()
	m.width, m.height, m.ready = 40, 12, true

	out := m.View().Content
	for i, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > m.width {
			t.Errorf("line %d is %d cells wide, frame is %d", i, w, m.width)
		}
	}
	if !strings.Contains(out, "current") {
		t.Error("truncated row lost its label")
	}
}

func TestRebuildFiltered_sessionOrder(t *testing.T) {
	ts := tmux.State{
		Sessions:      map[string]bool{"here": true, "before": true, "group": true},
		Windows:       map[string]bool{"group/repo": true},
		Current:       "here",
		CurrentWindow: "zsh",
		Last:          "before",
	}
	cs := []cand.Candidate{
		{AbsPath: "/p/here", RelPath: "here"},
		{AbsPath: "/p/top", RelPath: "top"},
		{AbsPath: "/p/before", RelPath: "before"},
		{AbsPath: "/p/group/repo", RelPath: "group/repo", IsRepo: true},
	}
	scores := map[string]float64{"/p/here": 9, "/p/top": 5, "/p/group/repo": 2, "/p/before": 1}
	m := newModel(cs, scores, ts, false, config.Config{}, false)

	order := func() []string {
		var out []string
		for _, it := range m.filtered {
			out = append(out, it.base.candidate.RelPath)
		}
		return out
	}

	want := []string{"before", "top", "group/repo", "here"}
	if got := order(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("empty query: got %v, want %v", got, want)
	}

	m.width, m.height, m.ready = 80, 12, true
	if out := m.View().Content; !strings.Contains(out, "current") {
		t.Error("current row should be labelled")
	}

	m.tiQuery.SetValue("e")
	m.rebuildFiltered()
	if got := order(); got[0] != "here" {
		t.Errorf("query: got %v, want here first", got)
	}
}

func TestCleanTmp_enterTargetsCursorRow(t *testing.T) {
	tmpDir := t.TempDir()
	var all []baseItem
	for _, name := range []string{"one", "two", "three"} {
		p := filepath.Join(tmpDir, name)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		all = append(all, baseItem{candidate: cand.Candidate{RelPath: name, IsTmp: true, AbsPath: p}, active: name == "two"})
	}
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{TmpPath: tmpDir}, false)
	var killed []string
	m.killSession = func(_ tmux.State, s string) error {
		killed = append(killed, s)
		return nil
	}
	m.loadState = func() tmux.State { return tmux.State{} }
	m.all = all
	m.rebuildCleanFiltered()
	m.clean.cursor = 1
	m.inputMode = modeCleanTmp

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updated, _ = updated.(model).Update(tea.KeyPressMsg{Text: "y", Code: 'y'})
	m = updated.(model)

	for name, wantGone := range map[string]bool{"one": false, "two": true, "three": false} {
		_, err := os.Stat(filepath.Join(tmpDir, name))
		if gone := os.IsNotExist(err); gone != wantGone {
			t.Errorf("%s: gone = %v, want %v", name, gone, wantGone)
		}
	}
	if len(killed) != 1 || killed[0] != "two" {
		t.Errorf("killed sessions = %v, want [two]", killed)
	}
}

func TestConfirmClean_overflow(t *testing.T) {
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	m.clean.selected = map[string]bool{}
	for i := range 20 {
		p := fmt.Sprintf("/t/tmp-%02d", i)
		m.all = append(m.all, baseItem{candidate: cand.Candidate{AbsPath: p, RelPath: filepath.Base(p), IsTmp: true}})
		m.clean.selected[p] = true
	}
	m.width, m.height, m.ready = 60, 10, true // 6 body rows: header + 4 rows + count
	m.inputMode = modeConfirmClean

	out := m.View().Content
	if !strings.Contains(out, "tmp-03") || strings.Contains(out, "tmp-04") {
		t.Errorf("expected rows 00-03 only: %q", out)
	}
	if !strings.Contains(out, "… and 16 more") {
		t.Errorf("missing overflow count: %q", out)
	}
}

func TestDialog_fitsFrame(t *testing.T) {
	longErr := strings.Repeat("fatal: could not read from remote repository ", 8)
	tests := []struct {
		name  string
		mode  inputMode
		wants []string
	}{
		{"url", modeURLInput, []string{"Clone repository", "Next", "Cancel"}},
		{"tmp name", modeNameInput, []string{"New tmp project", "Create", "Cancel"}},
		{"clone name", modeCloneName, []string{"Name conflict", "repo already exists", "Clone as", "Back"}},
		{"confirm", modeConfirmClean, []string{"Delete 1 tmp project?", "scratch", "Delete", "Cancel"}},
		{"error", modeError, []string{"Error", "fatal: could not", "Dismiss"}},
		{"cloning", modeCloning, []string{"Cloning", "→ proj/repo", "Cancel"}},
		{"close", modeConfirmClose, []string{"Close session", "scratch", "Cancel"}},
		{"help", modeHelp, []string{"Help", "Navigate", "Close"}},
		{"setup", modeSetup, []string{"Add a project root", "Save", "Skip"}},
		{"new project", modeNewProjName, []string{"New project", "Create", "Back"}},
	}
	sizes := []struct{ w, h int }{{30, 9}, {60, 16}, {70, 20}, {100, 24}}
	for _, tt := range tests {
		for _, sz := range sizes {
			t.Run(fmt.Sprintf("%s %dx%d", tt.name, sz.w, sz.h), func(t *testing.T) {
				m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
				m.all = []baseItem{{candidate: cand.Candidate{AbsPath: "/t/scratch", RelPath: "scratch", IsTmp: true}}}
				m.rebuildFiltered()
				m.rebuildCleanFiltered()
				m.clone.tiURL.SetValue("https://example.com/owner/repo.git")
				m.errMsg = longErr
				m.result.Clone = &CloneRequest{Dest: "/dest/proj/repo"}
				m.closeTarget = m.all[0]
				updated, _ := m.Update(tea.WindowSizeMsg{Width: sz.w, Height: sz.h})
				m = updated.(model)
				m.inputMode = tt.mode

				out := m.View().Content
				lines := strings.Split(out, "\n")
				if want := max(5, sz.h-4) + 4; len(lines) != want {
					t.Errorf("frame has %d lines, want %d", len(lines), want)
				}
				for i, line := range lines {
					if w := lipgloss.Width(line); w > sz.w {
						t.Errorf("line %d is %d cells wide, frame is %d", i, w, sz.w)
					}
				}
				plain := ansi.Strip(out)
				for _, w := range tt.wants {
					if !strings.Contains(plain, w) {
						t.Errorf("missing %q in:\n%s", w, plain)
					}
				}
			})
		}
	}
}

func TestDialog_backdrop(t *testing.T) {
	tests := []struct {
		name   string
		mode   inputMode
		errRet inputMode
		want   inputMode
	}{
		{"url over picker", modeURLInput, modeNormal, modeNormal},
		{"tmp name over picker", modeNameInput, modeNormal, modeNormal},
		{"clone name over dest picker", modeCloneName, modeNormal, modeDestPicker},
		{"confirm over clean list", modeConfirmClean, modeNormal, modeCleanTmp},
		{"cloning over dest picker", modeCloning, modeNormal, modeDestPicker},
		{"help over picker", modeHelp, modeNormal, modeNormal},
		{"setup over picker", modeSetup, modeNormal, modeNormal},
		{"new project name over picker", modeNewProjName, modeNormal, modeNormal},
		{"close over picker", modeConfirmClose, modeNormal, modeNormal},
		{"error over picker", modeError, modeNormal, modeNormal},
		{"error from url input over picker", modeError, modeURLInput, modeNormal},
		{"no dialog", modeDestPicker, modeNormal, modeDestPicker},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
			m.inputMode, m.errReturnMode = tt.mode, tt.errRet
			if got := m.backdropMode(); got != tt.want {
				t.Errorf("backdropMode = %v, want %v", got, tt.want)
			}
		})
	}

	m := newModel([]cand.Candidate{{AbsPath: "/p/visible", RelPath: "visible"}}, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	m.width, m.height, m.ready = 80, 24, true
	m.inputMode = modeURLInput
	if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, "visible") {
		t.Errorf("list should stay visible behind the dialog:\n%s", plain)
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
	ctrlU := tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
	ctrlD := tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}

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
				updated, _ := m.Update(k)
				m = updated.(model)
			}
			if m.cursor != tt.want {
				t.Errorf("cursor = %d, want %d", m.cursor, tt.want)
			}
		})
	}

	m := newModel(cs, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	m.width, m.height, m.ready = 80, 12, true
	updated, _ := m.Update(pgdn)
	if out := updated.(model).View().Content; !strings.Contains(out, "9/30") {
		t.Errorf("status should show position 9/30: %q", out)
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
		updated, _ := m.Update(st.key)
		m = updated.(model)
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
	ctrlQ := tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl}
	y := tea.KeyPressMsg{Text: "y", Code: 'y'}
	n := tea.KeyPressMsg{Text: "n", Code: 'n'}

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
			m.width, m.height, m.ready = 80, 24, true
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

			updated, _ := m.Update(ctrlQ)
			m = updated.(model)
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

			updated, _ = m.Update(tt.confirm)
			m = updated.(model)
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

func TestURLInput_expandsShorthand(t *testing.T) {
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{CloneShorthand: "https://github.com/{repo}.git"}, false)
	m.inputMode = modeURLInput
	m.clone.tiURL.SetValue("rwilgaard/thop")
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(model)
	if m.inputMode != modeDestPicker {
		t.Fatalf("mode = %v, want modeDestPicker", m.inputMode)
	}
	if got := m.clone.tiURL.Value(); got != "https://github.com/rwilgaard/thop.git" {
		t.Errorf("url = %q, want expanded shorthand", got)
	}
}

func TestClone_cancel(t *testing.T) {
	esc := tea.KeyPressMsg{Code: tea.KeyEscape}
	start := func(t *testing.T) model {
		t.Helper()
		m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
		m.width, m.height, m.ready = 80, 24, true
		m.clone.tiURL.SetValue("https://example.com/owner/repo.git")
		updated, cmd := m.startClone("/dest/proj/repo")
		if cmd == nil {
			t.Fatal("startClone should return a cmd")
		}
		return updated.(model)
	}

	t.Run("dialog while cloning", func(t *testing.T) {
		m := start(t)
		plain := ansi.Strip(m.View().Content)
		for _, w := range []string{"Cloning", "https://example.com/owner/repo.git", "→ proj/repo", "Cancel"} {
			if !strings.Contains(plain, w) {
				t.Errorf("missing %q in:\n%s", w, plain)
			}
		}
	})

	t.Run("esc cancels and returns to the url dialog", func(t *testing.T) {
		m := start(t)
		updated, _ := m.Update(esc)
		m = updated.(model)
		if !m.clone.cancelled || m.inputMode != modeCloning {
			t.Fatalf("esc should mark cancelled and wait for git, got cancelled=%v mode=%v", m.clone.cancelled, m.inputMode)
		}
		if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, "Cancelling…") {
			t.Errorf("missing cancelling state in:\n%s", plain)
		}

		updated, _ = m.Update(cloneDoneMsg{err: context.Canceled})
		m = updated.(model)
		if m.inputMode != modeURLInput {
			t.Errorf("mode = %v, want modeURLInput (no error dialog)", m.inputMode)
		}
		if m.clone.cancel != nil || m.clone.cancelled || m.result.Clone != nil {
			t.Error("clone state should be reset after cancel")
		}
		if got := m.clone.tiURL.Value(); got != "https://example.com/owner/repo.git" {
			t.Errorf("url = %q, should be kept for retry", got)
		}
	})

	t.Run("failure without cancel still shows the error", func(t *testing.T) {
		m := start(t)
		updated, _ := m.Update(cloneDoneMsg{err: os.ErrPermission})
		if got := updated.(model).inputMode; got != modeError {
			t.Errorf("mode = %v, want modeError", got)
		}
	})

	t.Run("esc does nothing for other loading states", func(t *testing.T) {
		m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
		m.inputMode, m.loadingText = modeLoading, "Opening…"
		updated, cmd := m.Update(esc)
		if cmd != nil || updated.(model).inputMode != modeLoading {
			t.Error("esc should be ignored while opening")
		}
	})
}

func TestInputRow_hintsFitWhole(t *testing.T) {
	cs := []cand.Candidate{
		{AbsPath: "/p/open", RelPath: "open"},
		{AbsPath: "/t/scratch", RelPath: "scratch", IsTmp: true},
	}
	ts := tmux.State{Sessions: map[string]bool{"open": true}}
	actions := []string{"Open", "Help", "Filter", "Close", "Clone", "Delete", "Select", "Cancel", "Back"}
	for _, mode := range []inputMode{modeNormal, modeCleanTmp, modeDestPicker} {
		for _, width := range []int{30, 40, 60, 100} {
			t.Run(fmt.Sprintf("mode %d width %d", mode, width), func(t *testing.T) {
				m := newModel(cs, map[string]float64{"/p/open": 1}, ts, false, config.Config{}, false)
				m.rebuildCleanFiltered()
				m.rebuildDestFiltered()
				m.inputMode = mode
				m.width, m.height, m.ready = width, 12, true
				line := ansi.Strip(strings.Split(m.View().Content, "\n")[0])
				if w := lipgloss.Width(line); w > width {
					t.Errorf("row is %d cells, frame is %d: %q", w, width, line)
				}
				// every "<key>" must be followed by its whole action word
				for _, part := range strings.Split(line, "<")[1:] {
					_, action, _ := strings.Cut(part, "> ")
					action = strings.TrimSpace(action)
					if !slices.Contains(actions, action) {
						t.Errorf("clipped hint %q in %q", action, line)
					}
				}
			})
		}
	}

	m := newModel(cs, map[string]float64{"/p/open": 1}, ts, false, config.Config{}, false)
	if line := ansi.Strip(m.searchLine(120)); !strings.Contains(line, "Close") {
		t.Errorf("open row should offer Close: %q", line)
	}
	m.cursor = 1
	if line := ansi.Strip(m.searchLine(120)); strings.Contains(line, "Close") {
		t.Errorf("row that isn't open should not offer Close: %q", line)
	}
}

func TestHelpDialog_columns(t *testing.T) {
	tests := []struct {
		name    string
		w, h    int
		sameRow []string // titles that must share a line
	}{
		{"three columns", 100, 24, []string{"Navigate", "Actions", "Filters"}},
		{"two columns", 70, 20, []string{"Navigate", "Actions"}},
		{"stacked", 40, 30, []string{"Navigate"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
			m.width, m.height, m.ready = tt.w, tt.h, true
			m.inputMode = modeHelp
			plain := ansi.Strip(m.View().Content)
			for _, w := range []string{"Actions", "Filters", "Next filter", "Close session"} {
				if !strings.Contains(plain, w) {
					t.Errorf("missing %q in:\n%s", w, plain)
				}
			}
			for _, line := range strings.Split(plain, "\n") {
				if !strings.Contains(line, "Navigate") {
					continue
				}
				n := 0
				for _, title := range []string{"Navigate", "Actions", "Filters"} {
					if strings.Contains(line, title) {
						n++
					}
				}
				if n != len(tt.sameRow) {
					t.Errorf("title row has %d groups, want %d: %q", n, len(tt.sameRow), line)
				}
			}
		})
	}
}

func TestSetup(t *testing.T) {
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	esc := tea.KeyPressMsg{Code: tea.KeyEscape}
	start := func(t *testing.T, add func(string) (string, []cand.Candidate, error)) model {
		t.Helper()
		tmp := []cand.Candidate{{AbsPath: "/t/scratch", RelPath: "scratch", IsTmp: true}}
		m := newModel(tmp, map[string]float64{}, tmux.State{}, false, config.Config{File: "/etc/thop/config.yaml"}, false)
		m.setup.add = add
		_ = m.openSetup()
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		m = updated.(model)
		m.setup.tiPath.SetValue("~/code")
		return m
	}
	found := func(string) (string, []cand.Candidate, error) {
		return "/r/projects", []cand.Candidate{{AbsPath: "/r/projects/app", Root: "/r/projects", RelPath: "app"}}, nil
	}

	t.Run("dialog starts empty", func(t *testing.T) {
		m := start(t, found)
		m.setup.tiPath.SetValue("")
		updated, cmd := m.Update(enter)
		if cmd != nil || updated.(model).inputMode != modeSetup {
			t.Error("enter on an empty path should do nothing")
		}
		plain := ansi.Strip(m.View().Content)
		for _, w := range []string{"Add a project root", "Save", "Skip"} {
			if !strings.Contains(plain, w) {
				t.Errorf("missing %q in:\n%s", w, plain)
			}
		}
	})

	t.Run("save fills the picker", func(t *testing.T) {
		var got string
		m := start(t, func(p string) (string, []cand.Candidate, error) {
			got = p
			return found(p)
		})
		updated, _ := m.Update(enter)
		m = updated.(model)
		if got != "~/code" {
			t.Errorf("add called with %q, want ~/code", got)
		}
		if m.inputMode != modeNormal {
			t.Fatalf("mode = %v, want modeNormal", m.inputMode)
		}
		var names []string
		for _, it := range m.filtered {
			names = append(names, it.base.candidate.RelPath)
		}
		if !slices.Equal(names, []string{"app", "scratch"}) {
			t.Errorf("picker rows = %v, want [app scratch]", names)
		}
		if !slices.Equal(m.paths, []string{"/r/projects"}) {
			t.Errorf("paths = %v, want the saved root", m.paths)
		}
	})

	t.Run("error stays in the dialog until edited", func(t *testing.T) {
		m := start(t, func(string) (string, []cand.Candidate, error) { return "", nil, errors.New("not a directory") })
		updated, _ := m.Update(enter)
		m = updated.(model)
		if m.inputMode != modeSetup {
			t.Fatalf("mode = %v, want modeSetup", m.inputMode)
		}
		if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, "not a directory") {
			t.Errorf("missing error in:\n%s", plain)
		}
		updated, _ = m.Update(tea.KeyPressMsg{Text: "x", Code: 'x'})
		if got := updated.(model).setup.err; got != "" {
			t.Errorf("typing should clear the error, got %q", got)
		}
	})

	t.Run("skip lands in a picker that names the config file", func(t *testing.T) {
		m := start(t, found)
		m.all = nil
		updated, _ := m.Update(esc)
		m = updated.(model)
		if m.inputMode != modeNormal {
			t.Fatalf("mode = %v, want modeNormal", m.inputMode)
		}
		if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, "No project roots. Add paths in /etc/thop/config.yaml") {
			t.Errorf("missing empty-state hint in:\n%s", plain)
		}
	})

	t.Run("with a clone url waiting, continues to the dest picker", func(t *testing.T) {
		m := start(t, found)
		m.clone.tiURL.SetValue("https://example.com/a/b.git")
		updated, _ := m.Update(enter)
		if got := updated.(model).inputMode; got != modeDestPicker {
			t.Errorf("mode = %v, want modeDestPicker", got)
		}
	})
}

func TestMissingRoots(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "ok"), 0o755); err != nil {
		t.Fatal(err)
	}
	ok, gone := filepath.Join(root, "ok"), filepath.Join(root, "wrok")
	var cs []cand.Candidate
	for i := range 30 {
		name := fmt.Sprintf("p%02d", i)
		cs = append(cs, cand.Candidate{AbsPath: filepath.Join(ok, name), Root: ok, RelPath: name})
	}

	t.Run("all roots present", func(t *testing.T) {
		m := newModel(cs, map[string]float64{}, tmux.State{}, false, config.Config{Paths: []string{ok}}, false)
		m.width, m.height, m.ready = 80, 12, true
		if out := m.View().Content; strings.Contains(out, "Not found") {
			t.Errorf("unexpected warning: %q", out)
		}
		if got := m.maxRows(); got != 8 {
			t.Errorf("maxRows = %d, want 8", got)
		}
	})

	for _, width := range []int{30, 80, 200} {
		t.Run(fmt.Sprintf("one missing, width %d", width), func(t *testing.T) {
			cfg := config.Config{Paths: []string{ok, gone}, File: "/etc/thop/config.yaml"}
			m := newModel(cs, map[string]float64{}, tmux.State{}, false, cfg, false)
			m.width, m.height, m.ready = width, 12, true
			if got := m.maxRows(); got != 7 {
				t.Errorf("maxRows = %d, want 7 (one row given to the warning)", got)
			}
			lines := strings.Split(m.View().Content, "\n")
			if len(lines) != 12 {
				t.Errorf("frame has %d lines, want 12", len(lines))
			}
			for i, line := range lines {
				if w := lipgloss.Width(line); w > width {
					t.Errorf("line %d is %d cells wide, frame is %d", i, w, width)
				}
			}
			warn := ansi.Strip(lines[9])
			if !strings.Contains(warn, "Not found") {
				t.Errorf("warning row missing above the bottom separator: %q", warn)
			}
			if width == 200 && !strings.Contains(warn, gone+" — check /etc/thop/config.yaml") {
				t.Errorf("warning should name the path and config file: %q", warn)
			}
		})
	}
}

func TestNewProject(t *testing.T) {
	ctrlN := tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl}
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	esc := tea.KeyPressMsg{Code: tea.KeyEscape}
	down := tea.KeyPressMsg{Code: tea.KeyDown}
	send := func(m model, msgs ...tea.KeyPressMsg) model {
		for _, msg := range msgs {
			updated, _ := m.Update(msg)
			m = updated.(model)
		}
		return m
	}
	setup := func(t *testing.T, roots int) (model, []string) {
		t.Helper()
		var paths []string
		var cs []cand.Candidate
		for i := range roots {
			p := filepath.Join(t.TempDir(), fmt.Sprintf("root%d", i))
			if err := os.MkdirAll(filepath.Join(p, "taken"), 0o755); err != nil {
				t.Fatal(err)
			}
			paths = append(paths, p)
			cs = append(cs, cand.Candidate{AbsPath: filepath.Join(p, "taken"), Root: p, RelPath: "taken"})
		}
		m := newModel(cs, map[string]float64{}, tmux.State{}, false, config.Config{Paths: paths}, false)
		m.width, m.height, m.ready = 80, 24, true
		return m, paths
	}

	t.Run("one root skips the picker", func(t *testing.T) {
		m, paths := setup(t, 1)
		m = send(m, ctrlN)
		if m.inputMode != modeNewProjName || m.newProj.root != paths[0] {
			t.Fatalf("mode = %v root = %q, want name dialog in %q", m.inputMode, m.newProj.root, paths[0])
		}
		if m = send(m, esc); m.inputMode != modeNormal {
			t.Errorf("esc: mode = %v, want modeNormal", m.inputMode)
		}
	})

	t.Run("several roots: pick, then esc steps back", func(t *testing.T) {
		m, paths := setup(t, 2)
		m = send(m, ctrlN)
		if m.inputMode != modeNewProjRoot || len(m.newProj.filtered) != 2 {
			t.Fatalf("mode = %v with %d roots, want root picker with 2", m.inputMode, len(m.newProj.filtered))
		}
		m = send(m, down, enter)
		if m.inputMode != modeNewProjName || m.newProj.root != paths[1] {
			t.Fatalf("mode = %v root = %q, want name dialog in %q", m.inputMode, m.newProj.root, paths[1])
		}
		if m = send(m, esc); m.inputMode != modeNewProjRoot {
			t.Errorf("first esc: mode = %v, want modeNewProjRoot", m.inputMode)
		}
		if m = send(m, esc); m.inputMode != modeNormal {
			t.Errorf("second esc: mode = %v, want modeNormal", m.inputMode)
		}
	})

	t.Run("name validation", func(t *testing.T) {
		tests := []struct{ name, wantErr string }{
			{"", ""},
			{"a/b", "Invalid name"},
			{"..", "Invalid name"},
			{".", "Invalid name"},
			{"   ", ""},
			{"taken", "Already exists"},
		}
		for _, tt := range tests {
			m, _ := setup(t, 1)
			m = send(m, ctrlN)
			m.newProj.tiName.SetValue(tt.name)
			m = send(m, enter)
			if m.inputMode != modeNewProjName || m.newProj.err != tt.wantErr {
				t.Errorf("%q: mode = %v err = %q, want dialog with %q", tt.name, m.inputMode, m.newProj.err, tt.wantErr)
			}
			if tt.wantErr != "" {
				if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, tt.wantErr) {
					t.Errorf("%q: missing %q in:\n%s", tt.name, tt.wantErr, plain)
				}
			}
		}
	})

	t.Run("create adds the row and selects it", func(t *testing.T) {
		m, paths := setup(t, 1)
		m.view = viewRepo
		m.tiQuery.SetValue("zzz")
		m.rebuildFiltered()
		m = send(m, ctrlN)
		m.newProj.tiName.SetValue("fresh")
		m = send(m, enter)

		dest := filepath.Join(paths[0], "fresh")
		if fi, err := os.Stat(dest); err != nil || !fi.IsDir() {
			t.Fatalf("%s not created: %v", dest, err)
		}
		if m.inputMode != modeNormal || m.view != viewAll || m.tiQuery.Value() != "" {
			t.Errorf("mode = %v view = %v query = %q, want normal/all/empty", m.inputMode, m.view, m.tiQuery.Value())
		}
		got := m.filtered[m.cursor].base.candidate
		if got.AbsPath != dest || got.Root != paths[0] || got.RelPath != "fresh" {
			t.Errorf("cursor on %+v, want the new project", got)
		}
	})

	t.Run("no usable root", func(t *testing.T) {
		m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{Paths: []string{"/nope/gone"}}, false)
		if m = send(m, ctrlN); m.inputMode != modeError || !strings.Contains(m.errMsg, "No root") {
			t.Errorf("mode = %v err = %q, want an error saying no root is usable", m.inputMode, m.errMsg)
		}
	})

	t.Run("no roots configured reopens setup", func(t *testing.T) {
		m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
		m.setup.add = func(string) (string, []cand.Candidate, error) { return "", nil, nil }
		_ = m.openSetup()
		m = send(m, esc, ctrlN)
		if m.inputMode != modeSetup {
			t.Errorf("mode = %v, want modeSetup", m.inputMode)
		}
	})
}

func TestCollidingRows(t *testing.T) {
	cs := cand.Resolve([]cand.Candidate{
		{AbsPath: "/code/alpha", Root: "/code", RelPath: "alpha"},
		{AbsPath: "/work/alpha", Root: "/work", RelPath: "alpha"},
		{AbsPath: "/code/beta", Root: "/code", RelPath: "beta"},
	})
	ts := tmux.State{Sessions: map[string]bool{"alpha@work": true}}
	m := newModel(cs, map[string]float64{}, ts, false, config.Config{}, false)
	m.width, m.height, m.ready = 60, 12, true

	rows := map[string]string{}
	for _, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		for _, root := range []string{"/code", "/work"} {
			if strings.Contains(line, "alpha") && strings.Contains(line, root) {
				rows[root] = line
			}
		}
		if strings.Contains(line, "beta") && strings.Contains(line, "/code") {
			t.Errorf("unique name should not show its root: %q", line)
		}
	}
	if len(rows) != 2 {
		t.Fatalf("both alpha rows should name their root, got %v", rows)
	}
	if strings.Contains(rows["/code"], "open") || !strings.Contains(rows["/work"], "open") {
		t.Errorf("only the /work alpha is open: %v", rows)
	}

	m.width = 20
	for i, line := range strings.Split(m.View().Content, "\n") {
		if w := lipgloss.Width(line); w > 20 {
			t.Errorf("line %d is %d cells wide, frame is 20", i, w)
		}
	}
}

func TestAddCandidates_reresolves(t *testing.T) {
	cs := cand.Resolve([]cand.Candidate{{AbsPath: "/code/alpha", Root: "/code", RelPath: "alpha"}})
	m := newModel(cs, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	if got := m.sessionOf("/code/alpha"); got != "alpha" {
		t.Fatalf("session = %q, want alpha", got)
	}
	m.addCandidates(cand.Candidate{AbsPath: "/work/alpha", Root: "/work", RelPath: "alpha"})
	if a, b := m.sessionOf("/code/alpha"), m.sessionOf("/work/alpha"); a != "alpha@code" || b != "alpha@work" {
		t.Errorf("sessions = %q, %q; want alpha@code, alpha@work", a, b)
	}
}

func TestDefaultKeys_nav(t *testing.T) {
	cs := []cand.Candidate{
		{AbsPath: "/p/a", RelPath: "a"},
		{AbsPath: "/p/b", RelPath: "b"},
		{AbsPath: "/p/c", RelPath: "c"},
	}
	m := newModel(cs, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	ctrl := func(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl} }

	// ctrl+p is unbound: it must neither move the cursor nor leave the picker
	steps := []struct {
		key  rune
		want int
	}{{'j', 1}, {'j', 2}, {'k', 1}, {'p', 1}}
	for i, st := range steps {
		updated, _ := m.Update(ctrl(st.key))
		m = updated.(model)
		if m.cursor != st.want || m.inputMode != modeNormal {
			t.Errorf("step %d (ctrl+%c): cursor = %d mode = %v, want cursor %d in modeNormal", i, st.key, m.cursor, m.inputMode, st.want)
		}
	}

	updated, _ := m.Update(ctrl('t'))
	if got := updated.(model).inputMode; got != modeNameInput {
		t.Errorf("ctrl+t: mode = %v, want modeNameInput", got)
	}
}

func TestClone_escAfterSuccess(t *testing.T) {
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	m.clone.tiURL.SetValue("https://example.com/owner/repo.git")
	updated, _ := m.startClone("/dest/proj/repo")
	updated, _ = updated.(model).Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	updated, cmd := updated.(model).Update(cloneDoneMsg{path: "/dest/proj/repo"})
	m = updated.(model)
	if m.result.Clone == nil || m.result.Clone.Cloned != "/dest/proj/repo" {
		t.Errorf("a clone that finished must be kept, got %+v", m.result.Clone)
	}
	if m.inputMode == modeURLInput || cmd == nil {
		t.Errorf("mode = %v, should go on to open the clone", m.inputMode)
	}
}

func TestMissingRoots_bottomLayout(t *testing.T) {
	root := t.TempDir()
	cs := []cand.Candidate{{AbsPath: filepath.Join(root, "a"), Root: root, RelPath: "a"}}
	cfg := config.Config{Paths: []string{root, filepath.Join(root, "gone")}, Layout: "bottom"}
	m := newModel(cs, map[string]float64{}, tmux.State{}, false, cfg, false)
	m.width, m.height, m.ready = 80, 12, true

	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	// status, separator, body…, separator, search
	if !strings.Contains(lines[2], "Not found") {
		t.Errorf("warning should sit at the top of the list, away from the search bar: %q", lines[2])
	}
	if best := lines[len(lines)-3]; !strings.Contains(best, "a") || strings.Contains(best, "Not found") {
		t.Errorf("best match should hug the search bar: %q", best)
	}
}

func TestDeleteTmp_reresolves(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmpDir, "foo"), 0o755); err != nil {
		t.Fatal(err)
	}
	cs := cand.Resolve([]cand.Candidate{
		{AbsPath: "/code/foo", Root: "/code", RelPath: "foo"},
		cand.Tmp(tmpDir, "foo"),
	})
	m := newModel(cs, map[string]float64{}, tmux.State{}, false, config.Config{TmpPath: tmpDir}, false)
	if got := m.sessionOf("/code/foo"); got != "foo@code" {
		t.Fatalf("session = %q, want foo@code while the tmp twin exists", got)
	}
	if errs := m.deleteTmp(map[string]bool{filepath.Join(tmpDir, "foo"): true}); len(errs) > 0 {
		t.Fatal(errs)
	}
	if got := m.sessionOf("/code/foo"); got != "foo" {
		t.Errorf("session = %q, want plain foo once the twin is gone", got)
	}
}
