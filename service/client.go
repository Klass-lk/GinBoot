package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel/propagation"
)

// DefaultServiceClient manages service resolution, transports, and headers propagation.
type DefaultServiceClient struct {
	mu         sync.RWMutex
	resolver   ServiceResolver
	transports map[string]ServiceTransport
	propagator propagation.TextMapPropagator
}

// NewServiceClient creates a DefaultServiceClient with HTTPTransport pre-registered
func NewServiceClient(resolver ServiceResolver) *DefaultServiceClient {
	if resolver == nil {
		resolver = NewConfigServiceResolver(nil)
	}

	client := &DefaultServiceClient{
		resolver:   resolver,
		transports: make(map[string]ServiceTransport),
		propagator: propagation.TraceContext{},
	}

	// Register default HTTP transport
	client.RegisterTransport(NewHTTPTransport(nil))
	return client
}

func (c *DefaultServiceClient) SetResolver(resolver ServiceResolver) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resolver = resolver
}

func (c *DefaultServiceClient) RegisterTransport(transport ServiceTransport) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.transports[transport.Protocol()] = transport
}

func (c *DefaultServiceClient) getTransport(protocol string) (ServiceTransport, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if t, exists := c.transports[protocol]; exists {
		return t, nil
	}
	return nil, fmt.Errorf("unsupported service transport protocol: %s", protocol)
}

func (c *DefaultServiceClient) prepareHeaders(ctx context.Context, extraHeaders map[string]string) map[string]string {
	headers := make(map[string]string)

	// 1. Copy any extra headers passed
	for k, v := range extraHeaders {
		headers[k] = v
	}

	// 2. Inject OpenTelemetry W3C Trace Context (traceparent, tracestate)
	if ctx != nil && c.propagator != nil {
		carrier := propagation.MapCarrier(headers)
		c.propagator.Inject(ctx, carrier)
	}

	return headers
}

func (c *DefaultServiceClient) Call(ctx context.Context, serviceName string, action string, payload interface{}, target interface{}) error {
	return c.CallWithMethod(ctx, "POST", serviceName, action, payload, target)
}

func (c *DefaultServiceClient) CallWithMethod(ctx context.Context, method string, serviceName string, action string, payload interface{}, target interface{}) error {
	c.mu.RLock()
	resolver := c.resolver
	c.mu.RUnlock()

	endpoint, err := resolver.ResolveEndpoint(serviceName)
	if err != nil {
		return fmt.Errorf("failed to resolve service %s: %w", serviceName, err)
	}

	transport, err := c.getTransport(endpoint.Protocol)
	if err != nil {
		return err
	}

	req := ServiceRequest{
		ServiceName: serviceName,
		Action:      action,
		Method:      method,
		Headers:     c.prepareHeaders(ctx, nil),
		Payload:     payload,
	}

	resp, err := transport.Call(ctx, endpoint, req)
	if err != nil {
		return err
	}

	if target != nil && len(resp.Body) > 0 {
		if err := json.Unmarshal(resp.Body, target); err != nil {
			return fmt.Errorf("failed to decode response from service %s into target struct: %w", serviceName, err)
		}
	}

	return nil
}

func (c *DefaultServiceClient) CallAsync(ctx context.Context, serviceName string, action string, payload interface{}) error {
	return c.CallAsyncWithMethod(ctx, "POST", serviceName, action, payload)
}

func (c *DefaultServiceClient) CallAsyncWithMethod(ctx context.Context, method string, serviceName string, action string, payload interface{}) error {
	c.mu.RLock()
	resolver := c.resolver
	c.mu.RUnlock()

	endpoint, err := resolver.ResolveEndpoint(serviceName)
	if err != nil {
		return fmt.Errorf("failed to resolve service %s: %w", serviceName, err)
	}

	transport, err := c.getTransport(endpoint.Protocol)
	if err != nil {
		return err
	}

	req := ServiceRequest{
		ServiceName: serviceName,
		Action:      action,
		Method:      method,
		Headers:     c.prepareHeaders(ctx, nil),
		Payload:     payload,
	}

	return transport.CallAsync(ctx, endpoint, req)
}
