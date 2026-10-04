package parity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"mime"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Response is one side of a comparison.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// DiffKind names the way two responses differ at one path.
type DiffKind string

const (
	DiffStatus  DiffKind = "status"  // different HTTP status codes
	DiffHeader  DiffKind = "header"  // a header listed in Rules.Headers differs
	DiffMissing DiffKind = "missing" // present in the reference, absent in the candidate
	DiffExtra   DiffKind = "extra"   // absent in the reference, present in the candidate
	DiffType    DiffKind = "type"    // e.g. string vs number, array vs null
	DiffValue   DiffKind = "value"   // same type, different value
	DiffLength  DiffKind = "length"  // arrays of different length
	DiffBody    DiffKind = "body"    // non-JSON bodies that differ
)

// Difference is one place where the candidate does not answer like the
// reference. Ref and Cand hold the raw values; reports redact them unless
// asked not to.
type Difference struct {
	Path string   `json:"path"`
	Kind DiffKind `json:"kind"`
	Ref  any      `json:"ref,omitempty"`
	Cand any      `json:"cand,omitempty"`
}

func (d Difference) String() string {
	return fmt.Sprintf("%s %s", d.Kind, d.Path)
}

// Compare reports how cand differs from ref under the given rules. It returns
// an error only for invalid rules; differing responses are not an error.
func Compare(ref, cand Response, rules Rules) ([]Difference, error) {
	c, err := rules.compile()
	if err != nil {
		return nil, err
	}
	return c.compare(ref, cand), nil
}

func (c *compiledRules) compare(ref, cand Response) []Difference {
	var diffs []Difference
	if !c.IgnoreStatus && ref.Status != cand.Status {
		diffs = append(diffs, Difference{Path: "status", Kind: DiffStatus, Ref: ref.Status, Cand: cand.Status})
	}
	headers := c.Headers
	if headers == nil {
		headers = []string{"Content-Type"}
	}
	for _, h := range headers {
		rv, cv := ref.Header.Get(h), cand.Header.Get(h)
		if strings.EqualFold(h, "Content-Type") {
			rv, cv = mediaType(rv), mediaType(cv)
		}
		if rv != cv {
			diffs = append(diffs, Difference{Path: "header." + h, Kind: DiffHeader, Ref: rv, Cand: cv})
		}
	}

	rj, rok := decodeJSON(ref.Body)
	cj, cok := decodeJSON(cand.Body)
	if rok && cok {
		return c.walk("$", rj, cj, diffs)
	}
	if !bytes.Equal(bytes.TrimSpace(ref.Body), bytes.TrimSpace(cand.Body)) {
		diffs = append(diffs, Difference{Path: "body", Kind: DiffBody, Ref: string(ref.Body), Cand: string(cand.Body)})
	}
	return diffs
}

func mediaType(v string) string {
	if v == "" {
		return ""
	}
	mt, _, err := mime.ParseMediaType(v)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(v))
	}
	return mt
}

func decodeJSON(b []byte) (any, bool) {
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	if dec.More() {
		return nil, false
	}
	return v, true
}

func (c *compiledRules) walk(path string, ref, cand any, diffs []Difference) []Difference {
	if matchesAny(c.ignore, path) {
		return diffs
	}
	switch r := ref.(type) {
	case map[string]any:
		cm, ok := cand.(map[string]any)
		if !ok {
			return append(diffs, Difference{Path: path, Kind: DiffType, Ref: typeName(ref), Cand: typeName(cand)})
		}
		return c.walkObject(path, r, cm, diffs)
	case []any:
		ca, ok := cand.([]any)
		if !ok {
			return append(diffs, Difference{Path: path, Kind: DiffType, Ref: typeName(ref), Cand: typeName(cand)})
		}
		return c.walkArray(path, r, ca, diffs)
	}
	if typeName(ref) != typeName(cand) {
		return append(diffs, Difference{Path: path, Kind: DiffType, Ref: ref, Cand: cand})
	}
	if !c.scalarEqual(ref, cand) {
		return append(diffs, Difference{Path: path, Kind: DiffValue, Ref: ref, Cand: cand})
	}
	return diffs
}

