# 📦 Filtrate Backups
Filtrate Backups is a Go utility for filtering SQL dump archives.
It unpacks `.tar.gz` backups, removes unwanted `CREATE TABLE`/`DROP TABLE`/`INSERT` statements for selected tables, and repacks a cleaned dump.

## 🚀 Features
- Streams dump files line-by-line.
- Handles very large SQL lines with configurable memory limits (`maxLineBytes` in file config, `MAX_LINE_BYTES` in env).
- Runs once or as an internal scheduler (`mode: schedule` in file config, `SCHEDULE_EVERY=...` in env).
- Supports deployment as:
  - a containerized scheduler,
  - a scheduler near a dedicated S3 service (e.g. MinIO),
  - a system scheduler via `systemd` timer.
- Supports multiple configuration formats through strategy-based loading:
  - `.yaml/.yml`
  - `.toml`
  - `.json`
  - `.conf/.ini`
- Supports combined configuration sources (file + env + CLI overrides).
- Supports local paths, `file://`, and `s3://bucket/key` input/output URIs.
- Supports `.tar.gz`, `.sql.gz`, and `.sql` dump formats.
- S3-compatible object storage is currently targeted at MinIO-style deployments.

## ⚙️ Configuration sources (strategy)
The app uses layered config with strategy selection:
1. defaults,
2. config file (`--config` + `--config-format`),
3. environment variables,
4. CLI flags (highest priority).

CLI switches for strategy:
- `--config ./examples/config.yaml`
- `--config-format auto|yaml|toml|json|conf`
- `--config-strategy merge|file-only|env-only`

Default strategy is `merge`.

### File config
File configs accept only `camelCase` keys. Full spec: `docs/config.md`, machine-readable schema: `docs/config.schema.json`.

```yaml
dumpFile: ./data/source.tar.gz
outputFile: ./output/filtered_result.tar.gz
tmpDir: ./tmp
maxLineBytes: 8388608
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
      - ^b_.+_tmp$

  - action: insert
    tables:
      - ^b_search_content_tmp$

  - action: locks
    tables: all
```

### Environment variables
Environment variables use `UPPER_CASE` names:

```env
DUMPFILE="./data/source.tar.gz"
OUTPUT_FILE="./output/filtered_result.tar.gz"
TMP_DIR="./tmp"
MAX_LINE_BYTES=8388608
DB_DRIVER="mysql"
MODE="once"
SCHEDULE_EVERY="1h"
REPORT_FILE="./output/report.json"
S3_ENDPOINT="http://minio:9000"
S3_REGION="us-east-1"
S3_REQUEST_TIMEOUT="30s"
S3_RETRY_MAX_ATTEMPTS=3
S3_ACCESS_KEY="minioadmin"
S3_SECRET_KEY="minioadmin"
S3_FORCE_PATH_STYLE=true
S3_INSECURE=false
```

`DB_DRIVER` currently supports `mysql` aliases.

`dumpFile` and `outputFile` accept local paths, `file://`, and `s3://bucket/key` URIs for `.tar.gz`, `.sql.gz`, and `.sql` objects.

Deprecated env fallback:

```env
TABLE_MAP="^tmp_:^log_"
```

`TABLE_MAP` is deprecated, env-only, and prints a runtime warning when it is actually used. File configs must use `filterRules` instead.

### Structured filter rules
Use `filterRules` when you need readable aliases and `tables: all`:

```yaml
dbDriver: mysql
filterRules:
  - action: ddl
    tables:
      - ^tmp_
      - ^b_.+_tmp$

  - action: insert
    tables:
      - ^b_search_content_tmp$

  - action: locks
    tables: all
```

Supported MySQL aliases:
- `insert`
- `create_table`
- `drop_table`
- `ddl`
- `locks`
- `all`

### Combined configuration example
Keep operational logic in YAML, and secrets/urgent overrides in env:

```bash
MODE=once go run . --config ./examples/config.yaml
```

Deprecated env override example:

```bash
TABLE_MAP='^tmp_:^audit_' go run . --config-strategy env-only
```

## 🗂️ CLI usage
```bash
go run . --input ./dump.tar.gz --output ./output/filtered_result.tar.gz --skip '^tmp_:^log_'
```

`--skip` remains a CLI alias for legacy pattern filtering. Prefer `filterRules` in file configs for new setups.

Print build metadata:

```bash
go run . --version
go run ./cmd/dumpgen --version
```

Useful flags:
- `--db-driver mysql`
- `--mode once|schedule`
- `--every 30m`
- `--max-line-bytes 16777216`
- `--report-file ./output/report.json`
- `--s3-request-timeout 30s`
- `--s3-retry-max-attempts 3`

### Runtime report
Set `reportFile` in file config, `REPORT_FILE` in env, or `--report-file` in CLI to write a JSON report for each run. This works with local paths, `file://`, and `s3://bucket/key`.

The report includes overall counters and per-file stats, which is especially useful for multi-file `.tar.gz` inputs.

