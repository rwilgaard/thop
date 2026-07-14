package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
		{"items", "item count"},
	}
	for _, c := range checks {
		if !strings.Contains(out, c.want) {
			t.Errorf("View() missing %s: %q not found", c.desc, c.want)
		}
	}
	if strings.Contains(out, "Clone repository") {
		t.Error("help content should be hidden by default")
	}

	m.showHelp = true
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
	if m.inputMode != modeLoading {
		t.Errorf("should be modeLoading after confirming clone name, got %v", m.inputMode)
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

	if m.showHelp {
		t.Fatal("help should start hidden")
	}

	q := tea.KeyPressMsg{Text: "?", Code: '?'}
	updated, _ := m.Update(q)
	m = updated.(model)
	if !m.showHelp {
		t.Error("? should show help")
	}

	out := m.View().Content
	for _, w := range []string{"Navigate", "Actions", "Filters", "Clone repository", "Move up", "Projects only"} {
		if !strings.Contains(out, w) {
			t.Errorf("help overlay missing %q", w)
		}
	}

	updated, _ = m.Update(q)
	m = updated.(model)
	if m.showHelp {
		t.Error("second ? should hide help")
	}

	m.showHelp = true
	esc := tea.KeyPressMsg{Code: tea.KeyEscape}
	updated, _ = m.Update(esc)
	m = updated.(model)
	if m.showHelp {
		t.Error("esc should hide help")
	}
}

func TestHelpOverlay_narrow(t *testing.T) {
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	m.width, m.height, m.ready = 60, 24, true
	m.showHelp = true

	out := m.View().Content
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 60 {
			t.Errorf("line exceeds width 60 (got %d): %q", w, line)
		}
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
		{modeURLInput, []string{"Clone repository ❯", "Clone", "Cancel"}},
		{modeDestPicker, []string{"Clone › Destination ❯", "Select", "Back"}},
		{modeCloneName, []string{"Clone › Name conflict ❯", "Clone as", "Back"}},
		{modeNameInput, []string{"New tmp project ❯", "Create", "Cancel"}},
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

	m.clean.selected = map[string]bool{"/t/a": true}
	if out := m.View().Content; !strings.Contains(out, "Delete 1 tmp project?") {
		t.Errorf("singular form missing: %q", out)
	}
	m.clean.selected = map[string]bool{"/t/a": true, "/t/b": true}
	if out := m.View().Content; !strings.Contains(out, "Delete 2 tmp projects?") {
		t.Errorf("plural form missing: %q", out)
	}
}

func TestStatusBar_modes(t *testing.T) {
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	m.width, m.height, m.ready = 100, 24, true

	// normal, wide: spelled+bracketed keys with dot separators, matching the
	// other hint rows
	out := m.View().Content
	for _, w := range []string{"Filter", "<ctrl-a>", "All", "<ctrl-p>", "Projects", "<ctrl-r>", "Repos", "<ctrl-t>", "Tmp", "•", "0 items"} {
		if !strings.Contains(out, w) {
			t.Errorf("wide normal status missing %q: %q", w, out)
		}
	}

	// normal, narrow: falls back to compact bare carets, no dots, so it fits
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
	if !strings.Contains(out, "1 selected") || !strings.Contains(out, "1 items") {
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
	// clamp: down at index 0 (visual bottom) stays put
	updated, _ = m.Update(down)
	m = updated.(model)
	if m.cursor != 0 {
		t.Errorf("bottom layout: down at bottom should clamp, got %d", m.cursor)
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

	// top layout: best match (index 0) is visually at the top.
	// Up at index 0 clamps (stays 0).
	updated, _ := m.Update(up)
	m = updated.(model)
	if m.cursor != 0 {
		t.Errorf("top layout: up from index 0 should clamp, got %d", m.cursor)
	}
	// Down moves to index 1.
	updated, _ = m.Update(down)
	m = updated.(model)
	if m.cursor != 1 {
		t.Errorf("top layout: down should reach index 1, got %d", m.cursor)
	}
	// Down at last index clamps (stays 1).
	updated, _ = m.Update(down)
	m = updated.(model)
	if m.cursor != 1 {
		t.Errorf("top layout: down at last index should clamp, got %d", m.cursor)
	}
	// Up returns to 0.
	updated, _ = m.Update(up)
	m = updated.(model)
	if m.cursor != 0 {
		t.Errorf("top layout: up should return to index 0, got %d", m.cursor)
	}
}
