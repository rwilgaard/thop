package ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/rwilgaard/thop/internal/config"
)

func TestBuildKeyMap_defaults(t *testing.T) {
	km := buildKeyMap(config.Config{})
	if got := km.Up.Keys(); len(got) != 2 || got[0] != "up" || got[1] != "ctrl+k" {
		t.Errorf("Up.Keys() = %v, want [up ctrl+k]", got)
	}
	if got := km.Help.Keys(); len(got) != 1 || got[0] != "?" {
		t.Errorf("Help.Keys() = %v, want [?]", got)
	}
}

func TestBuildKeyMap_override(t *testing.T) {
	cfg := config.Config{Keymap: map[string][]string{
		"up":   {"k"},
		"help": {"h"},
	}}
	km := buildKeyMap(cfg)
	if got := km.Up.Keys(); len(got) != 1 || got[0] != "k" {
		t.Errorf("Up.Keys() = %v, want [k]", got)
	}
	if got := km.Up.Help().Key; got != "k" {
		t.Errorf("Up.Help().Key = %q, want %q (label regenerated on override)", got, "k")
	}
	if got := km.Up.Help().Desc; got != "Move up" {
		t.Errorf("Up.Help().Desc = %q, want %q (description stays default on override)", got, "Move up")
	}
	if got := km.Help.Keys(); len(got) != 1 || got[0] != "h" {
		t.Errorf("Help.Keys() = %v, want [h]", got)
	}
	// unrelated binding unaffected
	if got := km.Down.Keys(); len(got) != 2 || got[0] != "down" || got[1] != "ctrl+j" {
		t.Errorf("Down.Keys() = %v, want [down ctrl+j] (unaffected by unrelated override)", got)
	}
	if got := km.Down.Help().Key; got != "↓/ctrl-j" {
		t.Errorf("Down.Help().Key = %q, want %q", got, "↓/ctrl-j")
	}
}

func TestBuildKeyMap_unknownAndEmptyIgnored(t *testing.T) {
	cfg := config.Config{Keymap: map[string][]string{
		"nonsense": {"x"},
		"quit":     {},
	}}
	km := buildKeyMap(cfg)
	if got := km.Quit.Keys(); len(got) != 2 || got[0] != "esc" || got[1] != "ctrl+c" {
		t.Errorf("Quit.Keys() = %v, want [esc ctrl+c] (empty override list ignored)", got)
	}
}

func TestBuildKeyMap_legacyAliases(t *testing.T) {
	cfg := config.Config{Keymap: map[string][]string{"clone": {"ctrl+i"}}}
	km := buildKeyMap(cfg)
	got := km.Clone.Keys()
	if len(got) != 2 || got[0] != "ctrl+i" || got[1] != "tab" {
		t.Errorf("Clone.Keys() = %v, want [ctrl+i tab] (legacy encoding sends ctrl+i as tab)", got)
	}
	if label := km.Clone.Help().Key; label != "ctrl-i" {
		t.Errorf("Clone.Help().Key = %q, want %q (label keeps user's spelling)", label, "ctrl-i")
	}
}

func TestValidateKeymap_legacyAliasCollision(t *testing.T) {
	// ctrl+m is enter on the wire — binding it must collide with enter.
	cfg := config.Config{Keymap: map[string][]string{"clone": {"ctrl+m"}}}
	if err := ValidateKeymap(cfg); err == nil {
		t.Error("ValidateKeymap = nil, want collision error (ctrl+m aliases enter)")
	}
}

func TestKeyMapByName_complete(t *testing.T) {
	var km keyMap
	if got, want := len(km.byName()), reflect.TypeFor[keyMap]().NumField(); got != want {
		t.Errorf("byName() has %d entries, keyMap has %d fields — new bindings must be added to byName", got, want)
	}
}

func TestValidateKeymap(t *testing.T) {
	if err := ValidateKeymap(config.Config{}); err != nil {
		t.Errorf("ValidateKeymap(defaults) = %v, want nil", err)
	}
	cfg := config.Config{Keymap: map[string][]string{"help": {"esc"}}}
	err := ValidateKeymap(cfg)
	if err == nil {
		t.Fatal("ValidateKeymap = nil, want duplicate-key error (esc bound to quit and help)")
	}
	if !strings.Contains(err.Error(), "esc") {
		t.Errorf("error %q does not name the duplicate key", err)
	}
}

func TestKeyLabel(t *testing.T) {
	if got := keyLabel([]string{"up", "ctrl+k"}); got != "↑/ctrl-k" {
		t.Errorf("keyLabel = %q, want %q", got, "↑/ctrl-k")
	}
	if got := keyLabel([]string{"ctrl+shift+x"}); got != "ctrl-shift-x" {
		t.Errorf("keyLabel = %q, want %q", got, "ctrl-shift-x")
	}
}

func TestCaretLabel(t *testing.T) {
	km := buildKeyMap(config.Config{Keymap: map[string][]string{
		"all":   {"ctrl+b"},
		"repos": {"ctrl+shift+x"},
	}})
	if got := caretLabel(km.All); got != "^B" {
		t.Errorf("caretLabel(All) = %q, want %q", got, "^B")
	}
	if got := caretLabel(km.Projects); got != "^P" {
		t.Errorf("caretLabel(Projects) = %q, want %q", got, "^P")
	}
	if got := caretLabel(km.Repos); got != "ctrl-shift-x" {
		t.Errorf("caretLabel(Repos) = %q, want %q (caret form only for plain ctrl)", got, "ctrl-shift-x")
	}
}

func TestBuildHelpGroups(t *testing.T) {
	km := buildKeyMap(config.Config{})
	groups := buildHelpGroups(km)
	if len(groups) != 3 {
		t.Fatalf("len(groups) = %d, want 3", len(groups))
	}
	if groups[0].title != "Navigate" {
		t.Errorf("groups[0].title = %q, want %q", groups[0].title, "Navigate")
	}
}
