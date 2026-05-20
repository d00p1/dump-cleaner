package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/d00p1/filtrate-backups/internal/config"
	"github.com/d00p1/filtrate-backups/internal/pipeline"
	"github.com/d00p1/filtrate-backups/internal/storage"
)

type RunReport struct {
	Status         string          `json:"status"`
	Mode           string          `json:"mode"`
	Input          string          `json:"input"`
	Output         string          `json:"output"`
	ReportFile     string          `json:"reportFile,omitempty"`
	DBDriver       string          `json:"dbDriver"`
	Warnings       []string        `json:"warnings,omitempty"`
	StartedAt      string          `json:"startedAt"`
	FinishedAt     string          `json:"finishedAt"`
	DurationMillis int64           `json:"durationMillis"`
	TotalLines     int             `json:"totalLines,omitempty"`
	FilteredLines  int             `json:"filteredLines,omitempty"`
	OutputPath     string          `json:"outputPath,omitempty"`
	Files          []RunReportFile `json:"files,omitempty"`
	Error          string          `json:"error,omitempty"`
}

type RunReportFile struct {
	Name          string `json:"name"`
	TotalLines    int    `json:"totalLines"`
	FilteredLines int    `json:"filteredLines"`
}

func makeRunReport(cfg config.Config, startedAt, finishedAt time.Time, result pipeline.Result, runErr error) RunReport {
	report := RunReport{
		Status:         "ok",
		Mode:           cfg.Mode,
		Input:          cfg.Input,
		Output:         cfg.Output,
		ReportFile:     cfg.ReportFile,
		DBDriver:       cfg.DBDriver,
		Warnings:       append([]string(nil), cfg.Warnings...),
		StartedAt:      startedAt.UTC().Format(time.RFC3339Nano),
		FinishedAt:     finishedAt.UTC().Format(time.RFC3339Nano),
		DurationMillis: finishedAt.Sub(startedAt).Milliseconds(),
		TotalLines:     result.TotalLines,
		FilteredLines:  result.FilteredLines,
		OutputPath:     result.OutputPath,
		Files:          make([]RunReportFile, 0, len(result.Files)),
	}
	for _, file := range result.Files {
		report.Files = append(report.Files, RunReportFile{
			Name:          file.Name,
			TotalLines:    file.TotalLines,
			FilteredLines: file.FilteredLines,
		})
	}
	if runErr != nil {
		report.Status = "error"
		report.Error = runErr.Error()
	}
	return report
}

func writeRunReport(ctx context.Context, store storage.Store, reportFile string, report RunReport) error {
	if reportFile == "" {
		return nil
	}
	writer, err := store.Create(ctx, reportFile)
	if err != nil {
		return fmt.Errorf("create report output %q: %w", reportFile, err)
	}
	defer writer.Close()

	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("encode report %q: %w", reportFile, err)
	}
	return nil
}
