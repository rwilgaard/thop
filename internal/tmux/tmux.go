package tmux

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// State holds the set of active session and window names.
type State struct {
	Sessions map[string]bool // session name → exists
	Windows  map[string]bool // "session/window" → exists
}

// LoadState queries tmux for all active sessions and windows.
func LoadState() State {
	ts := State{
		Sessions: map[string]bool{},
		Windows:  map[string]bool{},
	}
	out, err := tmuxOutput("list-windows", "-a", "-F", "#{session_name}/#{window_name}")
	if err != nil {
		return ts
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		entry := sc.Text()
		if entry == "" {
			continue
		}
		ts.Windows[entry] = true
		if session, _, ok := strings.Cut(entry, "/"); ok {
			ts.Sessions[session] = true
		}
	}
	return ts
}

// HandleSelection creates or switches to the appropriate tmux session for the given path.
// root is the Candidate.Root for the selected path: a direct child of root gets a flat
// session (no window), a nested path gets a window within its parent's session.
func HandleSelection(selected, root string) error {
	var projectDir string
	var isRepo bool

	if root != "" && filepath.Dir(selected) == root {
		projectDir = selected
		isRepo = false
	} else {
		projectDir = filepath.Dir(selected)
		isRepo = true
	}

	sessionName := Sessionize(filepath.Base(projectDir))

	if !hasSession(sessionName) {
		// Create session with projectDir as root so user-created windows inherit it.
		if err := newSession(sessionName, projectDir, ""); err != nil {
			return fmt.Errorf("new session: %w", err)
		}
		if isRepo {
			// kill the initial window so only the repo window remains.
			if err := newWindow(sessionName, selected); err != nil {
				return fmt.Errorf("open window: %w", err)
			}
			_ = tmuxRun("kill-window", "-t", exact(sessionName)+":^")
		}
		hydrate(sessionName, projectDir)
		return switchTo(sessionName)
	}

	if isRepo {
		if err := openRepoWindow(sessionName, selected); err != nil {
			return fmt.Errorf("open window: %w", err)
		}
	}

	return switchTo(sessionName)
}

// Sessionize maps a directory name to a valid tmux session name.
// Shared with candidate matching so active-session lookups stay in sync.
func Sessionize(name string) string {
	return strings.ReplaceAll(name, ".", "_")
}

// exact prefixes a session name with "=" so tmux matches it exactly
// instead of falling back to prefix matching.
func exact(session string) string {
	return "=" + session
}

// targetWindow returns a tmux target path.
// If the window name has a dot, we replace it with '?' to stop tmux
// from splitting the name as window.pane. We can't use '=' because it
// disables wildcards.
func targetWindow(session, window string) string {
	if strings.Contains(window, ".") {
		searchTarget := strings.ReplaceAll(window, ".", "?")
		return exact(session) + ":" + searchTarget
	}
	return exact(session) + ":=" + window
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func hasSession(name string) bool {
	return exec.Command("tmux", "has-session", "-t", exact(name)).Run() == nil
}

func newSession(session, startDir, windowName string) error {
	args := []string{"new-session", "-ds", session, "-c", startDir}
	if windowName != "" {
		args = append(args, "-n", windowName)
	}
	return tmuxRun(args...)
}

func openRepoWindow(session, repoPath string) error {
	windowName := filepath.Base(repoPath)
	if err := tmuxRun("select-window", "-t", targetWindow(session, windowName)); err != nil {
		return newWindow(session, repoPath)
	}
	return nil
}

// newWindow appends a window named after path's base dir at the end of session.
func newWindow(session, path string) error {
	return tmuxRun("new-window", "-a", "-t", exact(session)+":{end}", "-n", filepath.Base(path), "-c", path)
}

func hydrate(session, projectDir string) {
	home, _ := os.UserHomeDir()
	local := filepath.Join(projectDir, ".thop")
	global := filepath.Join(home, ".thop")

	var src string
	switch {
	case pathExists(local):
		src = local
	case pathExists(global):
		src = global
	}
	if src != "" {
		_ = tmuxRun("send-keys", "-t", exact(session), "source "+shellQuote(src), "Enter")
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func switchTo(session string) error {
	if os.Getenv("TMUX") == "" {
		cmd := exec.Command("tmux", "attach-session", "-t", exact(session))
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	return tmuxRun("switch-client", "-t", exact(session))
}

func tmuxRun(args ...string) error {
	return exec.Command("tmux", args...).Run()
}

func tmuxOutput(args ...string) ([]byte, error) {
	return exec.Command("tmux", args...).Output()
}
