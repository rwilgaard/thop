package ui

import (
	"os"
	"path/filepath"
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/rwilgaard/thop/internal/candidates"
)

// projectRoots returns the scan roots a new project can go in: those that
// exist and aren't themselves a git repo.
func (m model) projectRoots() []baseItem {
	var out []baseItem
	for _, p := range m.paths {
		isRepo := slices.ContainsFunc(m.all, func(it baseItem) bool {
			return it.candidate.AbsPath == p && it.candidate.IsRepo
		})
		if isRepo || slices.Contains(m.missing, p) {
			continue
		}
		out = append(out, baseItem{candidate: candidates.Candidate{AbsPath: p, RelPath: tilde(p)}})
	}
	return out
}

func (m *model) rebuildNewProjFiltered() {
	m.newProj.filtered = m.filterScored(m.projectRoots(), m.newProj.tiRoot.Value())
	if m.newProj.cursor >= len(m.newProj.filtered) {
		m.newProj.cursor = 0
	}
}

func (m *model) openNewProject() tea.Cmd {
	if len(m.paths) == 0 && m.setup.add != nil {
		return m.openSetup()
	}
	roots := m.projectRoots()
	if len(roots) == 0 {
		return nil
	}
	m.tiQuery.Blur()
	m.newProj.picked = len(roots) > 1
	if !m.newProj.picked {
		return m.openNewProjName(roots[0].candidate.AbsPath)
	}
	m.newProj.cursor = 0
	m.newProj.tiRoot.SetValue("")
	m.rebuildNewProjFiltered()
	m.inputMode = modeNewProjRoot
	return m.newProj.tiRoot.Focus()
}

func (m *model) openNewProjName(root string) tea.Cmd {
	m.newProj.root, m.newProj.err = root, ""
	m.newProj.tiName.SetValue("")
	m.inputMode = modeNewProjName
	return m.newProj.tiName.Focus()
}

func (m model) updateNewProjRoot(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	n := len(m.newProj.filtered)
	if cur, ok := m.navCursor(msg, m.newProj.cursor, n); ok {
		m.newProj.cursor = cur
		return m, nil
	}
	switch {
	case msg.String() == "ctrl+c":
		return m, tea.Quit
	case msg.String() == "esc":
		m.newProj.tiRoot.Blur()
		m.inputMode = modeNormal
		return m, m.tiQuery.Focus()
	case key.Matches(msg, m.keys.Enter):
		if m.newProj.cursor >= n {
			return m, nil
		}
		m.newProj.tiRoot.Blur()
		return m, m.openNewProjName(m.newProj.filtered[m.newProj.cursor].base.candidate.AbsPath)
	default:
		return m.forwardInput(msg)
	}
}

func (m model) updateNewProjName(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "ctrl+c":
		return m, tea.Quit
	case msg.String() == "esc":
		m.newProj.tiName.Blur()
		if m.newProj.picked {
			m.inputMode = modeNewProjRoot
			return m, m.newProj.tiRoot.Focus()
		}
		m.inputMode = modeNormal
		return m, m.tiQuery.Focus()
	case key.Matches(msg, m.keys.Enter):
		name := m.newProj.tiName.Value()
		if name == "" {
			return m, nil
		}
		if !candidates.ValidName(name) {
			m.newProj.err = "Invalid name"
			return m, nil
		}
		dest := filepath.Join(m.newProj.root, name)
		if _, err := os.Stat(dest); err == nil {
			m.newProj.err = "Already exists"
			return m, nil
		}
		if err := os.Mkdir(dest, 0o755); err != nil {
			return m.showError(err.Error(), modeNewProjName), nil
		}
		return m, m.showNewProject(candidates.Candidate{AbsPath: dest, Root: m.newProj.root, RelPath: name})
	default:
		return m.forwardInput(msg)
	}
}

// showNewProject adds c to the picker and puts the cursor on it, resetting
// query and filter so the row is visible.
func (m *model) showNewProject(c candidates.Candidate) tea.Cmd {
	m.newProj.tiName.Blur()
	m.addCandidates(c)
	m.view = viewAll
	m.tiQuery.SetValue("")
	m.rebuildFiltered()
	m.cursor = max(0, slices.IndexFunc(m.filtered, func(it scoredItem) bool {
		return it.base.candidate.AbsPath == c.AbsPath
	}))
	m.inputMode = modeNormal
	return m.tiQuery.Focus()
}
