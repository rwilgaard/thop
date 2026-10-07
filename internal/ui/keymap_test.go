package ui

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/rwilgaard/thop/internal/config"
)

func TestBuildKeyMap_defaults(t *testing.T) {
	km := buildKeyMap(config.Config{})
	if got := km.Up.Keys(); !slices.Equal(got, []string{"up", "ctrl+k"}) {
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
	if got := km.Down.Keys(); !slices.Equal(got, []string{"down", "ctrl+j"}) {
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
	cfg := config.Config{Keymap: map[string][]string{"help": {"f1"}, "clone": {"f1"}}}
	err := ValidateKeymap(cfg)
	if err == nil {
		t.Fatal("ValidateKeymap = nil, want duplicate-key error (f1 bound to help and clone)")
	}
	if !strings.Contains(err.Error(), "f1") {
		t.Errorf("error %q does not name the duplicate key", err)
	}
}

func TestBuildKeyMap_userBindingTakesDefault(t *testing.T) {
	tests := []struct {
		name    string
		keymap  map[string][]string
		check   func(keyMap) (got []string, want []string)
		wantErr string
	}{
		{
			name:   "old newtmp config unbinds new project",
			keymap: map[string][]string{"newtmp": {"ctrl+n"}},
			check:  func(km keyMap) ([]string, []string) { return km.NewProject.Keys(), nil },
		},
		{
			name:   "one of several default keys is taken",
			keymap: map[string][]string{"help": {"esc"}},
			check:  func(km keyMap) ([]string, []string) { return km.Quit.Keys(), []string{"ctrl+c"} },
		},
		{
			name:    "last key of a required action",
			keymap:  map[string][]string{"clone": {"enter"}},
			wantErr: "enter has no key left",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Config{Keymap: tt.keymap}
			err := ValidateKeymap(cfg)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ValidateKeymap = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateKeymap = %v, want nil", err)
			}
			if got, want := tt.check(buildKeyMap(cfg)); !slices.Equal(got, want) {
				t.Errorf("keys = %v, want %v", got, want)
			}
		})
	}

	km := buildKeyMap(config.Config{Keymap: map[string][]string{"newtmp": {"ctrl+n"}}})
	for _, g := range buildHelpGroups(km) {
		for _, b := range g.keys {
			if b.Help().Desc == "New project" {
				t.Error("unbound action should not be listed in help")
			}
		}
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
	if got := caretLabel(km.Projects); got != "" {
		t.Errorf("caretLabel(Projects) = %q, want empty (unbound by default)", got)
	}
	if got := caretLabel(km.Close); got != "^Q" {
		t.Errorf("caretLabel(Close) = %q, want %q", got, "^Q")
	}
	if got := caretLabel(km.Repos); got != "ctrl-shift-x" {
		t.Errorf("caretLabel(Repos) = %q, want %q (caret form only for plain ctrl)", got, "ctrl-shift-x")
	}
}
