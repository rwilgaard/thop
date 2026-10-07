package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	cand "github.com/rwilgaard/thop/internal/candidates"
	"github.com/rwilgaard/thop/internal/config"
	"github.com/rwilgaard/thop/internal/tmux"
)

func TestNewProject(t *testing.T) {
	ctrlN := keyCtrl('n')
	send := func(m model, msgs ...tea.KeyPressMsg) model {
		for _, msg := range msgs {
			m = press(m, msg)
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
		m = sized(m, 80, 24)
		return m, paths
	}

	t.Run("one root skips the picker", func(t *testing.T) {
		m, paths := setup(t, 1)
		m = send(m, ctrlN)
		if m.inputMode != modeNewProjName || m.newProj.root != paths[0] {
			t.Fatalf("mode = %v root = %q, want name dialog in %q", m.inputMode, m.newProj.root, paths[0])
		}
		if m = send(m, keyEsc); m.inputMode != modeNormal {
			t.Errorf("esc: mode = %v, want modeNormal", m.inputMode)
		}
	})

	t.Run("several roots: pick, then esc steps back", func(t *testing.T) {
		m, paths := setup(t, 2)
		m = send(m, ctrlN)
		if m.inputMode != modeNewProjRoot || len(m.newProj.filtered) != 2 {
			t.Fatalf("mode = %v with %d roots, want root picker with 2", m.inputMode, len(m.newProj.filtered))
		}
		m = send(m, keyDown, keyEnter)
		if m.inputMode != modeNewProjName || m.newProj.root != paths[1] {
			t.Fatalf("mode = %v root = %q, want name dialog in %q", m.inputMode, m.newProj.root, paths[1])
		}
		if m = send(m, keyEsc); m.inputMode != modeNewProjRoot {
			t.Errorf("first esc: mode = %v, want modeNewProjRoot", m.inputMode)
		}
		if m = send(m, keyEsc); m.inputMode != modeNormal {
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
			m = send(m, keyEnter)
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
		m = send(m, keyEnter)

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
		m := testModel()
		m.setup.add = func(string) (string, []cand.Candidate, error) { return "", nil, nil }
		_ = m.openSetup()
		m = send(m, keyEsc, ctrlN)
		if m.inputMode != modeSetup {
			t.Errorf("mode = %v, want modeSetup", m.inputMode)
		}
	})
}
