package ui

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
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

	Close      key.Binding
	PageUp     key.Binding
	PageDown   key.Binding
	NextFilter key.Binding
	PrevFilter key.Binding
	Open       key.Binding
	NewProject key.Binding
}

// byName maps config keymap names to their bindings. Every keyMap field must
// appear here — TestKeyMapByName_complete enforces it.
func (km *keyMap) byName() map[string]*key.Binding {
	return map[string]*key.Binding{
		"up": &km.Up, "down": &km.Down, "enter": &km.Enter, "quit": &km.Quit,
		"help": &km.Help, "clone": &km.Clone, "newtmp": &km.NewTmp,
		"cleantmp": &km.CleanTmp, "all": &km.All, "projects": &km.Projects,
		"repos": &km.Repos, "tmp": &km.Tmp, "open": &km.Open,
		"close": &km.Close, "pageup": &km.PageUp, "pagedown": &km.PageDown,
		"nextfilter": &km.NextFilter, "prevfilter": &km.PrevFilter,
		"newproject": &km.NewProject,
	}
}

// buildKeyMap returns the default keyMap with any bindings present in
// cfg.Keymap overwritten. Unset or unknown entries keep their defaults;
// overridden bindings get a regenerated help label so help and hints stay
// truthful after a remap.
func buildKeyMap(cfg config.Config) keyMap {
	km := keyMap{
		Up:       key.NewBinding(key.WithKeys("up", "ctrl+k"), key.WithHelp("↑/ctrl-k", "Move up")),
		Down:     key.NewBinding(key.WithKeys("down", "ctrl+j"), key.WithHelp("↓/ctrl-j", "Move down")),
		Enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "Open selected")),
		Quit:     key.NewBinding(key.WithKeys("esc", "ctrl+c"), key.WithHelp("esc", "Quit")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "Toggle help")),
		Clone:    key.NewBinding(key.WithKeys("ctrl+g"), key.WithHelp("ctrl-g", "Clone repository")),
		NewTmp:   key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl-t", "New tmp project")),
		CleanTmp: key.NewBinding(key.WithKeys("ctrl+x"), key.WithHelp("ctrl-x", "Delete tmp projects")),
		// Direct filter jumps have no default keys; tab cycles.
		All:      key.NewBinding(key.WithHelp("", "Show all")),
		Projects: key.NewBinding(key.WithHelp("", "Projects only")),
		Repos:    key.NewBinding(key.WithHelp("", "Repos only")),
		Tmp:      key.NewBinding(key.WithHelp("", "Tmp only")),
		Open:     key.NewBinding(key.WithHelp("", "Open only")),

		Close:      key.NewBinding(key.WithKeys("ctrl+q"), key.WithHelp("ctrl-q", "Close session")),
		PageUp:     key.NewBinding(key.WithKeys("pgup", "ctrl+u"), key.WithHelp("pgup/ctrl-u", "Page up")),
		PageDown:   key.NewBinding(key.WithKeys("pgdown", "ctrl+d"), key.WithHelp("pgdn/ctrl-d", "Page down")),
		NewProject: key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("ctrl-n", "New project")),
		NextFilter: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "Next filter")),
		PrevFilter: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift-tab", "Previous filter")),
	}

	bindings := km.byName()
	taken := map[string]bool{} // keys the user bound
	for name, keyStrs := range cfg.Keymap {
		b, ok := bindings[name]
		if !ok || len(keyStrs) == 0 {
			continue
		}
		b.SetKeys(expandLegacyAliases(keyStrs)...)
		b.SetHelp(keyLabel(keyStrs), b.Help().Desc)
		for _, k := range b.Keys() {
			taken[k] = true
		}
	}
	// A key the user bound wins over another action's default, so a config
	// written for older defaults keeps working when defaults move.
	for name, b := range bindings {
		if len(cfg.Keymap[name]) > 0 {
			continue
		}
		kept := slices.DeleteFunc(slices.Clone(b.Keys()), func(k string) bool { return taken[k] })
		if len(kept) < len(b.Keys()) {
			b.SetKeys(kept...)
			b.SetHelp(keyLabel(kept), b.Help().Desc)
		}
	}
	return km
}

// required names the actions the picker can't work without.
var required = []string{"up", "down", "enter", "quit"}

// legacyAliases maps control keys that legacy terminal encoding sends as the
// same byte as a named key — the event arrives as the named key, so a binding
// on the control spelling alone would never match.
var legacyAliases = map[string]string{
	"ctrl+i": "tab",   // 0x09
	"ctrl+m": "enter", // 0x0d
	"ctrl+[": "esc",   // 0x1b
}

// expandLegacyAliases appends the named-key equivalent of any aliased control
// key so bindings match under both legacy and enhanced keyboard encodings.
func expandLegacyAliases(keys []string) []string {
	out := slices.Clone(keys)
	for _, k := range keys {
		if alias, ok := legacyAliases[k]; ok && !slices.Contains(out, alias) {
			out = append(out, alias)
		}
	}
	return out
}

// ValidateKeymap rejects a cfg.Keymap that binds the same key to two actions —
// dispatch order would silently shadow one of them — or that takes the last
// key from a required action.
func ValidateKeymap(cfg config.Config) error {
	km := buildKeyMap(cfg)
	byName := km.byName()
	for _, name := range required {
		if len(byName[name].Keys()) == 0 {
			return fmt.Errorf("keymap: %s has no key left, bind one", name)
		}
	}
	// Sorted so a collision reports the two actions deterministically.
	seen := map[string]string{} // key string -> binding name
	for _, name := range slices.Sorted(maps.Keys(byName)) {
		for _, k := range byName[name].Keys() {
			if prev, dup := seen[k]; dup {
				return fmt.Errorf("keymap: %q bound to both %s and %s", k, prev, name)
			}
			seen[k] = name
		}
	}
	return nil
}

// keyLabel renders override keys as a help label: "↑/ctrl-k" style,
// matching the default labels.
func keyLabel(keys []string) string {
	labels := make([]string, len(keys))
	for i, k := range keys {
		switch k {
		case "up":
			labels[i] = "↑"
		case "down":
			labels[i] = "↓"
		case "left":
			labels[i] = "←"
		case "right":
			labels[i] = "→"
		default:
			labels[i] = strings.ReplaceAll(k, "+", "-")
		}
	}
	return strings.Join(labels, "/")
}

// caretLabel renders a binding's first key in status-bar form: "ctrl+a" → "^A".
func caretLabel(b key.Binding) string {
	keys := b.Keys()
	if len(keys) == 0 {
		return ""
	}
	if rest, ok := strings.CutPrefix(keys[0], "ctrl+"); ok && !strings.Contains(rest, "+") {
		return "^" + strings.ToUpper(rest)
	}
	return keyLabel(keys[:1])
}

type helpGroup struct {
	title string
	keys  []key.Binding
}

func buildHelpGroups(km keyMap) []helpGroup {
	filters := []key.Binding{km.NextFilter, km.PrevFilter, km.All, km.Projects, km.Repos, km.Tmp, km.Open}
	bound := func(bs ...key.Binding) []key.Binding {
		return slices.DeleteFunc(bs, func(b key.Binding) bool { return len(b.Keys()) == 0 })
	}
	return []helpGroup{
		{"Navigate", bound(km.Up, km.Down, km.PageUp, km.PageDown, km.Enter, km.Quit)},
		{"Actions", bound(km.Clone, km.NewProject, km.NewTmp, km.CleanTmp, km.Close, km.Help)},
		{"Filters", bound(filters...)},
	}
}
