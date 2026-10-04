package parity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// Target is one of the two services being compared.
type Target struct {
	Name    string      `yaml:"name" json:"name"`
	BaseURL string      `yaml:"baseURL" json:"baseURL"`
	Header  http.Header `yaml:"-" json:"-"`
}

// Principal is who a request is sent as: the headers and cookies that carry
// its credentials, plus variables (an institute id, a user id) that cases can
// use in their paths. Both targets receive exactly the same credentials.
type Principal struct {
	Header  http.Header       `json:"header,omitempty"`
	Cookies map[string]string `json:"cookies,omitempty"`
	Vars    map[string]string `json:"vars,omitempty"`
}

// Anonymous is the principal name for requests sent without credentials. It
// needs no entry in the principals map.
const Anonymous = "anonymous"

// Case is one request, sent once per principal.
type Case struct {
	Name   string            `yaml:"name" json:"name"`
	Method string            `yaml:"method" json:"method,omitempty"` // default GET
	Path   string            `yaml:"path" json:"path"`               // may contain {var}
	Query  map[string]string `yaml:"query" json:"query,omitempty"`   // values may contain {var}
	Header map[string]string `yaml:"header" json:"header,omitempty"`

	// Principals lists who to send the request as. Empty means anonymous only.
	Principals []string `yaml:"principals" json:"principals,omitempty"`

	// Vars are fixed values for {var} placeholders. Principal and captured
	// variables override nothing here: case values win.
	Vars map[string]string `yaml:"vars" json:"vars,omitempty"`

	// Capture stores values from the reference response for later cases sent
	// as the same principal: variable name -> path (see Extract).
	Capture map[string]string `yaml:"capture" json:"capture,omitempty"`

	// Rules are added to the run-wide rules for this case only.
	Rules Rules `yaml:"rules" json:"rules,omitempty"`

	// Skip disables the case and records why.
	Skip string `yaml:"skip" json:"skip,omitempty"`
}

// Options control a run.
type Options struct {
	// AllowMethods lists the HTTP methods Run may send. Nil means GET and
	// HEAD only. A case using any other method fails without being sent.
	AllowMethods []string

	// Timeout bounds each request. Zero means 30 seconds.
	Timeout time.Duration

	// Client sends the requests. Nil means a client with Timeout.
	Client *http.Client

	// MaxBodyBytes caps how much of each body is read. Zero means 32 MiB.
	MaxBodyBytes int64

	// Filter, when set, runs only the cases and principals it returns true for.
	Filter func(c Case, principal string) bool
}

// Result is the outcome of one case sent as one principal.
type Result struct {
	Case       string        `json:"case"`
	Principal  string        `json:"principal"`
	Method     string        `json:"method"`
	Path       string        `json:"path"` // with variables substituted
	RefStatus  int           `json:"refStatus,omitempty"`
	CandStatus int           `json:"candStatus,omitempty"`
	Diffs      []Difference  `json:"diffs,omitempty"`
	Skipped    string        `json:"skipped,omitempty"`
	Err        string        `json:"error,omitempty"`
	RefTime    time.Duration `json:"refTime,omitempty"`
	CandTime   time.Duration `json:"candTime,omitempty"`
}

// Passed reports whether the two services answered alike. A skipped result
// counts as passed; an error does not.
func (r Result) Passed() bool { return r.Err == "" && len(r.Diffs) == 0 }

// Report is the outcome of a whole run.
type Report struct {
	Reference string    `json:"reference"`
	Candidate string    `json:"candidate"`
	Started   time.Time `json:"started"`
	Results   []Result  `json:"results"`
}

// Passed reports whether every result passed.
func (r *Report) Passed() bool {
	for _, res := range r.Results {
		if !res.Passed() {
			return false
		}
	}
	return true
}

// Counts returns the number of passed, failed and skipped results.
func (r *Report) Counts() (passed, failed, skipped int) {
	for _, res := range r.Results {
		switch {
		case res.Skipped != "":
			skipped++
		case res.Passed():
			passed++
		default:
			failed++
		}
	}
	return
}

