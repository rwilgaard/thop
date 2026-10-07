package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rwilgaard/thop/internal/candidates"
	"github.com/rwilgaard/thop/internal/config"
	"github.com/rwilgaard/thop/internal/frecency"
	"github.com/rwilgaard/thop/internal/tmux"
	"github.com/rwilgaard/thop/internal/ui"
)

var version = "dev"

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: thop [flags] [path | clone <url> | tmp [name]]\n\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  thop                open the project picker\n")
		fmt.Fprintf(os.Stderr, "  thop <path>         open a path directly\n")
		fmt.Fprintf(os.Stderr, "  thop clone <url>    pick a destination and clone\n")
		fmt.Fprintf(os.Stderr, "  thop tmp [name]     create and open a tmp project\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	var (
		switchOnly  = flag.Bool("s", false, "start in the open-sessions filter")
		popup       = flag.Bool("popup", false, "internal: already running inside tmux popup")
		showVersion = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fatalf("home dir: %v", err)
	}

	xdgData := envOr("XDG_DATA_HOME", home+"/.local/share")
	xdgCache := envOr("XDG_CACHE_HOME", home+"/.cache")
	xdgConfig := envOr("XDG_CONFIG_HOME", home+"/.config")
	frecencyFile := xdgData + "/thop/history"
	cacheFile := xdgCache + "/thop/candidates"
	cfg, cfgErr := config.Load(xdgConfig, xdgCache, home)
	if cfgErr != nil {
		fmt.Fprintf(os.Stderr, "thop: config: %v\n", cfgErr)
	}
	inTmux := os.Getenv("TMUX") != ""

	// No setup dialog over a config that failed to parse: it would look
	// unconfigured and the dialog would edit the broken file.
	var addRoot ui.AddRoot
	if len(cfg.Paths) == 0 && cfgErr == nil {
		addRoot = firstRun(cfg.File, home, cacheFile)
	}

	// Only the TUI paths use the keymap, so validate there (not for direct
	// open or tmp). Skip in the popup child — the parent already validated
	// before re-exec, so this also avoids building the keymap twice.
	validateKeymap := func() {
		if *popup {
			return
		}
		if cfgErr != nil && len(cfg.Paths) == 0 {
			fatalf("no paths loaded, fix %s", cfg.File)
		}
		if err := ui.ValidateKeymap(cfg); err != nil {
			fatalf("config: %v", err)
		}
	}

	switch {
	case flag.Arg(0) == "clone":
		if flag.NArg() != 2 {
			fatalf("usage: thop clone <url>")
		}
		validateKeymap()
		if runInPopupIfNeeded(inTmux, *popup, cfg) {
			return
		}
		doClone(flag.Arg(1), cfg, cacheFile, frecencyFile, inTmux, addRoot)
		return
	case flag.Arg(0) == "tmp":
		doTmp(cfg, flag.Arg(1), cacheFile, frecencyFile)
		return
	case flag.NArg() == 1:
		arg, err := filepath.Abs(flag.Arg(0))
		if err != nil {
			fatalf("resolve path: %v", err)
		}
		root := guessRoot(arg, cfg.Paths)
		if err := frecency.Record(frecencyFile, arg); err != nil {
			fmt.Fprintln(os.Stderr, "frecency:", err)
		}
		if err := tmux.HandleSelection(arg, root, sessionFor(arg, loadAll(cfg, cacheFile))); err != nil {
			fatalf("%v", err)
		}
		return
	}

	validateKeymap()
	if runInPopupIfNeeded(inTmux, *popup, cfg) {
		return
	}

	var (
		all           []candidates.Candidate
		tmuxState     tmux.State
		scores        map[string]float64
		candidatesErr error
		frecencyErr   error
		wg            sync.WaitGroup
	)
	wg.Add(3)
	go func() {
		defer wg.Done()
		all, candidatesErr = candidates.Load(cfg.Paths, cfg.TmpPath, cacheFile)
	}()
	go func() {
		defer wg.Done()
		tmuxState = tmux.LoadState()
	}()
	go func() {
		defer wg.Done()
		scores, frecencyErr = frecency.Load(frecencyFile)
	}()
	wg.Wait()

	if candidatesErr != nil {
		fmt.Fprintf(os.Stderr, "thop: candidates: %v\n", candidatesErr)
	}
	if frecencyErr != nil {
		fmt.Fprintf(os.Stderr, "thop: frecency: %v\n", frecencyErr)
	}

	result, err := ui.Run(all, scores, tmuxState, *switchOnly, cfg, inTmux, addRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "thop:", err)
		return
	}

	openResult(result, cfg, frecencyFile, inTmux)
}

