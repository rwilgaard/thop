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
	"github.com/rwilgaard/thop/internal/git"
	"github.com/rwilgaard/thop/internal/tmux"
	"github.com/rwilgaard/thop/internal/ui"
)

var version = "dev"

func main() {
	var (
		switchOnly  = flag.Bool("s", false, "only show active sessions")
		popup       = flag.Bool("popup", false, "")
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
	cfg := config.Load(xdgConfig, home)

	if len(cfg.Paths) == 0 {
		fatalf("no paths configured — edit %s/thop/config.yaml", strings.TrimSuffix(xdgConfig, "/"))
	}

	// Guard against clone with wrong argument count
	if flag.NArg() >= 1 && flag.Arg(0) == "clone" && flag.NArg() != 2 {
		fatalf("usage: thop clone <url>")
	}

	// Subcommand: thop clone <url>
	if flag.NArg() == 2 && flag.Arg(0) == "clone" {
		url := flag.Arg(1)
		if os.Getenv("TMUX") != "" && !*popup {
			args := append([]string{"display-popup", "-E", "-w", "60%", "-h", "50%", os.Args[0], "--popup"}, os.Args[1:]...)
			// display-popup -E forwards the inner command's exit status to the outer
			// tmux process, so a non-zero ExitError here means the inner binary ran
			// and failed (e.g. handleSelection errored). Propagate that exit code
			// rather than relaunching the TUI. A non-ExitError means popup creation
			// itself failed (unsupported tmux version, etc.) — fall through instead.
			if err := exec.Command("tmux", args...).Run(); err != nil {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					os.Exit(exitErr.ExitCode())
				}
			} else {
				return
			}
		}
		static, loadErr := candidates.LoadCandidates(cfg.Paths, cacheFile)
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "thop: candidates: %v\n", loadErr)
		}
		dest, err := ui.RunDestPicker(static, cfg)
		if err != nil {
			fatalf("dest picker: %v", err)
		}
		if dest == "" {
			return
		}
		doClone(url, dest, frecencyFile)
		return
	}

	// Direct argument: skip UI entirely.
	if flag.NArg() == 1 {
		arg := flag.Arg(0)
		// Best-effort root detection for direct invocation: walk config paths.
		root := guessRoot(arg, cfg.Paths)
		if err := tmux.HandleSelection(arg, root); err != nil {
			fatalf("%v", err)
		}
		return
	}

	// Inside tmux and not already in a popup: self-launch as a tmux popup.
	if os.Getenv("TMUX") != "" && !*popup {
		args := append([]string{"display-popup", "-E", "-w", "60%", "-h", "50%", os.Args[0], "--popup"}, os.Args[1:]...)
		// display-popup -E forwards the inner command's exit status to the outer
		// tmux process, so a non-zero ExitError here means the inner binary ran
		// and failed (e.g. handleSelection errored). Propagate that exit code
		// rather than relaunching the TUI. A non-ExitError means popup creation
		// itself failed (unsupported tmux version, etc.) — fall through instead.
		if err := exec.Command("tmux", args...).Run(); err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				os.Exit(exitErr.ExitCode())
			}
		} else {
			return
		}
	}

	// Concurrent load: static candidate cache, live tmux state, frecency scores.
	var (
		static    []candidates.Candidate
		tmuxState tmux.TmuxState
		scores    map[string]float64
		wg        sync.WaitGroup
	)
	var (
		candidatesErr error
		frecencyErr   error
	)
	wg.Add(3)
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
	wg.Wait()

	if candidatesErr != nil {
		fmt.Fprintf(os.Stderr, "thop: candidates: %v\n", candidatesErr)
	}
	if frecencyErr != nil {
		fmt.Fprintf(os.Stderr, "thop: frecency: %v\n", frecencyErr)
	}

	result, err := ui.Run(static, scores, tmuxState, *switchOnly, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "thop:", err)
		return
	}

	switch {
	case result.Clone != nil:
		doClone(result.Clone.URL, result.Clone.Dest, frecencyFile)
	case result.Candidate.AbsPath != "":
		if err := frecency.Record(frecencyFile, result.Candidate.AbsPath); err != nil {
			fmt.Fprintln(os.Stderr, "frecency:", err)
		}
		if err := tmux.HandleSelection(result.Candidate.AbsPath, result.Candidate.Root); err != nil {
			fatalf("%v", err)
		}
	}
}

func doClone(url, destPath, frecencyFile string) {
	cloned, err := git.Clone(url, destPath)
	if err != nil {
		fatalf("clone: %v", err)
	}
	if err := frecency.Record(frecencyFile, cloned); err != nil {
		fmt.Fprintln(os.Stderr, "frecency:", err)
	}
	if err := tmux.HandleSelection(cloned, ""); err != nil {
		fatalf("%v", err)
	}
}

// guessRoot returns the Candidate.Root for a directly-specified absolute path.
// If arg is a direct child of a configured path, that path is the root.
// If arg itself is a configured path (direct candidate), its parent dir is the root.
// Returns empty string if no match; HandleSelection treats that as a nested repo.
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