// Run sends every case to both targets, as each of its principals, and
// compares the answers. Cases run in order, so a case can use values captured
// by an earlier one. The two requests of one case are sent concurrently.
//
// Run returns an error only when it cannot start (invalid rules); request
// failures and differences are recorded in the report.
func Run(ctx context.Context, ref, cand Target, principals map[string]Principal, cases []Case, rules Rules, opt Options) (*Report, error) {
	if _, err := rules.compile(); err != nil {
		return nil, err
	}
	client := opt.Client
	if client == nil {
		timeout := opt.Timeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		client = &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse // compare redirects, do not follow them
		}}
	}
	allowed := opt.AllowMethods
	if allowed == nil {
		allowed = []string{http.MethodGet, http.MethodHead}
	}
	maxBody := opt.MaxBodyBytes
	if maxBody == 0 {
		maxBody = 32 << 20
	}

	report := &Report{Reference: ref.Name, Candidate: cand.Name, Started: time.Now()}
	captured := map[string]map[string]string{} // principal -> var -> value

	for _, c := range cases {
		method := strings.ToUpper(c.Method)
		if method == "" {
			method = http.MethodGet
		}
		names := c.Principals
		if len(names) == 0 {
			names = []string{Anonymous}
		}
		caseRules, err := rules.Merge(c.Rules).compile()
		if err != nil {
			return nil, fmt.Errorf("parity: case %q: %w", c.Name, err)
		}
		for _, name := range names {
			if opt.Filter != nil && !opt.Filter(c, name) {
				continue
			}
			res := Result{Case: c.Name, Principal: name, Method: method, Path: c.Path}
			if c.Skip != "" {
				res.Skipped = c.Skip
				report.Results = append(report.Results, res)
				continue
			}
			if !slices.Contains(allowed, method) {
				res.Err = fmt.Sprintf("method %s is not allowed (Options.AllowMethods)", method)
				report.Results = append(report.Results, res)
				continue
			}
			p, known := principals[name]
			if !known && name != Anonymous {
				res.Skipped = fmt.Sprintf("principal %q is not configured", name)
				report.Results = append(report.Results, res)
				continue
			}
			vars := mergeVars(p.Vars, captured[name], c.Vars)
			target, missing := expand(c.Path, vars)
			query := url.Values{}
			for k, v := range c.Query {
				ev, m := expand(v, vars)
				missing = append(missing, m...)
				query.Set(k, ev)
			}
			if len(missing) > 0 {
				res.Skipped = "no value for {" + strings.Join(missing, "}, {") + "}"
				report.Results = append(report.Results, res)
				continue
			}
			if len(query) > 0 {
				target += "?" + query.Encode()
			}
			res.Path = target

			var wg sync.WaitGroup
			var rr, cr Response
			var rerr, cerr error
			wg.Add(2)
			go func() {
				defer wg.Done()
				rr, res.RefTime, rerr = send(ctx, client, ref, method, target, c.Header, p, maxBody)
			}()
			go func() {
				defer wg.Done()
				cr, res.CandTime, cerr = send(ctx, client, cand, method, target, c.Header, p, maxBody)
			}()
			wg.Wait()
			res.RefStatus, res.CandStatus = rr.Status, cr.Status
			switch {
			case rerr != nil:
				res.Err = ref.Name + ": " + rerr.Error()
			case cerr != nil:
				res.Err = cand.Name + ": " + cerr.Error()
			default:
				res.Diffs = caseRules.compare(rr, cr)
				for v, path := range c.Capture {
					val, err := Extract(rr.Body, path)
					if err != nil {
						continue // later cases that need it are skipped with a reason
					}
					if captured[name] == nil {
						captured[name] = map[string]string{}
					}
					captured[name][v] = val
				}
			}
			report.Results = append(report.Results, res)
		}
	}
	return report, nil
}

func mergeVars(layers ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, l := range layers {
		for k, v := range l {
			out[k] = v
		}
	}
	return out
}

var placeholder = regexp.MustCompile(`\{([A-Za-z0-9_.-]+)\}`)

// expand replaces {name} placeholders, path-escaping each value, and returns
// the names that had no value.
func expand(s string, vars map[string]string) (string, []string) {
	var missing []string
	out := placeholder.ReplaceAllStringFunc(s, func(m string) string {
		name := m[1 : len(m)-1]
		v, ok := vars[name]
		if !ok {
			missing = append(missing, name)
			return m
		}
		return url.PathEscape(v)
	})
	return out, missing
}

func send(ctx context.Context, client *http.Client, t Target, method, target string, header map[string]string, p Principal, maxBody int64) (Response, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(t.BaseURL, "/")+target, nil)
	if err != nil {
		return Response{}, 0, err
	}
	for k, vs := range t.Header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	for k, vs := range p.Header {
		req.Header.Del(k)
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	for k, v := range p.Cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return Response{}, time.Since(start), err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	elapsed := time.Since(start)
	if err != nil {
		return Response{}, elapsed, err
	}
	return Response{Status: resp.StatusCode, Header: resp.Header, Body: bytes.TrimSpace(body)}, elapsed, nil
}

func shortHash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:4])
}
