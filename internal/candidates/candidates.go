package candidates

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/rwilgaard/thop/internal/atomicfile"
	"github.com/rwilgaard/thop/internal/tmux"
)

// Candidate is a project directory or git repository openable as a tmux session.
type Candidate struct {
	AbsPath string
	Root    string // scan root this candidate belongs to (or parent dir for direct candidates)
	RelPath string // relative to Root, used for display and session-name lookup
	IsRepo  bool
	IsTmp   bool

	// Set by Resolve.
	Session  string // tmux session c opens in
	Collides bool   // RelPath alone doesn't tell c apart from another candidate
}

// Resolve assigns each candidate its tmux session name and flags the ones
// that look alike. A session is named after its directory; when two
// directories would get the same name, each gets "name@root" instead.
func Resolve(cs []Candidate) []Candidate {
	shown := map[string]int{}
	named := map[string]int{}
	for _, c := range cs {
		shown[c.RelPath]++
		if !nested(c) {
			named[tmux.Sessionize(c.RelPath)]++
		}
	}
	for i, c := range cs {
		// a nested candidate shares its parent's root, so both get the same
		// session name, and an ambiguous parent makes the child ambiguous too
		top, _, _ := strings.Cut(c.RelPath, "/")
		name := tmux.Sessionize(top)
		if named[name] > 1 {
			name += "@" + tmux.Sessionize(filepath.Base(c.Root))
		}
		cs[i].Session = name
		cs[i].Collides = shown[c.RelPath] > 1 || shown[top] > 1
	}
	return cs
}

// Load returns every candidate, scanned and tmp, with sessions resolved.
func Load(roots []string, tmpPath, cacheFile string) ([]Candidate, error) {
	static, err := LoadCandidates(roots, cacheFile)
	return Resolve(append(static, LoadTmp(tmpPath)...)), err
}

func MissingRoots(roots []string) []string {
	var missing []string
	for _, r := range roots {
		if fi, err := os.Stat(r); err != nil || !fi.IsDir() {
			missing = append(missing, r)
		}
	}
	return missing
}

func nested(c Candidate) bool {
	return strings.Contains(c.RelPath, "/")
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

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// childDirs returns path's entries that are directories; nil on read error.
func childDirs(path string) []os.DirEntry {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}
	dirs := entries[:0]
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e)
		}
	}
	return dirs
}

// cacheStale checks 2 levels below each root (matching rebuildCache scan depth).
func cacheStale(roots []string, cacheFile string) bool {
	info, err := os.Stat(cacheFile)
	if err != nil {
		return true
	}
	cacheTime := info.ModTime()
	newer := func(e os.DirEntry) bool {
		fi, err := e.Info()
		return err != nil || fi.ModTime().After(cacheTime)
	}

	for _, root := range roots {
		if di, err := os.Stat(root); err != nil || di.ModTime().After(cacheTime) {
			return true
		}
		for _, e := range childDirs(root) {
			if newer(e) {
				return true
			}
			if slices.ContainsFunc(childDirs(filepath.Join(root, e.Name())), newer) {
				return true
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

		for _, e := range childDirs(root) {
			name := e.Name()
			absPath := filepath.Join(root, name)
			isRepo := pathExists(filepath.Join(absPath, ".git"))
			cands = append(cands, Candidate{AbsPath: absPath, Root: root, RelPath: name, IsRepo: isRepo})
			if isRepo {
				continue
			}
			for _, s := range childDirs(absPath) {
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
	return atomicfile.Write(cacheFile, func(w io.Writer) error {
		_, _ = fmt.Fprintf(w, "#%s\n", strings.Join(roots, "\t"))
		for _, c := range cands {
			isRepo := "0"
			if c.IsRepo {
				isRepo = "1"
			}
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", c.AbsPath, c.Root, isRepo)
		}
		return nil
	})
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

// Target returns the tmux session c opens in; window is empty for flat candidates.
func Target(c Candidate) (session, window string) {
	parent, _, isNested := strings.Cut(c.RelPath, "/")
	session = c.Session
	if session == "" {
		session = tmux.Sessionize(parent)
	}
	if isNested {
		window = filepath.Base(c.AbsPath)
	}
	return session, window
}

func Active(c Candidate, ts tmux.State) bool {
	session, window := Target(c)
	if window != "" {
		return ts.Windows[session+"/"+window]
	}
	return ts.Sessions[session]
}

// Current reports whether opening c would land where the client already is.
func Current(c Candidate, ts tmux.State) bool {
	session, window := Target(c)
	if ts.Current == "" || session != ts.Current {
		return false
	}
	return window == "" || window == ts.CurrentWindow
}

func Previous(c Candidate, ts tmux.State) bool {
	session, window := Target(c)
	return ts.Last != "" && window == "" && session == ts.Last
}

func ValidName(s string) bool {
	return s != "" && s != "." && s == strings.TrimSpace(s) &&
		!strings.Contains(s, "/") && !strings.Contains(s, "..")
}

// Empty is allowed: the name is then generated.
func ValidTmpName(s string) bool {
	return s == "" || ValidName(s)
}

func AutoTmpName() string {
	return "tmp-" + time.Now().Format("20060102-150405")
}

func LoadTmp(tmpPath string) []Candidate {
	entries, err := os.ReadDir(tmpPath)
	if err != nil {
		return nil
	}
	var out []Candidate
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		out = append(out, Tmp(tmpPath, e.Name()))
	}
	return out
}

func Tmp(tmpPath, name string) Candidate {
	return Candidate{
		AbsPath: filepath.Join(tmpPath, name),
		Root:    tmpPath,
		RelPath: name,
		IsTmp:   true,
	}
}
