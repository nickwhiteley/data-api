package output

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Writer abstracts the destination filesystem for output files.
type Writer interface {
	Create(path string) (io.WriteCloser, error)
}

// LocalWriter creates files on the local filesystem.
type LocalWriter struct{}

// Create ensures the parent directory exists then creates the file at path.
func (w *LocalWriter) Create(path string) (io.WriteCloser, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir %s: %w", dir, err)
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create output file %s: %w", path, err)
	}
	return f, nil
}
