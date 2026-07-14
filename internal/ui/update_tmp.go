package ui

import (
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/rwilgaard/thop/internal/candidates"
)

func (m model) updateNameInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "ctrl+c":
		return m, tea.Quit
	case msg.String() == "esc":
		m.tmp.tiName.Blur()
		m.tmp.conflict = false
		m.inputMode = modeNormal
		return m, m.tiQuery.Focus()
	case key.Matches(msg, m.keys.Enter):
		name := m.tmp.tiName.Value()
		if !candidates.ValidTmpName(name) {
			m.tmp.conflict = true
			return m, nil
		}
		if name != "" && m.tmpPath != "" && !m.tmp.conflict {
			if _, err := os.Stat(filepath.Join(m.tmpPath, name)); err == nil {
				m.tmp.conflict = true
				return m, nil
			}
		}
		if name == "" {
			name = candidates.AutoTmpName()
		}
		m.result.Tmp = &TmpRequest{Name: name}
		m.tmp.tiName.Blur()
		m.loadingText = "Creating…"
		m.inputMode = modeLoading
		return m, tea.Batch(cmdCreateTmp(m.tmpPath, name), m.spin.Tick)
	default:
		return m.forwardInput(msg)
	}
}

func (m model) updateCleanTmp(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "ctrl+c":
		return m, tea.Quit
	case msg.String() == "esc":
		m.clean.tiQuery.Blur()
		m.inputMode = modeNormal
		m.clean.selected = make(map[string]bool)
		return m, m.tiQuery.Focus()
	case key.Matches(msg, m.keys.Up):
		m.clean.cursor = moveCursor(m.clean.cursor, m.visualStep(-1), len(m.clean.filtered))
	case key.Matches(msg, m.keys.Down):
		m.clean.cursor = moveCursor(m.clean.cursor, m.visualStep(1), len(m.clean.filtered))
	case msg.String() == "space":
		if m.clean.cursor < len(m.clean.filtered) {
			path := m.clean.filtered[m.clean.cursor].base.candidate.AbsPath
			if m.clean.selected[path] {
				delete(m.clean.selected, path)
			} else {
				m.clean.selected[path] = true
			}
		}
	case key.Matches(msg, m.keys.Enter):
		if len(m.tmpItems()) > 0 {
			m.clean.tiQuery.Blur()
			m.inputMode = modeConfirmClean
		}
	default:
		return m.forwardInput(msg)
	}
	return m, nil
}

func (m model) updateConfirmClean(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.inputMode = modeCleanTmp
		return m, m.clean.tiQuery.Focus()
	default:
		if msg.Key().Text != "y" {
			m.inputMode = modeCleanTmp
			return m, m.clean.tiQuery.Focus()
		}
		toDelete := m.clean.selected
		if len(toDelete) == 0 {
			toDelete = make(map[string]bool, len(m.clean.filtered))
			for _, item := range m.clean.filtered {
				toDelete[item.base.candidate.AbsPath] = true
			}
		}
		errMsgs := m.deleteTmp(toDelete)
		m.clean.selected = make(map[string]bool)
		m.rebuildFiltered()
		if len(errMsgs) > 0 {
			return m.showError(strings.Join(errMsgs, "\n"), modeNormal), nil
		}
		m.inputMode = modeNormal
		return m, m.tiQuery.Focus()
	}
}

// deleteTmp removes the given tmp dirs from disk and from m.all. Dirs that
// fail to delete stay listed; their errors are returned.
func (m *model) deleteTmp(toDelete map[string]bool) []string {
	var kept []baseItem
	var errMsgs []string
	for _, item := range m.all {
		c := item.candidate
		if c.IsTmp && toDelete[c.AbsPath] {
			if err := os.RemoveAll(c.AbsPath); err != nil {
				kept = append(kept, item)
				errMsgs = append(errMsgs, err.Error())
			}
			continue
		}
		kept = append(kept, item)
	}
	m.all = kept
	return errMsgs
}
