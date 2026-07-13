package candidates

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/rwilgaard/thop/internal/config"
)

func TestSessionize(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"foo.bar", "foo_bar"},
		{"foo", "foo"},
		{"a.b.c", "a_b_c"},
		{"nodots", "nodots"},
	}
	for _, tt := range tests {
		if got := sessionize(tt.in); got != tt.want {
			t.Errorf("sessionize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCandidateActive(t *testing.T) {
	tests := []struct {
		name     string
		c        Candidate
		sessions map[string]bool
		windows  map[string]bool
		want     bool
	}{
		{
			name:     "flat session active",
			c:        Candidate{AbsPath: "/root/myproject", Root: "/root", RelPath: "myproject"},
			sessions: map[string]bool{"myproject": true},
			windows:  map[string]bool{},
			want:     true,
		},
		{
			name:     "flat session dot in name",
			c:        Candidate{AbsPath: "/root/foo.bar", Root: "/root", RelPath: "foo.bar"},
			sessions: map[string]bool{"foo_bar": true},
			windows:  map[string]bool{},
			want:     true,
		},
		{
			name:     "nested window active",
			c:        Candidate{AbsPath: "/root/myproject/repo", Root: "/root", RelPath: "myproject/repo"},
			sessions: map[string]bool{},
			windows:  map[string]bool{"myproject/repo": true},
			want:     true,
		},
		{
			name:     "inactive",
			c:        Candidate{AbsPath: "/root/foo", Root: "/root", RelPath: "foo"},
			sessions: map[string]bool{},
			windows:  map[string]bool{},
			want:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CandidateActive(tt.c, tt.sessions, tt.windows); got != tt.want {
				t.Errorf("CandidateActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIcon(t *testing.T) {
	ic := config.Icons{Project: "P", Repo: "R", Tmp: "T"}
	tests := []struct {
		name      string
		c         Candidate
		wantGlyph string
		wantColor color.Color
	}{
		{"project", Candidate{}, "P", lipgloss.Blue},
		{"repo", Candidate{IsRepo: true}, "R", lipgloss.Green},
		{"tmp", Candidate{IsTmp: true}, "T", lipgloss.Magenta},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			glyph, col := Icon(tt.c, ic)
			if glyph != tt.wantGlyph || col != tt.wantColor {
				t.Errorf("Icon() = %q,%v want %q,%v", glyph, col, tt.wantGlyph, tt.wantColor)
			}
		})
	}
}

func TestCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "projects")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	roots := []string{root}
	cands := []Candidate{
		{AbsPath: filepath.Join(root, "foo"), Root: root, RelPath: "foo", IsRepo: false},
		{AbsPath: filepath.Join(root, "bar"), Root: root, RelPath: "bar", IsRepo: true},
	}
	cacheFile := filepath.Join(dir, "cache.tsv")
	if err := writeCache(cacheFile, roots, cands); err != nil {
		t.Fatal(err)
	}
	got, err := readCache(cacheFile, roots)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(cands) {
		t.Fatalf("got %d candidates, want %d", len(got), len(cands))
	}
	for i, c := range cands {
		if got[i].AbsPath != c.AbsPath || got[i].IsRepo != c.IsRepo || got[i].RelPath != c.RelPath {
			t.Errorf("candidate %d: got %+v, want %+v", i, got[i], c)
		}
	}
}

func TestReadCache_rootsMismatch(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "cache.tsv")
	if err := writeCache(cacheFile, []string{"/root/one"}, nil); err != nil {
		t.Fatal(err)
	}
	_, err := readCache(cacheFile, []string{"/root/two"})
	if err == nil {
		t.Error("expected error on roots mismatch")
	}
}

func TestReadCache_emptyFile(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "cache.tsv")
	if err := os.WriteFile(cacheFile, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := readCache(cacheFile, []string{"/root"})
	if err == nil {
		t.Error("expected error on empty cache file")
	}
}

func TestLoadTmpCandidates(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"scratch-a", "scratch-b"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cands := LoadTmpCandidates(dir)
	if len(cands) != 2 {
		t.Fatalf("got %d candidates, want 2", len(cands))
	}
	for _, c := range cands {
		if !c.IsTmp {
			t.Errorf("candidate %q: IsTmp = false, want true", c.RelPath)
		}
		if c.IsRepo {
			t.Errorf("candidate %q: IsRepo = true, want false", c.RelPath)
		}
		if c.Root != dir {
			t.Errorf("candidate %q: Root = %q, want %q", c.RelPath, c.Root, dir)
		}
	}
}

func TestLoadTmpCandidates_missingDir(t *testing.T) {
	cands := LoadTmpCandidates("/nonexistent/path/thop/tmp")
	if cands != nil {
		t.Errorf("expected nil for missing dir, got %v", cands)
	}
}
