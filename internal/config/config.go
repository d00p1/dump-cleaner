package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/d00p1/filtrate-backups/internal/filter"
	"github.com/joho/godotenv"
)

type FilterRule struct {
	Action string
	Tables TableSelector
}

type TableSelector struct {
	All      bool
	Patterns []string
}

type Config struct {
	Input            string
	Output           string
	TablesSkipRaw    string
	TmpDir           string
	MaxLineBytes     int
	ScheduleInterval time.Duration
	Mode             string
	DBDriver         string
	ReportFile       string
	S3Endpoint       string
	S3Region         string
	S3RequestTimeout time.Duration
	S3RetryMax       int
	S3AccessKey      string
	S3SecretKey      string
	S3SessionToken   string
	S3ForcePathStyle bool
	S3Insecure       bool
	TablesSkip       []string
	FilterRules      []FilterRule
	CompiledRules    []filter.Rule
	Warnings         []string
}

const tableMapDeprecatedWarning = "TABLE_MAP is deprecated; use filterRules in config files"

type bootstrapOptions struct {
	ConfigPath     string
	ConfigFormat   string
	ConfigStrategy string
}

func Load(args []string) (Config, error) {
	_ = godotenv.Load("./.env")

	boot, err := parseBootstrap(args)
	if err != nil {
		return Config{}, err
	}

	cfg := defaultConfig()

	if boot.ConfigPath != "" {
		strategy, err := ResolveStrategy(boot.ConfigFormat, boot.ConfigPath)
		if err != nil {
			return Config{}, err
		}
		fileValues, err := strategy.Load(boot.ConfigPath)
		if err != nil {
			return Config{}, fmt.Errorf("load config file: %w", err)
		}
		if err := cfg.applyFileConfig(fileValues); err != nil {
			return Config{}, err
		}
	}

	if boot.ConfigStrategy == "merge" || boot.ConfigStrategy == "env-only" {
		cfg.applyKeyValues(readKnownEnv())
	}

	if err := applyCLIOverrides(args, &cfg); err != nil {
		return Config{}, err
	}

	cfg.TablesSkip = splitPatterns(cfg.TablesSkipRaw)
	if err := validate(cfg); err != nil {
		return Config{}, err
	}

	compiledRules, err := buildCompiledRules(cfg)
	if err != nil {
		return Config{}, err
	}
	cfg.CompiledRules = compiledRules
	if cfg.TablesSkipRaw != "" && len(cfg.FilterRules) == 0 {
		cfg.Warnings = append(cfg.Warnings, tableMapDeprecatedWarning)
	}

	return cfg, nil
}

func parseBootstrap(args []string) (bootstrapOptions, error) {
	boot := bootstrapOptions{ConfigFormat: "auto", ConfigStrategy: "merge"}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		next := func() string {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}

		switch {
		case arg == "--config":
			boot.ConfigPath = next()
		case strings.HasPrefix(arg, "--config="):
			boot.ConfigPath = strings.TrimPrefix(arg, "--config=")
		case arg == "--config-format":
			boot.ConfigFormat = next()
		case strings.HasPrefix(arg, "--config-format="):
			boot.ConfigFormat = strings.TrimPrefix(arg, "--config-format=")
		case arg == "--config-strategy":
			boot.ConfigStrategy = next()
		case strings.HasPrefix(arg, "--config-strategy="):
			boot.ConfigStrategy = strings.TrimPrefix(arg, "--config-strategy=")
		}
	}

	if boot.ConfigStrategy != "merge" && boot.ConfigStrategy != "file-only" && boot.ConfigStrategy != "env-only" {
		return bootstrapOptions{}, fmt.Errorf("invalid --config-strategy=%q", boot.ConfigStrategy)
	}
	if boot.ConfigStrategy == "file-only" && boot.ConfigPath == "" {
		return bootstrapOptions{}, errors.New("--config is required when --config-strategy=file-only")
	}
	return boot, nil
}

