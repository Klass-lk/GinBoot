package parity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is a parity run described in YAML:
//
//	reference: { name: old, baseURL: https://api.example.com }
//	candidate: { name: new, baseURL: http://localhost:8080 }
//	principals:
//	  admin:   { token: "env:ADMIN_TOKEN" }
//	  student: { source: "exec:./get-principal student" }
//	rules:   { ignore: ["$..signedUrl"], timeAsInstant: true }
//	include: [ "cases/*.yaml" ]
//	cases:
//	  - { name: list courses, path: /api/courses, principals: [admin, student, anonymous] }
//
// Included files hold either a list of cases or a mapping with "rules" and
// "cases"; their rules apply to their own cases only.
type Config struct {
	Reference    TargetConfig               `yaml:"reference"`
	Candidate    TargetConfig               `yaml:"candidate"`
	Principals   map[string]PrincipalConfig `yaml:"principals"`
	Rules        Rules                      `yaml:"rules"`
	Include      []string                   `yaml:"include"`
	Cases        []Case                     `yaml:"cases"`
	AllowMethods []string                   `yaml:"allowMethods"`

	dir string
}

// TargetConfig is a Target in YAML.
type TargetConfig struct {
	Name    string            `yaml:"name"`
	BaseURL string            `yaml:"baseURL"`
	Header  map[string]string `yaml:"header"`
}

// Target converts the configuration to a Target.
func (t TargetConfig) Target() Target {
	h := http.Header{}
	for k, v := range t.Header {
		h.Set(k, v)
	}
	return Target{Name: t.Name, BaseURL: t.BaseURL, Header: h}
}

// PrincipalConfig describes how to obtain a principal's credentials.
//
// Values of Token, Source, Header and Cookies may be literal, or "env:NAME",
// "file:path" or "exec:command" (run with sh -c in the config file's
// directory; its trimmed stdout is the value).
type PrincipalConfig struct {
	// Token is sent as "Authorization: Bearer <token>".
	Token string `yaml:"token"`
	// Source yields a whole principal as JSON:
	// {"token": "...", "header": {...}, "cookies": {...}, "vars": {...}}.
	// Fields set directly in this config override what Source returns.
	Source  string            `yaml:"source"`
	Header  map[string]string `yaml:"header"`
	Cookies map[string]string `yaml:"cookies"`
	Vars    map[string]string `yaml:"vars"`
}

// LoadConfig reads a configuration file and the case files it includes.
func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parity: %s: %w", path, err)
	}
	cfg.dir = filepath.Dir(path)
	for _, pattern := range cfg.Include {
		if err := cfg.AddCases(filepath.Join(cfg.dir, pattern)); err != nil {
			return nil, err
		}
	}
	return &cfg, nil
}

// AddCases appends the cases from every file matching a glob pattern.
func (c *Config) AddCases(pattern string) error {
	files, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("parity: no case files match %q", pattern)
	}
	for _, f := range files {
		cases, err := loadCaseFile(f)
		if err != nil {
			return err
		}
		c.Cases = append(c.Cases, cases...)
	}
	return nil
}

func loadCaseFile(path string) ([]Case, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var list []Case
	if err := yaml.Unmarshal(b, &list); err == nil {
		return list, nil
	}
	var file struct {
		Rules Rules  `yaml:"rules"`
		Cases []Case `yaml:"cases"`
	}
	if err := yaml.Unmarshal(b, &file); err != nil {
		return nil, fmt.Errorf("parity: %s: %w", path, err)
	}
	for i := range file.Cases {
		file.Cases[i].Rules = file.Rules.Merge(file.Cases[i].Rules)
	}
	return file.Cases, nil
}

// ResolvePrincipals obtains every principal's credentials. Only principals
// some case uses are resolved, so an unused principal that cannot log in does
// not stop the run.
func (c *Config) ResolvePrincipals(ctx context.Context) (map[string]Principal, error) {
	used := map[string]bool{}
	for _, cs := range c.Cases {
		for _, p := range cs.Principals {
			used[p] = true
		}
	}
	out := map[string]Principal{}
	for name, pc := range c.Principals {
		if !used[name] {
			continue
		}
		p, err := c.resolvePrincipal(ctx, pc)
		if err != nil {
			return nil, fmt.Errorf("parity: principal %q: %w", name, err)
		}
		out[name] = p
	}
	return out, nil
}

func (c *Config) resolvePrincipal(ctx context.Context, pc PrincipalConfig) (Principal, error) {
	p := Principal{Header: http.Header{}, Cookies: map[string]string{}, Vars: map[string]string{}}
	if pc.Source != "" {
		raw, err := c.value(ctx, pc.Source)
		if err != nil {
			return p, err
		}
		var src struct {
			Token   string            `json:"token"`
			Header  map[string]string `json:"header"`
			Cookies map[string]string `json:"cookies"`
			Vars    map[string]string `json:"vars"`
		}
		if err := json.Unmarshal([]byte(raw), &src); err != nil {
			return p, fmt.Errorf("source did not print principal JSON: %w", err)
		}
		if src.Token != "" {
			p.Header.Set("Authorization", "Bearer "+src.Token)
		}
		for k, v := range src.Header {
			p.Header.Set(k, v)
		}
		for k, v := range src.Cookies {
			p.Cookies[k] = v
		}
		for k, v := range src.Vars {
			p.Vars[k] = v
		}
	}
	if pc.Token != "" {
		tok, err := c.value(ctx, pc.Token)
		if err != nil {
			return p, err
		}
		p.Header.Set("Authorization", "Bearer "+tok)
	}
	for k, v := range pc.Header {
		val, err := c.value(ctx, v)
		if err != nil {
			return p, err
		}
		p.Header.Set(k, val)
	}
	for k, v := range pc.Cookies {
		val, err := c.value(ctx, v)
		if err != nil {
			return p, err
		}
		p.Cookies[k] = val
	}
	for k, v := range pc.Vars {
		p.Vars[k] = v
	}
	return p, nil
}

// value resolves a literal, env:, file: or exec: value.
func (c *Config) value(ctx context.Context, v string) (string, error) {
	switch {
	case strings.HasPrefix(v, "env:"):
		name := strings.TrimPrefix(v, "env:")
		val, ok := os.LookupEnv(name)
		if !ok {
			return "", fmt.Errorf("environment variable %s is not set", name)
		}
		return val, nil
	case strings.HasPrefix(v, "file:"):
		path := strings.TrimPrefix(v, "file:")
		if !filepath.IsAbs(path) {
			path = filepath.Join(c.dir, path)
		}
		b, err := os.ReadFile(path)
		return strings.TrimSpace(string(b)), err
	case strings.HasPrefix(v, "exec:"):
		cmd := exec.CommandContext(ctx, "sh", "-c", strings.TrimPrefix(v, "exec:"))
		cmd.Dir = c.dir
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("%s: %w: %s", v, err, strings.TrimSpace(stderr.String()))
		}
		return strings.TrimSpace(string(out)), nil
	}
	return v, nil
}
