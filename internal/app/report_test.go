package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRunWritesSuccessReport(t *testing.T) {
	dir := t.TempDir()
	tmpDir := filepath.Join(dir, "tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		t.Fatalf("create tmp dir: %v", err)
	}
	inputPath := filepath.Join(dir, "input.sql")
	outputPath := filepath.Join(dir, "output.sql")
	reportPath := filepath.Join(dir, "report.json")
	input := "CREATE TABLE `tmp_log` (id int);\nINSERT INTO `tmp_log` VALUES (1);\nCREATE TABLE `users` (id int);\n"
	if err := os.WriteFile(inputPath, []byte(input), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	err := Run(context.Background(), []string{
		"--input", inputPath,
		"--output", outputPath,
		"--tmp-dir", tmpDir,
		"--db-driver", "mysql",
		"--skip", "^tmp_",
		"--report-file", reportPath,
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	report := readReport(t, reportPath)
	if report.Status != "ok" {
		t.Fatalf("expected ok status, got %#v", report)
	}
	if report.FilteredLines == 0 || report.TotalLines == 0 {
		t.Fatalf("expected line stats in report, got %#v", report)
	}
	if report.OutputPath != outputPath {
		t.Fatalf("unexpected output path in report: %#v", report)
	}
	if len(report.Files) != 1 {
		t.Fatalf("expected one file report entry, got %#v", report.Files)
	}
	if report.Files[0].FilteredLines == 0 || report.Files[0].TotalLines == 0 {
		t.Fatalf("expected per-file report stats, got %#v", report.Files[0])
	}
}

func TestRunWritesErrorReport(t *testing.T) {
	dir := t.TempDir()
	tmpDir := filepath.Join(dir, "tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		t.Fatalf("create tmp dir: %v", err)
	}
	reportPath := filepath.Join(dir, "report-error.json")

	err := Run(context.Background(), []string{
		"--input", filepath.Join(dir, "missing.sql"),
		"--output", filepath.Join(dir, "output.sql"),
		"--tmp-dir", tmpDir,
		"--db-driver", "mysql",
		"--report-file", reportPath,
	})
	if err == nil {
		t.Fatalf("expected run failure")
	}

	report := readReport(t, reportPath)
	if report.Status != "error" {
		t.Fatalf("expected error status, got %#v", report)
	}
	if report.Error == "" {
		t.Fatalf("expected error message in report, got %#v", report)
	}
}

func readReport(t *testing.T, path string) RunReport {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report RunReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	return report
}
