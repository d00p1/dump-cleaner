package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestParseLocationLocalPath(t *testing.T) {
	loc, err := ParseLocation("./data/source.tar.gz")
	if err != nil {
		t.Fatalf("parse local path: %v", err)
	}
	if loc.Scheme != "file" || loc.Path != "./data/source.tar.gz" {
		t.Fatalf("unexpected local location: %#v", loc)
	}
}

func TestParseLocationFileURI(t *testing.T) {
	loc, err := ParseLocation("file:///tmp/source.tar.gz")
	if err != nil {
		t.Fatalf("parse file URI: %v", err)
	}
	if loc.Scheme != "file" || loc.Path != "/tmp/source.tar.gz" {
		t.Fatalf("unexpected file URI location: %#v", loc)
	}
}

func TestParseLocationS3URI(t *testing.T) {
	loc, err := ParseLocation("s3://bucket-name/path/to/source.tar.gz")
	if err != nil {
		t.Fatalf("parse s3 URI: %v", err)
	}
	if loc.Scheme != "s3" || loc.Bucket != "bucket-name" || loc.Key != "path/to/source.tar.gz" {
		t.Fatalf("unexpected s3 location: %#v", loc)
	}
}

func TestParseLocationRejectsInvalidS3URI(t *testing.T) {
	_, err := ParseLocation("s3://bucket-name")
	if err == nil {
		t.Fatalf("expected empty key error")
	}
}

func TestResolverLocalCreateAndOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "out.txt")
	resolver := NewResolver(Config{})

	writer, err := resolver.Create(context.Background(), path)
	if err != nil {
		t.Fatalf("create local output: %v", err)
	}
	if _, err := io.WriteString(writer, "hello"); err != nil {
		t.Fatalf("write local output: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close local output: %v", err)
	}

	reader, err := resolver.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open local output: %v", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read local output: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected file contents: %q", data)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected local file to exist: %v", err)
	}
}
