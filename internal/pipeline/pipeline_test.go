package pipeline

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d00p1/filtrate-backups/internal/filter"
	"github.com/d00p1/filtrate-backups/internal/storage"
	"github.com/d00p1/filtrate-backups/pkg/archive"
)

func TestRunSupportsSQL(t *testing.T) {
	runPipelineFormatTest(t, ".sql", writeSQLInput, readSQLFile)
}

func TestRunSupportsSQLGZ(t *testing.T) {
	runPipelineFormatTest(t, ".sql.gz", writeSQLGZInput, readSQLGZFile)
}

func TestRunSupportsTarGZ(t *testing.T) {
	runPipelineFormatTest(t, ".tar.gz", writeTarGZInput, readTarGZOutput)
}

func TestRunTracksPerFileStatsForTarGZ(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.tar.gz")
	outputPath := filepath.Join(dir, "output.tar.gz")
	tmpDir := filepath.Join(dir, "tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		t.Fatalf("create tmp dir: %v", err)
	}
	if err := writeMultiFileTarGZInput(inputPath); err != nil {
		t.Fatalf("write tar input: %v", err)
	}

	rules, err := filter.CompileRules("mysql", []filter.RuleDefinition{
		{Action: "ddl", Tables: []string{"^tmp_"}},
		{Action: "insert", Tables: []string{"^tmp_"}},
	})
	if err != nil {
		t.Fatalf("compile rules: %v", err)
	}

	result, err := Run(context.Background(), Options{
		InputPath:    inputPath,
		OutputPath:   outputPath,
		Rules:        rules,
		TmpDir:       tmpDir,
		MaxLineBytes: 1024 * 1024,
		Store:        storage.NewResolver(storage.Config{}),
	})
	if err != nil {
		t.Fatalf("run pipeline: %v", err)
	}
	if len(result.Files) != 2 {
		t.Fatalf("expected 2 file results, got %#v", result.Files)
	}
	seen := map[string]FileResult{}
	for _, file := range result.Files {
		seen[file.Name] = file
	}
	if seen["first.sql"].FilteredLines == 0 {
		t.Fatalf("expected filtered lines for first.sql, got %#v", seen["first.sql"])
	}
	if seen["second.sql"].TotalLines == 0 {
		t.Fatalf("expected stats for second.sql, got %#v", seen["second.sql"])
	}
}

func runPipelineFormatTest(t *testing.T, ext string, writeInput func(string, string) error, readOutput func(string) (string, error)) {
	t.Helper()
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input"+ext)
	outputPath := filepath.Join(dir, "output"+ext)
	tmpDir := filepath.Join(dir, "tmp")
	inputSQL := sampleSQL()
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		t.Fatalf("create tmp dir: %v", err)
	}

	if err := writeInput(inputPath, inputSQL); err != nil {
		t.Fatalf("write input: %v", err)
	}

	rules, err := filter.CompileRules("mysql", []filter.RuleDefinition{
		{Action: "ddl", Tables: []string{"^tmp_"}},
		{Action: "insert", Tables: []string{"^tmp_"}},
	})
	if err != nil {
		t.Fatalf("compile rules: %v", err)
	}

	result, err := Run(context.Background(), Options{
		InputPath:    inputPath,
		OutputPath:   outputPath,
		Rules:        rules,
		TmpDir:       tmpDir,
		MaxLineBytes: 1024 * 1024,
		Store:        storage.NewResolver(storage.Config{}),
	})
	if err != nil {
		t.Fatalf("run pipeline: %v", err)
	}
	if result.FilteredLines == 0 {
		t.Fatalf("expected filtered lines > 0")
	}
	if len(result.Files) != 1 {
		t.Fatalf("expected one file result, got %#v", result.Files)
	}
	if result.Files[0].FilteredLines == 0 || result.Files[0].TotalLines == 0 {
		t.Fatalf("expected per-file stats, got %#v", result.Files[0])
	}

	outputSQL, err := readOutput(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if strings.Contains(outputSQL, "tmp_log") {
		t.Fatalf("expected tmp_log statements to be removed, got %q", outputSQL)
	}
	if !strings.Contains(outputSQL, "CREATE TABLE `users`") {
		t.Fatalf("expected users table to remain, got %q", outputSQL)
	}
}

func sampleSQL() string {
	return "CREATE TABLE `tmp_log` (id int);\n" +
		"INSERT INTO `tmp_log` VALUES (1);\n" +
		"CREATE TABLE `users` (id int);\n" +
		"INSERT INTO `users` VALUES (7);\n"
}

func writeSQLInput(path, contents string) error {
	return os.WriteFile(path, []byte(contents), 0o644)
}

func writeSQLGZInput(path, contents string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	if _, err := io.WriteString(gz, contents); err != nil {
		return err
	}
	return gz.Close()
}

func writeTarGZInput(path, contents string) error {
	workDir, err := os.MkdirTemp(filepath.Dir(path), "tar-src-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)
	if err := os.WriteFile(filepath.Join(workDir, "dump.sql"), []byte(contents), 0o644); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(f)
	if err := archive.Pack(workDir, gz); err != nil {
		return err
	}
	return nil
}

func writeMultiFileTarGZInput(path string) error {
	workDir, err := os.MkdirTemp(filepath.Dir(path), "tar-src-multi-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)
	if err := os.WriteFile(filepath.Join(workDir, "first.sql"), []byte(sampleSQL()), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(workDir, "second.sql"), []byte("CREATE TABLE `users_archive` (id int);\nINSERT INTO `users_archive` VALUES (2);\n"), 0o644); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(f)
	if err := archive.Pack(workDir, gz); err != nil {
		return err
	}
	return nil
}

func readSQLFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func readSQLGZFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	data, err := io.ReadAll(gz)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func readTarGZOutput(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	workDir, err := os.MkdirTemp(filepath.Dir(path), "tar-out-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(workDir)
	if err := archive.Unpack(gz, workDir); err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(workDir, "dump.sql"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
