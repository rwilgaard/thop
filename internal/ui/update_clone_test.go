package ui

import (
	"context"
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

func TestUpdateURLInput(t *testing.T) {
	m := typeText(press(testModel(), keyCtrl('g')), "http")
	if m.inputMode != modeURLInput || m.clone.tiURL.Value() != "http" {
		t.Fatalf("mode = %v url = %q, want URL input holding %q", m.inputMode, m.clone.tiURL.Value(), "http")
	}
	if m = press(m, tea.KeyPressMsg{Code: tea.KeyBackspace}); m.clone.tiURL.Value() != "htt" {
		t.Errorf("after backspace url = %q, want %q", m.clone.tiURL.Value(), "htt")
	}

	if next := press(m, keyEnter); next.inputMode != modeDestPicker {
		t.Errorf("enter: mode = %v, want modeDestPicker", next.inputMode)
	}

	m = press(m, keyEsc)
	if m.inputMode != modeNormal || m.clone.tiURL.Value() != "" || !m.tiQuery.Focused() {
		t.Errorf("esc: mode = %v url = %q query focused = %v; want picker, cleared, focused",
			m.inputMode, m.clone.tiURL.Value(), m.tiQuery.Focused())
	}
}

func TestURLInput_expandsShorthand(t *testing.T) {
	m := newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{CloneShorthand: "https://github.com/{repo}.git"}, false)
	m.inputMode = modeURLInput
	m.clone.tiURL.SetValue("rwilgaard/thop")
	m = press(m, keyEnter)
	if m.inputMode != modeDestPicker {
		t.Fatalf("mode = %v, want modeDestPicker", m.inputMode)
	}
	if got := m.clone.tiURL.Value(); got != "https://github.com/rwilgaard/thop.git" {
		t.Errorf("url = %q, want expanded shorthand", got)
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
	m := testModel(cs...)
	m.inputMode = modeDestPicker
	m.clone.tiURL.SetValue("https://github.com/user/myrepo")
	_ = m.clone.tiDest.Focus()
	m.rebuildDestFiltered()

	// Select the candidate and press enter — should detect conflict.
	m = press(m, keyEnter)
	if m.inputMode != modeCloneName {
		t.Errorf("conflict should advance to modeCloneName, got %v", m.inputMode)
	}
	if m.clone.tiName.Value() != "myrepo" {
		t.Errorf("clone.tiName = %q, want %q", m.clone.tiName.Value(), "myrepo")
	}

	// Type a new name and confirm.
	for _, ch := range []string{"2"} {
		msg := tea.KeyPressMsg{Text: ch, Code: rune(ch[0])}
		m = press(m, msg)
	}
	m = press(m, keyEnter)
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
	m := testModel()
	m.inputMode = modeCloneName
	m.clone.tiName.SetValue("myrepo")
	_ = m.clone.tiName.Focus()
	m = press(m, keyEsc)
	if m.inputMode != modeDestPicker {
		t.Errorf("esc should return to modeDestPicker, got %v", m.inputMode)
	}
}

func TestClone_cancel(t *testing.T) {
	start := func(t *testing.T) model {
		t.Helper()
		m := testModel()
		m = sized(m, 80, 24)
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
		m = press(m, keyEsc)
		if !m.clone.cancelled || m.inputMode != modeCloning {
			t.Fatalf("esc should mark cancelled and wait for git, got cancelled=%v mode=%v", m.clone.cancelled, m.inputMode)
		}
		if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, "Cancelling…") {
			t.Errorf("missing cancelling state in:\n%s", plain)
		}

		m = press(m, cloneDoneMsg{err: context.Canceled})
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
		if got := press(m, cloneDoneMsg{err: os.ErrPermission}).inputMode; got != modeError {
			t.Errorf("mode = %v, want modeError", got)
		}
	})

	t.Run("esc does nothing for other loading states", func(t *testing.T) {
		m := testModel()
		m.inputMode, m.loadingText = modeLoading, "Opening…"
		updated, cmd := m.Update(keyEsc)
		if cmd != nil || updated.(model).inputMode != modeLoading {
			t.Error("esc should be ignored while opening")
		}
	})
}

func TestClone_escAfterSuccess(t *testing.T) {
	m := testModel()
	m.clone.tiURL.SetValue("https://example.com/owner/repo.git")
	started, _ := m.startClone("/dest/proj/repo")
	updated, cmd := press(started.(model), keyEsc).Update(cloneDoneMsg{path: "/dest/proj/repo"})
	m = updated.(model)
	if m.result.Clone == nil || m.result.Clone.Cloned != "/dest/proj/repo" {
		t.Errorf("a clone that finished must be kept, got %+v", m.result.Clone)
	}
	if m.inputMode == modeURLInput || cmd == nil {
		t.Errorf("mode = %v, should go on to open the clone", m.inputMode)
	}
}
