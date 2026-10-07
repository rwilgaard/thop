package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// startServer points the tmux commands of this package at a private server
// and returns the directory to create projects in. The server's socket path
// has a length limit, so it lives under /tmp rather than t.TempDir().
func startServer(t *testing.T) (root string, switched *[]string) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	dir, err := os.MkdirTemp("/tmp", "thop-")
	if err != nil {
		t.Fatal(err)
	}
	if dir, err = filepath.EvalSymlinks(dir); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX_TMPDIR", dir)
	t.Setenv("TMUX", "")
	t.Setenv("HOME", dir) // no user tmux.conf
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("SHELL", "/bin/sh")

	var got []string
	orig := switchTo
	switchTo = func(session string) error {
		got = append(got, session)
		return nil
	}
	t.Cleanup(func() {
		switchTo = orig
		_ = tmuxRun("kill-server")
		_ = os.RemoveAll(dir)
	})
	return filepath.Join(dir, "root"), &got
}

func mkdir(t *testing.T, parts ...string) string {
	t.Helper()
	p := filepath.Join(parts...)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func query(t *testing.T, args ...string) []string {
	t.Helper()
	out, err := tmuxOutput(args...)
	if err != nil {
		t.Fatalf("tmux %v: %v", args, err)
	}
	return strings.Fields(string(out))
}

func TestHandleSelection_integration(t *testing.T) {
	root, switched := startServer(t)
	proj := mkdir(t, root, "proj")
	api := mkdir(t, root, "group", "api")
	web := mkdir(t, root, "group", "web.nvim")
	dotted := mkdir(t, root, "foo.bar")
	twin := mkdir(t, root, "twin")

	open := func(path, session string) {
		t.Helper()
		if err := HandleSelection(path, root, session); err != nil {
			t.Fatalf("HandleSelection(%s): %v", path, err)
		}
	}
	windows := func(session string) []string {
		t.Helper()
		return query(t, "list-windows", "-t", exact(session), "-F", "#{window_name}")
	}

	t.Run("project gets a session rooted in its dir", func(t *testing.T) {
		open(proj, "")
		if got := query(t, "display-message", "-p", "-t", exact("proj")+":", "#{session_path}"); !slices.Equal(got, []string{proj}) {
			t.Errorf("session path = %v, want %s", got, proj)
		}
		if last := (*switched)[len(*switched)-1]; last != "proj" {
			t.Errorf("switched to %q, want proj", last)
		}
	})

	t.Run("repo opens as the only window of its project's session", func(t *testing.T) {
		open(api, "")
		if got := windows("group"); !slices.Equal(got, []string{"api"}) {
			t.Errorf("windows = %v, want [api]", got)
		}
	})

	t.Run("second repo adds a window, reopening adds none", func(t *testing.T) {
		open(web, "")
		open(web, "")
		open(api, "")
		if got := windows("group"); !slices.Equal(got, []string{"api", "web.nvim"}) {
			t.Errorf("windows = %v, want [api web.nvim]", got)
		}
	})

	t.Run("dots in a project name", func(t *testing.T) {
		open(dotted, "")
		open(dotted, "")
		if got := windows("foo_bar"); len(got) != 1 {
			t.Errorf("foo_bar windows = %v, want one", got)
		}
	})

	t.Run("explicit session name", func(t *testing.T) {
		open(twin, "twin@root")
		if !hasSession("twin@root") || hasSession("twin") {
			t.Error("want session twin@root and no plain twin")
		}
	})

	t.Run("state, then kill window and session", func(t *testing.T) {
		ts := LoadState()
		for _, s := range []string{"proj", "group", "foo_bar", "twin@root"} {
			if !ts.Sessions[s] {
				t.Errorf("LoadState misses session %q: %v", s, ts.Sessions)
			}
		}
		if !ts.Windows["group/api"] || !ts.Windows["group/web.nvim"] {
			t.Errorf("LoadState misses group windows: %v", ts.Windows)
		}

		if err := KillWindow("group", "web.nvim"); err != nil {
			t.Fatal(err)
		}
		if got := windows("group"); !slices.Equal(got, []string{"api"}) {
			t.Errorf("after KillWindow: windows = %v, want [api]", got)
		}
		if err := ts.KillSession("twin@root"); err != nil {
			t.Fatal(err)
		}
		if hasSession("twin@root") {
			t.Error("session still there after KillSession")
		}
	})
}