func applyCLIOverrides(args []string, cfg *Config) error {
	fs := flag.NewFlagSet("mysql-dump-cleaner", flag.ContinueOnError)
	fs.StringVar(&cfg.Input, "input", cfg.Input, "input dump path")
	fs.StringVar(&cfg.Output, "output", cfg.Output, "output archive path")
	fs.StringVar(&cfg.TablesSkipRaw, "skip", cfg.TablesSkipRaw, "colon-separated regex list of tables to remove")
	fs.StringVar(&cfg.TmpDir, "tmp-dir", cfg.TmpDir, "tmp directory")
	fs.IntVar(&cfg.MaxLineBytes, "max-line-bytes", cfg.MaxLineBytes, "max bytes per SQL line")
	fs.DurationVar(&cfg.ScheduleInterval, "every", cfg.ScheduleInterval, "run as scheduler with interval, e.g. 30m")
	fs.StringVar(&cfg.Mode, "mode", cfg.Mode, "run mode: once or schedule")
	fs.StringVar(&cfg.DBDriver, "db-driver", cfg.DBDriver, "database driver aliases: mysql")
	fs.StringVar(&cfg.ReportFile, "report-file", cfg.ReportFile, "write JSON runtime report to file or URI")
	fs.DurationVar(&cfg.S3RequestTimeout, "s3-request-timeout", cfg.S3RequestTimeout, "per-request timeout for S3 operations, e.g. 30s")
	fs.IntVar(&cfg.S3RetryMax, "s3-retry-max-attempts", cfg.S3RetryMax, "maximum S3 retry attempts")

	var ignored string
	fs.StringVar(&ignored, "config", "", "")
	fs.StringVar(&ignored, "config-format", "", "")
	fs.StringVar(&ignored, "config-strategy", "", "")

	return fs.Parse(args)
}

func (cfg *Config) applyFileConfig(loaded fileConfig) error {
	if loaded.Input != "" {
		cfg.Input = loaded.Input
	}
	if loaded.Output != "" {
		cfg.Output = loaded.Output
	}
	if loaded.TmpDir != "" {
		cfg.TmpDir = loaded.TmpDir
	}
	if loaded.MaxLineBytes > 0 {
		cfg.MaxLineBytes = loaded.MaxLineBytes
	}
	if loaded.Mode != "" {
		cfg.Mode = strings.ToLower(loaded.Mode)
	}
	if loaded.Schedule != "" {
		d, err := time.ParseDuration(loaded.Schedule)
		if err != nil {
			return fmt.Errorf("invalid scheduleEvery %q: %w", loaded.Schedule, err)
		}
		cfg.ScheduleInterval = d
	}
	if loaded.DBDriver != "" {
		cfg.DBDriver = strings.ToLower(strings.TrimSpace(loaded.DBDriver))
	}
	if loaded.ReportFile != "" {
		cfg.ReportFile = loaded.ReportFile
	}
	if loaded.S3Timeout != "" {
		d, err := time.ParseDuration(loaded.S3Timeout)
		if err != nil {
			return fmt.Errorf("invalid s3RequestTimeout %q: %w", loaded.S3Timeout, err)
		}
		cfg.S3RequestTimeout = d
	}
	if loaded.S3RetryMax > 0 {
		cfg.S3RetryMax = loaded.S3RetryMax
	}
	if loaded.S3Endpoint != "" {
		cfg.S3Endpoint = loaded.S3Endpoint
	}
	if loaded.S3Region != "" {
		cfg.S3Region = loaded.S3Region
	}
	if loaded.S3AccessKey != "" {
		cfg.S3AccessKey = loaded.S3AccessKey
	}
	if loaded.S3SecretKey != "" {
		cfg.S3SecretKey = loaded.S3SecretKey
	}
	if loaded.S3SessionTok != "" {
		cfg.S3SessionToken = loaded.S3SessionTok
	}
	if loaded.S3PathStyle != nil {
		cfg.S3ForcePathStyle = *loaded.S3PathStyle
	}
	if loaded.S3Insecure != nil {
		cfg.S3Insecure = *loaded.S3Insecure
	}

	if len(loaded.FilterRules) > 0 {
		rules, err := normalizeFilterRules(loaded.FilterRules)
		if err != nil {
			return err
		}
		cfg.FilterRules = rules
	}

	return nil
}

