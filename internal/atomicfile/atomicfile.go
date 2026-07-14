// Package atomicfile replaces files via temp file + rename so readers never
// see partial content.
package atomicfile

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
)

// Write replaces path with the content produced by write, staging it in a
// temp file in the same directory and renaming it into place. Buffered-write
// errors surface via the internal flush.
func Write(path string, write func(io.Writer) error) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	w := bufio.NewWriter(tmp)
	if err := write(w); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
