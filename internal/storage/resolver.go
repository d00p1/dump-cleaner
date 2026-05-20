package storage

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

type Config struct {
	Endpoint       string
	Region         string
	RequestTimeout time.Duration
	RetryMax       int
	AccessKey      string
	SecretKey      string
	SessionToken   string
	ForcePathStyle bool
	Insecure       bool
}

type Store interface {
	Open(ctx context.Context, uri string) (io.ReadCloser, error)
	Create(ctx context.Context, uri string) (io.WriteCloser, error)
}

type Resolver struct {
	config Config
	local  localBackend

	mu sync.Mutex
	s3 *s3Backend
}

func NewResolver(cfg Config) *Resolver {
	return &Resolver{config: cfg}
}

func (r *Resolver) Open(ctx context.Context, uri string) (io.ReadCloser, error) {
	loc, err := ParseLocation(uri)
	if err != nil {
		return nil, fmt.Errorf("resolve input URI: %w", err)
	}
	backend, err := r.backendFor(loc)
	if err != nil {
		return nil, err
	}
	reader, err := backend.Open(ctx, loc)
	if err != nil {
		return nil, fmt.Errorf("open input %q: %w", uri, err)
	}
	return reader, nil
}

func (r *Resolver) Create(ctx context.Context, uri string) (io.WriteCloser, error) {
	loc, err := ParseLocation(uri)
	if err != nil {
		return nil, fmt.Errorf("resolve output URI: %w", err)
	}
	backend, err := r.backendFor(loc)
	if err != nil {
		return nil, err
	}
	writer, err := backend.Create(ctx, loc)
	if err != nil {
		return nil, fmt.Errorf("create output %q: %w", uri, err)
	}
	return writer, nil
}

type backend interface {
	Open(ctx context.Context, loc Location) (io.ReadCloser, error)
	Create(ctx context.Context, loc Location) (io.WriteCloser, error)
}

func (r *Resolver) backendFor(loc Location) (backend, error) {
	switch loc.Scheme {
	case "file":
		return r.local, nil
	case "s3":
		return r.s3Backend()
	default:
		return nil, fmt.Errorf("unsupported backend for scheme %q", loc.Scheme)
	}
}

func (r *Resolver) s3Backend() (*s3Backend, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.s3 != nil {
		return r.s3, nil
	}
	backend, err := newS3Backend(r.config)
	if err != nil {
		return nil, fmt.Errorf("create S3 backend: %w", err)
	}
	r.s3 = backend
	return r.s3, nil
}
