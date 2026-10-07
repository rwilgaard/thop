package ui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	cand "github.com/rwilgaard/thop/internal/candidates"
	"github.com/rwilgaard/thop/internal/config"
	"github.com/rwilgaard/thop/internal/tmux"
)

func TestSetup(t *testing.T) {
	start := func(t *testing.T, add func(string) (string, []cand.Candidate, error)) model {
		t.Helper()
		tmp := []cand.Candidate{{AbsPath: "/t/scratch", RelPath: "scratch", IsTmp: true}}
		m := newModel(tmp, map[string]float64{}, tmux.State{}, false, config.Config{File: "/etc/thop/config.yaml"}, false)
		m.setup.add = add
		_ = m.openSetup()
		m = press(m, tea.WindowSizeMsg{Width: 80, Height: 24})
		m.setup.tiPath.SetValue("~/code")
		return m
	}
	found := func(string) (string, []cand.Candidate, error) {
		return "/r/projects", []cand.Candidate{{AbsPath: "/r/projects/app", Root: "/r/projects", RelPath: "app"}}, nil
	}

	t.Run("dialog starts empty", func(t *testing.T) {
		m := start(t, found)
		m.setup.tiPath.SetValue("")
		updated, cmd := m.Update(keyEnter)
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
		m = press(m, keyEnter)
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
		m = press(m, keyEnter)
		if m.inputMode != modeSetup {
			t.Fatalf("mode = %v, want modeSetup", m.inputMode)
		}
		if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, "not a directory") {
			t.Errorf("missing error in:\n%s", plain)
		}
		if got := press(m, keyRune('x')).setup.err; got != "" {
			t.Errorf("typing should clear the error, got %q", got)
		}
	})

	t.Run("skip lands in a picker that names the config file", func(t *testing.T) {
		m := start(t, found)
		m.all = nil
		m = press(m, keyEsc)
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
		if got := press(m, keyEnter).inputMode; got != modeDestPicker {
			t.Errorf("mode = %v, want modeDestPicker", got)
		}
	})
}
