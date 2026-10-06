package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	serve := func(body string) *httptest.Server {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer tok" && r.URL.Path != "/public" {
				w.WriteHeader(401)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(s.Close)
		return s
	}
	same, other := serve(`{"a":1}`), serve(`{"a":2}`)

	dir := t.TempDir()
	cfg := filepath.Join(dir, "parity.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte(`
reference: { name: old, baseURL: "`+same.URL+`" }
candidate: { name: new, baseURL: "`+same.URL+`" }
principals:
  admin: { token: tok }
  broken: { token: "env:GINBOOT_PARITY_TEST_UNSET" }
cases:
  - { name: private, path: /private, principals: [admin, broken] }
  - { name: public, path: /public }
`), 0o644))

	var out, errOut bytes.Buffer
	code := run([]string{"-config", cfg, "-principal", "admin"}, &out, &errOut)
	assert.Equal(t, 0, code, errOut.String())
	assert.Contains(t, out.String(), "1 passed", "filtering skips the broken principal and the anonymous case")

	out.Reset()
	report := filepath.Join(dir, "reports", "r.md")
	code = run([]string{"-config", cfg, "-cand", other.URL, "-only", "private", "-principal", "admin", "-format", "markdown", "-out", report}, &out, &errOut)
	assert.Equal(t, 1, code)
	assert.Contains(t, out.String(), "**value** `$.a`")
	written, err := os.ReadFile(report)
	require.NoError(t, err)
	assert.Equal(t, out.String(), string(written))

	assert.Equal(t, 2, run([]string{"-config", cfg}, &out, &errOut), "an unresolvable principal is a configuration error")
	assert.Equal(t, 2, run([]string{"-config", filepath.Join(dir, "missing.yaml")}, &out, &errOut))
	assert.Equal(t, 2, run([]string{"-nope"}, &out, &errOut))
}
