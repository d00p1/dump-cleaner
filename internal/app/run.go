package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/d00p1/filtrate-backups/internal/config"
	"github.com/d00p1/filtrate-backups/internal/pipeline"
	"github.com/d00p1/filtrate-backups/internal/storage"
)

func Run(ctx context.Context, args []string) error {
	cfg, err := config.Load(args)
	if err != nil {
		return err
	}
	for _, warning := range cfg.Warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", warning)
	}
	store := storage.NewResolver(storage.Config{
		Endpoint:       cfg.S3Endpoint,
		Region:         cfg.S3Region,
		RequestTimeout: cfg.S3RequestTimeout,
		RetryMax:       cfg.S3RetryMax,
		AccessKey:      cfg.S3AccessKey,
		SecretKey:      cfg.S3SecretKey,
		SessionToken:   cfg.S3SessionToken,
		ForcePathStyle: cfg.S3ForcePathStyle,
		Insecure:       cfg.S3Insecure,
	})

	runOnce := func() error {
		startedAt := time.Now()
		result, err := pipeline.Run(ctx, pipeline.Options{
			InputPath:    cfg.Input,
			OutputPath:   cfg.Output,
			Rules:        cfg.CompiledRules,
			TmpDir:       cfg.TmpDir,
			MaxLineBytes: cfg.MaxLineBytes,
			Store:        store,
		})
		finishedAt := time.Now()
		report := makeRunReport(cfg, startedAt, finishedAt, result, err)
		if reportErr := writeRunReport(ctx, store, cfg.ReportFile, report); reportErr != nil {
			if err != nil {
				return fmt.Errorf("%w; %v", err, reportErr)
			}
			return reportErr
		}
		if err != nil {
			return err
		}

		fmt.Printf("✅ filtered lines: %d/%d\n", result.FilteredLines, result.TotalLines)
		fmt.Printf("✅ output: %s\n", result.OutputPath)
		return nil
	}

	if cfg.Mode == "once" {
		return runOnce()
	}

	ticker := time.NewTicker(cfg.ScheduleInterval)
	defer ticker.Stop()

	if err := runOnce(); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := runOnce(); err != nil {
				fmt.Printf("run failed: %v\n", err)
			}
		}
	}
}
