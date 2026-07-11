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
}

// byName maps config keymap names to their bindings. Every keyMap field must
// appear here — TestKeyMapByName_complete enforces it.
func (km *keyMap) byName() map[string]*key.Binding {
	return map[string]*key.Binding{
		"up": &km.Up, "down": &km.Down, "enter": &km.Enter, "quit": &km.Quit,
		"help": &km.Help, "clone": &km.Clone, "newtmp": &km.NewTmp,
		"cleantmp": &km.CleanTmp, "all": &km.All, "projects": &km.Projects,
		"repos": &km.Repos, "tmp": &km.Tmp,
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
		NewTmp:   key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("ctrl-n", "New tmp project")),
		CleanTmp: key.NewBinding(key.WithKeys("ctrl+x"), key.WithHelp("ctrl-x", "Delete tmp projects")),
		All:      key.NewBinding(key.WithKeys("ctrl+a"), key.WithHelp("ctrl-a", "Show all")),
		Projects: key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("ctrl-p", "Projects only")),
		Repos:    key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl-r", "Repos only")),
		Tmp:      key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl-t", "Tmp only")),
	}

	overrides := km.byName()
	for name, keyStrs := range cfg.Keymap {
		b, ok := overrides[name]
		if !ok || len(keyStrs) == 0 {
			continue
		}
		b.SetKeys(expandLegacyAliases(keyStrs)...)
		b.SetHelp(keyLabel(keyStrs), b.Help().Desc)
	}
	return km
}

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
// dispatch order would silently shadow one of them.
func ValidateKeymap(cfg config.Config) error {
	km := buildKeyMap(cfg)
	byName := km.byName()
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
	return []helpGroup{
		{"Navigate", []key.Binding{km.Up, km.Down, km.Enter, km.Quit}},
		{"Actions", []key.Binding{km.Clone, km.NewTmp, km.CleanTmp, km.Help}},
		{"Filters", []key.Binding{km.All, km.Projects, km.Repos, km.Tmp}},
	}
}
