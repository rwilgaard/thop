package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestExpandHome(t *testing.T) {
	tests := []struct {
		path, home, want string
	}{
		{"~", "/home/user", "/home/user"},
		{"~/projects", "/home/user", "/home/user/projects"},
		{"~/a/b/c", "/home/user", "/home/user/a/b/c"},
		{"/absolute/path", "/home/user", "/absolute/path"},
		{"relative/path", "/home/user", "relative/path"},
	}
	for _, tt := range tests {
		if got := ExpandHome(tt.path, tt.home); got != tt.want {
			t.Errorf("ExpandHome(%q, %q) = %q, want %q", tt.path, tt.home, got, tt.want)
		}
	}
}

func TestLoad_missingConfig(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	cfg, err := Load(dir, t.TempDir(), home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// defaults returned
	if cfg.Colors.SelectionBg == "" {
		t.Error("expected default SelectionBg")
	}
	if cfg.Popup.Width != "60%" || cfg.Popup.Height != "50%" {
		t.Errorf("Popup = %+v, want default 60%%/50%%", cfg.Popup)
	}
	// example config file created
	if _, err := os.Stat(filepath.Join(dir, "thop", "config.yaml")); err != nil {
		t.Errorf("expected example config to be created: %v", err)
	}
}

func TestLoad_existingConfig(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	cfgDir := filepath.Join(dir, "thop")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "paths:\n  - ~/projects\n  - /absolute\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir, t.TempDir(), home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(cfg.Paths))
	}
	if cfg.Paths[0] != filepath.Join(home, "projects") {
		t.Errorf("expected expanded path, got %q", cfg.Paths[0])
	}
	if cfg.Paths[1] != "/absolute" {
		t.Errorf("expected unchanged absolute path, got %q", cfg.Paths[1])
	}
}

func TestLoad_invalidYAML(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	cfgDir := filepath.Join(dir, "thop")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte("paths: [unclosed"), 0o644); err != nil {
		t.Fatal(err)
	}
	// returns defaults plus a parse error the caller can warn about
	cfg, err := Load(dir, t.TempDir(), home)
	if err == nil {
		t.Error("expected parse error on invalid YAML")
	}
	if cfg.Colors.SelectionBg == "" {
		t.Error("expected defaults on invalid YAML")
	}
}

func TestLoad_tmpPathExplicit(t *testing.T) {
	dir := t.TempDir()
	cache := t.TempDir()
	home := t.TempDir()
	cfgDir := filepath.Join(dir, "thop")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "paths:\n  - ~/projects\ntmp_path: ~/scratch\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir, cache, home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(home, "scratch")
	if cfg.TmpPath != want {
		t.Errorf("TmpPath = %q, want %q", cfg.TmpPath, want)
	}
}

func TestLoad_layout(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "thop")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte("layout: \"bottom\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir, t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Layout != "bottom" {
		t.Errorf("Layout = %q, want %q", cfg.Layout, "bottom")
	}
}

func TestLoad_tmpPathDefault(t *testing.T) {
	dir := t.TempDir()
	cache := t.TempDir()
	home := t.TempDir()
	cfgDir := filepath.Join(dir, "thop")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte("paths:\n  - ~/projects\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir, cache, home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(cache, "thop", "tmp")
	if cfg.TmpPath != want {
		t.Errorf("TmpPath = %q, want %q", cfg.TmpPath, want)
	}
}

func TestLoad_popupOverride(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "thop")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "popup:\n  width: \"80%\"\n  height: \"70%\"\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir, t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Popup.Width != "80%" {
		t.Errorf("Popup.Width = %q, want %q", cfg.Popup.Width, "80%")
	}
	if cfg.Popup.Height != "70%" {
		t.Errorf("Popup.Height = %q, want %q", cfg.Popup.Height, "70%")
	}
}

func TestLoad_popupEmptyBackfilled(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "thop")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "popup:\n  width: \"\"\n  height: \"70%\"\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir, t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Popup.Width != "60%" {
		t.Errorf("Popup.Width = %q, want %q (empty falls back to default)", cfg.Popup.Width, "60%")
	}
	if cfg.Popup.Height != "70%" {
		t.Errorf("Popup.Height = %q, want %q", cfg.Popup.Height, "70%")
	}
}

