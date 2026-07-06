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
	"time"

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
		switchOnly  = flag.Bool("s", false, "only show active sessions")
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

	if len(cfg.Paths) == 0 {
		fatalf("no paths configured — edit %s/thop/config.yaml", strings.TrimSuffix(xdgConfig, "/"))
	}

	if flag.NArg() >= 1 && flag.Arg(0) == "clone" && flag.NArg() != 2 {
		fatalf("usage: thop clone <url>")
	}

	if flag.NArg() == 2 && flag.Arg(0) == "clone" {
		url := flag.Arg(1)
		if runInPopupIfNeeded(inTmux, *popup) {
			return
		}
		static, loadErr := candidates.LoadCandidates(cfg.Paths, cacheFile)
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "thop: candidates: %v\n", loadErr)
		}
		result, err := ui.RunDestPicker(append(static, candidates.LoadTmpCandidates(cfg.TmpPath)...), cfg, inTmux, url)
		if err != nil {
			fatalf("dest picker: %v", err)
		}
		openResult(result, cfg, frecencyFile, inTmux)
		return
	}

	if flag.NArg() >= 1 && flag.Arg(0) == "tmp" {
		name := ""
		if flag.NArg() >= 2 {
			name = flag.Arg(1)
		}
		doTmp(cfg.TmpPath, name, frecencyFile)
		return
	}

	if flag.NArg() == 1 {
		arg, err := filepath.Abs(flag.Arg(0))
		if err != nil {
			fatalf("resolve path: %v", err)
		}
		// Best-effort root detection for direct invocation: walk config paths.
		root := guessRoot(arg, cfg.Paths)
		if err := frecency.Record(frecencyFile, arg); err != nil {
			fmt.Fprintln(os.Stderr, "frecency:", err)
		}
		if err := tmux.HandleSelection(arg, root); err != nil {
			fatalf("%v", err)
		}
		return
	}

	if runInPopupIfNeeded(inTmux, *popup) {
		return
	}

	var (
		static    []candidates.Candidate
		tmpCands  []candidates.Candidate
		tmuxState tmux.TmuxState
		scores    map[string]float64
		wg        sync.WaitGroup
	)
	var (
		candidatesErr error
		frecencyErr   error
	)
	wg.Add(4)
	go func() {
		defer wg.Done()
		static, candidatesErr = candidates.LoadCandidates(cfg.Paths, cacheFile)
	}()
	go func() {
		defer wg.Done()
		tmuxState = tmux.LoadState()
	}()
	go func() {
		defer wg.Done()
		scores, frecencyErr = frecency.Load(frecencyFile)
		if scores == nil {
			scores = map[string]float64{}
		}
	}()
	go func() {
		defer wg.Done()
		tmpCands = candidates.LoadTmpCandidates(cfg.TmpPath)
	}()
	wg.Wait()

	if candidatesErr != nil {
		fmt.Fprintf(os.Stderr, "thop: candidates: %v\n", candidatesErr)
	}
	if frecencyErr != nil {
		fmt.Fprintf(os.Stderr, "thop: frecency: %v\n", frecencyErr)
	}

	result, err := ui.Run(append(static, tmpCands...), scores, tmuxState, *switchOnly, cfg, inTmux)
	if err != nil {
		fmt.Fprintln(os.Stderr, "thop:", err)
		return
	}

	openResult(result, cfg, frecencyFile, inTmux)
}

func openResult(result ui.Result, cfg config.Config, frecencyFile string, inTmux bool) {
	switch {
	case result.Clone != nil && result.Clone.Cloned != "":
		handleOpen(result.Clone.Cloned, "", frecencyFile, inTmux)
	case result.Tmp != nil && result.Tmp.Path != "":
		handleOpen(result.Tmp.Path, cfg.TmpPath, frecencyFile, inTmux)
	case result.Candidate.AbsPath != "":
		handleOpen(result.Candidate.AbsPath, result.Candidate.Root, frecencyFile, inTmux)
	}
}

// runInPopupIfNeeded re-execs the current command inside a tmux display-popup
// when running inside tmux without --popup. Returns true if the popup launched
// (caller should return). display-popup -E propagates the inner exit code, so a
// non-zero ExitError means the inner binary failed — exit with that code. A
// non-ExitError means popup creation failed (old tmux, etc.) — fall through.
func runInPopupIfNeeded(inTmux, popup bool) bool {
	if !inTmux || popup {
		return false
	}
	args := append([]string{"display-popup", "-E", "-w", "60%", "-h", "50%", os.Args[0], "--popup"}, os.Args[1:]...)
	if err := exec.Command("tmux", args...).Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		return false
	}
	return true
}

func handleOpen(path, root, frecencyFile string, inTmux bool) {
	if err := frecency.Record(frecencyFile, path); err != nil {
		fmt.Fprintln(os.Stderr, "frecency:", err)
	}
	if !inTmux {
		if err := tmux.HandleSelection(path, root); err != nil {
			fatalf("%v", err)
		}
	}
}

func doTmp(tmpPath, name, frecencyFile string) {
	if name != "" && (strings.Contains(name, "/") || strings.Contains(name, "..")) {
		fatalf("tmp name must not contain path separators or '..'")
	}
	if name == "" {
		name = "tmp-" + time.Now().Format("20060102-150405")
	}
	dest := filepath.Join(tmpPath, name)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		fatalf("create tmp: %v", err)
	}
	if err := frecency.Record(frecencyFile, dest); err != nil {
		fmt.Fprintln(os.Stderr, "frecency:", err)
	}
	if err := tmux.HandleSelection(dest, tmpPath); err != nil {
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
