# Config Specification

## File Config
File configs accept only `camelCase` keys.

| Field | Type | Required | Default | Env | CLI | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| `dumpFile` | string | yes | none | `DUMPFILE` | `--input` | Input URI. Supports local paths, `file://`, and `s3://bucket/key` with `.tar.gz`, `.sql.gz`, or `.sql`. |
| `outputFile` | string | no | `./output/filtered_result.tar.gz` | `OUTPUT_FILE` | `--output` | Output URI. Supports local paths, `file://`, and `s3://bucket/key` with `.tar.gz`, `.sql.gz`, or `.sql`. |
| `tmpDir` | string | no | `./tmp` | `TMP_DIR` | `--tmp-dir` | Temporary work directory. |
| `maxLineBytes` | integer | no | `8388608` | `MAX_LINE_BYTES` | `--max-line-bytes` | Must be `>= 1024`. |
| `mode` | `once` or `schedule` | no | `once` | `MODE` | `--mode` | Runtime mode. |
| `scheduleEvery` | string | when `mode: schedule` | none | `SCHEDULE_EVERY` | `--every` | Go duration string like `30m` or `1h`. |
| `dbDriver` | `mysql` | no | `mysql` | `DB_DRIVER` | `--db-driver` | Driver alias registry selector. |
| `reportFile` | string | no | none | `REPORT_FILE` | `--report-file` | Write a JSON runtime report to a local path, `file://`, or `s3://bucket/key`. |
| `s3Endpoint` | string | no | none | `S3_ENDPOINT` | none | S3-compatible endpoint override, for example `http://minio:9000`. |
| `s3Region` | string | no | `us-east-1` | `S3_REGION` | none | Region passed to the S3 client. |
| `s3RequestTimeout` | string | no | none | `S3_REQUEST_TIMEOUT` | `--s3-request-timeout` | Per-request timeout for S3 operations, for example `30s`. |
| `s3RetryMaxAttempts` | integer | no | `3` | `S3_RETRY_MAX_ATTEMPTS` | `--s3-retry-max-attempts` | Maximum S3 retry attempts. Must be `>= 1`. |
| `s3AccessKey` | string | no | none | `S3_ACCESS_KEY` | none | S3 access key. Prefer env in production. |
| `s3SecretKey` | string | no | none | `S3_SECRET_KEY` | none | S3 secret key. Prefer env in production. |
| `s3SessionToken` | string | no | none | `S3_SESSION_TOKEN` | none | Optional session token. |
| `s3ForcePathStyle` | boolean | no | `true` | `S3_FORCE_PATH_STYLE` | none | Keep `true` for MinIO-style path access. |
| `s3Insecure` | boolean | no | `false` | `S3_INSECURE` | none | Skip TLS verification for S3 HTTPS endpoints. |
| `filterRules` | array | no | none | none | none | Preferred filtering mechanism for file configs. |

### `filterRules`
Each item supports only these fields:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `action` | string | yes | One of `insert`, `create_table`, `drop_table`, `ddl`, `locks`, `all`. |
| `tables` | `"all"` or `string[]` | yes | Use scalar `all` for every table, or an array of regex patterns. |

Example:

```yaml
dumpFile: ./data/source.tar.gz
outputFile: ./output/filtered_result.tar.gz
dbDriver: mysql
s3Endpoint: http://minio:9000
s3Region: us-east-1
s3ForcePathStyle: true
mode: schedule
scheduleEvery: 1h
filterRules:
  - action: ddl
    tables:
      - ^tmp_
      - ^log_
  - action: locks
    tables: all
```

## Environment Variables
Environment variables use only `UPPER_CASE` names.

```env
DUMPFILE=./data/source.tar.gz
OUTPUT_FILE=./output/filtered_result.tar.gz
TMP_DIR=./tmp
MAX_LINE_BYTES=8388608
DB_DRIVER=mysql
MODE=once
SCHEDULE_EVERY=1h
REPORT_FILE=./output/report.json
S3_ENDPOINT=http://minio:9000
S3_REGION=us-east-1
S3_REQUEST_TIMEOUT=30s
S3_RETRY_MAX_ATTEMPTS=3
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin
S3_FORCE_PATH_STYLE=true
S3_INSECURE=false
```

## S3 Notes
- Target remote backend for this stage is MinIO via an S3-compatible API.
- `s3://bucket/key` URIs are supported for input and output.
- Supported object/file formats are `.tar.gz`, `.sql.gz`, and `.sql`.
- By default, keep the same format for input and output.
- `s3RequestTimeout` applies a per-operation timeout to `GetObject` and `PutObject` requests.
- `s3RetryMaxAttempts` configures the AWS SDK retryer for S3 operations.
- Secrets are supported in file config, but env variables are recommended for production deployments.
- The pipeline still materializes data in `tmpDir`; `s3://` does not make processing fully streaming.

## Runtime Report
When `reportFile` or `REPORT_FILE` is set, the app writes a JSON report for each run.

Fields include:
- `status`
- `input`
- `output`
- `dbDriver`
- `warnings`
- `startedAt`
- `finishedAt`
- `durationMillis`
- `totalLines`
- `filteredLines`
- `outputPath`
- `files[]` with per-file `name`, `totalLines`, and `filteredLines`
- `error`

## CLI Flags

```bash
go run . \
  --input ./data/source.tar.gz \
  --output ./output/filtered_result.tar.gz \
  --tmp-dir ./tmp \
  --max-line-bytes 8388608 \
  --db-driver mysql \
  --mode once
```

## Deprecations
`TABLE_MAP` is deprecated and supported only through environment variables.

- Deprecated env key: `TABLE_MAP="^tmp_:^log_"`
- Replacement in file configs: `filterRules`
- Runtime behavior: when `TABLE_MAP` is actually used, the app prints a warning

`tableMap` and `TABLE_MAP` are rejected in file configs.