func TestLoad_colorsEmptyBackfilled(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "thop")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Explicit empty overrides a non-empty default; must fall back.
	content := "colors:\n  status_active_color: \"\"\n  selection_bg: \"\"\n  match_color: \"\"\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir, t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Colors.StatusActiveColor != "11" {
		t.Errorf("StatusActiveColor = %q, want %q (empty falls back)", cfg.Colors.StatusActiveColor, "11")
	}
	if cfg.Colors.SelectionBg != "8" {
		t.Errorf("SelectionBg = %q, want %q (empty falls back)", cfg.Colors.SelectionBg, "8")
	}
	// match_color's default is empty, so "" stays "" (no forced fallback).
	if cfg.Colors.MatchColor != "" {
		t.Errorf("MatchColor = %q, want empty (its default is empty)", cfg.Colors.MatchColor)
	}
}

func TestLoad_keymapOverride(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "thop")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "keymap:\n  up: [\"k\"]\n  help: [\"h\"]\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir, t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cfg.Keymap["up"]; len(got) != 1 || got[0] != "k" {
		t.Errorf("Keymap[up] = %v, want [k]", got)
	}
	if got := cfg.Keymap["help"]; len(got) != 1 || got[0] != "h" {
		t.Errorf("Keymap[help] = %v, want [h]", got)
	}
}

func TestLoad_cloneShorthand(t *testing.T) {
	tests := []struct {
		name, content, want string
	}{
		{"absent keeps default", "layout: top\n", "https://github.com/{repo}.git"},
		{"empty backfilled", "clone_shorthand: \"\"\n", "https://github.com/{repo}.git"},
		{"override", "clone_shorthand: \"git@github.com:{repo}.git\"\n", "git@github.com:{repo}.git"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgDir := filepath.Join(dir, "thop")
			if err := os.MkdirAll(cfgDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(dir, t.TempDir(), t.TempDir())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.CloneShorthand != tt.want {
				t.Errorf("CloneShorthand = %q, want %q", cfg.CloneShorthand, tt.want)
			}
		})
	}
}

func TestAddPath(t *testing.T) {
	tests := []struct {
		name    string
		content *string // nil: file does not exist
		want    []string
		keeps   string // substring that must survive
	}{
		{"missing file", nil, []string{"/r/projects"}, ""},
		{"example config", new(exampleConfig), []string{"/r/projects"}, "# layout: \"bottom\""},
		{"empty flow list", new("layout: bottom\npaths: []\n"), []string{"/r/projects"}, "layout: bottom"},
		{"bare key with comment", new("paths: # roots\ntmp_path: /t\n"), []string{"/r/projects"}, "tmp_path: /t"},
		{"no paths key", new("layout: bottom"), []string{"/r/projects"}, "layout: bottom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			file := configFile(dir)
			if tt.content != nil {
				if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte(*tt.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := AddPath(file, "/r/projects"); err != nil {
				t.Fatalf("AddPath: %v", err)
			}
			cfg, err := Load(dir, t.TempDir(), "/home/u")
			if err != nil {
				t.Fatalf("Load after AddPath: %v", err)
			}
			if !slices.Equal(cfg.Paths, tt.want) {
				t.Errorf("Paths = %v, want %v", cfg.Paths, tt.want)
			}
			data, _ := os.ReadFile(file)
			if !strings.Contains(string(data), tt.keeps) {
				t.Errorf("lost %q from:\n%s", tt.keeps, data)
			}
		})
	}

	t.Run("tilde path expands on load", func(t *testing.T) {
		dir := t.TempDir()
		if err := AddPath(configFile(dir), "~/projects"); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(dir, t.TempDir(), "/home/u")
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(cfg.Paths, []string{"/home/u/projects"}) {
			t.Errorf("Paths = %v, want [/home/u/projects]", cfg.Paths)
		}
	})
}

func TestAddPath_existingEntries(t *testing.T) {
	dir := t.TempDir()
	file := configFile(dir)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	before := "paths: ~\n"
	if err := os.WriteFile(file, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AddPath(file, "/r/projects"); err == nil {
		t.Error("expected an error rather than a second paths key")
	}
	if data, _ := os.ReadFile(file); string(data) != before {
		t.Errorf("file changed:\n%s", data)
	}
}