func (cfg *Config) applyKeyValues(values map[string]string) {
	for key, value := range values {
		norm := normalizeKey(key)
		switch norm {
		case "DUMPFILE", "INPUT":
			if value != "" {
				cfg.Input = value
			}
		case "OUTPUT_FILE", "OUTPUT", "OUTPUT_DIR":
			if value != "" {
				cfg.Output = value
			}
		case "TABLE_MAP", "TABLES_SKIP", "SKIP", "SKIP_TABLES":
			cfg.TablesSkipRaw = normalizePatterns(value)
		case "TMP_DIR", "TMPDIR":
			if value != "" {
				cfg.TmpDir = value
			}
		case "MAX_LINE_BYTES", "TOKEN_SIZE":
			if parsed, err := parseInt(value); err == nil && parsed > 0 {
				cfg.MaxLineBytes = parsed
			}
		case "MODE":
			if value != "" {
				cfg.Mode = strings.ToLower(value)
			}
		case "SCHEDULE_EVERY", "EVERY", "INTERVAL":
			if d, err := time.ParseDuration(value); err == nil {
				cfg.ScheduleInterval = d
			}
		case "DB_DRIVER":
			if value != "" {
				cfg.DBDriver = strings.ToLower(strings.TrimSpace(value))
			}
		case "REPORT_FILE":
			if value != "" {
				cfg.ReportFile = strings.TrimSpace(value)
			}
		case "S3_REQUEST_TIMEOUT":
			if d, err := time.ParseDuration(value); err == nil {
				cfg.S3RequestTimeout = d
			}
		case "S3_RETRY_MAX_ATTEMPTS":
			if parsed, err := parseInt(value); err == nil && parsed > 0 {
				cfg.S3RetryMax = parsed
			}
		case "S3_ENDPOINT":
			if value != "" {
				cfg.S3Endpoint = strings.TrimSpace(value)
			}
		case "S3_REGION":
			if value != "" {
				cfg.S3Region = strings.TrimSpace(value)
			}
		case "S3_ACCESS_KEY":
			if value != "" {
				cfg.S3AccessKey = value
			}
		case "S3_SECRET_KEY":
			if value != "" {
				cfg.S3SecretKey = value
			}
		case "S3_SESSION_TOKEN":
			if value != "" {
				cfg.S3SessionToken = value
			}
		case "S3_FORCE_PATH_STYLE":
			if parsed, err := parseBool(value); err == nil {
				cfg.S3ForcePathStyle = parsed
			}
		case "S3_INSECURE":
			if parsed, err := parseBool(value); err == nil {
				cfg.S3Insecure = parsed
			}
		}
	}
}

func defaultConfig() Config {
	return Config{
		Output:           "./output/filtered_result.tar.gz",
		TmpDir:           "./tmp",
		MaxLineBytes:     8 * 1024 * 1024,
		ScheduleInterval: 0,
		Mode:             "once",
		DBDriver:         "mysql",
		S3Region:         "us-east-1",
		S3RetryMax:       3,
		S3ForcePathStyle: true,
	}
}

func parseBool(v string) (bool, error) {
	return strconv.ParseBool(strings.TrimSpace(v))
}

func splitPatterns(raw string) []string {
	if raw == "" {
		return nil
	}
	raw = strings.ReplaceAll(raw, ",", ":")
	parts := strings.Split(raw, ":")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.Trim(strings.TrimSpace(p), "\"'")
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func normalizePatterns(v string) string {
	clean := strings.TrimSpace(v)
	clean = strings.TrimPrefix(clean, "[")
	clean = strings.TrimSuffix(clean, "]")
	clean = strings.ReplaceAll(clean, "\"", "")
	clean = strings.ReplaceAll(clean, "'", "")
	clean = strings.ReplaceAll(clean, ",", ":")
	return clean
}

func normalizeFilterRules(rawRules []rawFilterRule) ([]FilterRule, error) {
	rules := make([]FilterRule, 0, len(rawRules))
	for _, raw := range rawRules {
		action := strings.ToLower(strings.TrimSpace(raw.Action))
		if action == "" {
			return nil, errors.New("filterRules[].action is required")
		}

		tables, err := normalizeTableSelector(raw.Tables)
		if err != nil {
			return nil, fmt.Errorf("filterRules action %q: %w", action, err)
		}

		rules = append(rules, FilterRule{Action: action, Tables: tables})
	}
	return rules, nil
}

func normalizeTableSelector(value any) (TableSelector, error) {
	switch v := value.(type) {
	case string:
		if strings.EqualFold(strings.TrimSpace(v), "all") {
			return TableSelector{All: true}, nil
		}
		return TableSelector{}, errors.New("tables must be a list of regex patterns or scalar 'all'")
	case []string:
		return TableSelector{Patterns: trimPatterns(v)}, nil
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, fmt.Sprint(item))
		}
		return TableSelector{Patterns: trimPatterns(parts)}, nil
	case nil:
		return TableSelector{}, errors.New("tables is required")
	default:
		return TableSelector{}, fmt.Errorf("unsupported tables type %T", value)
	}
}

