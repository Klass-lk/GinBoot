package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// HTTPTransport executes service calls using standard HTTP/REST requests.
type HTTPTransport struct {
	client *http.Client
	logger *slog.Logger
}

// NewHTTPTransport creates an HTTP transport driver with default timeouts
func NewHTTPTransport(customClient *http.Client) *HTTPTransport {
	if customClient == nil {
		customClient = &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		}
	}
	return &HTTPTransport{
		client: customClient,
		logger: slog.Default(),
	}
}

func (t *HTTPTransport) Protocol() string {
	return "http"
}

func (t *HTTPTransport) Call(ctx context.Context, endpoint ServiceEndpoint, req ServiceRequest) (*ServiceResponse, error) {
	fullURL := endpoint.Target
	action := req.Action
	if action != "" {
		if !strings.HasPrefix(action, "/") {
			action = "/" + action
		}
		fullURL += action
	}

	method := strings.ToUpper(req.Method)
	if method == "" {
		method = http.MethodPost
	}

	var bodyReader io.Reader
	if req.Payload != nil {
		jsonBytes, err := json.Marshal(req.Payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request payload for service %s: %w", req.ServiceName, err)
		}
		bodyReader = bytes.NewReader(jsonBytes)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request for service %s: %w", req.ServiceName, err)
	}

	if bodyReader != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	httpReq.Header.Set("Accept", "application/json")

	// Apply endpoint headers and request headers
	for k, v := range endpoint.Headers {
		httpReq.Header.Set(k, v)
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("service call to %s failed: %w", req.ServiceName, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body from service %s: %w", req.ServiceName, err)
	}

	respHeaders := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			respHeaders[k] = v[0]
		}
	}

	if resp.StatusCode >= 400 {
		return &ServiceResponse{
			StatusCode: resp.StatusCode,
			Headers:    respHeaders,
			Body:       respBody,
		}, fmt.Errorf("service %s returned status %d: %s", req.ServiceName, resp.StatusCode, string(respBody))
	}

	return &ServiceResponse{
		StatusCode: resp.StatusCode,
		Headers:    respHeaders,
		Body:       respBody,
	}, nil
}

func (t *HTTPTransport) CallAsync(ctx context.Context, endpoint ServiceEndpoint, req ServiceRequest) error {
	// Create detached background context to ensure goroutine is not cancelled when caller request finishes
	asyncCtx := context.WithoutCancel(ctx)

	go func() {
		_, err := t.Call(asyncCtx, endpoint, req)
		if err != nil {
			t.logger.Error("Async service call failed",
				"service", req.ServiceName,
				"action", req.Action,
				"error", err,
			)
		}
	}()

	return nil
}
