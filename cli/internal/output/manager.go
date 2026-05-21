package output

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/nickwhiteley/data-api/cli/internal/format"
	"github.com/nickwhiteley/data-api/cli/internal/partition"
)

// OutputManager manages writing extracted rows to output files.
// It supports non-partitioned output (single file per table) and partitioned
// output via DATERUN (single subdirectory) or DATEMOD (one file per partition value).
type OutputManager struct {
	writer    Writer
	tableName string
	startedAt time.Time
	fmt       format.OutputFormat
	outputDir string
	partition *partition.Partition // nil = no partitioning

	header  []string
	handles map[string]*fileHandle // key = partDir ("" for non-partitioned)
	files   []string
}

// fileHandle holds an open output file with its buffered writer and formatter.
type fileHandle struct {
	file      io.WriteCloser
	buf       *bufio.Writer
	formatter format.Formatter
}

// New returns an OutputManager for the given table and format.
// p may be nil for non-partitioned output.
func New(w Writer, tableName string, startedAt time.Time, f format.OutputFormat, p *partition.Partition, outputDir string) *OutputManager {
	return &OutputManager{
		writer:    w,
		tableName: tableName,
		startedAt: startedAt,
		fmt:       f,
		outputDir: outputDir,
		partition: p,
		handles:   make(map[string]*fileHandle),
	}
}

// WriteHeader stores the column names to be written as the first row of each output file.
// It must be called before the first WriteRow.
func (m *OutputManager) WriteHeader(cols []string) error {
	m.header = cols
	return nil
}

// WriteRow writes a data row to the appropriate output file, creating it lazily on first use.
// modifiedAt is used for DATEMOD partitioning; ignored otherwise.
func (m *OutputManager) WriteRow(row []string, modifiedAt string) error {
	partDir := m.resolvePartDir(modifiedAt)
	h, ok := m.handles[partDir]
	if !ok {
		var err error
		h, err = m.openFile(partDir)
		if err != nil {
			return err
		}
		m.handles[partDir] = h
	}
	return h.formatter.WriteRow(row)
}

// Files returns the paths of all files written so far.
func (m *OutputManager) Files() []string {
	return m.files
}

// Close flushes and closes all open output files.
func (m *OutputManager) Close() error {
	var firstErr error
	for _, h := range m.handles {
		if err := h.formatter.Flush(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("flush %s: %w", m.tableName, err)
		}
		if err := h.buf.Flush(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("flush buffer %s: %w", m.tableName, err)
		}
		if err := h.file.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close file %s: %w", m.tableName, err)
		}
	}
	m.handles = make(map[string]*fileHandle)
	return firstErr
}

// resolvePartDir returns the partition subdirectory for the given modifiedAt value.
// Returns "" for non-partitioned mode, "unknown" for null/unparseable DATEMOD values.
func (m *OutputManager) resolvePartDir(modifiedAt string) string {
	if m.partition == nil {
		return ""
	}
	switch m.partition.Mode {
	case "DATERUN":
		return m.partition.Resolve(m.startedAt)
	case "DATEMOD":
		if modifiedAt == "" {
			return "unknown"
		}
		ts, err := partition.ParseTimestamp(modifiedAt)
		if err != nil {
			return "unknown"
		}
		return m.partition.Resolve(ts)
	}
	return ""
}

// openFile creates the output file, writes the header, and returns the open handle.
func (m *OutputManager) openFile(partDir string) (*fileHandle, error) {
	ext := format.FileExtension(m.fmt)

	ts := m.startedAt.Format("20060102-150405")
	name := fmt.Sprintf("%s-%s%s", m.tableName, ts, ext)
	var path string
	if partDir != "" {
		path = filepath.Join(m.outputDir, m.tableName, partDir, name)
	} else {
		path = filepath.Join(m.outputDir, name)
	}

	f, createErr := m.writer.Create(path)
	if createErr != nil {
		return nil, fmt.Errorf("create output file: %w", createErr)
	}

	buf := bufio.NewWriter(f)
	fmtr, fmtErr := format.NewFormatter(m.fmt, buf)
	if fmtErr != nil {
		_ = f.Close()
		return nil, fmt.Errorf("create formatter: %w", fmtErr)
	}

	if len(m.header) > 0 {
		if hdrErr := fmtr.WriteHeader(m.header); hdrErr != nil {
			_ = f.Close()
			return nil, fmt.Errorf("write header: %w", hdrErr)
		}
	}

	m.files = append(m.files, path)
	return &fileHandle{file: f, buf: buf, formatter: fmtr}, nil
}
