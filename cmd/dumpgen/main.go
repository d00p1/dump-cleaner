package main

import (
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/d00p1/filtrate-backups/internal/version"
	"github.com/d00p1/filtrate-backups/pkg/archive"
)

const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func main() {
	var (
		output        string
		tableName     string
		tablesRaw     string
		targetSizeRaw string
		rowsPerInsert int
		valueSize     int
		seed          int64
		showVersion   bool
	)

	flag.StringVar(&output, "output", "./data/generated_dump_1gb.tar.gz", "output tar.gz path")
	flag.StringVar(&tableName, "table", "bench_data", "table name for generated INSERTs (legacy alias)")
	flag.StringVar(&tablesRaw, "tables", "", "comma-separated table names, e.g. users,orders,events")
	flag.StringVar(&targetSizeRaw, "target-size", "1GB", "target SQL payload size (e.g. 1GB, 1024MB, 500MiB)")
	flag.IntVar(&rowsPerInsert, "rows-per-insert", 1000, "rows per INSERT statement")
	flag.IntVar(&valueSize, "value-size", 128, "string payload size per row")
	flag.Int64Var(&seed, "seed", time.Now().UnixNano(), "random seed for payload generation")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.Parse()

	if showVersion {
		fmt.Printf("dumpgen %s\n", version.String())
		return
	}

	tables := parseTables(tablesRaw)
	if len(tables) == 0 {
		tables = []string{strings.TrimSpace(tableName)}
	}

	targetBytes, err := parseSize(targetSizeRaw)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	if rowsPerInsert <= 0 || valueSize <= 0 {
		fmt.Println("Error: rows-per-insert and value-size must be > 0")
		os.Exit(1)
	}
	if len(tables) == 0 || tables[0] == "" {
		fmt.Println("Error: at least one table is required")
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	rng := rand.New(rand.NewSource(seed))
	sqlBytes, err := generateTarGz(output, tables, targetBytes, rowsPerInsert, valueSize, rng)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	fmt.Printf("✅ generated: %s (sql bytes: %d, tables: %d, seed: %d)\n", output, sqlBytes, len(tables), seed)
}

func parseTables(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	tables := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		t := strings.TrimSpace(part)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		tables = append(tables, t)
		seen[t] = struct{}{}
	}
	return tables
}

func generateTarGz(output string, tables []string, targetBytes int64, rowsPerInsert, valueSize int, rng *rand.Rand) (int64, error) {
	tmpDir, err := os.MkdirTemp("", "dumpgen-")
	if err != nil {
		return 0, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	sqlPath := filepath.Join(tmpDir, "dump.sql")
	sqlFile, err := os.Create(sqlPath)
	if err != nil {
		return 0, fmt.Errorf("create temp sql: %w", err)
	}

	sqlBytes, err := writeSQL(sqlFile, tables, targetBytes, rowsPerInsert, valueSize, rng)
	if closeErr := sqlFile.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return 0, err
	}

	out, err := os.Create(output)
	if err != nil {
		return 0, fmt.Errorf("create output: %w", err)
	}
	defer out.Close()

	gz := gzip.NewWriter(out)
	if err := archive.Pack(tmpDir, gz); err != nil {
		return 0, fmt.Errorf("pack tar.gz: %w", err)
	}

	return sqlBytes, nil
}

func writeSQL(w io.Writer, tables []string, targetBytes int64, rowsPerInsert, valueSize int, rng *rand.Rand) (int64, error) {
	var written int64
	for _, tableName := range tables {
		base := "CREATE TABLE `" + tableName + "` (id BIGINT PRIMARY KEY, payload TEXT);\n"
		n, err := io.WriteString(w, base)
		if err != nil {
			return written, fmt.Errorf("write ddl: %w", err)
		}
		written += int64(n)
	}

	ids := make([]int64, len(tables))
	for i := range ids {
		ids[i] = 1
	}

	tableIdx := 0
	for written < targetBytes {
		tableName := tables[tableIdx%len(tables)]
		tablePos := tableIdx % len(tables)
		tableIdx++

		var b strings.Builder
		b.Grow(rowsPerInsert * (valueSize + 48))
		b.WriteString("INSERT INTO `")
		b.WriteString(tableName)
		b.WriteString("` VALUES ")
		for i := 0; i < rowsPerInsert; i++ {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString("(")
			b.WriteString(strconv.FormatInt(ids[tablePos], 10))
			b.WriteString(",'")
			b.WriteString(randomString(rng, valueSize))
			b.WriteString("')")
			ids[tablePos]++
		}
		b.WriteString(";\n")

		line := b.String()
		n, err := io.WriteString(w, line)
		if err != nil {
			return written, fmt.Errorf("write insert: %w", err)
		}
		written += int64(n)
	}

	return written, nil
}

func randomString(rng *rand.Rand, length int) string {
	buf := make([]byte, length)
	for i := range buf {
		buf[i] = alphabet[rng.Intn(len(alphabet))]
	}
	return string(buf)
}

func parseSize(raw string) (int64, error) {
	raw = strings.TrimSpace(strings.ToUpper(raw))
	ordered := []struct {
		suffix string
		mul    int64
	}{
		{"GIB", 1024 * 1024 * 1024},
		{"MIB", 1024 * 1024},
		{"KIB", 1024},
		{"TB", 1000 * 1000 * 1000 * 1000},
		{"GB", 1000 * 1000 * 1000},
		{"MB", 1000 * 1000},
		{"KB", 1000},
		{"B", 1},
	}

	for _, item := range ordered {
		if strings.HasSuffix(raw, item.suffix) {
			numPart := strings.TrimSpace(strings.TrimSuffix(raw, item.suffix))
			v, err := strconv.ParseInt(numPart, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid size %q", raw)
			}
			if v <= 0 {
				return 0, fmt.Errorf("size must be > 0")
			}
			return v * item.mul, nil
		}
	}

	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("unknown size format %q", raw)
	}
	if v <= 0 {
		return 0, fmt.Errorf("size must be > 0")
	}
	return v, nil
}
