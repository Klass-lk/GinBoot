package parity

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func jsonResp(status int, body string) Response {
	return Response{Status: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(body)}
}

func kinds(diffs []Difference) map[string]DiffKind {
	out := map[string]DiffKind{}
	for _, d := range diffs {
		out[d.Path] = d.Kind
	}
	return out
}

func TestCompare(t *testing.T) {
	tests := []struct {
		name  string
		ref   string
		cand  string
		rules Rules
		want  map[string]DiffKind
	}{
		{name: "identical objects in a different key order",
			ref: `{"a":1,"b":{"c":"x","d":[1,2]}}`, cand: `{"b":{"d":[1,2],"c":"x"},"a":1}`, want: map[string]DiffKind{}},
		{name: "changed value",
			ref: `{"a":"x"}`, cand: `{"a":"y"}`, want: map[string]DiffKind{"$.a": DiffValue}},
		{name: "missing and extra fields",
			ref: `{"a":1,"b":2}`, cand: `{"a":1,"c":3}`, want: map[string]DiffKind{"$.b": DiffMissing, "$.c": DiffExtra}},
		{name: "type change",
			ref: `{"a":"1"}`, cand: `{"a":1}`, want: map[string]DiffKind{"$.a": DiffType}},
		{name: "null versus empty array",
			ref: `{"a":[]}`, cand: `{"a":null}`, want: map[string]DiffKind{"$.a": DiffType}},
		{name: "null versus missing is a difference by default",
			ref: `{"a":null}`, cand: `{}`, want: map[string]DiffKind{"$.a": DiffMissing}},
		{name: "null versus missing allowed by rule",
			ref: `{"a":null,"b":{"c":null}}`, cand: `{"b":{}}`, rules: Rules{NullIsMissing: []string{"$..*"}}, want: map[string]DiffKind{}},
		{name: "numbers compare by value by default",
			ref: `{"a":5.0,"b":1e2}`, cand: `{"a":5,"b":100}`, want: map[string]DiffKind{}},
		{name: "strict numbers",
			ref: `{"a":5.0}`, cand: `{"a":5}`, rules: Rules{StrictNumbers: true}, want: map[string]DiffKind{"$.a": DiffValue}},
		{name: "array length and extra element",
			ref: `[1,2]`, cand: `[1,2,3]`, want: map[string]DiffKind{"$": DiffLength, "$[2]": DiffExtra}},
		{name: "order matters by default",
			ref: `[1,2]`, cand: `[2,1]`, want: map[string]DiffKind{"$[0]": DiffValue, "$[1]": DiffValue}},
		{name: "unordered multiset",
			ref: `{"t":["a","b","c"]}`, cand: `{"t":["c","a","b"]}`, rules: Rules{Unordered: map[string]string{"$.t": ""}}, want: map[string]DiffKind{}},
		{name: "unordered by key reports field changes on the reference index",
			ref:   `[{"id":"1","v":"a"},{"id":"2","v":"b"}]`,
			cand:  `[{"id":"2","v":"B"},{"id":"1","v":"a"},{"id":"3","v":"c"}]`,
			rules: Rules{Unordered: map[string]string{"$": "id"}},
			want:  map[string]DiffKind{"$[1].v": DiffValue, "$[2]": DiffExtra}},
		{name: "ignore by wildcard",
			ref: `{"items":[{"url":"a?sig=1","id":1}]}`, cand: `{"items":[{"url":"a?sig=2","id":1}]}`,
			rules: Rules{Ignore: []string{"$.items[*].url"}}, want: map[string]DiffKind{}},
		{name: "ignore by recursive descent also hides missing fields",
			ref: `{"a":{"b":{"signedUrl":"x"}}}`, cand: `{"a":{"b":{}}}`,
			rules: Rules{Ignore: []string{"$..signedUrl"}}, want: map[string]DiffKind{}},
		{name: "ignored arrays do not change multiset order",
			ref:   `{"t":[{"n":"a","ts":1},{"n":"b","ts":2}]}`,
			cand:  `{"t":[{"n":"b","ts":9},{"n":"a","ts":8}]}`,
			rules: Rules{Unordered: map[string]string{"$.t": ""}, Ignore: []string{"$.t[*].ts"}},
			want:  map[string]DiffKind{}},
		{name: "times are strings unless TimeAsInstant",
			ref: `{"at":"2026-01-02T10:00:00+05:30"}`, cand: `{"at":"2026-01-02T04:30:00Z"}`, want: map[string]DiffKind{"$.at": DiffValue}},
		{name: "times as instants",
			ref: `{"at":"2026-01-02T10:00:00+05:30"}`, cand: `{"at":"2026-01-02T04:30:00.000Z"}`, rules: Rules{TimeAsInstant: true}, want: map[string]DiffKind{}},
		{name: "time tolerance",
			ref: `{"at":"2026-01-02T04:30:00Z"}`, cand: `{"at":"2026-01-02T04:30:03Z"}`, rules: Rules{TimeAsInstant: true, TimeTolerance: 5 * time.Second}, want: map[string]DiffKind{}},
		{name: "time outside tolerance",
			ref: `{"at":"2026-01-02T04:30:00Z"}`, cand: `{"at":"2026-01-02T04:31:00Z"}`, rules: Rules{TimeAsInstant: true, TimeTolerance: 5 * time.Second}, want: map[string]DiffKind{"$.at": DiffValue}},
		{name: "URLs without their query string",
			ref:   `{"u":["https://s3/b/k.pdf?X-Sig=1&exp=2"],"v":"https://s3/b/k.pdf?a=1"}`,
			cand:  `{"u":["https://s3/b/k.pdf?X-Sig=9"],"v":"https://s3/b/other.pdf?a=1"}`,
			rules: Rules{IgnoreQuery: []string{"$.u[*]", "$.v"}},
			want:  map[string]DiffKind{"$.v": DiffValue}},
		{name: "a field-name dot pattern does not match a sibling prefix",
			ref: `{"ab":1,"a":{"b":1}}`, cand: `{"ab":2,"a":{"b":2}}`, rules: Rules{Ignore: []string{"$.a.b"}}, want: map[string]DiffKind{"$.ab": DiffValue}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diffs, err := Compare(jsonResp(200, tt.ref), jsonResp(200, tt.cand), tt.rules)
			require.NoError(t, err)
			assert.Equal(t, tt.want, kinds(diffs), "%v", diffs)
		})
	}
}

