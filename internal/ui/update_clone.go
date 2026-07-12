package ui

import (
	"os"
	"path/filepath"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/rwilgaard/thop/internal/git"
)

func (m model) updateURLInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "esc":
		m.tiURL.Blur()
		m.tiURL.SetValue("")
		m.inputMode = modeNormal
		return m, m.tiQuery.Focus()
	case key.Matches(msg, m.keys.Enter):
		if m.tiURL.Value() != "" {
			m.tiURL.Blur()
			m.tiDest.SetValue("")
			m.destCursor = 0
			m.rebuildDestFiltered()
			m.inputMode = modeDestPicker
			return m, m.tiDest.Focus()
		}
	default:
		var cmd tea.Cmd
		m.tiURL, cmd = m.tiURL.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) updateDestPicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "esc":
		m.tiDest.Blur()
		m.inputMode = modeURLInput
		return m, m.tiURL.Focus()
	case key.Matches(msg, m.keys.Enter):
		if m.destCursor < len(m.destFiltered) {
			chosen := m.destFiltered[m.destCursor].base.candidate.AbsPath
			name := git.RepoNameFromURL(m.tiURL.Value())
			fullDest := filepath.Join(chosen, name)
			if _, err := os.Stat(fullDest); err == nil {
				m.tiDest.Blur()
				m.cloneDestDir = chosen
				m.tiCloneName.SetValue(name)
				m.inputMode = modeCloneName
				return m, m.tiCloneName.Focus()
			}
			m.result.Clone = &CloneRequest{URL: m.tiURL.Value(), Dest: fullDest}
			m.tiDest.Blur()
			m.loadingText = "Cloning…"
			m.inputMode = modeLoading
			return m, tea.Batch(cmdClone(m.ctx, m.tiURL.Value(), fullDest), m.spin.Tick)
		}
		return m, tea.Quit
	case key.Matches(msg, m.keys.Up):
		m.destCursor = moveCursor(m.destCursor, m.visualStep(-1), len(m.destFiltered))
	case key.Matches(msg, m.keys.Down):
		m.destCursor = moveCursor(m.destCursor, m.visualStep(1), len(m.destFiltered))
	default:
		prev := m.tiDest.Value()
		var cmd tea.Cmd
		m.tiDest, cmd = m.tiDest.Update(msg)
		if m.tiDest.Value() != prev {
			m.destCursor = 0
			m.rebuildDestFiltered()
		}
		return m, cmd
	}
	return m, nil
}

func (m model) updateCloneName(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "esc":
		m.tiCloneName.Blur()
		m.inputMode = modeDestPicker
		return m, m.tiDest.Focus()
	case key.Matches(msg, m.keys.Enter):
		if m.tiCloneName.Value() != "" {
			dest := filepath.Join(m.cloneDestDir, m.tiCloneName.Value())
			m.result.Clone = &CloneRequest{
				URL:  m.tiURL.Value(),
				Dest: dest,
			}
			m.tiCloneName.Blur()
			m.loadingText = "Cloning…"
			m.inputMode = modeLoading
			return m, tea.Batch(cmdClone(m.ctx, m.tiURL.Value(), dest), m.spin.Tick)
		}
	default:
		var cmd tea.Cmd
		m.tiCloneName, cmd = m.tiCloneName.Update(msg)
		return m, cmd
	}
	return m, nil
}
