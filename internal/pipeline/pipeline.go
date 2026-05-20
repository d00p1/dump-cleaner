package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/d00p1/filtrate-backups/internal/filter"
	"github.com/d00p1/filtrate-backups/internal/format"
	"github.com/d00p1/filtrate-backups/internal/storage"
)

type Options struct {
	InputPath    string
	OutputPath   string
	Rules        []filter.Rule
	TmpDir       string
	MaxLineBytes int
	Store        storage.Store
}

type Result struct {
	OutputPath    string
	TotalLines    int
	FilteredLines int
	Files         []FileResult
}

type FileResult struct {
	Name          string
	TotalLines    int
	FilteredLines int
}

func Run(ctx context.Context, opts Options) (Result, error) {
	tmpDir, err := os.MkdirTemp(opts.TmpDir, "cache-")
	if err != nil {
		return Result{}, fmt.Errorf("mkdir temp: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if opts.Store == nil {
		return Result{}, fmt.Errorf("storage store is required")
	}

	inputFile, err := opts.Store.Open(ctx, opts.InputPath)
	if err != nil {
		return Result{}, err
	}
	defer inputFile.Close()

	inputFormat, err := format.Resolve(opts.InputPath)
	if err != nil {
		return Result{}, err
	}
	inputFiles, err := inputFormat.Extract(inputFile, tmpDir)
	if err != nil {
		return Result{}, err
	}

	filteredDir := filepath.Join(tmpDir, "filtered")
	if err := os.MkdirAll(filteredDir, 0o750); err != nil {
		return Result{}, fmt.Errorf("create filtered dir: %w", err)
	}

	var totalLines, filteredLines int
	filteredFiles := make([]string, 0, len(inputFiles))
	fileResults := make([]FileResult, 0, len(inputFiles))
	for _, name := range inputFiles {
		srcPath := filepath.Join(tmpDir, name)
		dstPath := filepath.Join(filteredDir, name)

		// #nosec G304 -- paths are from extracted files in controlled temp directory
		srcFile, err := os.Open(srcPath)
		if err != nil {
			return Result{}, fmt.Errorf("open extracted file: %w", err)
		}

		// #nosec G304 -- path is in controlled temp directory
		dstFile, err := os.Create(dstPath)
		if err != nil {
			_ = srcFile.Close()
			return Result{}, fmt.Errorf("create filtered file: %w", err)
		}

		stats, err := filter.SQLFilter(srcFile, dstFile, opts.Rules, opts.MaxLineBytes)
		closeErr := srcFile.Close()
		closeErr2 := dstFile.Close()
		if err != nil {
			return Result{}, fmt.Errorf("filter %s: %w", name, err)
		}
		if closeErr != nil {
			return Result{}, fmt.Errorf("close src file %s: %w", name, closeErr)
		}
		if closeErr2 != nil {
			return Result{}, fmt.Errorf("close dst file %s: %w", name, closeErr2)
		}

		filteredFiles = append(filteredFiles, name)
		totalLines += stats.TotalLines
		filteredLines += stats.FilteredLines
		fileResults = append(fileResults, FileResult{
			Name:          name,
			TotalLines:    stats.TotalLines,
			FilteredLines: stats.FilteredLines,
		})
	}

	outputFormat, err := format.Resolve(opts.OutputPath)
	if err != nil {
		return Result{}, err
	}
	if err := packOutput(ctx, opts.Store, outputFormat, filteredDir, filteredFiles, opts.OutputPath); err != nil {
		return Result{}, err
	}

	return Result{OutputPath: opts.OutputPath, TotalLines: totalLines, FilteredLines: filteredLines, Files: fileResults}, nil
}

func packOutput(ctx context.Context, store storage.Store, outputFormat format.Processor, srcDir string, files []string, outputFile string) error {
	f, err := store.Create(ctx, outputFile)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := outputFormat.Pack(srcDir, files, f); err != nil {
		return err
	}
	return nil
}
