package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func (m *model) openSetup() tea.Cmd {
	m.setup.tiPath.SetValue("")
	m.setup.err = ""
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
		root, cs, err := m.setup.add(path)
		if err != nil {
			m.setup.err = err.Error()
			return m, nil
		}
		m.addCandidates(cs...)
		m.paths = append(m.paths, root)
		return m, m.closeSetup()
	default:
		return m.forwardInput(msg)
	}
}
