package config

import (
	"os"
	"path/filepath"
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
		if got := expandHome(tt.path, tt.home); got != tt.want {
			t.Errorf("expandHome(%q, %q) = %q, want %q", tt.path, tt.home, got, tt.want)
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
