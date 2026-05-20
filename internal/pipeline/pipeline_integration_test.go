package pipeline

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/d00p1/filtrate-backups/internal/filter"
	"github.com/d00p1/filtrate-backups/internal/storage"
	"github.com/d00p1/filtrate-backups/pkg/archive"
)

func TestRunSupportsS3SQLWithMinIO(t *testing.T) {
	runMinIOPipelineFormatTest(t, ".sql", writeMinIOSQLInput, readMinIOSQLFile)
}

func TestRunSupportsS3SQLGZWithMinIO(t *testing.T) {
	runMinIOPipelineFormatTest(t, ".sql.gz", writeMinIOSQLGZInput, readMinIOSQLGZFile)
}

func TestRunSupportsS3TarGZWithMinIO(t *testing.T) {
	runMinIOPipelineFormatTest(t, ".tar.gz", writeMinIOTarGZInput, readMinIOTarGZOutput)
}

func runMinIOPipelineFormatTest(t *testing.T, ext string, writeInput func(context.Context, *s3.Client, string, string, string) error, readOutput func(context.Context, *s3.Client, string, string) (string, error)) {
	t.Helper()
	if os.Getenv("RUN_MINIO_TESTS") != "1" {
		t.Skip("set RUN_MINIO_TESTS=1 to run MinIO integration tests")
	}

	endpoint := getenvOrDefault("MINIO_ENDPOINT", "http://127.0.0.1:9000")
	region := getenvOrDefault("MINIO_REGION", "us-east-1")
	accessKey := getenvOrDefault("MINIO_ROOT_USER", "minioadmin")
	secretKey := getenvOrDefault("MINIO_ROOT_PASSWORD", "minioadmin")
	bucket := fmt.Sprintf("dump-cleaner-%d", time.Now().UnixNano())
	inputKey := "input/source" + ext
	outputKey := "output/filtered" + ext

	ctx := context.Background()
	client, err := newMinIOTestClient(ctx, endpoint, region, accessKey, secretKey)
	if err != nil {
		t.Fatalf("create minio test client: %v", err)
	}

	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &bucket}); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	t.Cleanup(func() {
		deleteAllBucketObjects(ctx, client, bucket)
		_, _ = client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: &bucket})
	})

	inputSQL := sampleSQL()
	if err := writeInput(ctx, client, bucket, inputKey, inputSQL); err != nil {
		t.Fatalf("put input object: %v", err)
	}

	rules, err := filter.CompileRules("mysql", []filter.RuleDefinition{
		{Action: "ddl", Tables: []string{"^tmp_"}},
		{Action: "insert", Tables: []string{"^tmp_"}},
	})
	if err != nil {
		t.Fatalf("compile rules: %v", err)
	}

	tmpDir := t.TempDir()
	store := storage.NewResolver(storage.Config{
		Endpoint:       endpoint,
		Region:         region,
		AccessKey:      accessKey,
		SecretKey:      secretKey,
		ForcePathStyle: true,
		Insecure:       strings.HasPrefix(endpoint, "http://"),
	})

	result, err := Run(ctx, Options{
		InputPath:    fmt.Sprintf("s3://%s/%s", bucket, inputKey),
		OutputPath:   fmt.Sprintf("s3://%s/%s", bucket, outputKey),
		Rules:        rules,
		TmpDir:       tmpDir,
		MaxLineBytes: 1024 * 1024,
		Store:        store,
	})
	if err != nil {
		t.Fatalf("run pipeline: %v", err)
	}
	if result.FilteredLines == 0 {
		t.Fatalf("expected filtered lines > 0")
	}

	outputSQL, err := readOutput(ctx, client, bucket, outputKey)
	if err != nil {
		t.Fatalf("read output object: %v", err)
	}
	if strings.Contains(outputSQL, "tmp_log") {
		t.Fatalf("expected tmp_log statements to be removed, got %q", outputSQL)
	}
	if !strings.Contains(outputSQL, "CREATE TABLE `users`") {
		t.Fatalf("expected users table to remain, got %q", outputSQL)
	}
}

func writeMinIOSQLInput(ctx context.Context, client *s3.Client, bucket, key, inputSQL string) error {
	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &bucket,
		Key:    &key,
		Body:   bytes.NewReader([]byte(inputSQL)),
	})
	return err
}

func writeMinIOSQLGZInput(ctx context.Context, client *s3.Client, bucket, key, inputSQL string) error {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	if _, err := gzWriter.Write([]byte(inputSQL)); err != nil {
		return err
	}
	if err := gzWriter.Close(); err != nil {
		return err
	}
	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &bucket,
		Key:    &key,
		Body:   bytes.NewReader(buf.Bytes()),
	})
	return err
}

func writeMinIOTarGZInput(ctx context.Context, client *s3.Client, bucket, key, inputSQL string) error {
	workDir, err := os.MkdirTemp("", "minio-tar-src-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)
	if err := os.WriteFile(filepath.Join(workDir, "dump.sql"), []byte(inputSQL), 0o644); err != nil {
		return err
	}
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	if err := archive.Pack(workDir, gzWriter); err != nil {
		return err
	}
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &bucket,
		Key:    &key,
		Body:   bytes.NewReader(buf.Bytes()),
	})
	return err
}

func readMinIOSQLFile(ctx context.Context, client *s3.Client, bucket, key string) (string, error) {
	resp, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: &bucket, Key: &key})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	output, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func readMinIOSQLGZFile(ctx context.Context, client *s3.Client, bucket, key string) (string, error) {
	resp, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: &bucket, Key: &key})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	reader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	output, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func readMinIOTarGZOutput(ctx context.Context, client *s3.Client, bucket, key string) (string, error) {
	resp, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: &bucket, Key: &key})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	reader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	workDir, err := os.MkdirTemp("", "minio-tar-out-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(workDir)
	if err := archive.Unpack(reader, workDir); err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(workDir, "dump.sql"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func newMinIOTestClient(ctx context.Context, endpoint, region, accessKey, secretKey string) (*s3.Client, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
		o.BaseEndpoint = &endpoint
	})
	return client, nil
}

func deleteAllBucketObjects(ctx context.Context, client *s3.Client, bucket string) {
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{Bucket: &bucket})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return
		}
		if len(page.Contents) == 0 {
			continue
		}
		objects := make([]types.ObjectIdentifier, 0, len(page.Contents))
		for _, obj := range page.Contents {
			objects = append(objects, types.ObjectIdentifier{Key: obj.Key})
		}
		_, _ = client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: &bucket,
			Delete: &types.Delete{Objects: objects, Quiet: boolPtr(true)},
		})
	}
}

func boolPtr(v bool) *bool {
	return &v
}

func getenvOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
