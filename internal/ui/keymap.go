package ui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
	"github.com/rwilgaard/thop/internal/config"
)

type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Enter    key.Binding
	Quit     key.Binding
	Help     key.Binding
	Clone    key.Binding
	NewTmp   key.Binding
	CleanTmp key.Binding
	All      key.Binding
	Projects key.Binding
	Repos    key.Binding
	Tmp      key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter, k.Quit},
		{k.Clone, k.NewTmp, k.CleanTmp},
		{k.All, k.Projects, k.Repos, k.Tmp},
	}
}

var keys = keyMap{
	Up:       key.NewBinding(key.WithKeys("up", "ctrl+k"), key.WithHelp("↑/ctrl-k", "move up")),
	Down:     key.NewBinding(key.WithKeys("down", "ctrl+j"), key.WithHelp("↓/ctrl-j", "move down")),
	Enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open selected")),
	Quit:     key.NewBinding(key.WithKeys("esc", "ctrl+c"), key.WithHelp("esc", "quit")),
	Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Clone:    key.NewBinding(key.WithKeys("ctrl+g"), key.WithHelp("ctrl-g", "clone repo")),
	NewTmp:   key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("ctrl-n", "new tmp project")),
	CleanTmp: key.NewBinding(key.WithKeys("ctrl+x"), key.WithHelp("ctrl-x", "delete tmp")),
	All:      key.NewBinding(key.WithKeys("ctrl+a"), key.WithHelp("ctrl-a", "show all")),
	Projects: key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("ctrl-p", "projects only")),
	Repos:    key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl-r", "repos only")),
	Tmp:      key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl-t", "tmp only")),
}

func newHelpModel(c config.Colors) help.Model {
	h := help.New()
	keyStyle := lipgloss.NewStyle().Bold(true)
	if c.HelpKeyColor != "" {
		keyStyle = keyStyle.Foreground(lipgloss.Color(c.HelpKeyColor))
	}
	descStyle := lipgloss.NewStyle().Faint(true)
	if c.HelpDescColor != "" {
		descStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(c.HelpDescColor))
	}
	h.Styles.FullKey = keyStyle
	h.Styles.ShortKey = keyStyle
	h.Styles.FullDesc = descStyle
	h.Styles.ShortDesc = descStyle
	return h
}
