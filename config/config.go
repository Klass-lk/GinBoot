package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Supported Syntax Patterns for Environment Variable Injection in YAML:
// 1. ${VAR_NAME:default_value}  -> Braced with default value (e.g., ${SERVICE_USER_URL:http://localhost:8081})
// 2. ${VAR_NAME}                -> Braced without default value (e.g., ${DATABASE_URL})
// 3. env(VAR_NAME, default)     -> Function style (e.g., env(PORT, 8080))
// 4. $VAR_NAME                  -> Simple prefix style (e.g., $SERVICE_USER_URL)

var (
	envBracedPattern = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)(?::([^}]*))?\}`)
	envFuncPattern   = regexp.MustCompile(`env\(\s*([A-Za-z0-9_]+)\s*(?:,\s*([^)]*))?\s*\)`)
	envSimplePattern = regexp.MustCompile(`\$([A-Za-z0-9_]+)`)
)

// ServerConfig options for Ginboot Server
type ServerConfig struct {
	Port     int    `yaml:"port"`
	BasePath string `yaml:"base-path"`
	Env      string `yaml:"env"`
}

// ServiceTargetConfig options for a target downstream service
type ServiceTargetConfig struct {
	URL      string        `yaml:"url"`
	Protocol string        `yaml:"protocol"`
	Timeout  time.Duration `yaml:"timeout"`
}

// DBConfig options for Database connections
type DBConfig struct {
	Driver       string `yaml:"driver"`
	URL          string `yaml:"url"`
	MaxOpenConns int    `yaml:"max-open-conns"`
	MaxIdleConns int    `yaml:"max-idle-conns"`
}

// TelemetryConfig options for OpenTelemetry & Logging
type TelemetryConfig struct {
	Enabled            bool   `yaml:"enabled"`
	ServiceName        string `yaml:"service-name"`
	ServiceVersion     string `yaml:"service-version"`
	Environment        string `yaml:"environment"`
	Exporter           string `yaml:"exporter"`
	Endpoint           string `yaml:"endpoint"`
	Headers            string `yaml:"headers"`
	Protocol           string `yaml:"protocol"`
	ResourceAttributes string `yaml:"resource-attributes"`
}

// OpenAPIConfig controls whether a server serves its own specification, and to
// whom.
//
// A specification is a complete description of an application's attack surface —
// every route, every parameter, every shape it accepts — so exposing one is a
// decision rather than a default worth assuming.
type OpenAPIConfig struct {
	// Access is one of "public", "token" or "disabled". Empty means public,
	// which is what a developer running locally wants; a deployment that cares
	// sets it explicitly.
	Access string `yaml:"access"`
	// Token is the shared secret required when Access is "token". Access is
	// treated as disabled when this is empty, so a half-finished configuration
	// cannot publish the specification by accident.
	Token string `yaml:"token"`
	// Path the specification is served at. Defaults to /openapi.json.
	Path string `yaml:"path"`
}

// GinbootRootConfig is the root configuration structure
type GinbootRootConfig struct {
	Server    ServerConfig                   `yaml:"server"`
	Services  map[string]ServiceTargetConfig `yaml:"services"`
	DB        DBConfig                       `yaml:"db"`
	Telemetry TelemetryConfig                `yaml:"telemetry"`
	OpenAPI   OpenAPIConfig                  `yaml:"openapi"`
}

// Config wraps the root ginboot configuration block
type Config struct {
	Ginboot GinbootRootConfig `yaml:"ginboot"`
}

// ExpandEnvVars expands all environment variable syntax variations in YAML content.
func ExpandEnvVars(content string) string {
	// 1. Expand function style env(VAR, default)
	res := envFuncPattern.ReplaceAllStringFunc(content, func(match string) string {
		submatches := envFuncPattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		varName := strings.TrimSpace(submatches[1])
		defaultValue := ""
		if len(submatches) >= 3 {
			defaultValue = strings.TrimSpace(submatches[2])
		}
		if val, exists := os.LookupEnv(varName); exists && val != "" {
			return val
		}
		return defaultValue
	})

	// 2. Expand braced style ${VAR:default} or ${VAR}
	res = envBracedPattern.ReplaceAllStringFunc(res, func(match string) string {
		submatches := envBracedPattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		varName := submatches[1]
		defaultValue := ""
		if len(submatches) >= 3 {
			defaultValue = submatches[2]
		}
		if val, exists := os.LookupEnv(varName); exists && val != "" {
			return val
		}
		return defaultValue
	})

	// 3. Expand simple style $VAR_NAME (only if not preceded by backslash)
	res = envSimplePattern.ReplaceAllStringFunc(res, func(match string) string {
		submatches := envSimplePattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		varName := submatches[1]
		if val, exists := os.LookupEnv(varName); exists && val != "" {
			return val
		}
		return ""
	})

	return res
}

// ApplyEnvironmentOverrides applies explicit environment variable overrides
// to fields if they are present in environment variables.
func (cfg *Config) ApplyEnvironmentOverrides() {
	// 1. Server overrides
	if portStr := getFirstEnv("GINBOOT_SERVER_PORT", "PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			cfg.Ginboot.Server.Port = p
		}
	}
	if bp := getFirstEnv("GINBOOT_SERVER_BASE_PATH", "BASE_PATH"); bp != "" {
		cfg.Ginboot.Server.BasePath = bp
	}

	// 2. DB overrides
	if dbURL := getFirstEnv("DATABASE_URL", "GINBOOT_DB_URL"); dbURL != "" {
		cfg.Ginboot.DB.URL = dbURL
	}
	if dbDriver := getFirstEnv("DATABASE_DRIVER", "GINBOOT_DB_DRIVER"); dbDriver != "" {
		cfg.Ginboot.DB.Driver = dbDriver
	}

	// 3. Telemetry overrides
	if otelEndpoint := getFirstEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "GINBOOT_TELEMETRY_ENDPOINT"); otelEndpoint != "" {
		cfg.Ginboot.Telemetry.Endpoint = otelEndpoint
	}
	if otelHeaders := getFirstEnv("OTEL_EXPORTER_OTLP_HEADERS", "GINBOOT_TELEMETRY_HEADERS"); otelHeaders != "" {
		cfg.Ginboot.Telemetry.Headers = otelHeaders
	}
	if otelProto := getFirstEnv("OTEL_EXPORTER_OTLP_PROTOCOL", "GINBOOT_TELEMETRY_PROTOCOL"); otelProto != "" {
		cfg.Ginboot.Telemetry.Protocol = otelProto
	}
	if otelResAttrs := getFirstEnv("OTEL_RESOURCE_ATTRIBUTES", "GINBOOT_TELEMETRY_RESOURCE_ATTRIBUTES"); otelResAttrs != "" {
		cfg.Ginboot.Telemetry.ResourceAttributes = otelResAttrs
	}
	if svcName := getFirstEnv("OTEL_SERVICE_NAME", "GINBOOT_TELEMETRY_SERVICE_NAME"); svcName != "" {
		cfg.Ginboot.Telemetry.ServiceName = svcName
	}

	// 4. OpenAPI overrides. The environment wins because the platform hosting an
	// application decides this, not a file committed to its repository.
	if access := os.Getenv("GINBOOT_OPENAPI_ACCESS"); access != "" {
		cfg.Ginboot.OpenAPI.Access = access
	}
	if token := os.Getenv("GINBOOT_OPENAPI_TOKEN"); token != "" {
		cfg.Ginboot.OpenAPI.Token = token
	}
	if path := os.Getenv("GINBOOT_OPENAPI_PATH"); path != "" {
		cfg.Ginboot.OpenAPI.Path = path
	}
}

func getFirstEnv(keys ...string) string {
	for _, key := range keys {
		if val := os.Getenv(key); val != "" {
			return val
		}
	}
	return ""
}

// LoadDotEnv automatically loads environment variables from .env, .env.local, or .env.development files if present.
func LoadDotEnv(filenames ...string) {
	if len(filenames) == 0 {
		filenames = []string{".env", ".env.local", ".env.development"}
	}

	for _, filename := range filenames {
		data, err := os.ReadFile(filename)
		if err != nil {
			continue
		}

		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}

			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) != 2 {
				continue
			}

			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			val = strings.Trim(val, `"'`)

			// Only set if not already set in process environment
			if _, exists := os.LookupEnv(key); !exists {
				_ = os.Setenv(key, val)
			}
		}
	}
}

