package parity

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "cases/a.yaml", `
- name: list
  path: /api/things
  principals: [admin]
`)
	writeFile(t, dir, "cases/b.yaml", `
rules:
  ignore: ["$.at"]
cases:
  - name: one
    path: /api/things/{id}
    principals: [student, anonymous]
    rules: { unordered: { "$.tags": "" } }
`)
	writeFile(t, dir, "student.token", "S-TOKEN\n")
	cfgPath := writeFile(t, dir, "parity.yaml", `
reference: { name: old, baseURL: "https://old", header: { X-Env: prod } }
candidate: { name: new, baseURL: "http://localhost:8080" }
rules: { timeAsInstant: true, timeTolerance: 2s }
include: ["cases/*.yaml"]
principals:
  admin:
    source: "exec:printf '{\"token\":\"A-TOKEN\",\"cookies\":{\"device\":\"d1\"},\"vars\":{\"tenant\":\"t1\"}}'"
    vars: { tenant: override }
  student:
    token: "file:student.token"
    header: { X-Role: "env:PARITY_TEST_ROLE" }
  unused:
    token: "env:PARITY_TEST_UNSET_VARIABLE"
`)
	t.Setenv("PARITY_TEST_ROLE", "learner")

	cfg, err := LoadConfig(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "prod", cfg.Reference.Target().Header.Get("X-Env"))
	assert.True(t, cfg.Rules.TimeAsInstant)
	assert.Equal(t, 2*time.Second, cfg.Rules.TimeTolerance)
	require.Len(t, cfg.Cases, 2)
	assert.Equal(t, []string{"$.at"}, cfg.Cases[1].Rules.Ignore, "file rules apply to the file's cases")
	assert.Equal(t, map[string]string{"$.tags": ""}, cfg.Cases[1].Rules.Unordered, "case rules are kept")

	ps, err := cfg.ResolvePrincipals(context.Background())
	require.NoError(t, err, "an unused principal with a missing token must not fail the run")
	assert.Equal(t, "Bearer A-TOKEN", ps["admin"].Header.Get("Authorization"))
	assert.Equal(t, "d1", ps["admin"].Cookies["device"])
	assert.Equal(t, "override", ps["admin"].Vars["tenant"], "config values win over the source")
	assert.Equal(t, "Bearer S-TOKEN", ps["student"].Header.Get("Authorization"))
	assert.Equal(t, "learner", ps["student"].Header.Get("X-Role"))
	assert.NotContains(t, ps, "unused")
}

func TestResolvePrincipalErrors(t *testing.T) {
	dir := t.TempDir()
	for name, principal := range map[string]string{
		"missing env":     `{ token: "env:PARITY_TEST_UNSET_VARIABLE" }`,
		"failing command": `{ token: "exec:echo nope >&2; exit 3" }`,
		"source not JSON": `{ source: "exec:echo plain" }`,
		"missing file":    `{ token: "file:nope.txt" }`,
	} {
		p := writeFile(t, dir, "c.yaml", "principals:\n  p: "+principal+"\ncases:\n  - { name: c, path: /, principals: [p] }\n")
		cfg, err := LoadConfig(p)
		require.NoError(t, err)
		_, err = cfg.ResolvePrincipals(context.Background())
		assert.Error(t, err, name)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadConfig(writeFile(t, dir, "a.yaml", "include: [\"none/*.yaml\"]\n"))
	assert.ErrorContains(t, err, "no case files")
	_, err = LoadConfig(writeFile(t, dir, "b.yaml", "cases: {not: a list}\n"))
	assert.Error(t, err)
	_, err = LoadConfig(filepath.Join(dir, "missing.yaml"))
	assert.Error(t, err)
}
