package ui

import (
	"charm.land/bubbles/v2/key"
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

var keys = keyMap{
	Up:       key.NewBinding(key.WithKeys("up", "ctrl+k"), key.WithHelp("↑/ctrl-k", "Move up")),
	Down:     key.NewBinding(key.WithKeys("down", "ctrl+j"), key.WithHelp("↓/ctrl-j", "Move down")),
	Enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "Open selected")),
	Quit:     key.NewBinding(key.WithKeys("esc", "ctrl+c"), key.WithHelp("esc", "Quit")),
	Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "Toggle help")),
	Clone:    key.NewBinding(key.WithKeys("ctrl+g"), key.WithHelp("ctrl-g", "Clone repository")),
	NewTmp:   key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("ctrl-n", "New tmp project")),
	CleanTmp: key.NewBinding(key.WithKeys("ctrl+x"), key.WithHelp("ctrl-x", "Delete tmp projects")),
	All:      key.NewBinding(key.WithKeys("ctrl+a"), key.WithHelp("ctrl-a", "Show all")),
	Projects: key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("ctrl-p", "Projects only")),
	Repos:    key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl-r", "Repos only")),
	Tmp:      key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl-t", "Tmp only")),
}

var helpGroups = []struct {
	title string
	keys  []key.Binding
}{
	{"Navigate", []key.Binding{keys.Up, keys.Down, keys.Enter, keys.Quit}},
	{"Actions", []key.Binding{keys.Clone, keys.NewTmp, keys.CleanTmp, keys.Help}},
	{"Filters", []key.Binding{keys.All, keys.Projects, keys.Repos, keys.Tmp}},
}