// LoadConfig loads and unmarshals configuration from a YAML file.
// Automatically loads .env files beforehand.
// If path is empty, it searches for ginboot.yml, application.yml, ginboot.yaml, or application.yaml in current directory.
func LoadConfig(path string) (*Config, error) {
	// Auto-load .env files by default
	LoadDotEnv()

	if path == "" {
		candidates := []string{"ginboot.yml", "application.yml", "ginboot.yaml", "application.yaml"}
		for _, cand := range candidates {
			if _, err := os.Stat(cand); err == nil {
				path = cand
				break
			}
		}
	}

	if path == "" {
		// Return empty default config if no config file is present
		cfg := &Config{
			Ginboot: GinbootRootConfig{
				Server:   ServerConfig{Port: 8080},
				Services: make(map[string]ServiceTargetConfig),
			},
		}
		cfg.ApplyEnvironmentOverrides()
		return cfg, nil
	}

	cleanPath := filepath.Clean(path)
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file at %s: %w", path, err)
	}

	expanded := ExpandEnvVars(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse yaml config at %s: %w", path, err)
	}

	if cfg.Ginboot.Services == nil {
		cfg.Ginboot.Services = make(map[string]ServiceTargetConfig)
	}

	cfg.ApplyEnvironmentOverrides()
	return &cfg, nil
}
