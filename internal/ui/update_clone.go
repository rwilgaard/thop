package ui

import (
	"context"
	"os"
	"path/filepath"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/rwilgaard/thop/internal/git"
)

func (m model) updateURLInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "ctrl+c":
		return m, tea.Quit
	case msg.String() == "esc":
		m.clone.tiURL.Blur()
		m.clone.tiURL.SetValue("")
		m.inputMode = modeNormal
		return m, m.tiQuery.Focus()
	case key.Matches(msg, m.keys.Enter):
		if m.clone.tiURL.Value() != "" {
			m.clone.tiURL.Blur()
			return m, m.openDestPicker()
		}
		return m, nil
	default:
		return m.forwardInput(msg)
	}
}

// openDestPicker expands shorthand in the entered URL and switches to a fresh
// destination picker.
func (m *model) openDestPicker() tea.Cmd {
	m.clone.tiURL.SetValue(git.ExpandShorthand(m.clone.tiURL.Value(), m.clone.shorthand))
	m.clone.tiDest.SetValue("")
	m.clone.destCursor = 0
	m.rebuildDestFiltered()
	m.inputMode = modeDestPicker
	return m.clone.tiDest.Focus()
}

func (m model) updateCloning(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "ctrl+c":
		return m, tea.Quit
	case msg.String() == "esc" && !m.clone.cancelled:
		m.clone.cancelled = true
		m.clone.cancel()
	}
	return m, nil
}

func (m model) updateDestPicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if cur, ok := m.navCursor(msg, m.clone.destCursor, len(m.clone.destFiltered)); ok {
		m.clone.destCursor = cur
		return m, nil
	}
	switch {
	case msg.String() == "ctrl+c":
		return m, tea.Quit
	case msg.String() == "esc":
		m.clone.tiDest.Blur()
		m.inputMode = modeURLInput
		return m, m.clone.tiURL.Focus()
	case key.Matches(msg, m.keys.Enter):
		if m.clone.destCursor < len(m.clone.destFiltered) {
			chosen := m.clone.destFiltered[m.clone.destCursor].base.candidate.AbsPath
			name := git.RepoNameFromURL(m.clone.tiURL.Value())
			fullDest := filepath.Join(chosen, name)
			if _, err := os.Stat(fullDest); err == nil {
				m.clone.tiDest.Blur()
				m.clone.destDir = chosen
				m.clone.tiName.SetValue(name)
				m.inputMode = modeCloneName
				return m, m.clone.tiName.Focus()
			}
			return m.startClone(fullDest)
		}
		return m, nil
	default:
		return m.forwardInput(msg)
	}
}

func (m model) updateCloneName(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "ctrl+c":
		return m, tea.Quit
	case msg.String() == "esc":
		m.clone.tiName.Blur()
		m.inputMode = modeDestPicker
		return m, m.clone.tiDest.Focus()
	case key.Matches(msg, m.keys.Enter):
		if m.clone.tiName.Value() != "" {
			return m.startClone(filepath.Join(m.clone.destDir, m.clone.tiName.Value()))
		}
		return m, nil
	default:
		return m.forwardInput(msg)
	}
}

func (m model) startClone(dest string) (tea.Model, tea.Cmd) {
	m.result.Clone = &CloneRequest{URL: m.clone.tiURL.Value(), Dest: dest, Session: m.sessionOf(filepath.Dir(dest))}
	m.clone.tiDest.Blur()
	m.clone.tiName.Blur()
	m.inputMode = modeCloning
	ctx, cancel := context.WithCancel(m.ctx)
	m.clone.cancel, m.clone.cancelled = cancel, false
	return m, tea.Batch(cmdClone(ctx, m.clone.tiURL.Value(), dest), m.spin.Tick)
}
