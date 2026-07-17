package frecency

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRecencyWeight(t *testing.T) {
	now := time.Now().Unix()
	tests := []struct {
		name   string
		lastTs int64
		want   float64
	}{
		{"under 1 hour", now - 1800, 4.0},
		{"under 1 day", now - 7200, 2.0},
		{"under 1 week", now - 172800, 1.0},
		{"under 30 days", now - 1209600, 0.5},
		{"over 30 days", now - 2592001, 0.25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := recencyWeight(tt.lastTs); got != tt.want {
				t.Errorf("recencyWeight() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoad_missingFile(t *testing.T) {
	scores, err := Load(filepath.Join(t.TempDir(), "nonexistent.tsv"))
	if err != nil {
		t.Fatalf("Load missing file returned error: %v", err)
	}
	if len(scores) != 0 {
		t.Errorf("expected empty map, got %v", scores)
	}
}

func TestLoad_existing(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "history.tsv")
	now := time.Now().Unix()
	content := fmt.Sprintf("foo/bar\t2\t%d\n", now-60) // 2 visits, < 1 hour ago
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	scores, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := scores["foo/bar"]
	if !ok {
		t.Fatal("expected entry for foo/bar")
	}
	want := 2.0 * 4.0 // visits * weight(<1h)
	if got != want {
		t.Errorf("score = %v, want %v", got, want)
	}
}

func TestLoad_skipsInvalidLines(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "history.tsv")
	now := time.Now().Unix()
	content := fmt.Sprintf("bad line\nfoo/bar\t1\t%d\n", now-60)
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	scores, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := scores["foo/bar"]; !ok {
		t.Error("expected valid entry to be loaded")
	}
}

func TestRecord_incrementsVisits(t *testing.T) {
	file := filepath.Join(t.TempDir(), "history.tsv")
	for range 3 {
		if err := Record(file, "foo/bar"); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := readEntries(file)
	if err != nil {
		t.Fatal(err)
	}
	if entries["foo/bar"].visits != 3 {
		t.Errorf("visits = %d, want 3", entries["foo/bar"].visits)
	}
}

func TestRecord_concurrent(t *testing.T) {
	file := filepath.Join(t.TempDir(), "history.tsv")
	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Go(func() {
			errs <- Record(file, "foo/bar")
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	entries, err := readEntries(file)
	if err != nil {
		t.Fatal(err)
	}
	if entries["foo/bar"].visits != n {
		t.Errorf("visits = %d, want %d", entries["foo/bar"].visits, n)
	}
}

func TestLoadTimes(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "history")
	// path \t visits \t lastTs
	data := "/a/repo\t3\t1000\n/b/proj\t1\t2000\n"
	if err := os.WriteFile(file, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	times, err := LoadTimes(file)
	if err != nil {
		t.Fatalf("LoadTimes: %v", err)
	}
	if times["/a/repo"] != 1000 || times["/b/proj"] != 2000 {
		t.Fatalf("got %v", times)
	}
}

func TestLoadTimesMissingFile(t *testing.T) {
	times, err := LoadTimes(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("want nil err, got %v", err)
	}
	if times == nil || len(times) != 0 {
		t.Fatalf("want empty non-nil map, got %v", times)
	}
}
