package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.helpModel.SetWidth(msg.Width)
	case selectionDoneMsg:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.inputMode = modeError
			return m, nil
		}
		return m, tea.Quit
	case cloneDoneMsg:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.inputMode = modeError
			return m, nil
		}
		m.result.Clone.Cloned = msg.path
		if m.inTmux {
			m.loadingText = "opening…"
			return m, cmdRunSelection(msg.path, "")
		}
		return m, tea.Quit
	case tmpCreatedMsg:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.inputMode = modeError
			return m, nil
		}
		m.result.Tmp.Path = msg.path
		if m.inTmux {
			m.loadingText = "opening…"
			return m, cmdRunSelection(msg.path, m.tmpPath)
		}
		return m, tea.Quit
	case tea.KeyPressMsg:
		switch m.inputMode {
		case modeURLInput:
			return m.updateURLInput(msg)
		case modeDestPicker:
			return m.updateDestPicker(msg)
		case modeCloneName:
			return m.updateCloneName(msg)
		case modeNameInput:
			return m.updateNameInput(msg)
		case modeCleanTmp:
			return m.updateCleanTmp(msg)
		case modeConfirmClean:
			return m.updateConfirmClean(msg)
		case modeLoading:
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		case modeError:
			return m.updateError(msg)
		default:
			return m.updateNormal(msg)
		}
	}
	// Forward all messages to the active textinput so paste, blink, etc. work.
	var cmd tea.Cmd
	switch m.inputMode {
	case modeNormal:
		prev := m.tiQuery.Value()
		m.tiQuery, cmd = m.tiQuery.Update(msg)
		if m.tiQuery.Value() != prev {
			m.cursor = 0
			m.rebuildFiltered()
		}
	case modeURLInput:
		m.tiURL, cmd = m.tiURL.Update(msg)
	case modeDestPicker:
		prev := m.tiDest.Value()
		m.tiDest, cmd = m.tiDest.Update(msg)
		if m.tiDest.Value() != prev {
			m.destCursor = 0
			m.rebuildDestFiltered()
		}
	case modeCloneName:
		m.tiCloneName, cmd = m.tiCloneName.Update(msg)
	case modeNameInput:
		m.tiName, cmd = m.tiName.Update(msg)
	case modeCleanTmp:
		m.tiClean, cmd = m.tiClean.Update(msg)
	case modeLoading, modeError:
	}
	return m, cmd
}

func (m model) updateNormal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.helpModel.ShowAll {
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "?", "esc":
			m.helpModel.ShowAll = false
			return m, m.tiQuery.Focus()
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, keys.Enter):
		if len(m.filtered) > 0 {
			c := m.filtered[m.cursor].base.candidate
			m.result.Candidate = c
			if m.inTmux {
				m.loadingText = "opening…"
				m.inputMode = modeLoading
				return m, cmdRunSelection(c.AbsPath, c.Root)
			}
		}
		return m, tea.Quit
	case key.Matches(msg, keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(msg, keys.Down):
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
	case key.Matches(msg, keys.All):
		m.view, m.cursor = viewAll, 0
		m.rebuildFiltered()
	case key.Matches(msg, keys.Projects):
		m.view, m.cursor = viewProject, 0
		m.rebuildFiltered()
	case key.Matches(msg, keys.Repos):
		m.view, m.cursor = viewRepo, 0
		m.rebuildFiltered()
	case key.Matches(msg, keys.Tmp):
		m.view, m.cursor = viewTmp, 0
		m.rebuildFiltered()
	case key.Matches(msg, keys.Clone):
		m.tiQuery.Blur()
		m.inputMode = modeURLInput
		m.tiURL.SetValue("")
		return m, m.tiURL.Focus()
	case key.Matches(msg, keys.NewTmp):
		m.tiQuery.Blur()
		m.tiName.SetValue("")
		m.inputMode = modeNameInput
		return m, m.tiName.Focus()
	case key.Matches(msg, keys.CleanTmp):
		m.cleanCursor = 0
		m.selected = make(map[string]bool)
		m.tiClean.SetValue("")
		m.rebuildCleanFiltered()
		m.tiQuery.Blur()
		m.inputMode = modeCleanTmp
		return m, m.tiClean.Focus()
	case key.Matches(msg, keys.Help):
		m.tiQuery.Blur()
		m.helpModel.ShowAll = true
	default:
		prev := m.tiQuery.Value()
		var cmd tea.Cmd
		m.tiQuery, cmd = m.tiQuery.Update(msg)
		if m.tiQuery.Value() != prev {
			m.cursor = 0
			m.rebuildFiltered()
		}
		return m, cmd
	}
	return m, nil
}

func (m model) updateError(_ tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return m, tea.Quit
}