func (c *compiledRules) walkObject(path string, ref, cand map[string]any, diffs []Difference) []Difference {
	keys := make([]string, 0, len(ref)+len(cand))
	for k := range ref {
		keys = append(keys, k)
	}
	for k := range cand {
		if _, ok := ref[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		p := path + "." + k
		if matchesAny(c.ignore, p) {
			continue
		}
		rv, rok := ref[k]
		cv, cok := cand[k]
		switch {
		case rok && cok:
			diffs = c.walk(p, rv, cv, diffs)
		case rok:
			if rv == nil && matchesAny(c.nullIsMissing, p) {
				continue
			}
			diffs = append(diffs, Difference{Path: p, Kind: DiffMissing, Ref: rv})
		default:
			if cv == nil && matchesAny(c.nullIsMissing, p) {
				continue
			}
			diffs = append(diffs, Difference{Path: p, Kind: DiffExtra, Cand: cv})
		}
	}
	return diffs
}

func (c *compiledRules) walkArray(path string, ref, cand []any, diffs []Difference) []Difference {
	if key, ok := c.unorderedKey(path); ok {
		if key != "" {
			return c.walkKeyedArray(path, key, ref, cand, diffs)
		}
		ref, cand = c.sortCanonical(path, ref), c.sortCanonical(path, cand)
	}
	if len(ref) != len(cand) {
		diffs = append(diffs, Difference{Path: path, Kind: DiffLength, Ref: len(ref), Cand: len(cand)})
	}
	n := min(len(ref), len(cand))
	for i := 0; i < n; i++ {
		diffs = c.walk(fmt.Sprintf("%s[%d]", path, i), ref[i], cand[i], diffs)
	}
	for i := n; i < len(ref); i++ {
		diffs = append(diffs, Difference{Path: fmt.Sprintf("%s[%d]", path, i), Kind: DiffMissing, Ref: ref[i]})
	}
	for i := n; i < len(cand); i++ {
		diffs = append(diffs, Difference{Path: fmt.Sprintf("%s[%d]", path, i), Kind: DiffExtra, Cand: cand[i]})
	}
	return diffs
}

// walkKeyedArray pairs elements by the value of their key field. Paths in the
// report use the reference index, or the candidate index for extras.
func (c *compiledRules) walkKeyedArray(path, key string, ref, cand []any, diffs []Difference) []Difference {
	candByKey := map[string]int{}
	for i, e := range cand {
		if k, ok := elementKey(e, key); ok {
			candByKey[k] = i
		}
	}
	matched := map[int]bool{}
	for i, e := range ref {
		p := fmt.Sprintf("%s[%d]", path, i)
		k, ok := elementKey(e, key)
		j, found := candByKey[k]
		if !ok || !found {
			diffs = append(diffs, Difference{Path: p, Kind: DiffMissing, Ref: e})
			continue
		}
		matched[j] = true
		diffs = c.walk(p, e, cand[j], diffs)
	}
	for j, e := range cand {
		if !matched[j] {
			diffs = append(diffs, Difference{Path: fmt.Sprintf("%s[%d]", path, j), Kind: DiffExtra, Cand: e})
		}
	}
	return diffs
}

func elementKey(e any, key string) (string, bool) {
	m, ok := e.(map[string]any)
	if !ok {
		return "", false
	}
	v, ok := m[key]
	if !ok {
		return "", false
	}
	return fmt.Sprint(v), true
}

// sortCanonical orders elements by their JSON encoding, with ignored paths
// removed first so that a value that is allowed to differ cannot change the
// order.
func (c *compiledRules) sortCanonical(path string, items []any) []any {
	type keyed struct {
		key  string
		item any
	}
	ks := make([]keyed, len(items))
	for i, it := range items {
		stripped := c.stripIgnored(path+"[0]", it)
		b, _ := json.Marshal(stripped)
		ks[i] = keyed{string(b), it}
	}
	sort.SliceStable(ks, func(i, j int) bool { return ks[i].key < ks[j].key })
	out := make([]any, len(items))
	for i, k := range ks {
		out[i] = k.item
	}
	return out
}

func (c *compiledRules) stripIgnored(path string, v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			p := path + "." + k
			if matchesAny(c.ignore, p) {
				continue
			}
			out[k] = c.stripIgnored(p, e)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = c.stripIgnored(fmt.Sprintf("%s[%d]", path, i), e)
		}
		return out
	}
	return v
}

func (c *compiledRules) scalarEqual(ref, cand any) bool {
	switch r := ref.(type) {
	case json.Number:
		cn := cand.(json.Number)
		if c.StrictNumbers {
			return r == cn
		}
		return numbersEqual(r, cn)
	case string:
		cs := cand.(string)
		if r == cs {
			return true
		}
		if c.TimeAsInstant {
			rt, rok := parseTime(r)
			ct, cok := parseTime(cs)
			if rok && cok {
				d := rt.Sub(ct)
				if d < 0 {
					d = -d
				}
				return d <= c.TimeTolerance
			}
		}
		return false
	}
	return ref == cand
}

func numbersEqual(a, b json.Number) bool {
	if a == b {
		return true
	}
	af, _, errA := big.ParseFloat(string(a), 10, 256, big.ToNearestEven)
	bf, _, errB := big.ParseFloat(string(b), 10, 256, big.ToNearestEven)
	if errA != nil || errB != nil {
		return false
	}
	return af.Cmp(bf) == 0
}

var timeLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999Z0700",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05",
}

// parseTime accepts the common ISO-8601 forms. A timestamp without an offset
// is read as UTC, so two offset-less values still compare correctly.
func parseTime(s string) (time.Time, bool) {
	if len(s) < len("2006-01-02T15:04") || s[4] != '-' {
		return time.Time{}, false
	}
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func typeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case json.Number, float64, int:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return fmt.Sprintf("%T", v)
}

// describe renders a value for a redacted report: its type, its size and a
// short hash, so equal values can be spotted without being shown.
func describe(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.Itoa(t)
	}
	b, _ := json.Marshal(v)
	return fmt.Sprintf("%s(len=%d,#%s)", typeName(v), len(b), shortHash(b))
}
