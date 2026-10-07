package ui

import (
	"os"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	cand "github.com/rwilgaard/thop/internal/candidates"
	"github.com/rwilgaard/thop/internal/config"
	"github.com/rwilgaard/thop/internal/tmux"
)

var (
	keyEnter = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyEsc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	keyUp    = tea.KeyPressMsg{Code: tea.KeyUp}
	keyDown  = tea.KeyPressMsg{Code: tea.KeyDown}
)

func keyRune(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Text: string(r), Code: r} }

func keyCtrl(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl} }

// testModel returns a model over cs with no frecency, no tmux state and
// default config.
func testModel(cs ...cand.Candidate) model {
	return newModel(cs, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
}

// sized returns m ready to render at w×h.
func sized(m model, w, h int) model {
	m.width, m.height, m.ready = w, h, true
	return m
}

// press feeds msgs to m in order and returns the resulting model.
func press(m model, msgs ...tea.Msg) model {
	for _, msg := range msgs {
		updated, _ := m.Update(msg)
		m = updated.(model)
	}
	return m
}

// typeText presses one key per rune of s.
func typeText(m model, s string) model {
	for _, r := range s {
		m = press(m, keyRune(r))
	}
	return m
}

// plain renders m without ANSI styling.
func plain(m model) string { return ansi.Strip(m.View().Content) }

// rowNames lists the RelPaths of the main picker rows, in order.
func rowNames(m model) []string {
	names := make([]string, len(m.filtered))
	for i, it := range m.filtered {
		names[i] = it.base.candidate.RelPath
	}
	return names
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
