package storage

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

type Location struct {
	Raw    string
	Scheme string
	Path   string
	Bucket string
	Key    string
}

func ParseLocation(raw string) (Location, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Location{}, fmt.Errorf("empty URI")
	}

	if !strings.Contains(trimmed, "://") {
		return Location{Raw: raw, Scheme: "file", Path: trimmed}, nil
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return Location{}, fmt.Errorf("parse URI %q: %w", raw, err)
	}

	switch u.Scheme {
	case "file":
		if u.Host != "" && u.Host != "localhost" {
			return Location{}, fmt.Errorf("unsupported file URI host %q", u.Host)
		}
		if u.Path == "" {
			return Location{}, fmt.Errorf("file URI %q has empty path", raw)
		}
		return Location{Raw: raw, Scheme: "file", Path: filepath.FromSlash(u.Path)}, nil
	case "s3":
		bucket := strings.TrimSpace(u.Host)
		key := strings.TrimPrefix(path.Clean(u.Path), "/")
		if bucket == "" {
			return Location{}, fmt.Errorf("s3 URI %q has empty bucket", raw)
		}
		if key == "" || key == "." {
			return Location{}, fmt.Errorf("s3 URI %q has empty object key", raw)
		}
		return Location{Raw: raw, Scheme: "s3", Bucket: bucket, Key: key}, nil
	default:
		return Location{}, fmt.Errorf("unsupported URI scheme %q", u.Scheme)
	}
}