func TestCompareStatusHeadersAndBodies(t *testing.T) {
	ref := Response{Status: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`)}
	cand := Response{Status: 404, Header: http.Header{"Content-Type": {"application/json; charset=utf-8"}}, Body: []byte(`{}`)}
	diffs, err := Compare(ref, cand, Rules{})
	require.NoError(t, err)
	assert.Equal(t, map[string]DiffKind{"status": DiffStatus}, kinds(diffs), "charset must not count")

	diffs, _ = Compare(ref, cand, Rules{IgnoreStatus: true})
	assert.Empty(t, diffs)

	text := Response{Status: 200, Header: http.Header{"Content-Type": {"text/plain"}}, Body: []byte("abc")}
	diffs, _ = Compare(text, Response{Status: 200, Header: http.Header{"Content-Type": {"text/plain"}}, Body: []byte("abd")}, Rules{})
	assert.Equal(t, map[string]DiffKind{"body": DiffBody}, kinds(diffs))

	diffs, _ = Compare(text, jsonResp(200, `"abc"`), Rules{})
	assert.Equal(t, DiffHeader, kinds(diffs)["header.Content-Type"])

	diffs, _ = Compare(jsonResp(200, `{}`), Response{Status: 200, Header: http.Header{"X-Trace": {"1"}}, Body: []byte(`{}`)}, Rules{Headers: []string{}})
	assert.Empty(t, diffs, "an empty Headers list compares no headers")
}

func TestCompareRejectsBadPaths(t *testing.T) {
	for _, p := range []string{"a.b", "$.a[", "$..", "$.a..", "$x"} {
		_, err := Compare(jsonResp(200, `{}`), jsonResp(200, `{}`), Rules{Ignore: []string{p}})
		assert.Error(t, err, p)
	}
}

func TestRulesMerge(t *testing.T) {
	a := Rules{Ignore: []string{"$.a"}, Unordered: map[string]string{"$.x": ""}, TimeTolerance: time.Second}
	b := Rules{Ignore: []string{"$.b"}, Unordered: map[string]string{"$.y": "id"}, TimeAsInstant: true, TimeTolerance: 3 * time.Second}
	m := a.Merge(b)
	assert.Equal(t, []string{"$.a", "$.b"}, m.Ignore)
	assert.Equal(t, map[string]string{"$.x": "", "$.y": "id"}, m.Unordered)
	assert.True(t, m.TimeAsInstant)
	assert.Equal(t, 3*time.Second, m.TimeTolerance)
	assert.Equal(t, []string{"$.a"}, a.Ignore, "Merge must not modify its receiver")
	assert.Nil(t, m.Headers, "nil Headers stays nil so the Content-Type default still applies")
}

func TestExtract(t *testing.T) {
	body := []byte(`{"items":[{"id":"c1","n":2}],"map":{"zeta":"z","alpha":"a"},"ok":true}`)
	tests := map[string]string{
		"$.items[0].id": "c1",
		"$.items[*].id": "c1",
		"$.items[0].n":  "2",
		"$.map.*~":      "alpha",
		"$.map.*":       "a",
		"$.items[0]~":   "0",
		"$.ok":          "true",
		"$.map[zeta]":   "z",
	}
	for path, want := range tests {
		got, err := Extract(body, path)
		require.NoError(t, err, path)
		assert.Equal(t, want, got, path)
	}
	for _, path := range []string{"$.nope", "$.items[3].id", "$.items", "$.map.zeta.x", "items"} {
		_, err := Extract(body, path)
		assert.Error(t, err, path)
	}
	_, err := Extract([]byte("not json"), "$.a")
	assert.Error(t, err)
}

func TestLookup(t *testing.T) {
	body := []byte(`{"a":{"n":1.5,"list":[1,2],"nil":null}}`)
	v, err := Lookup(body, "$.a.list")
	require.NoError(t, err)
	assert.Len(t, v, 2)
	v, err = Lookup(body, "$.a.nil")
	require.NoError(t, err)
	assert.Nil(t, v)
	_, err = Lookup(body, "$.a.zzz")
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = Lookup(body, "a")
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotFound, "a malformed path is not a missing value")
}
