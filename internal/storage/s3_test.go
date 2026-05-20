package storage

import (
	"testing"
	"time"
)

func TestNewS3BackendAppliesRequestPolicy(t *testing.T) {
	backend, err := newS3Backend(Config{
		Endpoint:       "http://minio:9000",
		Region:         "us-east-1",
		RequestTimeout: 30 * time.Second,
		RetryMax:       5,
		ForcePathStyle: true,
		Insecure:       true,
	})
	if err != nil {
		t.Fatalf("newS3Backend failed: %v", err)
	}
	if backend.requestTimeout != 30*time.Second {
		t.Fatalf("unexpected request timeout: %v", backend.requestTimeout)
	}
	if backend.retryMax != 5 {
		t.Fatalf("unexpected retry max: %d", backend.retryMax)
	}
}
