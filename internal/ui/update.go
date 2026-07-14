package ui

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		return m, nil
	case spinner.TickMsg:
		if m.inputMode == modeLoading {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
		return m, nil
	case selectionDoneMsg:
		if msg.err != nil {
			return m.showError(msg.err.Error(), modeNormal), nil
		}
		return m, tea.Quit
	case cloneDoneMsg:
		if msg.err != nil {
			return m.showError(msg.err.Error(), modeURLInput), nil
		}
		m.result.Clone.Cloned = msg.path
		if m.inTmux {
			m.loadingText = "Opening…"
			return m, tea.Batch(cmdRunSelection(msg.path, ""), m.spin.Tick)
		}
		return m, tea.Quit
	case tmpCreatedMsg:
		if msg.err != nil {
			return m.showError(msg.err.Error(), modeNameInput), nil
		}
		m.result.Tmp.Path = msg.path
		if m.inTmux {
			m.loadingText = "Opening…"
			return m, tea.Batch(cmdRunSelection(msg.path, m.tmpPath), m.spin.Tick)
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
	// Forward all other messages (paste, cursor blink, …) to the active textinput.
	return m.forwardInput(msg)
}

// forwardInput routes msg to the active mode's textinput and triggers the
// matching rebuild when its value changes. Shared by the per-mode key
// handlers and the catch-all message path.
func (m model) forwardInput(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		m.clone.tiURL, cmd = m.clone.tiURL.Update(msg)
	case modeDestPicker:
		prev := m.clone.tiDest.Value()
		m.clone.tiDest, cmd = m.clone.tiDest.Update(msg)
		if m.clone.tiDest.Value() != prev {
			m.clone.destCursor = 0
			m.rebuildDestFiltered()
		}
	case modeCloneName:
		m.clone.tiName, cmd = m.clone.tiName.Update(msg)
	case modeNameInput:
		prev := m.tmp.tiName.Value()
		m.tmp.tiName, cmd = m.tmp.tiName.Update(msg)
		if m.tmp.tiName.Value() != prev {
			m.tmp.conflict = false
		}
	case modeCleanTmp:
		prev := m.clean.tiQuery.Value()
		m.clean.tiQuery, cmd = m.clean.tiQuery.Update(msg)
		if m.clean.tiQuery.Value() != prev {
			m.clean.cursor = 0
			m.rebuildCleanFiltered()
		}
	case modeConfirmClean, modeLoading, modeError:
	}
	return m, cmd
}

// showError switches to the error banner; returnMode is restored on dismiss.
func (m model) showError(msg string, returnMode inputMode) model {
	m.errMsg = msg
	m.errReturnMode = returnMode
	m.inputMode = modeError
	return m
}

func (m model) updateNormal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		switch {
		case msg.String() == "ctrl+c":
			return m, tea.Quit
		case key.Matches(msg, m.keys.Quit) || key.Matches(msg, m.keys.Help):
			m.showHelp = false
			return m, m.tiQuery.Focus()
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Enter):
		if len(m.filtered) > 0 {
			c := m.filtered[m.cursor].base.candidate
			m.result.Candidate = c
			if m.inTmux {
				m.loadingText = "Opening…"
				m.inputMode = modeLoading
				return m, tea.Batch(cmdRunSelection(c.AbsPath, c.Root), m.spin.Tick)
			}
		}
		return m, tea.Quit
	case key.Matches(msg, m.keys.Up):
		m.cursor = moveCursor(m.cursor, m.visualStep(-1), len(m.filtered))
	case key.Matches(msg, m.keys.Down):
		m.cursor = moveCursor(m.cursor, m.visualStep(1), len(m.filtered))
	case key.Matches(msg, m.keys.Clone):
		m.tiQuery.Blur()
		m.inputMode = modeURLInput
		m.clone.tiURL.SetValue("")
		return m, m.clone.tiURL.Focus()
	case key.Matches(msg, m.keys.NewTmp):
		m.tiQuery.Blur()
		m.tmp.tiName.SetValue("")
		m.inputMode = modeNameInput
		return m, m.tmp.tiName.Focus()
	case key.Matches(msg, m.keys.CleanTmp):
		m.clean.cursor = 0
		m.clean.selected = make(map[string]bool)
		m.clean.tiQuery.SetValue("")
		m.rebuildCleanFiltered()
		m.tiQuery.Blur()
		m.inputMode = modeCleanTmp
		return m, m.clean.tiQuery.Focus()
	case key.Matches(msg, m.keys.Help):
		m.tiQuery.Blur()
		m.showHelp = true
	default:
		// View filters share their binding↔mode pairs with the status bar tabs.
		for _, t := range m.filterTabList() {
			if key.Matches(msg, t.binding) {
				m.view, m.cursor = t.mode, 0
				m.rebuildFiltered()
				return m, nil
			}
		}
		return m.forwardInput(msg)
	}
	return m, nil
}

func (m model) updateError(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	m.errMsg = ""
	m.inputMode = m.errReturnMode
	switch m.errReturnMode {
	case modeURLInput:
		return m, m.clone.tiURL.Focus()
	case modeNameInput:
		return m, m.tmp.tiName.Focus()
	default:
		return m, m.tiQuery.Focus()
	}
}
