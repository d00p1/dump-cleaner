package storage

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsretry "github.com/aws/aws-sdk-go-v2/aws/retry"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type s3Backend struct {
	client         *s3.Client
	requestTimeout time.Duration
	retryMax       int
}

func newS3Backend(cfg Config) (*s3Backend, error) {
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "us-east-1"
	}

	loadOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
	}
	if cfg.RetryMax > 0 {
		maxAttempts := cfg.RetryMax
		loadOpts = append(loadOpts, awsconfig.WithRetryer(func() aws.Retryer {
			return awsretry.NewStandard(func(o *awsretry.StandardOptions) {
				o.MaxAttempts = maxAttempts
			})
		}))
	}
	if cfg.AccessKey != "" || cfg.SecretKey != "" || cfg.SessionToken != "" {
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, cfg.SessionToken)))
	}
	if cfg.Insecure {
		loadOpts = append(loadOpts, awsconfig.WithHTTPClient(&http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.ForcePathStyle
		if endpoint := strings.TrimSpace(cfg.Endpoint); endpoint != "" {
			o.BaseEndpoint = &endpoint
		}
	})

	return &s3Backend{client: client, requestTimeout: cfg.RequestTimeout, retryMax: cfg.RetryMax}, nil
}

func (b *s3Backend) Open(ctx context.Context, loc Location) (io.ReadCloser, error) {
	ctx, cancel := b.withRequestTimeout(ctx)
	resp, err := b.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &loc.Bucket, Key: &loc.Key})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("get s3://%s/%s: %w", loc.Bucket, loc.Key, err)
	}
	return &s3ReadCloser{ReadCloser: resp.Body, cancel: cancel}, nil
}

func (b *s3Backend) Create(ctx context.Context, loc Location) (io.WriteCloser, error) {
	ctx, cancel := b.withRequestTimeout(ctx)
	pr, pw := io.Pipe()
	errCh := make(chan error, 1)

	go func() {
		_, err := b.client.PutObject(ctx, &s3.PutObjectInput{Bucket: &loc.Bucket, Key: &loc.Key, Body: pr})
		_ = pr.Close()
		if err != nil {
			errCh <- fmt.Errorf("put s3://%s/%s: %w", loc.Bucket, loc.Key, err)
			return
		}
		errCh <- nil
	}()

	return &s3WriteCloser{pipe: pw, errCh: errCh, cancel: cancel}, nil
}

func (b *s3Backend) withRequestTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if b.requestTimeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, b.requestTimeout)
}

type s3ReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *s3ReadCloser) Close() error {
	err := r.ReadCloser.Close()
	r.cancel()
	return err
}

type s3WriteCloser struct {
	pipe   *io.PipeWriter
	errCh  chan error
	cancel context.CancelFunc
	closed bool
}

func (w *s3WriteCloser) Write(p []byte) (int, error) {
	return w.pipe.Write(p)
}

func (w *s3WriteCloser) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	closeErr := w.pipe.Close()
	uploadErr := <-w.errCh
	w.cancel()
	if closeErr != nil {
		return closeErr
	}
	return uploadErr
}
