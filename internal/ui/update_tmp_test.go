package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	cand "github.com/rwilgaard/thop/internal/candidates"
	"github.com/rwilgaard/thop/internal/config"
	"github.com/rwilgaard/thop/internal/tmux"
)

func TestUpdateNameInput(t *testing.T) {
	m := press(testModel(), keyCtrl('t'))
	if m.inputMode != modeNameInput {
		t.Fatalf("ctrl+t: mode = %v, want modeNameInput", m.inputMode)
	}
	m = press(typeText(m, "foo"), keyEnter)
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

	// first enter: should flag conflict, not quit
	m = press(m, keyEnter)
	if !m.tmp.conflict {
		t.Error("first enter on existing name should set nameConflict")
	}
	if m.result.Tmp != nil {
		t.Error("should not quit on first conflict")
	}

	// second enter: should proceed despite conflict (switches to modeLoading)
	m = press(m, keyEnter)
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
	ch := keyRune('x')
	m2 = press(m2, ch)
	if m2.tmp.conflict {
		t.Error("typing should reset nameConflict")
	}
}

func TestConfirmClean_dialog(t *testing.T) {
	m := testModel()
	m.clean.selected = map[string]bool{}
	for i := range 20 {
		p := fmt.Sprintf("/t/tmp-%02d", i)
		m.all = append(m.all, baseItem{candidate: cand.Candidate{AbsPath: p, RelPath: filepath.Base(p), IsTmp: true}, active: i == 1})
	}
	m.inputMode = modeConfirmClean

	titles := []struct {
		selected []string
		want     string
	}{
		{[]string{"/t/tmp-00"}, "Delete 1 tmp project?"},
		{[]string{"/t/tmp-00", "/t/tmp-01"}, "Delete 2 tmp projects (1 open)?"},
	}
	for _, tt := range titles {
		m.clean.selected = map[string]bool{}
		for _, p := range tt.selected {
			m.clean.selected[p] = true
		}
		if out := plain(sized(m, 100, 24)); !strings.Contains(out, tt.want) {
			t.Errorf("missing %q in:\n%s", tt.want, out)
		}
	}

	for _, it := range m.all {
		m.clean.selected[it.candidate.AbsPath] = true
	}
	out := plain(sized(m, 60, 10)) // room for 4 rows and the count
	if !strings.Contains(out, "tmp-03") || strings.Contains(out, "tmp-04") {
		t.Errorf("expected rows 00-03 only:\n%s", out)
	}
	if !strings.Contains(out, "… and 16 more") {
		t.Errorf("missing overflow count:\n%s", out)
	}
}

func TestCleanTmp(t *testing.T) {
	y, n, space := keyRune('y'), keyRune('n'), keyRune(' ')
	tests := []struct {
		name       string
		keys       []tea.Msg
		wantGone   []string
		wantKilled []string
		wantMode   inputMode
	}{
		{"enter targets the cursor row", []tea.Msg{keyDown, keyEnter, y}, []string{"two"}, []string{"two"}, modeNormal},
		{"space selects several", []tea.Msg{space, keyDown, keyDown, space, keyEnter, y}, []string{"one", "three"}, nil, modeNormal},
		{"space twice deselects", []tea.Msg{space, space, keyDown, keyDown, keyEnter, y}, []string{"three"}, nil, modeNormal},
		{"any key but y cancels", []tea.Msg{keyEnter, n}, nil, nil, modeCleanTmp},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{TmpPath: tmpDir}, false)
			for _, name := range []string{"one", "two", "three"} {
				mustMkdir(t, filepath.Join(tmpDir, name))
				m.all = append(m.all, baseItem{candidate: cand.Tmp(tmpDir, name), active: name == "two"})
			}
			var killed []string
			m.killSession = func(_ tmux.State, s string) error {
				killed = append(killed, s)
				return nil
			}
			m.loadState = func() tmux.State { return tmux.State{} }

			m = press(press(m, keyCtrl('x')), tt.keys...)

			if m.inputMode != tt.wantMode {
				t.Errorf("mode = %v, want %v", m.inputMode, tt.wantMode)
			}
			for _, name := range []string{"one", "two", "three"} {
				_, err := os.Stat(filepath.Join(tmpDir, name))
				if gone, want := os.IsNotExist(err), slices.Contains(tt.wantGone, name); gone != want {
					t.Errorf("%s: gone = %v, want %v", name, gone, want)
				}
			}
			if !slices.Equal(killed, tt.wantKilled) {
				t.Errorf("killed sessions = %v, want %v", killed, tt.wantKilled)
			}
			if tt.wantMode != modeNormal {
				return
			}
			if got, want := len(m.all), 3-len(tt.wantGone); got != want {
				t.Errorf("%d rows left, want %d", got, want)
			}
			if len(m.clean.selected) != 0 {
				t.Error("selection should be cleared after delete")
			}
		})
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