## Bitrix example
For excluding temporary/service Bitrix tables from a dump, use the ready example config:

```bash
go run . --config ./examples/config.bitrix-temp.yaml
```

The shipped pattern set is conservative and targets obvious temporary tables such as `tmp_*`, `b_*_tmp`, `b_*_temp`, and several search/import rebuild tables.


## 🧪 Large dump generator (1GB)
For stress testing in near-real conditions, use the built-in generator:

```bash
go run ./cmd/dumpgen --output ./data/generated_dump_1gb.tar.gz --target-size 1GB --tables users,orders,events
```

This creates a `.tar.gz` archive with `dump.sql` inside, containing multiple `CREATE TABLE` and batched `INSERT` statements until the SQL payload reaches the target size. Payload values are random alphanumeric strings (not constant `x`).

Useful tuning flags:
- `--tables` (comma-separated list, e.g. `users,orders,events`)
- `--table` (legacy alias for single table)
- `--rows-per-insert` (default `1000`)
- `--value-size` (default `128`)
- `--seed` (optional random seed for reproducible data)
- `--target-size` supports `MB/GB` and `MiB/GiB`

## Tests
Default test run:

```bash
go test ./...
```

Optional MinIO integration smoke test:

```bash
RUN_MINIO_TESTS=1 MINIO_ENDPOINT=http://127.0.0.1:9000 go test ./internal/pipeline -run TestRunSupportsS3SQLWithMinIO
```

Additional format-specific MinIO smoke tests:

```bash
RUN_MINIO_TESTS=1 MINIO_ENDPOINT=http://127.0.0.1:9000 go test ./internal/pipeline -run 'TestRunSupportsS3(SQLGZ|TarGZ)WithMinIO'
```

Defaults for the integration test match the bundled `docker-compose.yml` MinIO service:
- `MINIO_ENDPOINT=http://127.0.0.1:9000`
- `MINIO_REGION=us-east-1`
- `MINIO_ROOT_USER=minioadmin`
- `MINIO_ROOT_PASSWORD=minioadmin`

## Releases
GitHub Releases are built with GoReleaser from tags matching `v*`.

Published artifacts:
- `mysql-dump-cleaner` for `linux`, `darwin`, and `windows`
- `dumpgen` for `linux`, `darwin`, and `windows`
- `checksums.txt`

Supported architectures:
- `amd64`
- `arm64`

Create a release:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Local snapshot build:

```bash
goreleaser release --snapshot --clean
```

## 🐳 Run in Docker (scheduler + standalone S3 service)
1. Fill `.env`.
2. Start stack:

```bash
docker compose up --build -d
```

`docker-compose.yml` starts:
- `cleaner` (scheduler in container)
- `minio` (separate S3-compatible service for backup infrastructure)

## 🖥️ Run as system scheduler
Systemd units are provided in `deploy/systemd/`:
- `mysql-dump-cleaner.service`
- `mysql-dump-cleaner.timer`

Example:
```bash
sudo cp deploy/systemd/mysql-dump-cleaner.{service,timer} /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now mysql-dump-cleaner.timer
```

## 🔭 Roadmap
✅ Current: Basic filtering for MySQL-compatible dumps.

### 🛠️ Planned / progress
- ✅ Make a smaller CLI app with runtime flags.
- ✅ Add configuration validation (required fields, values, regex validation).
- ✅ Add strategy-based configuration loading from yaml/toml/json/conf.
- ✅ Add combined config mode (file + env + CLI).
- ✅ Extend platform usage: local, container scheduler, system scheduler.
- ✅ Make utility ready for use inside Docker containers.
- ✅ Add flexible run modes with dynamic scheduling configuration.
- ⏳ Refactor deeper into reusable packages.
- ⏳ Support other SQL dialects (PostgreSQL, MSSQL, etc).
- ⏳ Support more dump formats (plain SQL, CSV, binary).


## ✅ Task итог (pre-merge summary)
What is implemented in this iteration:
- Refactored runtime into explicit packages (`internal/app`, `internal/config`, `internal/pipeline`, `internal/filter`) and reduced `main.go` to bootstrap + graceful shutdown.
- Added schedule mode (`mode: schedule` in file config, `SCHEDULE_EVERY` in env) and runtime flags for operational control.
- Added strategy-based layered config (defaults -> file -> env -> CLI) with format support: YAML/TOML/JSON/CONF.
- Added stress dump generator `cmd/dumpgen` with 1GB-scale targets, multi-table generation, random payloads, and deterministic seed support.
- Added deployment artifacts for container and system scheduler (`Dockerfile`, `docker-compose.yml`, `deploy/systemd/*`).
- Added/updated tests for config/filter/generator behavior.

Current limitation to keep in mind before merge:
- The runtime still materializes extracted data in local `tmpDir`; `s3://` support does not make processing fully streaming.

Recommended next PR:
- Refactor the filter backend into a shared engine plus driver-specific SQL backends.

## 📜 License
MIT.
