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
	t.Run("Empty service name error", func(t *testing.T) {
		resolver := NewConfigServiceResolver(nil)
		_, err := resolver.ResolveEndpoint("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "service name cannot be empty")
	})

	t.Run("Resolves from config", func(t *testing.T) {
		cfg := &config.Config{
			Ginboot: config.GinbootRootConfig{
				Services: map[string]config.ServiceTargetConfig{
					"user-service": {
						URL:      "http://user-service:8081/",
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
		t.Setenv("SERVICE_PAYMENT_SERVICE_URL", "https://payment.internal.cloud/")

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

func TestHTTPTransport_EdgeCases(t *testing.T) {
	transport := NewHTTPTransport(nil)
	assert.Equal(t, "http", transport.Protocol())

	t.Run("Invalid HTTP URL error", func(t *testing.T) {
		endpoint := ServiceEndpoint{Protocol: "http", Target: "http://invalid domain with spaces"}
		req := ServiceRequest{ServiceName: "test-svc", Action: "/test"}
		_, err := transport.Call(context.Background(), endpoint, req)
		assert.Error(t, err)
	})

	t.Run("Status code >= 400 error", func(t *testing.T) {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"bad request"}`))
		}))
		defer mockServer.Close()

		endpoint := ServiceEndpoint{Protocol: "http", Target: mockServer.URL}
		req := ServiceRequest{ServiceName: "test-svc", Action: "test-action"}
		resp, err := transport.Call(context.Background(), endpoint, req)
		assert.Error(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Contains(t, string(resp.Body), "bad request")
	})

	t.Run("CallAsync logs error on failure", func(t *testing.T) {
		endpoint := ServiceEndpoint{Protocol: "http", Target: "http://localhost:59999"}
		req := ServiceRequest{ServiceName: "failed-svc", Action: "/fail"}

		err := transport.CallAsync(context.Background(), endpoint, req)
		assert.NoError(t, err) // CallAsync returns nil immediately
	})
}

func TestServiceClient_MethodsAndErrors(t *testing.T) {
	t.Run("Unresolvable service name error", func(t *testing.T) {
		client := NewServiceClient(nil)
		err := client.Call(context.Background(), "", "/test", nil, nil)
		assert.Error(t, err)

		err = client.CallAsync(context.Background(), "", "/test", nil)
		assert.Error(t, err)
	})

	t.Run("Unsupported transport protocol error", func(t *testing.T) {
		mockResolver := &mockResolver{
			endpoint: ServiceEndpoint{Protocol: "grpc", Target: "localhost:50051"},
		}
		client := NewServiceClient(mockResolver)

		err := client.Call(context.Background(), "grpc-service", "/test", nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported service transport protocol")

		err = client.CallAsync(context.Background(), "grpc-service", "/test", nil)
		assert.Error(t, err)
	})

	t.Run("Invalid JSON response unmarshal target error", func(t *testing.T) {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not valid json"))
		}))
		defer mockServer.Close()

		cfg := &config.Config{
			Ginboot: config.GinbootRootConfig{
				Services: map[string]config.ServiceTargetConfig{
					"raw-service": {URL: mockServer.URL},
				},
			},
		}
		client := NewServiceClient(NewConfigServiceResolver(cfg))

		var target TestUserResponse
		err := client.Call(context.Background(), "raw-service", "/test", nil, &target)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode response")
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

type mockResolver struct {
	endpoint ServiceEndpoint
}

func (m *mockResolver) ResolveEndpoint(serviceName string) (ServiceEndpoint, error) {
	return m.endpoint, nil
}

func TestServiceClient_SetResolver(t *testing.T) {
	client := NewServiceClient(nil)
	resolver := &mockResolver{}
	client.SetResolver(resolver)

	reqEndpoint, err := resolver.ResolveEndpoint("test")
	assert.NoError(t, err)
	assert.Equal(t, ServiceEndpoint{}, reqEndpoint)
}
