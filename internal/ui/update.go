package ui

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/rwilgaard/thop/internal/candidates"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.sizeDialogInputs()
		return m, nil
	case spinner.TickMsg:
		if m.inputMode == modeLoading || m.inputMode == modeCloning {
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
		if m.clone.cancel != nil {
			m.clone.cancel()
			m.clone.cancel = nil
		}
		if m.clone.cancelled {
			m.clone.cancelled = false
			m.result.Clone = nil
			m.inputMode = modeURLInput
			return m, m.clone.tiURL.Focus()
		}
		if msg.err != nil {
			return m.showError(msg.err.Error(), modeURLInput), nil
		}
		m.result.Clone.Cloned = msg.path
		if m.inTmux {
			m.loadingText = "Opening…"
			m.inputMode = modeLoading
			return m, tea.Batch(cmdRunSelection(msg.path, "", m.result.Clone.Session), m.spin.Tick)
		}
		return m, tea.Quit
	case tmpCreatedMsg:
		if msg.err != nil {
			return m.showError(msg.err.Error(), modeNameInput), nil
		}
		m.result.Tmp.Path = msg.path
		m.addCandidates(candidates.Tmp(m.tmpPath, m.result.Tmp.Name))
		m.result.Tmp.Session = m.sessionOf(msg.path)
		if m.inTmux {
			m.loadingText = "Opening…"
			return m, tea.Batch(cmdRunSelection(msg.path, m.tmpPath, m.result.Tmp.Session), m.spin.Tick)
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
		case modeConfirmClose:
			return m.updateConfirmClose(msg)
		case modeCloning:
			return m.updateCloning(msg)
		case modeHelp:
			return m.updateHelp(msg)
		case modeSetup:
			return m.updateSetup(msg)
		case modeNewProjRoot:
			return m.updateNewProjRoot(msg)
		case modeNewProjName:
			return m.updateNewProjName(msg)
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
	var changed bool
	switch m.inputMode {
	case modeNormal:
		if cmd, changed = updateInput(&m.tiQuery, msg); changed {
			m.cursor = 0
			m.rebuildFiltered()
		}
	case modeURLInput:
		cmd, _ = updateInput(&m.clone.tiURL, msg)
	case modeDestPicker:
		if cmd, changed = updateInput(&m.clone.tiDest, msg); changed {
			m.clone.destCursor = 0
			m.rebuildDestFiltered()
		}
	case modeCloneName:
		cmd, _ = updateInput(&m.clone.tiName, msg)
	case modeNameInput:
		if cmd, changed = updateInput(&m.tmp.tiName, msg); changed {
			m.tmp.conflict = false
		}
	case modeCleanTmp:
		if cmd, changed = updateInput(&m.clean.tiQuery, msg); changed {
			m.clean.cursor = 0
			m.rebuildCleanFiltered()
		}
	case modeNewProjRoot:
		if cmd, changed = updateInput(&m.newProj.tiRoot, msg); changed {
			m.newProj.cursor = 0
			m.rebuildNewProjFiltered()
		}
	case modeNewProjName:
		if cmd, changed = updateInput(&m.newProj.tiName, msg); changed {
			m.newProj.err = ""
		}
	case modeSetup:
		if cmd, changed = updateInput(&m.setup.tiPath, msg); changed {
			m.setup.err = ""
		}
	case modeConfirmClean, modeConfirmClose, modeLoading, modeCloning, modeHelp, modeError:
	}
	return m, cmd
}

// updateInput feeds msg to ti and reports whether its value changed.
func updateInput(ti *textinput.Model, msg tea.Msg) (tea.Cmd, bool) {
	prev := ti.Value()
	var cmd tea.Cmd
	*ti, cmd = ti.Update(msg)
	return cmd, ti.Value() != prev
}

// navCursor applies a cursor-movement key to a list of n rows: up/down wrap,
// page keys clamp. ok is false when msg is not a movement key.
func (m model) navCursor(msg tea.KeyPressMsg, cur, n int) (next int, ok bool) {
	switch {
	case key.Matches(msg, m.keys.Up):
		return moveCursor(cur, m.visualStep(-1), n), true
	case key.Matches(msg, m.keys.Down):
		return moveCursor(cur, m.visualStep(1), n), true
	case key.Matches(msg, m.keys.PageUp):
		return pageCursor(cur, m.pageStep(-1), n), true
	case key.Matches(msg, m.keys.PageDown):
		return pageCursor(cur, m.pageStep(1), n), true
	}
	return cur, false
}

// showError switches to the error banner; returnMode is restored on dismiss.
func (m model) showError(msg string, returnMode inputMode) model {
	m.errMsg = msg
	m.errReturnMode = returnMode
	m.inputMode = modeError
	return m
}

func (m model) updateHelp(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "ctrl+c":
		return m, tea.Quit
	case key.Matches(msg, m.keys.Quit) || key.Matches(msg, m.keys.Help):
		m.inputMode = modeNormal
		return m, m.tiQuery.Focus()
	}
	return m, nil
}

func (m model) updateNormal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if cur, ok := m.navCursor(msg, m.cursor, len(m.filtered)); ok {
		m.cursor = cur
		return m, nil
	}
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Enter):
		if len(m.filtered) == 0 {
			return m, nil
		}
		c := m.filtered[m.cursor].base.candidate
		m.result.Candidate = c
		if m.inTmux {
			m.loadingText = "Opening…"
			m.inputMode = modeLoading
			return m, tea.Batch(cmdRunSelection(c.AbsPath, c.Root, c.Session), m.spin.Tick)
		}
		return m, tea.Quit
	case key.Matches(msg, m.keys.NextFilter):
		m.cycleFilter(1)
	case key.Matches(msg, m.keys.PrevFilter):
		m.cycleFilter(-1)
	case key.Matches(msg, m.keys.Close):
		if m.cursor < len(m.filtered) && m.filtered[m.cursor].base.active {
			m.closeTarget = m.filtered[m.cursor].base
			m.tiQuery.Blur()
			m.inputMode = modeConfirmClose
		}
	case key.Matches(msg, m.keys.Clone):
		m.tiQuery.Blur()
		m.inputMode = modeURLInput
		m.clone.tiURL.SetValue("")
		return m, m.clone.tiURL.Focus()
	case key.Matches(msg, m.keys.NewProject):
		return m, m.openNewProject()
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
		m.inputMode = modeHelp
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

func (m *model) cycleFilter(dir int) {
	tabs := m.filterTabList()
	for i, t := range tabs {
		if t.mode == m.view {
			m.view = tabs[moveCursor(i, dir, len(tabs))].mode
			break
		}
	}
	m.cursor = 0
	m.rebuildFiltered()
}

func (m model) updateConfirmClose(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	m.inputMode = modeNormal
	if msg.Key().Text != "y" {
		return m, m.tiQuery.Focus()
	}
	session, window := candidates.Target(m.closeTarget.candidate)
	var err error
	if window != "" {
		err = m.killWindow(session, window)
	} else {
		err = m.killSession(m.ts, session)
	}
	m.refreshTmux()
	m.rebuildFiltered()
	if err != nil {
		return m.showError("close "+m.closeTarget.candidate.RelPath+": "+err.Error(), modeNormal), nil
	}
	return m, m.tiQuery.Focus()
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
	case modeNewProjName:
		return m, m.newProj.tiName.Focus()
	default:
		return m, m.tiQuery.Focus()
	}
}
