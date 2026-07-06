package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRepoNameFromURL(t *testing.T) {
	tests := []struct{ url, want string }{
		{"https://github.com/foo/bar", "bar"},
		{"https://github.com/foo/bar.git", "bar"},
		{"git@github.com:foo/baz.git", "baz"},
		{"git@github.com:foo/baz", "baz"},
	}
	for _, tt := range tests {
		if got := RepoNameFromURL(tt.url); got != tt.want {
			t.Errorf("RepoNameFromURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestClone(t *testing.T) {
	// Create a local bare repo as clone source.
	src := t.TempDir()
	if err := exec.Command("git", "init", "--bare", src).Run(); err != nil {
		t.Fatalf("git init --bare: %v", err)
	}
	destPath := filepath.Join(t.TempDir(), RepoNameFromURL(src))
	cloned, err := Clone(t.Context(), src, destPath)
	if err != nil {
		t.Fatalf("Clone() error: %v", err)
	}
	if _, err := os.Stat(cloned); err != nil {
		t.Errorf("cloned dir does not exist: %v", err)
	}
	if cloned != destPath {
		t.Errorf("Clone() = %q, want %q", cloned, destPath)
	}
}

func TestClone_conflictDetection(t *testing.T) {
	src := t.TempDir()
	if err := exec.Command("git", "init", "--bare", src).Run(); err != nil {
		t.Fatalf("git init --bare: %v", err)
	}
	destPath := filepath.Join(t.TempDir(), "myrepo")
	// Pre-create destination with a file so git refuses to clone into it.
	if err := os.MkdirAll(destPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destPath, "existing"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Clone(t.Context(), src, destPath)
	if err == nil {
		t.Error("expected error cloning into non-empty dir, got nil")
	}
}
