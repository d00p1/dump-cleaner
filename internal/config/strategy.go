package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

type fileConfig struct {
	Input        string
	Output       string
	TmpDir       string
	MaxLineBytes int
	Mode         string
	Schedule     string
	DBDriver     string
	ReportFile   string
	S3Endpoint   string
	S3Region     string
	S3Timeout    string
	S3RetryMax   int
	S3AccessKey  string
	S3SecretKey  string
	S3SessionTok string
	S3PathStyle  *bool
	S3Insecure   *bool
	FilterRules  []rawFilterRule
}

type rawFilterRule struct {
	Action string `json:"action" yaml:"action" toml:"action"`
	Tables any    `json:"tables" yaml:"tables" toml:"tables"`
}

type LoadStrategy interface {
	Load(path string) (fileConfig, error)
}

type strategyFunc func(path string) (fileConfig, error)

func (f strategyFunc) Load(path string) (fileConfig, error) {
	return f(path)
}

func ResolveStrategy(format, path string) (LoadStrategy, error) {
	if format == "auto" {
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".yaml", ".yml":
			format = "yaml"
		case ".toml":
			format = "toml"
		case ".json":
			format = "json"
		case ".conf", ".cfg", ".ini":
			format = "conf"
		default:
			return nil, fmt.Errorf("unable to detect config format for %q", path)
		}
	}

	switch strings.ToLower(format) {
	case "yaml", "yml":
		return strategyFunc(loadYAML), nil
	case "toml":
		return strategyFunc(loadTOML), nil
	case "json":
		return strategyFunc(loadJSON), nil
	case "conf", "cfg", "ini":
		return strategyFunc(loadCONF), nil
	default:
		return nil, fmt.Errorf("unsupported config format: %s", format)
	}
}

func readKnownEnv() map[string]string {
	keys := []string{"DUMPFILE", "OUTPUT_FILE", "TABLE_MAP", "TMP_DIR", "MAX_LINE_BYTES", "MODE", "SCHEDULE_EVERY", "DB_DRIVER", "REPORT_FILE", "S3_ENDPOINT", "S3_REGION", "S3_REQUEST_TIMEOUT", "S3_RETRY_MAX_ATTEMPTS", "S3_ACCESS_KEY", "S3_SECRET_KEY", "S3_SESSION_TOKEN", "S3_FORCE_PATH_STYLE", "S3_INSECURE"}
	res := make(map[string]string, len(keys))
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok && strings.TrimSpace(v) != "" {
			res[k] = v
		}
	}
	return res
}

func parseInt(v string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(v))
}

var camelBoundaryRE = regexp.MustCompile(`([a-z0-9])([A-Z])`)

func normalizeKey(k string) string {
	k = strings.TrimSpace(k)
	k = camelBoundaryRE.ReplaceAllString(k, `${1}_${2}`)
	k = strings.ToUpper(k)
	k = strings.ReplaceAll(k, ".", "_")
	k = strings.ReplaceAll(k, "-", "_")
	return k
}

func loadJSON(path string) (fileConfig, error) {
	// #nosec G304 -- config path is user-specified via CLI
	data, err := os.ReadFile(path)
	if err != nil {
		return fileConfig{}, err
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fileConfig{}, err
	}
	return decodeStructuredConfig(raw)
}

func loadYAML(path string) (fileConfig, error) {
	// #nosec G304 -- config path is user-specified via CLI
	data, err := os.ReadFile(path)
	if err != nil {
		return fileConfig{}, err
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fileConfig{}, err
	}
	return decodeStructuredConfig(raw)
}

func loadTOML(path string) (fileConfig, error) {
	var raw map[string]any
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return fileConfig{}, err
	}
	return decodeStructuredConfig(raw)
}

func loadCONF(path string) (fileConfig, error) {
	// #nosec G304 -- config path is user-specified via CLI
	file, err := os.Open(path)
	if err != nil {
		return fileConfig{}, err
	}
	defer file.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.Trim(strings.TrimSpace(line[idx+1:]), "\"'")
		values[key] = val
	}
	if err := scanner.Err(); err != nil {
		return fileConfig{}, err
	}

	raw := make(map[string]any, len(values))
	for key, value := range values {
		raw[key] = value
	}

	return decodeStructuredFileConfig(raw)
}