func trimPatterns(patterns []string) []string {
	result := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		trimmed := strings.Trim(strings.TrimSpace(pattern), "\"'")
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func buildCompiledRules(cfg Config) ([]filter.Rule, error) {
	defs := make([]filter.RuleDefinition, 0, len(cfg.FilterRules)+2)
	if len(cfg.FilterRules) > 0 {
		for _, rule := range cfg.FilterRules {
			defs = append(defs, filter.RuleDefinition{
				Action:    rule.Action,
				AllTables: rule.Tables.All,
				Tables:    append([]string(nil), rule.Tables.Patterns...),
			})
		}
	} else if len(cfg.TablesSkip) > 0 {
		defs = append(defs,
			filter.RuleDefinition{Action: "ddl", Tables: append([]string(nil), cfg.TablesSkip...)},
			filter.RuleDefinition{Action: "insert", Tables: append([]string(nil), cfg.TablesSkip...)},
		)
	}

	if len(defs) == 0 {
		return nil, nil
	}

	return filter.CompileRules(cfg.DBDriver, defs)
}

func validate(cfg Config) error {
	var allErrs []error

	if cfg.Input == "" {
		allErrs = append(allErrs, errors.New("dumpFile is required in config files, DUMPFILE in env, or --input via CLI"))
	}
	if cfg.Mode != "once" && cfg.Mode != "schedule" {
		allErrs = append(allErrs, fmt.Errorf("mode must be once or schedule, got %q", cfg.Mode))
	}
	if cfg.MaxLineBytes < 1024 {
		allErrs = append(allErrs, errors.New("maxLineBytes must be >= 1024"))
	}
	if cfg.Mode == "schedule" && cfg.ScheduleInterval <= 0 {
		allErrs = append(allErrs, errors.New("scheduleEvery is required for schedule mode, or SCHEDULE_EVERY/--every"))
	}
	if err := ensureDir(cfg.TmpDir); err != nil {
		allErrs = append(allErrs, fmt.Errorf("tmpDir error: %w", err))
	}
	if cfg.DBDriver == "" {
		allErrs = append(allErrs, errors.New("dbDriver is required in config files, DB_DRIVER in env, or --db-driver via CLI"))
	}
	if cfg.S3RequestTimeout < 0 {
		allErrs = append(allErrs, errors.New("s3RequestTimeout must be >= 0"))
	}
	if cfg.S3RetryMax < 1 {
		allErrs = append(allErrs, errors.New("s3RetryMaxAttempts must be >= 1"))
	}
	for _, pat := range cfg.TablesSkip {
		if _, err := regexp.Compile(pat); err != nil {
			allErrs = append(allErrs, fmt.Errorf("invalid deprecated TABLE_MAP pattern %q: %w", pat, err))
		}
	}
	for _, rule := range cfg.FilterRules {
		if !rule.Tables.All && len(rule.Tables.Patterns) == 0 {
			allErrs = append(allErrs, fmt.Errorf("filterRules action %q must define tables or use 'all'", rule.Action))
		}
		for _, pat := range rule.Tables.Patterns {
			if _, err := regexp.Compile(pat); err != nil {
				allErrs = append(allErrs, fmt.Errorf("invalid filterRules pattern %q: %w", pat, err))
			}
		}
	}

	return errors.Join(allErrs...)
}

func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	return nil
}
