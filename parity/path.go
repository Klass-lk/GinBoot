package parity

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Extract reads one value from a JSON body, for capturing ids from one
// response to use in a later request. The path syntax is the one Rules uses,
// with two differences: "*" (or "[*]") picks the *first* field or element
// rather than all of them, and a trailing "~" returns the key or index of the
// selected node instead of its value.
//
//	$.items[0].id   the id of the first item
//	$[*].id         the same, for a top-level array
//	$.*~            the first key of an object (keys are sorted)
func Extract(body []byte, path string) (string, error) {
	v, ok := decodeJSON(body)
	if !ok {
		return "", fmt.Errorf("parity: body is not JSON")
	}
	return extract(v, path)
}

// ErrNotFound is returned by Lookup when the path selects nothing.
var ErrNotFound = errors.New("parity: path not found")

// Lookup returns the value a path selects in a JSON body, with the selection
// rules of Extract. Numbers are json.Number. A path that selects nothing
// returns an error wrapping ErrNotFound.
func Lookup(body []byte, path string) (any, error) {
	v, ok := decodeJSON(body)
	if !ok {
		return nil, fmt.Errorf("parity: body is not JSON")
	}
	val, _, err := lookup(v, path)
	return val, err
}

func extract(v any, path string) (string, error) {
	val, key, err := lookup(v, path)
	if err != nil {
		return "", err
	}
	if strings.HasSuffix(path, "~") {
		return key, nil
	}
	return scalarString(val)
}

func lookup(v any, path string) (any, string, error) {
	if !strings.HasPrefix(path, "$") {
		return nil, "", fmt.Errorf("parity: path %q must start with $", path)
	}
	rest := strings.TrimSuffix(path[1:], "~")
	key := ""
	for len(rest) > 0 {
		var seg string
		switch {
		case rest[0] == '[':
			end := strings.IndexByte(rest, ']')
			if end < 0 {
				return nil, "", fmt.Errorf("parity: unclosed [ in %q", path)
			}
			seg, rest = rest[1:end], rest[end+1:]
		case rest[0] == '.':
			end := strings.IndexAny(rest[1:], ".[")
			if end < 0 {
				end = len(rest) - 1
			}
			seg, rest = rest[1:end+1], rest[end+1:]
		default:
			return nil, "", fmt.Errorf("parity: unexpected %q in %q", rest[:1], path)
		}
		var err error
		if v, key, err = step(v, seg); err != nil {
			return nil, "", fmt.Errorf("%w: %s at %q", ErrNotFound, err, path)
		}
	}
	return v, key, nil
}

func step(v any, seg string) (any, string, error) {
	switch t := v.(type) {
	case map[string]any:
		if seg == "*" {
			if len(t) == 0 {
				return nil, "", fmt.Errorf("empty object")
			}
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			return t[keys[0]], keys[0], nil
		}
		e, ok := t[seg]
		if !ok {
			return nil, "", fmt.Errorf("no field %q", seg)
		}
		return e, seg, nil
	case []any:
		i := 0
		if seg != "*" {
			n, err := strconv.Atoi(seg)
			if err != nil {
				return nil, "", fmt.Errorf("%q is not an index", seg)
			}
			i = n
		}
		if i < 0 || i >= len(t) {
			return nil, "", fmt.Errorf("index %d out of range (len %d)", i, len(t))
		}
		return t[i], strconv.Itoa(i), nil
	}
	return nil, "", fmt.Errorf("cannot select %q from %s", seg, typeName(v))
}

func scalarString(v any) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case json.Number:
		return t.String(), nil
	case bool:
		return strconv.FormatBool(t), nil
	case nil:
		return "", fmt.Errorf("parity: selected value is null")
	}
	return "", fmt.Errorf("parity: selected value is an %s, not a scalar", typeName(v))
}