func decodeStructuredConfig(raw map[string]any) (fileConfig, error) {
	return decodeStructuredFileConfig(raw)
}

func decodeStructuredFileConfig(raw map[string]any) (fileConfig, error) {
	var cfg fileConfig
	for key, value := range raw {
		switch key {
		case "dumpFile":
			cfg.Input = stringify(value)
		case "outputFile":
			cfg.Output = stringify(value)
		case "tmpDir":
			cfg.TmpDir = stringify(value)
		case "maxLineBytes":
			parsed, err := anyToInt(value)
			if err != nil {
				return fileConfig{}, fmt.Errorf("invalid %s: %w", key, err)
			}
			cfg.MaxLineBytes = parsed
		case "mode":
			cfg.Mode = stringify(value)
		case "scheduleEvery":
			cfg.Schedule = stringify(value)
		case "dbDriver":
			cfg.DBDriver = stringify(value)
		case "reportFile":
			cfg.ReportFile = stringify(value)
		case "s3Endpoint":
			cfg.S3Endpoint = stringify(value)
		case "s3Region":
			cfg.S3Region = stringify(value)
		case "s3RequestTimeout":
			cfg.S3Timeout = stringify(value)
		case "s3RetryMaxAttempts":
			parsed, err := anyToInt(value)
			if err != nil {
				return fileConfig{}, fmt.Errorf("invalid %s: %w", key, err)
			}
			cfg.S3RetryMax = parsed
		case "s3AccessKey":
			cfg.S3AccessKey = stringify(value)
		case "s3SecretKey":
			cfg.S3SecretKey = stringify(value)
		case "s3SessionToken":
			cfg.S3SessionTok = stringify(value)
		case "s3ForcePathStyle":
			parsed, err := anyToBool(value)
			if err != nil {
				return fileConfig{}, fmt.Errorf("invalid %s: %w", key, err)
			}
			cfg.S3PathStyle = &parsed
		case "s3Insecure":
			parsed, err := anyToBool(value)
			if err != nil {
				return fileConfig{}, fmt.Errorf("invalid %s: %w", key, err)
			}
			cfg.S3Insecure = &parsed
		case "filterRules":
			rules, err := decodeFilterRules(value)
			if err != nil {
				return fileConfig{}, err
			}
			cfg.FilterRules = rules
		default:
			return fileConfig{}, unsupportedFileConfigKeyError(key)
		}
	}
	return cfg, nil
}

func decodeFilterRules(value any) ([]rawFilterRule, error) {
	var items []any
	switch v := value.(type) {
	case []any:
		items = v
	case []map[string]any:
		items = make([]any, 0, len(v))
		for _, item := range v {
			items = append(items, item)
		}
	default:
		return nil, fmt.Errorf("filterRules must be a list")
	}

	rules := make([]rawFilterRule, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("filterRules items must be objects")
		}

		var rule rawFilterRule
		for key, rawValue := range m {
			switch key {
			case "action":
				rule.Action = stringify(rawValue)
			case "tables":
				rule.Tables = rawValue
			default:
				return nil, fmt.Errorf("unknown filterRules field %q", key)
			}
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

func stringify(value any) string {
	return strings.TrimSpace(fmt.Sprint(value))
}

func anyToInt(value any) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int32:
		return int(v), nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case string:
		return parseInt(v)
	default:
		return 0, fmt.Errorf("unsupported type %T", value)
	}
}

func anyToBool(value any) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		if err != nil {
			return false, err
		}
		return parsed, nil
	default:
		return false, fmt.Errorf("unsupported bool type %T", value)
	}
}

func unsupportedFileConfigKeyError(key string) error {
	if key == "tableMap" || key == "TABLE_MAP" {
		return fmt.Errorf("unsupported config key %q; use \"filterRules\"", key)
	}
	if normalizeKey(key) == key {
		return fmt.Errorf("unsupported config key %q; file configs use camelCase keys", key)
	}
	return fmt.Errorf("unknown config key %q", key)
}