func openResult(result ui.Result, cfg config.Config, frecencyFile string, inTmux bool) {
	switch {
	case result.Clone != nil && result.Clone.Cloned != "":
		handleOpen(result.Clone.Cloned, "", result.Clone.Session, frecencyFile, inTmux)
	case result.Tmp != nil && result.Tmp.Path != "":
		handleOpen(result.Tmp.Path, cfg.TmpPath, result.Tmp.Session, frecencyFile, inTmux)
	case result.Candidate.AbsPath != "":
		handleOpen(result.Candidate.AbsPath, result.Candidate.Root, result.Candidate.Session, frecencyFile, inTmux)
	}
}

// runInPopupIfNeeded re-execs the current command inside a tmux display-popup
// when running inside tmux without --popup. Returns true if the popup launched
// (caller should return). display-popup -E propagates the inner exit code, so a
// non-zero ExitError means the inner binary failed — exit with that code. A
// non-ExitError means popup creation failed (old tmux, etc.) — fall through.
func runInPopupIfNeeded(inTmux, popup bool, cfg config.Config) bool {
	if !inTmux || popup {
		return false
	}
	args := append([]string{"display-popup", "-E", "-w", cfg.Popup.Width, "-h", cfg.Popup.Height, os.Args[0], "--popup"}, os.Args[1:]...)
	cmd := exec.Command("tmux", args...)
	cmd.Stderr = os.Stderr // surface tmux errors (e.g. a bad popup size)
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		return false
	}
	return true
}

func handleOpen(path, root, session, frecencyFile string, inTmux bool) {
	if err := frecency.Record(frecencyFile, path); err != nil {
		fmt.Fprintln(os.Stderr, "frecency:", err)
	}
	if !inTmux {
		if err := tmux.HandleSelection(path, root, session); err != nil {
			fatalf("%v", err)
		}
	}
}

func loadAll(cfg config.Config, cacheFile string) []candidates.Candidate {
	all, err := candidates.Load(cfg.Paths, cfg.TmpPath, cacheFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "thop: candidates: %v\n", err)
	}
	return all
}

// sessionFor returns the session a directly-opened path belongs to: its own
// if it is a known candidate, else its parent project's. Empty when unknown.
func sessionFor(path string, cs []candidates.Candidate) string {
	parent := filepath.Dir(path)
	for _, c := range cs {
		// a repo inside a project is a window, not a session of its own, so
		// only a top-level parent lends its session
		if c.AbsPath == path || (c.AbsPath == parent && !strings.Contains(c.RelPath, "/")) {
			return c.Session
		}
	}
	return ""
}

func doClone(url string, cfg config.Config, cacheFile, frecencyFile string, inTmux bool, addRoot ui.AddRoot) {
	result, err := ui.RunDestPicker(loadAll(cfg, cacheFile), cfg, inTmux, url, addRoot)
	if err != nil {
		fatalf("dest picker: %v", err)
	}
	openResult(result, cfg, frecencyFile, inTmux)
}

// firstRun returns the save action of the setup dialog shown when no scan
// roots are configured: it writes the root to the config file and scans it.
func firstRun(file, home, cacheFile string) ui.AddRoot {
	return func(input string) (string, []candidates.Candidate, error) {
		input = strings.TrimSpace(input)
		dir, err := filepath.Abs(config.ExpandHome(input, home))
		if err != nil {
			return "", nil, err
		}
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			return "", nil, errors.New("not a directory")
		}
		entry := dir
		if strings.HasPrefix(input, "~") {
			entry = input
		}
		if err := config.AddPath(file, entry); err != nil {
			return "", nil, err
		}
		cs, err := candidates.LoadCandidates([]string{dir}, cacheFile)
		return dir, cs, err
	}
}

func doTmp(cfg config.Config, name, cacheFile, frecencyFile string) {
	tmpPath := cfg.TmpPath
	if !candidates.ValidTmpName(name) {
		fatalf("tmp name must not contain path separators or '..'")
	}
	if name == "" {
		name = candidates.AutoTmpName()
	}
	dest := filepath.Join(tmpPath, name)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		fatalf("create tmp: %v", err)
	}
	if err := frecency.Record(frecencyFile, dest); err != nil {
		fmt.Fprintln(os.Stderr, "frecency:", err)
	}
	if err := tmux.HandleSelection(dest, tmpPath, sessionFor(dest, loadAll(cfg, cacheFile))); err != nil {
		fatalf("%v", err)
	}
}

// guessRoot returns the Candidate.Root for a directly-specified absolute path.
// If arg is a direct child of a configured path, that path is the root.
// If arg itself is a configured path (direct candidate), its parent dir is the root.
func guessRoot(arg string, paths []string) string {
	parent := filepath.Dir(arg)
	for _, p := range paths {
		if parent == p {
			return p
		}
	}
	for _, p := range paths {
		if arg == p {
			return filepath.Dir(p)
		}
	}
	return ""
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "thop: "+format+"\n", args...)
	os.Exit(1)
}
