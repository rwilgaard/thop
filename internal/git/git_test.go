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

func TestExpandShorthand(t *testing.T) {
	const https = "https://github.com/{repo}.git"
	tests := []struct {
		name, in, tmpl, want string
	}{
		{"shorthand", "rwilgaard/thop", https, "https://github.com/rwilgaard/thop.git"},
		{"shorthand with .git", "rwilgaard/thop.git", https, "https://github.com/rwilgaard/thop.git"},
		{"dots and dashes", "my-org/kustomize-sync.nvim", https, "https://github.com/my-org/kustomize-sync.nvim.git"},
		{"ssh template", "rwilgaard/thop", "git@github.com:{repo}.git", "git@github.com:rwilgaard/thop.git"},
		{"https url untouched", "https://gitlab.com/a/b.git", https, "https://gitlab.com/a/b.git"},
		{"ssh url untouched", "git@github.com:a/b.git", https, "git@github.com:a/b.git"},
		{"relative path untouched", "./a/b", https, "./a/b"},
		{"three segments untouched", "a/b/c", https, "a/b/c"},
		{"bare name untouched", "thop", https, "thop"},
		{"empty template", "rwilgaard/thop", "", "rwilgaard/thop"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExpandShorthand(tt.in, tt.tmpl); got != tt.want {
				t.Errorf("ExpandShorthand(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestExpandShorthand_localPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "vendor", "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	const tmpl = "https://github.com/{repo}.git"
	if got := ExpandShorthand("vendor/lib", tmpl); got != "vendor/lib" {
		t.Errorf("existing local path was expanded to %q", got)
	}
	if got := ExpandShorthand("vendor/other", tmpl); got != "https://github.com/vendor/other.git" {
		t.Errorf("ExpandShorthand = %q, want the expanded URL", got)
	}
}
