package candidates

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rwilgaard/thop/internal/tmux"
)

func TestActive(t *testing.T) {
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
			ts := tmux.State{Sessions: tt.sessions, Windows: tt.windows}
			if got := Active(tt.c, ts); got != tt.want {
				t.Errorf("Active() = %v, want %v", got, tt.want)
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

func TestValidTmpName(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", true}, // empty = auto-name
		{"scratch", true},
		{"foo.bar", true},
		{"a/b", false},
		{"..", false},
		{"../up", false},
	}
	for _, tt := range tests {
		if got := ValidTmpName(tt.in); got != tt.want {
			t.Errorf("ValidTmpName(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestAutoTmpName(t *testing.T) {
	name := AutoTmpName()
	if !strings.HasPrefix(name, "tmp-") || len(name) != len("tmp-20060102-150405") {
		t.Errorf("AutoTmpName() = %q, want tmp-YYYYMMDD-HHMMSS form", name)
	}
}

func TestLoadTmp(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"scratch-a", "scratch-b"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cands := LoadTmp(dir)
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

func TestLoadTmp_missingDir(t *testing.T) {
	cands := LoadTmp("/nonexistent/path/thop/tmp")
	if cands != nil {
		t.Errorf("expected nil for missing dir, got %v", cands)
	}
}

func TestCurrentPrevious(t *testing.T) {
	ts := tmux.State{Current: "group", CurrentWindow: "api", Last: "foo_bar"}
	tests := []struct {
		name              string
		c                 Candidate
		current, previous bool
	}{
		{"current session", Candidate{AbsPath: "/r/group", RelPath: "group"}, true, false},
		{"current window", Candidate{AbsPath: "/r/group/api", RelPath: "group/api"}, true, false},
		{"other window in current session", Candidate{AbsPath: "/r/group/web", RelPath: "group/web"}, false, false},
		{"last session", Candidate{AbsPath: "/r/foo.bar", RelPath: "foo.bar"}, false, true},
		{"window in last session", Candidate{AbsPath: "/r/foo.bar/x", RelPath: "foo.bar/x"}, false, false},
		{"unrelated", Candidate{AbsPath: "/r/other", RelPath: "other"}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Current(tt.c, ts); got != tt.current {
				t.Errorf("Current = %v, want %v", got, tt.current)
			}
			if got := Previous(tt.c, ts); got != tt.previous {
				t.Errorf("Previous = %v, want %v", got, tt.previous)
			}
		})
	}
	t.Run("outside tmux", func(t *testing.T) {
		c := Candidate{AbsPath: "/r/group", RelPath: "group"}
		if Current(c, tmux.State{}) || Previous(c, tmux.State{}) {
			t.Error("empty state should match nothing")
		}
	})
}

func TestValidName(t *testing.T) {
	tests := []struct {
		in        string
		name, tmp bool
	}{
		{"app", true, true},
		{"my.app-2", true, true},
		{"", false, true},
		{"a/b", false, false},
		{"..", false, false},
		{"a..b", false, false},
	}
	for _, tt := range tests {
		if got := ValidName(tt.in); got != tt.name {
			t.Errorf("ValidName(%q) = %v, want %v", tt.in, got, tt.name)
		}
		if got := ValidTmpName(tt.in); got != tt.tmp {
			t.Errorf("ValidTmpName(%q) = %v, want %v", tt.in, got, tt.tmp)
		}
	}
}

func TestResolve(t *testing.T) {
	cs := Resolve([]Candidate{
		{AbsPath: "/code/alpha", Root: "/code", RelPath: "alpha"},
		{AbsPath: "/work/alpha", Root: "/work", RelPath: "alpha"},
		{AbsPath: "/work/alpha/api", Root: "/work", RelPath: "alpha/api", IsRepo: true},
		{AbsPath: "/code/beta", Root: "/code", RelPath: "beta"},
		{AbsPath: "/code/beta/api", Root: "/code", RelPath: "beta/api", IsRepo: true},
		{AbsPath: "/code/foo.bar", Root: "/code", RelPath: "foo.bar"},
		{AbsPath: "/work/foo_bar", Root: "/work", RelPath: "foo_bar"},
		{AbsPath: "/cache/tmp/scratch", Root: "/cache/tmp", RelPath: "scratch", IsTmp: true},
		{AbsPath: "/code/scratch", Root: "/code", RelPath: "scratch"},
	})
	want := []struct {
		path, session string
		collides      bool
	}{
		{"/code/alpha", "alpha@code", true},
		{"/work/alpha", "alpha@work", true},
		{"/work/alpha/api", "alpha@work", true}, // parent is ambiguous, so the repo row is too
		{"/code/beta", "beta", false},
		{"/code/beta/api", "beta", false},
		{"/code/foo.bar", "foo_bar@code", false}, // same session name, different label
		{"/work/foo_bar", "foo_bar@work", false},
		{"/cache/tmp/scratch", "scratch@tmp", true},
		{"/code/scratch", "scratch@code", true},
	}
	for i, w := range want {
		c := cs[i]
		if c.AbsPath != w.path || c.Session != w.session || c.Collides != w.collides {
			t.Errorf("%s: session = %q collides = %v, want %q %v", c.AbsPath, c.Session, c.Collides, w.session, w.collides)
		}
	}

	t.Run("target and active use the resolved session", func(t *testing.T) {
		ts := tmux.State{
			Sessions: map[string]bool{"alpha@work": true},
			Windows:  map[string]bool{"alpha@work/api": true},
		}
		for _, c := range cs[:3] {
			if got, want := Active(c, ts), c.Root == "/work"; got != want {
				t.Errorf("Active(%s) = %v, want %v", c.AbsPath, got, want)
			}
		}
		if s, w := Target(cs[2]); s != "alpha@work" || w != "api" {
			t.Errorf("Target = %q/%q, want alpha@work/api", s, w)
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		again := Resolve(slices.Clone(cs))
		if !slices.Equal(again, cs) {
			t.Errorf("second Resolve changed the result")
		}
	})
}
