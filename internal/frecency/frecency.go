package frecency

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rwilgaard/thop/internal/atomicfile"
)

type entry struct {
	visits int64
	lastTs int64
}

func recencyWeight(lastTs int64) float64 {
	age := time.Now().Unix() - lastTs
	switch {
	case age < 3600:
		return 4.0 // < 1 hour
	case age < 86400:
		return 2.0 // < 1 day
	case age < 604800:
		return 1.0 // < 1 week
	case age < 2592000:
		return 0.5 // < 30 days
	default:
		return 0.25
	}
}

// Load reads the history file and returns a frecency score per path.
// Score = visits * recencyWeight(lastAccess). Higher = more frequent and recent.
// The map is non-nil even on error, so callers can use it directly.
func Load(file string) (map[string]float64, error) {
	entries, err := readEntries(file)
	if err != nil {
		return map[string]float64{}, err
	}
	scores := make(map[string]float64, len(entries))
	for path, e := range entries {
		scores[path] = float64(e.visits) * recencyWeight(e.lastTs)
	}
	return scores, nil
}

// LoadTimes reads the history file and returns last-access unix timestamps per path.
// The map is non-nil even on error, so callers can use it directly.
func LoadTimes(file string) (map[string]int64, error) {
	entries, err := readEntries(file)
	if err != nil {
		return map[string]int64{}, err
	}
	times := make(map[string]int64, len(entries))
	for path, e := range entries {
		times[path] = e.lastTs
	}
	return times, nil
}

// Record increments the visit count and updates the last-access timestamp for relPath.
// A flock on a sidecar file serializes concurrent thop instances so no
// read-modify-write cycle loses updates.
func Record(file, relPath string) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	unlock, err := lock(file + ".lock")
	if err != nil {
		return err
	}
	defer unlock()
	entries, err := readEntries(file)
	if err != nil {
		return err
	}
	e := entries[relPath]
	e.visits++
	e.lastTs = time.Now().Unix()
	entries[relPath] = e
	return writeEntries(file, entries)
}

func lock(path string) (unlock func(), err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

func readEntries(file string) (map[string]entry, error) {
	f, err := os.Open(file)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]entry{}, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	entries := map[string]entry{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		p := strings.SplitN(sc.Text(), "\t", 3)
		if len(p) != 3 {
			continue
		}
		visits, err := strconv.ParseInt(p[1], 10, 64)
		if err != nil {
			continue
		}
		lastTs, err := strconv.ParseInt(p[2], 10, 64)
		if err != nil {
			continue
		}
		entries[p[0]] = entry{visits: visits, lastTs: lastTs}
	}
	return entries, sc.Err()
}

func writeEntries(file string, entries map[string]entry) error {
	return atomicfile.Write(file, func(w io.Writer) error {
		for path, e := range entries {
			_, _ = fmt.Fprintf(w, "%s\t%d\t%d\n", path, e.visits, e.lastTs)
		}
		return nil
	})
}
