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
	err := os.WriteFile(envPath, []byte("DOTENV_TEST_VAR=hello_from_dotenv\n# Comment line\n"), 0644)
	require.NoError(t, err)

	LoadDotEnv(envPath)
	assert.Equal(t, "hello_from_dotenv", os.Getenv("DOTENV_TEST_VAR"))
}

func TestApplyEnvironmentOverrides(t *testing.T) {
	t.Setenv("PORT", "3000")
	t.Setenv("DATABASE_URL", "postgres://prod-db:5432/main")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "otel-collector:4317")

	cfg, err := LoadConfig("")
	require.NoError(t, err)

	assert.Equal(t, 3000, cfg.Ginboot.Server.Port)
	assert.Equal(t, "postgres://prod-db:5432/main", cfg.Ginboot.DB.URL)
	assert.Equal(t, "otel-collector:4317", cfg.Ginboot.Telemetry.Endpoint)
}

func TestLoadConfig(t *testing.T) {
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
