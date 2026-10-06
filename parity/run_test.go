package parity

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// service builds a test server whose handlers see the token and cookie they
// were sent, so tests can check that both targets get the same credentials.
func service(t *testing.T, routes map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for p, h := range routes {
		mux.HandleFunc(p, h)
	}
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func write(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func TestRun(t *testing.T) {
	var refAuth, candAuth atomic.Value
	ref := service(t, map[string]http.HandlerFunc{
		"GET /api/courses": func(w http.ResponseWriter, r *http.Request) {
			refAuth.Store(r.Header.Get("Authorization") + "|" + cookie(r, "device"))
			write(200, `[{"id":"c1","name":"Maths"},{"id":"c2","name":"Art"}]`)(w, r)
		},
		"GET /api/courses/{id}": func(w http.ResponseWriter, r *http.Request) {
			write(200, `{"id":"`+r.PathValue("id")+`","signedUrl":"https://s3/x?sig=a","q":"`+r.URL.Query().Get("q")+`"}`)(w, r)
		},
		"GET /api/private": write(401, `{"error":"unauthorized"}`),
	})
	cand := service(t, map[string]http.HandlerFunc{
		"GET /api/courses": func(w http.ResponseWriter, r *http.Request) {
			candAuth.Store(r.Header.Get("Authorization") + "|" + cookie(r, "device"))
			write(200, `[{"id":"c1","name":"Maths"},{"id":"c2","name":"Art"}]`)(w, r)
		},
		"GET /api/courses/{id}": func(w http.ResponseWriter, r *http.Request) {
			write(200, `{"id":"`+r.PathValue("id")+`","signedUrl":"https://s3/x?sig=b","q":"`+r.URL.Query().Get("q")+`"}`)(w, r)
		},
		"GET /api/private": write(403, `{"error_code":"FORBIDDEN"}`),
	})

	principals := map[string]Principal{
		"admin": {
			Header:  http.Header{"Authorization": {"Bearer T"}},
			Cookies: map[string]string{"device": "D"},
			Vars:    map[string]string{"tenant": "i1"},
		},
	}
	cases := []Case{
		{Name: "list", Path: "/api/courses", Principals: []string{"admin"}, Capture: map[string]string{"courseId": "$[1].id"}},
		{Name: "one", Path: "/api/courses/{courseId}", Query: map[string]string{"q": "{tenant}"}, Principals: []string{"admin", Anonymous},
			Rules: Rules{Ignore: []string{"$.signedUrl"}}},
		{Name: "unsigned", Path: "/api/courses/{courseId}", Principals: []string{"admin"}},
		{Name: "private", Path: "/api/private"},
		{Name: "needs a var nobody set", Path: "/api/courses/{nope}"},
		{Name: "unknown principal", Path: "/api/courses", Principals: []string{"ghost"}},
		{Name: "disabled", Path: "/api/courses", Skip: "flaky upstream"},
		{Name: "write", Method: "post", Path: "/api/courses"},
	}

	rep, err := Run(context.Background(), Target{Name: "old", BaseURL: ref.URL}, Target{Name: "new", BaseURL: cand.URL + "/"}, principals, cases, Rules{}, Options{})
	require.NoError(t, err)

	byKey := map[string]Result{}
	for _, r := range rep.Results {
		byKey[r.Case+"/"+r.Principal] = r
	}
	assert.Equal(t, "Bearer T|D", refAuth.Load(), "reference gets the principal's credentials")
	assert.Equal(t, refAuth.Load(), candAuth.Load(), "both targets get the same credentials")

	assert.True(t, byKey["list/admin"].Passed())
	one := byKey["one/admin"]
	assert.True(t, one.Passed(), "%v", one.Diffs)
	assert.Equal(t, "/api/courses/c2?q=i1", one.Path, "captured and principal vars are substituted")
	assert.Equal(t, "no value for {courseId}, {tenant}", byKey["one/anonymous"].Skipped, "captures and principal vars are per principal")
	assert.Equal(t, DiffValue, kinds(byKey["unsigned/admin"].Diffs)["$.signedUrl"], "case rules apply to their case only")

	private := byKey["private/anonymous"]
	assert.Equal(t, 401, private.RefStatus)
	assert.Equal(t, 403, private.CandStatus)
	assert.Equal(t, DiffStatus, kinds(private.Diffs)["status"])

	assert.Equal(t, "no value for {nope}", byKey["needs a var nobody set/anonymous"].Skipped)
	assert.Contains(t, byKey["unknown principal/ghost"].Skipped, "not configured")
	assert.Equal(t, "flaky upstream", byKey["disabled/anonymous"].Skipped)
	assert.Contains(t, byKey["write/anonymous"].Err, "not allowed", "only GET and HEAD by default")

	passed, failed, skipped := rep.Counts()
	assert.Equal(t, []int{2, 3, 4}, []int{passed, failed, skipped})
	assert.False(t, rep.Passed())
}

func TestRunSharedCapture(t *testing.T) {
	s := service(t, map[string]http.HandlerFunc{
		"GET /admin/ids": write(200, `[{"id":"x1"}]`),
		"GET /public/{id}": func(w http.ResponseWriter, r *http.Request) {
			write(200, `{"id":"`+r.PathValue("id")+`"}`)(w, r)
		},
	})
	principals := map[string]Principal{"admin": {Header: http.Header{"Authorization": {"Bearer a"}}}}
	cand := service(t, map[string]http.HandlerFunc{ // does not serve /admin/ids
		"GET /public/{id}": func(w http.ResponseWriter, r *http.Request) {
			write(200, `{"id":"`+r.PathValue("id")+`"}`)(w, r)
		},
	})
	cases := []Case{
		{Name: "ids", Path: "/admin/ids", Principals: []string{"admin"}, Capture: map[string]string{"id": "$[0].id"},
			ShareCapture: true, ReferenceOnly: true},
		{Name: "public", Path: "/public/{id}"},
	}
	rep, err := Run(context.Background(), Target{BaseURL: s.URL}, Target{BaseURL: cand.URL}, principals, cases, Rules{}, Options{})
	require.NoError(t, err)
	require.Len(t, rep.Results, 2)
	assert.Equal(t, "reference only", rep.Results[0].Skipped, "a reference-only case is not compared")
	assert.Equal(t, 0, rep.Results[0].CandStatus, "and is never sent to the candidate")
	assert.Equal(t, "/public/x1", rep.Results[1].Path, "an anonymous case uses the admin's shared capture")
	assert.True(t, rep.Passed())
}

func TestRunAllowsOptInMethodsAndFilters(t *testing.T) {
	var posts atomic.Int32
	h := func(w http.ResponseWriter, r *http.Request) { posts.Add(1); write(200, `{}`)(w, r) }
	a := service(t, map[string]http.HandlerFunc{"POST /x": h, "GET /y": write(200, `{}`)})
	b := service(t, map[string]http.HandlerFunc{"POST /x": h, "GET /y": write(200, `{}`)})
	cases := []Case{{Name: "post", Method: "POST", Path: "/x"}, {Name: "get", Path: "/y"}}

	rep, err := Run(context.Background(), Target{BaseURL: a.URL}, Target{BaseURL: b.URL}, nil, cases, Rules{}, Options{
		AllowMethods: []string{"POST"},
		Filter:       func(c Case, _ string) bool { return c.Name == "post" },
	})
	require.NoError(t, err)
	require.Len(t, rep.Results, 1)
	assert.True(t, rep.Results[0].Passed())
	assert.Equal(t, int32(2), posts.Load())
}

func TestRunReportsUnreachableTargets(t *testing.T) {
	ok := service(t, map[string]http.HandlerFunc{"GET /x": write(200, `{}`)})
	rep, err := Run(context.Background(), Target{Name: "ref", BaseURL: ok.URL}, Target{Name: "cand", BaseURL: "http://127.0.0.1:1"}, nil,
		[]Case{{Name: "x", Path: "/x"}}, Rules{}, Options{})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(rep.Results[0].Err, "cand: "), rep.Results[0].Err)
}

func TestRunRejectsInvalidRules(t *testing.T) {
	_, err := Run(context.Background(), Target{}, Target{}, nil, nil, Rules{Ignore: []string{"bad"}}, Options{})
	assert.Error(t, err)
	_, err = Run(context.Background(), Target{}, Target{}, nil, []Case{{Name: "c", Path: "/", Rules: Rules{Ignore: []string{"bad"}}}}, Rules{}, Options{})
	assert.Error(t, err)
}

func TestReportsRedactValuesByDefault(t *testing.T) {
	rep := &Report{Reference: "old", Candidate: "new", Results: []Result{{
		Case: "c", Principal: "admin", Method: "GET", Path: "/x", RefStatus: 200, CandStatus: 200,
		Diffs: []Difference{
			{Path: "$.email", Kind: DiffValue, Ref: "jane@example.com", Cand: "john@example.com"},
			{Path: "status", Kind: DiffStatus, Ref: 200, Cand: 404},
			{Path: "$.a", Kind: DiffType, Ref: "string", Cand: "number"},
		},
	}}}
	for name, render := range map[string]func(*bytes.Buffer, RenderOptions) error{
		"text":     func(b *bytes.Buffer, o RenderOptions) error { return rep.WriteText(b, o) },
		"markdown": func(b *bytes.Buffer, o RenderOptions) error { return rep.WriteMarkdown(b, o) },
		"json":     func(b *bytes.Buffer, o RenderOptions) error { return rep.WriteJSON(b, o) },
	} {
		var hidden, shown bytes.Buffer
		require.NoError(t, render(&hidden, RenderOptions{}))
		require.NoError(t, render(&shown, RenderOptions{ShowValues: true}))
		assert.NotContains(t, hidden.String(), "jane@example.com", name)
		assert.Contains(t, hidden.String(), "404", name+": status codes are not redacted")
		assert.Contains(t, hidden.String(), "number", name+": type names are not redacted")
		assert.Contains(t, shown.String(), "jane@example.com", name)
	}
	assert.Equal(t, "jane@example.com", rep.Results[0].Diffs[0].Ref, "rendering must not modify the report")
}

func cookie(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}
