package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type localBackend struct{}

func (localBackend) Open(_ context.Context, loc Location) (io.ReadCloser, error) {
	file, err := os.Open(loc.Path)
	if err != nil {
		return nil, fmt.Errorf("open local file %q: %w", loc.Path, err)
	}
	return file, nil
}

func (localBackend) Create(_ context.Context, loc Location) (io.WriteCloser, error) {
	if err := os.MkdirAll(filepath.Dir(loc.Path), 0o755); err != nil {
		return nil, fmt.Errorf("create local output dir for %q: %w", loc.Path, err)
	}
	file, err := os.Create(loc.Path)
	if err != nil {
		return nil, fmt.Errorf("create local file %q: %w", loc.Path, err)
	}
	return file, nil
}
