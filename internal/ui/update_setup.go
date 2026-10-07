package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func (m *model) openSetup(s Setup) tea.Cmd {
	m.setup = setupFlow{tiPath: newTextInput("~/projects"), file: s.File, add: s.Add}
	m.sizeDialogInputs()
	m.tiQuery.Blur()
	m.inputMode = modeSetup
	return m.setup.tiPath.Focus()
}

// closeSetup continues to whatever the dialog was in front of: the clone
// destination picker when a URL is waiting, the main picker otherwise.
func (m *model) closeSetup() tea.Cmd {
	m.setup.tiPath.Blur()
	if m.clone.tiURL.Value() != "" {
		return m.openDestPicker()
	}
	m.inputMode = modeNormal
	m.rebuildFiltered()
	return m.tiQuery.Focus()
}

func (m model) updateSetup(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "ctrl+c":
		return m, tea.Quit
	case msg.String() == "esc":
		return m, m.closeSetup()
	case key.Matches(msg, m.keys.Enter):
		path := m.setup.tiPath.Value()
		if path == "" {
			return m, nil
		}
		cs, err := m.setup.add(path)
		if err != nil {
			m.setup.err = err.Error()
			return m, nil
		}
		roots := make([]baseItem, 0, len(cs)+len(m.all))
		for _, c := range cs {
			roots = append(roots, makeBaseItem(c, m.ts))
		}
		m.all = append(roots, m.all...)
		m.setup.file = ""
		return m, m.closeSetup()
	default:
		return m.forwardInput(msg)
	}
}
