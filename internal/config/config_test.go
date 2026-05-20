package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func clearKnownEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DUMPFILE", "")
	t.Setenv("OUTPUT_FILE", "")
	t.Setenv("TABLE_MAP", "")
	t.Setenv("TMP_DIR", "")
	t.Setenv("MAX_LINE_BYTES", "")
	t.Setenv("MODE", "")
	t.Setenv("SCHEDULE_EVERY", "")
	t.Setenv("DB_DRIVER", "")
	t.Setenv("REPORT_FILE", "")
	t.Setenv("S3_ENDPOINT", "")
	t.Setenv("S3_REGION", "")
	t.Setenv("S3_REQUEST_TIMEOUT", "")
	t.Setenv("S3_RETRY_MAX_ATTEMPTS", "")
	t.Setenv("S3_ACCESS_KEY", "")
	t.Setenv("S3_SECRET_KEY", "")
	t.Setenv("S3_SESSION_TOKEN", "")
	t.Setenv("S3_FORCE_PATH_STYLE", "")
	t.Setenv("S3_INSECURE", "")
}

func TestLoadFromJSONAndEnvOverride(t *testing.T) {
	clearKnownEnv(t)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	content := `{"dumpFile":"/data/in.tar.gz","outputFile":"/data/out.tar.gz","dbDriver":"mysql","filterRules":[{"action":"locks","tables":"all"}],"mode":"schedule","scheduleEvery":"15m"}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MODE", "once")

	cfg, err := Load([]string{"--config", cfgPath, "--config-format", "json"})
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if cfg.Mode != "once" {
		t.Fatalf("env should override file mode, got %q", cfg.Mode)
	}
	if cfg.Input != "/data/in.tar.gz" {
		t.Fatalf("unexpected input: %s", cfg.Input)
	}
	if len(cfg.FilterRules) != 1 || !cfg.FilterRules[0].Tables.All || cfg.FilterRules[0].Action != "locks" {
		t.Fatalf("unexpected filter rules: %#v", cfg.FilterRules)
	}
}

func TestLoadFromYAMLWithCLIOverride(t *testing.T) {
	clearKnownEnv(t)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := "dumpFile: /yaml/in.tar.gz\noutputFile: /yaml/out.tar.gz\ndbDriver: mysql\nfilterRules:\n  - action: ddl\n    tables:\n      - ^tmp_\nmode: once\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load([]string{"--config", cfgPath, "--input", "/cli/in.tar.gz"})
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.Input != "/cli/in.tar.gz" {
		t.Fatalf("cli override failed, got %s", cfg.Input)
	}
	if len(cfg.CompiledRules) != 1 {
		t.Fatalf("expected compiled rules, got %d", len(cfg.CompiledRules))
	}
}

func TestLoadFromTOMLWithCamelCaseKeys(t *testing.T) {
	clearKnownEnv(t)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := "dumpFile = \"/toml/in.tar.gz\"\noutputFile = \"/toml/out.tar.gz\"\ndbDriver = \"mysql\"\nmode = \"once\"\n[[filterRules]]\naction = \"locks\"\ntables = \"all\"\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load([]string{"--config", cfgPath, "--config-format", "toml"})
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if cfg.Input != "/toml/in.tar.gz" {
		t.Fatalf("unexpected input: %s", cfg.Input)
	}
	if len(cfg.FilterRules) != 1 || !cfg.FilterRules[0].Tables.All {
		t.Fatalf("unexpected filter rules: %#v", cfg.FilterRules)
	}
}

func TestLoadS3ConfigFromFileAndEnv(t *testing.T) {
	clearKnownEnv(t)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := "dumpFile: s3://source-bucket/in.tar.gz\noutputFile: s3://target-bucket/out.tar.gz\ndbDriver: mysql\ns3Endpoint: http://minio:9000\ns3Region: us-east-1\ns3AccessKey: file-access\ns3SecretKey: file-secret\ns3ForcePathStyle: false\ns3Insecure: true\nmode: once\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("S3_ACCESS_KEY", "env-access")
	t.Setenv("S3_SECRET_KEY", "env-secret")
	t.Setenv("S3_FORCE_PATH_STYLE", "true")
	t.Setenv("S3_REQUEST_TIMEOUT", "45s")
	t.Setenv("S3_RETRY_MAX_ATTEMPTS", "5")

	cfg, err := Load([]string{"--config", cfgPath})
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if cfg.S3Endpoint != "http://minio:9000" {
		t.Fatalf("unexpected endpoint: %q", cfg.S3Endpoint)
	}
	if cfg.S3AccessKey != "env-access" || cfg.S3SecretKey != "env-secret" {
		t.Fatalf("expected env S3 credentials to override file config")
	}
	if !cfg.S3ForcePathStyle {
		t.Fatalf("expected S3_FORCE_PATH_STYLE env override")
	}
	if !cfg.S3Insecure {
		t.Fatalf("expected file s3Insecure to be applied")
	}
	if cfg.S3RequestTimeout.Seconds() != 45 {
		t.Fatalf("expected S3_REQUEST_TIMEOUT env override, got %v", cfg.S3RequestTimeout)
	}
	if cfg.S3RetryMax != 5 {
		t.Fatalf("expected S3_RETRY_MAX_ATTEMPTS env override, got %d", cfg.S3RetryMax)
	}
}

func TestLoadReportFileFromFileAndCLI(t *testing.T) {
	clearKnownEnv(t)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := "dumpFile: ./data/source.sql\noutputFile: ./output/filtered.sql\ndbDriver: mysql\nreportFile: ./reports/from-file.json\nmode: once\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load([]string{"--config", cfgPath, "--report-file", "./reports/from-cli.json"})
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if cfg.ReportFile != "./reports/from-cli.json" {
		t.Fatalf("expected CLI report file override, got %q", cfg.ReportFile)
	}
}

func TestLoadDeprecatedTableMapFromEnv(t *testing.T) {
	clearKnownEnv(t)
	t.Setenv("DUMPFILE", "/env/in.tar.gz")
	t.Setenv("TABLE_MAP", "^tmp_:^log_")
	t.Setenv("DB_DRIVER", "mysql")

	cfg, err := Load([]string{"--config-strategy", "env-only"})
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if len(cfg.TablesSkip) != 2 {
		t.Fatalf("expected 2 legacy patterns, got %d", len(cfg.TablesSkip))
	}
	if len(cfg.CompiledRules) != 2 {
		t.Fatalf("expected legacy fallback rules, got %d", len(cfg.CompiledRules))
	}
	if len(cfg.Warnings) != 1 || cfg.Warnings[0] != tableMapDeprecatedWarning {
		t.Fatalf("unexpected warnings: %#v", cfg.Warnings)
	}
}

func TestRejectUpperCaseFileConfigKeys(t *testing.T) {
	clearKnownEnv(t)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := "DUMPFILE: /yaml/in.tar.gz\noutputFile: /yaml/out.tar.gz\ndbDriver: mysql\nmode: once\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load([]string{"--config", cfgPath})
	if err == nil || !strings.Contains(err.Error(), `unsupported config key "DUMPFILE"`) {
		t.Fatalf("expected upper-case key error, got %v", err)
	}
}

func TestRejectUpperCaseFilterRulesKeyInFileConfig(t *testing.T) {
	clearKnownEnv(t)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := "dumpFile: /yaml/in.tar.gz\noutputFile: /yaml/out.tar.gz\ndbDriver: mysql\nmode: once\nFILTER_RULES:\n  - action: locks\n    tables: all\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load([]string{"--config", cfgPath})
	if err == nil || !strings.Contains(err.Error(), `unsupported config key "FILTER_RULES"`) {
		t.Fatalf("expected FILTER_RULES error, got %v", err)
	}
}

func TestRejectTableMapInFileConfig(t *testing.T) {
	clearKnownEnv(t)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.conf")
	content := "dumpFile=/legacy/in.tar.gz\ntableMap=^tmp_:^log_\ndbDriver=mysql\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load([]string{"--config", cfgPath, "--config-format", "conf"})
	if err == nil || !strings.Contains(err.Error(), `unsupported config key "tableMap"; use "filterRules"`) {
		t.Fatalf("expected tableMap error, got %v", err)
	}
}

func TestRejectUnknownFileConfigKey(t *testing.T) {
	clearKnownEnv(t)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	content := `{"dumpFile":"/data/in.tar.gz","outputFile":"/data/out.tar.gz","dbDriver":"mysql","mode":"once","mysteryKey":true}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load([]string{"--config", cfgPath, "--config-format", "json"})
	if err == nil || !strings.Contains(err.Error(), `unknown config key "mysteryKey"`) {
		t.Fatalf("expected unknown key error, got %v", err)
	}
}

func TestRejectUnknownFilterRuleField(t *testing.T) {
	clearKnownEnv(t)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := "dumpFile = \"/toml/in.tar.gz\"\noutputFile = \"/toml/out.tar.gz\"\ndbDriver = \"mysql\"\nmode = \"once\"\n[[filterRules]]\naction = \"locks\"\ntables = \"all\"\ncomment = \"legacy\"\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load([]string{"--config", cfgPath, "--config-format", "toml"})
	if err == nil || !strings.Contains(err.Error(), `unknown filterRules field "comment"`) {
		t.Fatalf("expected unknown rule field error, got %v", err)
	}
}
