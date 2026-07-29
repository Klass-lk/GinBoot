package service

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/klass-lk/ginboot/config"
)

// ConfigServiceResolver resolves service endpoints using application.yml configuration
// with fallback to environment variables and convention-based URLs.
type ConfigServiceResolver struct {
	config *config.Config
}

// NewConfigServiceResolver creates a resolver bound to a Ginboot Config
func NewConfigServiceResolver(cfg *config.Config) *ConfigServiceResolver {
	return &ConfigServiceResolver{config: cfg}
}

func (r *ConfigServiceResolver) ResolveEndpoint(serviceName string) (ServiceEndpoint, error) {
	if serviceName == "" {
		return ServiceEndpoint{}, fmt.Errorf("service name cannot be empty")
	}

	// 1. Check loaded application.yml / ginboot.yml config
	if r.config != nil && r.config.Ginboot.Services != nil {
		if targetCfg, exists := r.config.Ginboot.Services[serviceName]; exists && targetCfg.URL != "" {
			protocol := targetCfg.Protocol
			if protocol == "" {
				protocol = "http"
			}
			timeout := targetCfg.Timeout
			if timeout == 0 {
				timeout = 10 * time.Second
			}
			return ServiceEndpoint{
				Protocol: protocol,
				Target:   strings.TrimRight(targetCfg.URL, "/"),
				Timeout:  timeout,
			}, nil
		}
	}

	// 2. Check environment variable convention: SERVICE_<UPPERCASE_NAME>_URL
	envKey := "SERVICE_" + strings.ToUpper(strings.ReplaceAll(serviceName, "-", "_")) + "_URL"
	if envURL := os.Getenv(envKey); envURL != "" {
		return ServiceEndpoint{
			Protocol: "http",
			Target:   strings.TrimRight(envURL, "/"),
			Timeout:  10 * time.Second,
		}, nil
	}

	// 3. Default local fallback
	defaultTarget := fmt.Sprintf("http://%s:8080", serviceName)
	return ServiceEndpoint{
		Protocol: "http",
		Target:   defaultTarget,
		Timeout:  10 * time.Second,
	}, nil
}
