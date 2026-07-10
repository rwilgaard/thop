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
