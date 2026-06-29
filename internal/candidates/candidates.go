package candidates

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	colorReset   = "\033[0m"
	colorProject = "\033[34m"
	colorRepo    = "\033[32m"
	colorActive  = "\033[33m"
	iconProject  = "󰉋"
	iconRepo     = ""
)

// Candidate is a project directory or git repository openable as a tmux session.
type Candidate struct {
	AbsPath string
	Root    string // scan root this candidate belongs to (or parent dir for direct candidates)
	RelPath string // relative to Root, used for display and session-name lookup
	IsRepo  bool
}

// LoadCandidates returns candidates from cache, rebuilding if stale.
// roots is the list of expanded absolute paths from config.Paths.
// If a root itself has a .git dir it is added as a direct candidate rather than scanned.
func LoadCandidates(roots []string, cacheFile string) ([]Candidate, error) {
	if !cacheStale(roots, cacheFile) {
		if c, err := readCache(cacheFile, roots); err == nil {
			return c, nil
		}
	}
	return rebuildCache(roots, cacheFile)
}

func sessionize(name string) string {
	return strings.ReplaceAll(name, ".", "_")
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// cacheStale checks 2 levels below each root (matching rebuildCache scan depth).
func cacheStale(roots []string, cacheFile string) bool {
	info, err := os.Stat(cacheFile)
	if err != nil {
		return true
	}
	cacheTime := info.ModTime()

	for _, root := range roots {
		if di, err := os.Stat(root); err != nil || di.ModTime().After(cacheTime) {
			return true
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if fi, err := e.Info(); err != nil || fi.ModTime().After(cacheTime) {
				return true
			}
			subs, err := os.ReadDir(filepath.Join(root, e.Name()))
			if err != nil {
				continue
			}
			for _, s := range subs {
				if !s.IsDir() {
					continue
				}
				if fi, err := s.Info(); err != nil || fi.ModTime().After(cacheTime) {
					return true
				}
			}
		}
	}
	return false
}

// rebuildCache scans 2 levels below each root (matching cacheStale scan depth).
func rebuildCache(roots []string, cacheFile string) ([]Candidate, error) {
	var cands []Candidate

	for _, root := range roots {
		// Root is itself a git repo → direct candidate, don't scan inside it.
		if pathExists(filepath.Join(root, ".git")) {
			cands = append(cands, Candidate{
				AbsPath: root,
				Root:    filepath.Dir(root),
				RelPath: filepath.Base(root),
				IsRepo:  true,
			})
			continue
		}

		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			absPath := filepath.Join(root, name)
			isRepo := pathExists(filepath.Join(absPath, ".git"))
			cands = append(cands, Candidate{AbsPath: absPath, Root: root, RelPath: name, IsRepo: isRepo})
			if isRepo {
				continue
			}
			subs, err := os.ReadDir(absPath)
			if err != nil {
				continue
			}
			for _, s := range subs {
				if !s.IsDir() {
					continue
				}
				d2 := filepath.Join(absPath, s.Name())
				if pathExists(filepath.Join(d2, ".git")) {
					cands = append(cands, Candidate{
						AbsPath: d2,
						Root:    root,
						RelPath: name + "/" + s.Name(),
						IsRepo:  true,
					})
				}
			}
		}
	}

	_ = writeCache(cacheFile, roots, cands)
	return cands, nil
}

// Cache format: first line "#root1\troot2\t..." header, then AbsPath\tRoot\tIsRepo per entry.
// RelPath is derived via filepath.Rel at read time.
func writeCache(cacheFile string, roots []string, cands []Candidate) error {
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(cacheFile), "candidates-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	w := bufio.NewWriter(tmp)
	_, _ = fmt.Fprintf(w, "#%s\n", strings.Join(roots, "\t"))
	for _, c := range cands {
		isRepo := "0"
		if c.IsRepo {
			isRepo = "1"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", c.AbsPath, c.Root, isRepo)
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, cacheFile)
}

func readCache(cacheFile string, roots []string) ([]Candidate, error) {
	f, err := os.Open(cacheFile)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)

	if !sc.Scan() {
		return nil, fmt.Errorf("empty cache")
	}
	header := sc.Text()
	if !strings.HasPrefix(header, "#") {
		return nil, fmt.Errorf("missing header")
	}
	stored := strings.Split(header[1:], "\t")
	if len(stored) != len(roots) {
		return nil, fmt.Errorf("roots mismatch")
	}
	for i, r := range roots {
		if stored[i] != r {
			return nil, fmt.Errorf("roots mismatch")
		}
	}

	var out []Candidate
	for sc.Scan() {
		p := strings.SplitN(sc.Text(), "\t", 3)
		if len(p) != 3 {
			continue
		}
		absPath, root := p[0], p[1]
		relPath, err := filepath.Rel(root, absPath)
		if err != nil {
			continue
		}
		out = append(out, Candidate{
			AbsPath: absPath,
			Root:    root,
			RelPath: relPath,
			IsRepo:  p[2] == "1",
		})
	}
	return out, sc.Err()
}

// CandidateActive reports whether c corresponds to an open tmux session or window.
func CandidateActive(c Candidate, sessions, windows map[string]bool) bool {
	if strings.Contains(c.RelPath, "/") {
		parent := sessionize(strings.SplitN(c.RelPath, "/", 2)[0])
		window := filepath.Base(c.AbsPath)
		return windows[parent+"/"+window]
	}
	return sessions[sessionize(c.RelPath)]
}

// FormatDisplay returns an ANSI-colored display string for a candidate row.
func FormatDisplay(c Candidate, active bool) string {
	icon, color := iconProject, colorProject
	if c.IsRepo {
		icon, color = iconRepo, colorRepo
	}
	indicator := ""
	if active {
		indicator = colorActive + " ●" + colorReset
	}
	return color + icon + colorReset + " " + c.RelPath + indicator
}
