package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandEnvVars_Syntaxes(t *testing.T) {
	t.Setenv("USER_SVC_HOST", "user-service.cloud.internal")
	t.Setenv("AUTH_SVC_PORT", "9000")
	t.Setenv("DB_PASS", "secret123")

	input := `
ginboot:
  server:
    port: env(AUTH_SVC_PORT, 8080)
  services:
    user-service:
      url: http://${USER_SVC_HOST}:8081
    auth-service:
      url: ${MISSING_VAR:http://auth-service.local:9090}
    payment-service:
      url: env(PAYMENT_URL, http://payment-service:8082)
    order-service:
      url: http://$USER_SVC_HOST:8083
  db:
    url: postgres://postgres:${DB_PASS}@localhost:5432/mydb
`
	expanded := ExpandEnvVars(input)

	assert.Contains(t, expanded, "port: 9000")
	assert.Contains(t, expanded, "url: http://user-service.cloud.internal:8081")
	assert.Contains(t, expanded, "url: http://auth-service.local:9090")
	assert.Contains(t, expanded, "url: http://payment-service:8082")
	assert.Contains(t, expanded, "url: http://user-service.cloud.internal:8083")
	assert.Contains(t, expanded, "url: postgres://postgres:secret123@localhost:5432/mydb")
}

func TestLoadDotEnv(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")
	envContent := `
# Comment line
DOTENV_TEST_VAR=hello_from_dotenv
QUOTED_VAR="quoted_val"
SINGLE_QUOTED='single_val'
INVALID_LINE_WITHOUT_EQUALS
EMPTY_VAL=
`
	err := os.WriteFile(envPath, []byte(envContent), 0644)
	require.NoError(t, err)

	LoadDotEnv(envPath)
	assert.Equal(t, "hello_from_dotenv", os.Getenv("DOTENV_TEST_VAR"))
	assert.Equal(t, "quoted_val", os.Getenv("QUOTED_VAR"))
	assert.Equal(t, "single_val", os.Getenv("SINGLE_QUOTED"))
}

func TestApplyEnvironmentOverrides(t *testing.T) {
	t.Setenv("GINBOOT_SERVER_PORT", "4000")
	t.Setenv("GINBOOT_SERVER_BASE_PATH", "/api/v2")
	t.Setenv("GINBOOT_DB_URL", "postgres://prod-db:5432/main")
	t.Setenv("GINBOOT_DB_DRIVER", "postgres")
	t.Setenv("GINBOOT_TELEMETRY_ENDPOINT", "otel-collector:4317")
	t.Setenv("GINBOOT_TELEMETRY_HEADERS", "Auth=Bearer123")
	t.Setenv("GINBOOT_TELEMETRY_PROTOCOL", "http/protobuf")
	t.Setenv("GINBOOT_TELEMETRY_RESOURCE_ATTRIBUTES", "env=prod")
	t.Setenv("GINBOOT_TELEMETRY_SERVICE_NAME", "my-app")

	cfg := &Config{}
	cfg.ApplyEnvironmentOverrides()

	assert.Equal(t, 4000, cfg.Ginboot.Server.Port)
	assert.Equal(t, "/api/v2", cfg.Ginboot.Server.BasePath)
	assert.Equal(t, "postgres://prod-db:5432/main", cfg.Ginboot.DB.URL)
	assert.Equal(t, "postgres", cfg.Ginboot.DB.Driver)
	assert.Equal(t, "otel-collector:4317", cfg.Ginboot.Telemetry.Endpoint)
	assert.Equal(t, "Auth=Bearer123", cfg.Ginboot.Telemetry.Headers)
	assert.Equal(t, "http/protobuf", cfg.Ginboot.Telemetry.Protocol)
	assert.Equal(t, "env=prod", cfg.Ginboot.Telemetry.ResourceAttributes)
	assert.Equal(t, "my-app", cfg.Ginboot.Telemetry.ServiceName)
}

func TestLoadConfig_ErrorsAndCandidateSearching(t *testing.T) {
	t.Run("Non-existent file error", func(t *testing.T) {
		_, err := LoadConfig("/nonexistent/file.yml")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read config file")
	})

	t.Run("YAML unmarshal error", func(t *testing.T) {
		tmpDir := t.TempDir()
		invalidYamlFile := filepath.Join(tmpDir, "invalid.yml")
		require.NoError(t, os.WriteFile(invalidYamlFile, []byte("ginboot: [broken yaml::"), 0644))

		_, err := LoadConfig(invalidYamlFile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse yaml config")
	})

	t.Run("Empty path candidate searching - ginboot.yaml", func(t *testing.T) {
		tmpDir := t.TempDir()
		origWd, err := os.Getwd()
		require.NoError(t, err)
		defer func() { _ = os.Chdir(origWd) }()

		require.NoError(t, os.Chdir(tmpDir))

		yamlPath := filepath.Join(tmpDir, "ginboot.yaml")
		yamlContent := `
ginboot:
  server:
    port: 7070
`
		require.NoError(t, os.WriteFile(yamlPath, []byte(yamlContent), 0644))

		cfg, err := LoadConfig("")
		require.NoError(t, err)
		assert.Equal(t, 7070, cfg.Ginboot.Server.Port)
	})

	t.Run("Empty path no candidate file present", func(t *testing.T) {
		tmpDir := t.TempDir()
		origWd, err := os.Getwd()
		require.NoError(t, err)
		defer func() { _ = os.Chdir(origWd) }()

		require.NoError(t, os.Chdir(tmpDir))

		cfg, err := LoadConfig("")
		require.NoError(t, err)
		assert.Equal(t, 8080, cfg.Ginboot.Server.Port)
		assert.NotNil(t, cfg.Ginboot.Services)
	})
}

func TestLoadConfig_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "application.yml")

	yamlContent := `
ginboot:
  server:
    port: 9090
    base-path: /api/v1
  services:
    user-service:
      url: http://user-service.internal:8081
      timeout: 5s
    notification-service:
      url: http://notification-service.internal:8082
      timeout: 10s
  db:
    driver: postgres
    url: postgres://localhost:5432/test
  telemetry:
    enabled: true
    service-name: test-app
`
	err := os.WriteFile(configFile, []byte(yamlContent), 0644)
	require.NoError(t, err)

	cfg, err := LoadConfig(configFile)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, 9090, cfg.Ginboot.Server.Port)
	assert.Equal(t, "/api/v1", cfg.Ginboot.Server.BasePath)

	assert.Len(t, cfg.Ginboot.Services, 2)
	assert.Equal(t, "http://user-service.internal:8081", cfg.Ginboot.Services["user-service"].URL)
	assert.Equal(t, 5*time.Second, cfg.Ginboot.Services["user-service"].Timeout)

	assert.Equal(t, "postgres", cfg.Ginboot.DB.Driver)
	assert.True(t, cfg.Ginboot.Telemetry.Enabled)
	assert.Equal(t, "test-app", cfg.Ginboot.Telemetry.ServiceName)
}
