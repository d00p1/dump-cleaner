package format

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/d00p1/filtrate-backups/pkg/archive"
)

const singleSQLFileName = "dump.sql"

type Processor interface {
	Extract(r io.Reader, workDir string) ([]string, error)
	Pack(workDir string, files []string, w io.WriteCloser) error
}

func Resolve(path string) (Processor, error) {
	lower := strings.ToLower(strings.TrimSpace(path))
	switch {
	case strings.HasSuffix(lower, ".tar.gz"):
		return tarGzProcessor{}, nil
	case strings.HasSuffix(lower, ".sql.gz"):
		return sqlGzProcessor{}, nil
	case strings.HasSuffix(lower, ".sql"):
		return sqlProcessor{}, nil
	default:
		return nil, fmt.Errorf("unsupported input/output format for %q", path)
	}
}

type tarGzProcessor struct{}

func (tarGzProcessor) Extract(r io.Reader, workDir string) ([]string, error) {
	gzReader, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("gzip reader error: %w", err)
	}
	defer gzReader.Close()

	if err := archive.Unpack(gzReader, workDir); err != nil {
		return nil, fmt.Errorf("unpack archive: %w", err)
	}
	return listFiles(workDir)
}

func (tarGzProcessor) Pack(workDir string, _ []string, w io.WriteCloser) error {
	gzWriter := gzip.NewWriter(w)
	defer gzWriter.Close()

	if err := archive.Pack(workDir, gzWriter); err != nil {
		return fmt.Errorf("pack directory: %w", err)
	}
	return nil
}

type sqlProcessor struct{}

func (sqlProcessor) Extract(r io.Reader, workDir string) ([]string, error) {
	path := filepath.Join(workDir, singleSQLFileName)
	if err := writeFileFromReader(path, r); err != nil {
		return nil, err
	}
	return []string{singleSQLFileName}, nil
}

func (sqlProcessor) Pack(workDir string, files []string, w io.WriteCloser) error {
	if len(files) != 1 {
		return fmt.Errorf("sql format expects exactly one file, got %d", len(files))
	}
	return copyFileToWriter(filepath.Join(workDir, files[0]), w)
}

type sqlGzProcessor struct{}

func (sqlGzProcessor) Extract(r io.Reader, workDir string) ([]string, error) {
	gzReader, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("gzip reader error: %w", err)
	}
	defer gzReader.Close()

	path := filepath.Join(workDir, singleSQLFileName)
	if err := writeFileFromReader(path, gzReader); err != nil {
		return nil, err
	}
	return []string{singleSQLFileName}, nil
}

func (sqlGzProcessor) Pack(workDir string, files []string, w io.WriteCloser) error {
	if len(files) != 1 {
		return fmt.Errorf("sql.gz format expects exactly one file, got %d", len(files))
	}
	gzWriter := gzip.NewWriter(w)
	defer gzWriter.Close()
	return copyFileToWriter(filepath.Join(workDir, files[0]), gzWriter)
}

func listFiles(workDir string) ([]string, error) {
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return nil, fmt.Errorf("read extracted files: %w", err)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		files = append(files, entry.Name())
	}
	return files, nil
}

func writeFileFromReader(path string, r io.Reader) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create extracted file: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("write extracted file: %w", err)
	}
	return nil
}

func copyFileToWriter(path string, w io.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open filtered file: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(w, f); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}
