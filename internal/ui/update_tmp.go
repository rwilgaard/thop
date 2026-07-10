package ui

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

func (m model) updateNameInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.tiName.Blur()
		m.nameConflict = false
		m.inputMode = modeNormal
		return m, m.tiQuery.Focus()
	case "enter":
		name := m.tiName.Value()
		if invalidTmpName(name) {
			m.nameConflict = true
			return m, nil
		}
		if name != "" && m.tmpPath != "" && !m.nameConflict {
			if _, err := os.Stat(filepath.Join(m.tmpPath, name)); err == nil {
				m.nameConflict = true
				return m, nil
			}
		}
		if name == "" {
			name = "tmp-" + time.Now().Format("20060102-150405")
		}
		m.result.Tmp = &TmpRequest{Name: name}
		m.tiName.Blur()
		m.loadingText = "creating…"
		m.inputMode = modeLoading
		return m, tea.Batch(cmdCreateTmp(m.tmpPath, name), m.spin.Tick)
	default:
		prev := m.tiName.Value()
		var cmd tea.Cmd
		m.tiName, cmd = m.tiName.Update(msg)
		if m.tiName.Value() != prev {
			m.nameConflict = false
		}
		return m, cmd
	}
}

func (m model) updateCleanTmp(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.tiClean.Blur()
		m.inputMode = modeNormal
		m.selected = make(map[string]bool)
		return m, m.tiQuery.Focus()
	case "up", "ctrl+k":
		if m.cleanCursor > 0 {
			m.cleanCursor--
		}
	case "down", "ctrl+j":
		if m.cleanCursor < len(m.cleanFiltered)-1 {
			m.cleanCursor++
		}
	case "space":
		if m.cleanCursor < len(m.cleanFiltered) {
			path := m.cleanFiltered[m.cleanCursor].base.candidate.AbsPath
			if m.selected[path] {
				delete(m.selected, path)
			} else {
				m.selected[path] = true
			}
		}
	case "enter":
		if len(m.tmpItems()) > 0 {
			m.tiClean.Blur()
			m.inputMode = modeConfirmClean
		}
	default:
		prev := m.tiClean.Value()
		var cmd tea.Cmd
		m.tiClean, cmd = m.tiClean.Update(msg)
		if m.tiClean.Value() != prev {
			m.cleanCursor = 0
			m.rebuildCleanFiltered()
		}
		return m, cmd
	}
	return m, nil
}

func (m model) updateConfirmClean(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.inputMode = modeCleanTmp
		return m, m.tiClean.Focus()
	default:
		if msg.Key().Text == "y" {
			toDelete := m.selected
			if len(toDelete) == 0 {
				toDelete = make(map[string]bool, len(m.cleanFiltered))
				for _, item := range m.cleanFiltered {
					toDelete[item.base.candidate.AbsPath] = true
				}
			}
			var kept []baseItem
			var errMsgs []string
			for _, item := range m.all {
				c := item.candidate
				if c.IsTmp && toDelete[c.AbsPath] {
					if err := os.RemoveAll(c.AbsPath); err != nil {
						kept = append(kept, item)
						errMsgs = append(errMsgs, err.Error())
					}
				} else {
					kept = append(kept, item)
				}
			}
			m.all = kept
			m.selected = make(map[string]bool)
			m.rebuildFiltered()
			if len(errMsgs) > 0 {
				m.errMsg = strings.Join(errMsgs, "\n")
				m.errReturnMode = modeNormal
				m.inputMode = modeError
				return m, nil
			}
			m.inputMode = modeNormal
			return m, m.tiQuery.Focus()
		}
		m.inputMode = modeCleanTmp
		return m, m.tiClean.Focus()
	}
}
