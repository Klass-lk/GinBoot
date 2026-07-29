package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/klass-lk/ginboot/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestUserPayload struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type TestUserResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func TestConfigServiceResolver(t *testing.T) {
	t.Run("Resolves from config", func(t *testing.T) {
		cfg := &config.Config{
			Ginboot: config.GinbootRootConfig{
				Services: map[string]config.ServiceTargetConfig{
					"user-service": {
						URL:      "http://user-service:8081",
						Protocol: "http",
						Timeout:  5 * time.Second,
					},
				},
			},
		}

		resolver := NewConfigServiceResolver(cfg)
		endpoint, err := resolver.ResolveEndpoint("user-service")
		require.NoError(t, err)

		assert.Equal(t, "http", endpoint.Protocol)
		assert.Equal(t, "http://user-service:8081", endpoint.Target)
		assert.Equal(t, 5*time.Second, endpoint.Timeout)
	})

	t.Run("Resolves from env var fallback", func(t *testing.T) {
		t.Setenv("SERVICE_PAYMENT_SERVICE_URL", "https://payment.internal.cloud")

		resolver := NewConfigServiceResolver(nil)
		endpoint, err := resolver.ResolveEndpoint("payment-service")
		require.NoError(t, err)

		assert.Equal(t, "http", endpoint.Protocol)
		assert.Equal(t, "https://payment.internal.cloud", endpoint.Target)
	})

	t.Run("Resolves default local fallback", func(t *testing.T) {
		resolver := NewConfigServiceResolver(nil)
		endpoint, err := resolver.ResolveEndpoint("unknown-service")
		require.NoError(t, err)

		assert.Equal(t, "http://unknown-service:8080", endpoint.Target)
	})
}

func TestServiceClientSyncCall(t *testing.T) {
	// Setup mock target HTTP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/users", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var reqPayload TestUserPayload
		err := json.NewDecoder(r.Body).Decode(&reqPayload)
		assert.NoError(t, err)
		assert.Equal(t, "John Doe", reqPayload.Name)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(TestUserResponse{
			ID:    "usr-123",
			Name:  reqPayload.Name,
			Email: reqPayload.Email,
		})
	}))
	defer mockServer.Close()

	cfg := &config.Config{
		Ginboot: config.GinbootRootConfig{
			Services: map[string]config.ServiceTargetConfig{
				"user-service": {
					URL: mockServer.URL,
				},
			},
		},
	}

	client := NewServiceClient(NewConfigServiceResolver(cfg))

	var resp TestUserResponse
	err := client.Call(context.Background(), "user-service", "/api/v1/users", TestUserPayload{
		Name:  "John Doe",
		Email: "john@example.com",
	}, &resp)

	require.NoError(t, err)
	assert.Equal(t, "usr-123", resp.ID)
	assert.Equal(t, "John Doe", resp.Name)
	assert.Equal(t, "john@example.com", resp.Email)
}

func TestServiceClientAsyncCall(t *testing.T) {
	called := make(chan bool, 1)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/events", r.URL.Path)
		w.WriteHeader(http.StatusAccepted)
		called <- true
	}))
	defer mockServer.Close()

	cfg := &config.Config{
		Ginboot: config.GinbootRootConfig{
			Services: map[string]config.ServiceTargetConfig{
				"event-service": {
					URL: mockServer.URL,
				},
			},
		},
	}

	client := NewServiceClient(NewConfigServiceResolver(cfg))

	err := client.CallAsync(context.Background(), "event-service", "/api/v1/events", map[string]string{
		"event": "user.registered",
	})
	require.NoError(t, err)

	select {
	case <-called:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Async call timed out")
	}
}
