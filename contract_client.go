package ginboot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/klass-lk/ginboot/service"
)

// NoRequest is the request type for contract methods that take none.
type NoRequest struct{}

// RemoteError is returned when a contract call reached the remote service and
// the service answered with a failure. It keeps the status and the decoded
// remote ApiError, so a caller can inspect the cause with errors.As.
//
// It is deliberately *not* the same thing as a local ApiError: by default
// ginboot answers a RemoteError with 502 Bad Gateway, because "user-service
// could not find that user" is rarely the same statement as "the resource you
// asked me for does not exist". When it is, say so explicitly with Propagate.
type RemoteError struct {
	Service    string
	Method     string
	StatusCode int
	Err        error

	// Propagate makes SendError answer with the remote status instead of 502.
	Propagate bool
}

func (e *RemoteError) Error() string {
	return fmt.Sprintf("service %s: %s returned %d: %v", e.Service, e.Method, e.StatusCode, e.Err)
}

func (e *RemoteError) Unwrap() error { return e.Err }

// Propagate marks a RemoteError as safe to forward to this service's own
// caller with the upstream status code intact. Any other error is returned
// unchanged.
func Propagate(err error) error {
	var remote *RemoteError
	if !errors.As(err, &remote) {
		return err
	}
	forwarded := *remote
	forwarded.Propagate = true
	return &forwarded
}

// Invoke performs a typed contract call. It resolves the route from the
// contract, fills path parameters and the query string from req, sends the
// body for methods that take one, and decodes the reply into Res.
//
// Res is given explicitly and Req is inferred:
//
//	ginboot.Invoke[UserResponse](ctx, c.client, userapi.Contract, "GetUser", req)
//
// The context, the error, and this call being a function of a *client* are all
// deliberately visible at the call site. A contract makes a remote call typed;
// it does not make it local.
func Invoke[Res any, Req any](ctx context.Context, client service.ServiceClient, contract Contract, method string, req Req) (Res, error) {
	var res Res

	route, err := contract.Lookup(method)
	if err != nil {
		return res, err
	}

	action, err := buildAction(contract.BasePath, route.Path, req)
	if err != nil {
		return res, fmt.Errorf("contract %s.%s: %w", contract.Service, method, err)
	}

	httpMethod := strings.ToUpper(route.Method)
	var payload any
	if hasBody(httpMethod) {
		payload = req
	} else {
		query, err := encodeQuery(req)
		if err != nil {
			return res, fmt.Errorf("contract %s.%s: %w", contract.Service, method, err)
		}
		if query != "" {
			action += "?" + query
		}
	}

	raw, ok := client.(service.RawCaller)
	if !ok {
		// A custom ServiceClient without raw access: fall back to the plain
		// call, at the cost of a flattened remote error.
		if err := client.CallWithMethod(ctx, httpMethod, contract.Service, action, payload, &res); err != nil {
			var zero Res
			return zero, err
		}
		return res, nil
	}

	resp, err := raw.CallRaw(ctx, httpMethod, contract.Service, action, payload, nil)
	if err != nil {
		return res, fmt.Errorf("service %s: %s: %w", contract.Service, method, err)
	}

	if resp.StatusCode >= 400 {
		return res, &RemoteError{
			Service:    contract.Service,
			Method:     method,
			StatusCode: resp.StatusCode,
			Err:        decodeRemoteError(resp),
		}
	}

	if len(resp.Body) == 0 {
		return res, nil
	}
	if err := json.Unmarshal(resp.Body, &res); err != nil {
		var zero Res
		return zero, fmt.Errorf("service %s: %s: decoding response: %w", contract.Service, method, err)
	}
	return res, nil
}

// decodeRemoteError turns a failed response into the ApiError the remote
// service meant to send, falling back to the raw body when it is not a ginboot
// error envelope.
func decodeRemoteError(resp *service.ServiceResponse) error {
	var envelope ErrorResponse
	if err := json.Unmarshal(resp.Body, &envelope); err == nil && (envelope.ErrorCode != "" || envelope.Message != "") {
		return ApiError{ErrorCode: envelope.ErrorCode, Message: envelope.Message}
	}
	message := strings.TrimSpace(string(resp.Body))
	if message == "" {
		message = http.StatusText(resp.StatusCode)
	}
	return ApiError{ErrorCode: strconv.Itoa(resp.StatusCode), Message: message}
}

// buildAction joins the contract base path with the route path, substituting
// ":name" and "*name" segments from fields of req tagged `uri:"name"`.
func buildAction(basePath, routePath string, req any) (string, error) {
	full := "/" + strings.Trim(strings.TrimSpace(basePath), "/")
	if trimmed := strings.Trim(strings.TrimSpace(routePath), "/"); trimmed != "" {
		if full == "/" {
			full = "/" + trimmed
		} else {
			full += "/" + trimmed
		}
	}
	if !strings.ContainsAny(full, ":*") {
		return full, nil
	}

	params := uriParams(req)
	segments := strings.Split(full, "/")
	for i, segment := range segments {
		if segment == "" || (segment[0] != ':' && segment[0] != '*') {
			continue
		}
		name := segment[1:]
		value, ok := params[name]
		if !ok {
			return "", fmt.Errorf("no field tagged `uri:%q` on %T to fill path parameter %s", name, req, segment)
		}
		if value == "" {
			return "", fmt.Errorf("path parameter %s is empty", segment)
		}
		segments[i] = url.PathEscape(value)
	}
	return strings.Join(segments, "/"), nil
}

// uriParams collects `uri`-tagged fields, keyed by tag name.
func uriParams(req any) map[string]string {
	params := map[string]string{}
	v := reflect.ValueOf(req)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return params
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return params
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name := strings.Split(field.Tag.Get("uri"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		params[name] = formatValue(v.Field(i))
	}
	return params
}

// encodeQuery renders `form`-tagged fields as a query string. Fields marked
// omitempty are skipped when zero, matching how gin binds them on the way in.
func encodeQuery(req any) (string, error) {
	v := reflect.ValueOf(req)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return "", nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return "", nil
	}

	values := url.Values{}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		tag := field.Tag.Get("form")
		if tag == "" {
			continue
		}
		parts := strings.Split(tag, ",")
		name := parts[0]
		if name == "" || name == "-" {
			continue
		}
		fieldValue := v.Field(i)
		if hasOption(parts[1:], "omitempty") && fieldValue.IsZero() {
			continue
		}
		if fieldValue.Kind() == reflect.Slice || fieldValue.Kind() == reflect.Array {
			for j := 0; j < fieldValue.Len(); j++ {
				values.Add(name, formatValue(fieldValue.Index(j)))
			}
			continue
		}
		values.Set(name, formatValue(fieldValue))
	}
	return values.Encode(), nil
}

func formatValue(v reflect.Value) string {
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64)
	default:
		if v.IsValid() && v.CanInterface() {
			if s, ok := v.Interface().(fmt.Stringer); ok {
				return s.String()
			}
		}
		return fmt.Sprintf("%v", v)
	}
}

func hasOption(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
