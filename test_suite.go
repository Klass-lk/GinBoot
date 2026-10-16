package ginboot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"
	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot/parity"
)

type DBSeeder interface {
	Seed(document string, data *godog.Table) error
}

// JSONSeeder is implemented by seeders that can insert documents written as
// JSON. The data is one JSON object or an array of objects; adapters for
// databases with richer types (such as MongoDB Extended JSON) interpret them.
type JSONSeeder interface {
	SeedJSON(document string, data []byte) error
}

// JSONInserter is the optional DBAdapter extension behind JSON seeding.
type JSONInserter interface {
	InsertJSON(collection string, data []byte) error
}

// DocumentReader is the optional DBAdapter extension behind the steps that
// check what a request stored. FindJSON returns the documents matching an
// Extended JSON filter (empty for all) as plain JSON values.
type DocumentReader interface {
	FindJSON(collection string, filter []byte) ([]any, error)
}

// TestSuite runs Gherkin features against a Ginboot application, either
// in-process through Router or over HTTP through BaseURL.
type TestSuite struct {
	T           *testing.T
	Router      *gin.Engine
	Server      *Server
	Resp        *http.Response
	RespBody    []byte
	Feature     *godog.Feature
	Storage     map[string]string
	RequestBody []byte
	BaseURL     string
	DbSeeders   map[string]DBSeeder

	// Adapter seeds and clears collections that have no seeder registered,
	// for the JSON seeding step. Optional.
	Adapter DBAdapter

	// Principals are the identities "I am authenticated as" can switch to.
	// PrincipalProvider is asked for any name not in the map, so tokens can be
	// minted on demand.
	Principals        map[string]parity.Principal
	PrincipalProvider func(name string) (parity.Principal, error)

	// ReferenceURL is the service the "to both services" steps compare
	// against; the application under test is the candidate. ParityRules
	// applies to every such comparison.
	ReferenceURL string
	ParityRules  parity.Rules

	// ReferenceAdapter is the reference service's database. When set, every
	// seeding step and ResetCollections also apply to it, so both services
	// start each scenario from the same data, and "should match in both
	// databases" compares what each stored. Table rows are mirrored through
	// a GenericDBSeeder's constructors; JSON seeding needs a JSONInserter.
	ReferenceAdapter DBAdapter

	// ReferenceSettle is how long "should match in both databases" keeps
	// retrying a comparison that differs, for services that write some
	// documents after they answer (an audit log fed by a queue). Default 0:
	// compare once.
	ReferenceSettle time.Duration

	// ReferenceAllowWrites lets the "to both services" steps send POST, PUT,
	// PATCH and DELETE to the reference. Off by default: a reference is often
	// a live service, and only a disposable one should receive writes.
	ReferenceAllowWrites bool

	// Tags filters scenarios (godog tag expression, e.g. "~@slow"). Without a
	// ReferenceURL, TestFeatures adds "~@parity" so scenarios that need the
	// reference are skipped.
	Tags string

	// SnapshotDir holds response snapshots; default "testdata/snapshots".
	// Set UPDATE_SNAPSHOTS=1 to (re)write them. SnapshotRules applies when
	// comparing against a snapshot.
	SnapshotDir   string
	SnapshotRules parity.Rules

	// Paths are the feature files or directories to run; default "features".
	Paths []string

	// Steps registers application-specific steps alongside the built-in ones.
	Steps func(ctx *godog.ScenarioContext)

	// ResetCollections are cleared through Adapter before every scenario, in
	// addition to the seeded ones — collections a request under test writes.
	ResetCollections []string

	collectionsToClear []string
	principal          parity.Principal
	header             http.Header
	cookies            map[string]string
	refResp            *parity.Response
}

type TestLogger struct {
	T *testing.T
}

func (ts *TestSuite) RegisterDBSeeder(document string, seeder DBSeeder) {
	if ts.DbSeeders == nil {
		ts.DbSeeders = make(map[string]DBSeeder)
	}
	ts.DbSeeders[document] = seeder
}

func (ts *TestSuite) SetBaseURL(baseURL string) {
	ts.BaseURL = baseURL
}

func (ts *TestSuite) InitializeTestSuite(ctx *godog.TestSuiteContext) {
	ctx.BeforeSuite(func() {
		ts.Storage = make(map[string]string)
	})
}

func (ts *TestSuite) InitializeScenario(ctx *godog.ScenarioContext) {
	// Data is cleared before each scenario and again after it, so the last
	// scenario's data does not outlive the run: a database that persists
	// between runs starts the next one clean.
	ctx.After(func(c context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		ts.clearData()
		return c, nil
	})
	ctx.BeforeScenario(func(sc *godog.Scenario) {
		ts.clearData()
		ts.Resp = nil
		ts.RespBody = nil
		ts.RequestBody = nil
		ts.principal = parity.Principal{}
		ts.header = http.Header{}
		ts.cookies = map[string]string{}
		ts.refResp = nil
		if ts.Storage == nil {
			ts.Storage = make(map[string]string)
		}
	})

	// Data
	ctx.Step(`^document "([^"]*)" has the following items$`, ts.documentHasTheFollowingItems)
	ctx.Step(`^document "([^"]*)" has the following JSON:?$`, ts.documentHasTheFollowingJSON)
	ctx.Step(`^document "([^"]*)" should have (\d+) items?$`, ts.documentShouldHaveItems)
	ctx.Step(`^document "([^"]*)" should have (\d+) items? matching JSON:?$`, ts.documentShouldHaveItemsMatching)
	ctx.Step(`^document "([^"]*)" should contain an item with JSON:?$`, ts.documentShouldContainAnItemWith)

	// Who and how
	ctx.Step(`^I am authenticated as "([^"]*)"$`, ts.iAmAuthenticatedAs)
	ctx.Step(`^I am not authenticated$`, ts.iAmNotAuthenticated)
	ctx.Step(`^I set the request header "([^"]*)" to "([^"]*)"$`, ts.iSetTheRequestHeader)
	ctx.Step(`^I set the cookie "([^"]*)" to "([^"]*)"$`, ts.iSetTheCookie)

	// Requests
	ctx.Step(`^I send a POST request to "([^"]*)" with body$`, ts.iSendAPOSTRequestToWithBody)
	ctx.Step(`^I send a PUT request to "([^"]*)" with body$`, ts.iSendAPUTRequestToWithBody)
	ctx.Step(`^I send a (POST|PUT|PATCH) request to "([^"]*)" with JSON:?$`, ts.iSendARequestWithJSON)
	ctx.Step(`^I send a (GET|HEAD|DELETE|POST|PUT|PATCH) request to "([^"]*)" with raw body:?$`, ts.iSendARequestWithRawBody)
	ctx.Step(`^I send a (GET|DELETE|HEAD|POST|PUT|PATCH) request to "([^"]*)"$`, ts.iSendARequestTo)
	ctx.Step(`^I send an authenticated GET request to "([^"]*)"$`, ts.iSendAnAuthenticatedGETRequestTo)
	ctx.Step(`^I send a (GET|HEAD|DELETE|POST|PUT|PATCH) request to "([^"]*)" to both services$`, ts.iSendARequestToBothServices)
	ctx.Step(`^I send a (POST|PUT|PATCH) request to "([^"]*)" to both services with JSON:?$`, ts.iSendARequestWithJSONToBothServices)
	ctx.Step(`^I send a (GET|HEAD|DELETE|POST|PUT|PATCH) request to "([^"]*)" to both services with raw body:?$`, ts.iSendARequestWithRawBodyToBothServices)
	ctx.Step(`^document "([^"]*)" should match in both databases$`, ts.documentShouldMatchInBothDatabases)
	ctx.Step(`^document "([^"]*)" should match in both databases ignoring:?$`, ts.documentShouldMatchInBothDatabasesIgnoring)

	// Assertions
	ctx.Step(`^the response status should be (\d+)$`, ts.theResponseStatusShouldBe)
	ctx.Step(`^the response body should be empty$`, ts.theResponseBodyShouldBeEmpty)
	ctx.Step(`^the response "([^"]*)" field is stored as "([^"]*)"$`, ts.theResponseFieldIsStoredAs)
	ctx.Step(`^the response should contain an item with$`, ts.theResponseShouldContainAnItemWith)
	ctx.Step(`^the response "([^"]*)" should be "((?:[^"\\]|\\.)*)"$`, ts.theResponsePathShouldBe)
	ctx.Step(`^the response "([^"]*)" should be null$`, ts.theResponsePathShouldBeNull)
	ctx.Step(`^the response "([^"]*)" should exist$`, ts.theResponsePathShouldExist)
	ctx.Step(`^the response "([^"]*)" should not exist$`, ts.theResponsePathShouldNotExist)
	ctx.Step(`^the response "([^"]*)" should have (\d+) items?$`, ts.theResponsePathShouldHaveItems)
	ctx.Step(`^the response header "([^"]*)" should be "([^"]*)"$`, ts.theResponseHeaderShouldBe)
	ctx.Step(`^the response should match snapshot "([^"]*)"$`, ts.theResponseShouldMatchSnapshot)
	ctx.Step(`^the reference service is at "([^"]*)"$`, ts.theReferenceServiceIsAt)
	ctx.Step(`^both responses should match$`, ts.bothResponsesShouldMatch)
	ctx.Step(`^both responses should match ignoring:?$`, ts.bothResponsesShouldMatchIgnoring)
	ctx.Step(`^both responses should match with lists in any order:?$`, ts.bothResponsesShouldMatchUnordered)
	ctx.Step(`^both responses should match under these rules:?$`, ts.bothResponsesShouldMatchUnder)

	if ts.Steps != nil {
		ts.Steps(ctx)
	}
}

// ---- data -------------------------------------------------------------------

func (ts *TestSuite) documentHasTheFollowingItems(document string, data *godog.Table) error {
	seeder, ok := ts.DbSeeders[document]
	if !ok {
		return fmt.Errorf("no seeder registered for document %s", document)
	}
	if err := ts.claim(document); err != nil {
		return err
	}
	if ts.ReferenceAdapter != nil {
		generic, ok := seeder.(*GenericDBSeeder)
		if !ok {
			return fmt.Errorf("the seeder for %s cannot mirror rows into the ReferenceAdapter: use a GenericDBSeeder or JSON seeding", document)
		}
		mirror := &GenericDBSeeder{Constructors: generic.Constructors, Adapter: ts.ReferenceAdapter}
		if err := mirror.Seed(document, data); err != nil {
			return fmt.Errorf("reference: %w", err)
		}
	}
	return seeder.Seed(document, data)
}

func (ts *TestSuite) documentHasTheFollowingJSON(document string, doc *godog.DocString) error {
	data := []byte(ts.substitute(doc.Content))
	if err := ts.claim(document); err != nil {
		return err
	}
	if ts.ReferenceAdapter != nil {
		ins, ok := ts.ReferenceAdapter.(JSONInserter)
		if !ok {
			return errors.New("the ReferenceAdapter must implement JSONInserter to mirror JSON seeding")
		}
		if err := ins.InsertJSON(document, data); err != nil {
			return fmt.Errorf("reference: %w", err)
		}
	}
	if s, ok := ts.DbSeeders[document].(JSONSeeder); ok {
		return s.SeedJSON(document, data)
	}
	if ins, ok := ts.Adapter.(JSONInserter); ok {
		return ins.InsertJSON(document, data)
	}
	return fmt.Errorf("no JSON seeder for document %s: register a JSONSeeder or set an Adapter that implements JSONInserter", document)
}

func (ts *TestSuite) reader() (DocumentReader, error) {
	r, ok := ts.Adapter.(DocumentReader)
	if !ok {
		return nil, fmt.Errorf("checking stored documents needs an Adapter that implements DocumentReader")
	}
	return r, nil
}

func (ts *TestSuite) documentShouldHaveItems(document string, n int) error {
	return ts.countMatching(document, "", n)
}

func (ts *TestSuite) documentShouldHaveItemsMatching(document string, n int, filter *godog.DocString) error {
	return ts.countMatching(document, ts.substitute(filter.Content), n)
}

func (ts *TestSuite) countMatching(document, filter string, n int) error {
	r, err := ts.reader()
	if err != nil {
		return err
	}
	docs, err := r.FindJSON(document, []byte(filter))
	if err != nil {
		return err
	}
	if len(docs) != n {
		return fmt.Errorf("document %q has %d matching items, want %d", document, len(docs), n)
	}
	return nil
}

// documentShouldContainAnItemWith passes when some stored item contains the
// given JSON: every field it names must be present with that value (nested
// objects compare the same way, arrays element by element). The string
// "<any>" matches any present value — ids and timestamps a request generates.
func (ts *TestSuite) documentShouldContainAnItemWith(document string, doc *godog.DocString) error {
	var want any
	if err := json.Unmarshal([]byte(ts.substitute(doc.Content)), &want); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	r, err := ts.reader()
	if err != nil {
		return err
	}
	docs, err := r.FindJSON(document, nil)
	if err != nil {
		return err
	}
	closest, closestPath := "", ""
	for _, d := range docs {
		path, ok := containsJSON("$", want, d)
		if ok {
			return nil
		}
		if closestPath == "" || len(path) > len(closestPath) {
			closestPath, closest = path, truncate(mustJSON(d), 400)
		}
	}
	if len(docs) == 0 {
		return fmt.Errorf("document %q is empty", document)
	}
	return fmt.Errorf("no item of %q contains the given JSON (%d items; closest differs at %s: %s)",
		document, len(docs), closestPath, closest)
}

// containsJSON reports whether got contains want, and where it first does not.
func containsJSON(path string, want, got any) (string, bool) {
	if s, ok := want.(string); ok && s == "<any>" {
		return path, got != nil
	}
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return path, false
		}
		for k, wv := range w {
			gv, present := g[k]
			if !present {
				return path + "." + k, false
			}
			if p, ok := containsJSON(path+"."+k, wv, gv); !ok {
				return p, false
			}
		}
		return path, true
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return path, false
		}
		for i := range w {
			if p, ok := containsJSON(fmt.Sprintf("%s[%d]", path, i), w[i], g[i]); !ok {
				return p, false
			}
		}
		return path, true
	}
	return path, reflect.DeepEqual(normaliseNumbers(want), normaliseNumbers(got))
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// clearData clears the seeded collections and ResetCollections.
func (ts *TestSuite) clearData() {
	ts.clearSeeded()
	for _, a := range []DBAdapter{ts.Adapter, ts.ReferenceAdapter} {
		if a == nil {
			continue
		}
		for _, name := range ts.ResetCollections {
			_ = a.Clear(name)
		}
	}
}

func (ts *TestSuite) clearSeeded() {
	for _, name := range ts.collectionsToClear {
		_ = ts.clearCollection(name)
	}
	ts.collectionsToClear = nil
}

// claim makes a collection the scenario's: the first time a scenario seeds
// it, whatever it holds is cleared (in the reference database too), so data
// left by an interrupted run or a manual probe cannot fail the scenario; it
// is cleared again when the scenario ends.
func (ts *TestSuite) claim(name string) error {
	if slices.Contains(ts.collectionsToClear, name) {
		return nil
	}
	ts.collectionsToClear = append(ts.collectionsToClear, name)
	return ts.clearCollection(name)
}

func (ts *TestSuite) clearCollection(name string) error {
	if ts.ReferenceAdapter != nil {
		if err := ts.ReferenceAdapter.Clear(name); err != nil {
			return fmt.Errorf("reference: clearing %s: %w", name, err)
		}
	}
	if seeder, ok := ts.DbSeeders[name].(*GenericDBSeeder); ok && seeder.Adapter != nil {
		return seeder.Adapter.Clear(name)
	} else if ts.Adapter != nil {
		return ts.Adapter.Clear(name)
	}
	return nil
}

// ---- who and how ------------------------------------------------------------

func (ts *TestSuite) iAmAuthenticatedAs(name string) error {
	if p, ok := ts.Principals[name]; ok {
		ts.principal = p
		return nil
	}
	if ts.PrincipalProvider == nil {
		return fmt.Errorf("unknown principal %q: add it to Principals or set PrincipalProvider", name)
	}
	p, err := ts.PrincipalProvider(name)
	if err != nil {
		return fmt.Errorf("principal %q: %w", name, err)
	}
	ts.principal = p
	return nil
}

func (ts *TestSuite) iAmNotAuthenticated() error {
	ts.principal = parity.Principal{}
	return nil
}

func (ts *TestSuite) iSetTheRequestHeader(name, value string) error {
	ts.header.Set(name, ts.substitute(value))
	return nil
}

func (ts *TestSuite) iSetTheCookie(name, value string) error {
	ts.cookies[name] = ts.substitute(value)
	return nil
}

// ---- requests ---------------------------------------------------------------

// Send sends a request to the application under test as the current
// principal, with the scenario's headers and cookies, and records the
// response in Resp and RespBody. "{{name}}" in path is replaced from Storage.
func (ts *TestSuite) Send(method, path string, body []byte) error {
	req, err := ts.newRequest(ts.BaseURL, method, path, body)
	if err != nil {
		return err
	}
	if ts.BaseURL != "" {
		ts.Resp, err = http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
	} else {
		if ts.Router == nil {
			return errors.New("TestSuite needs a Router or a BaseURL")
		}
		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)
		ts.Resp = w.Result()
	}
	defer ts.Resp.Body.Close()
	ts.RespBody, err = io.ReadAll(ts.Resp.Body)
	return err
}

func (ts *TestSuite) newRequest(base, method, path string, body []byte) (*http.Request, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, base+ts.substitute(path), r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	} else {
		// What a server hands a handler for a request without a body; the
		// in-process Router would otherwise see a nil Body.
		req.Body = http.NoBody
	}
	for k, vs := range ts.principal.Header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	for k, vs := range ts.header {
		req.Header.Del(k)
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	for k, v := range ts.principal.Cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	for k, v := range ts.cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	return req, nil
}

func (ts *TestSuite) iSendARequestTo(method, path string) error {
	return ts.Send(method, path, nil)
}

// iSendARequestWithRawBody sends the doc string as it is, valid JSON or not
// (an empty doc string sends an empty body), as application/json unless a
// Content-Type header was set.
func (ts *TestSuite) iSendARequestWithRawBody(method, path string, doc *godog.DocString) error {
	ts.RequestBody = []byte(ts.substitute(doc.Content))
	return ts.Send(method, path, ts.RequestBody)
}

func (ts *TestSuite) iSendAPOSTRequestToWithBody(path string, body *godog.Table) error {
	return ts.sendTable(http.MethodPost, path, body)
}

func (ts *TestSuite) iSendAPUTRequestToWithBody(path string, body *godog.Table) error {
	return ts.sendTable(http.MethodPut, path, body)
}

func (ts *TestSuite) sendTable(method, path string, body *godog.Table) error {
	var err error
	ts.RequestBody, err = ts.parseDataTableToJSON(body)
	if err != nil {
		return err
	}
	return ts.Send(method, path, ts.RequestBody)
}

func (ts *TestSuite) iSendARequestWithJSON(method, path string, doc *godog.DocString) error {
	ts.RequestBody = []byte(ts.substitute(doc.Content))
	if !json.Valid(ts.RequestBody) {
		return fmt.Errorf("request body is not valid JSON")
	}
	return ts.Send(method, path, ts.RequestBody)
}

// iSendAnAuthenticatedGETRequestTo sends the token stored as "authToken"
// (see "the response ... field is stored as") unless a principal is set.
func (ts *TestSuite) iSendAnAuthenticatedGETRequestTo(path string) error {
	if ts.principal.Header.Get("Authorization") == "" {
		ts.header.Set("Authorization", "Bearer "+ts.Storage["authToken"])
	}
	return ts.Send(http.MethodGet, path, nil)
}

// ---- comparison with a reference service ------------------------------------

func (ts *TestSuite) theReferenceServiceIsAt(u string) error {
	ts.ReferenceURL = u
	return nil
}

func (ts *TestSuite) iSendARequestToBothServices(method, path string) error {
	return ts.sendToBoth(method, path, nil)
}

func (ts *TestSuite) iSendARequestWithJSONToBothServices(method, path string, doc *godog.DocString) error {
	body := []byte(ts.substitute(doc.Content))
	if !json.Valid(body) {
		return fmt.Errorf("request body is not valid JSON")
	}
	return ts.sendToBoth(method, path, body)
}

func (ts *TestSuite) iSendARequestWithRawBodyToBothServices(method, path string, doc *godog.DocString) error {
	return ts.sendToBoth(method, path, []byte(ts.substitute(doc.Content)))
}

func (ts *TestSuite) sendToBoth(method, path string, body []byte) error {
	if ts.ReferenceURL == "" {
		return errors.New("no reference service: set ReferenceURL or use \"the reference service is at\"")
	}
	if method != http.MethodGet && method != http.MethodHead && !ts.ReferenceAllowWrites {
		return fmt.Errorf("refusing to send %s to the reference: set ReferenceAllowWrites for a disposable reference", method)
	}
	req, err := ts.newRequest(strings.TrimRight(ts.ReferenceURL, "/"), method, path, body)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("reference: %w", err)
	}
	defer resp.Body.Close()
	refBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reference: %w", err)
	}
	ts.refResp = &parity.Response{Status: resp.StatusCode, Header: resp.Header, Body: refBody}
	return ts.Send(method, path, body)
}

func (ts *TestSuite) documentShouldMatchInBothDatabases(document string) error {
	return ts.compareDatabases(document, parity.Rules{})
}

func (ts *TestSuite) documentShouldMatchInBothDatabasesIgnoring(document string, paths *godog.Table) error {
	extra := parity.Rules{}
	for _, row := range paths.Rows {
		if len(row.Cells) > 0 {
			extra.Ignore = append(extra.Ignore, row.Cells[0].Value)
		}
	}
	return ts.compareDatabases(document, extra)
}

// compareDatabases compares a collection's documents in the two databases
// as multisets (order is the database's, not the service's), under
// ParityRules plus extra; paths are rooted at the array, e.g. "$[*].createdAt".
func (ts *TestSuite) compareDatabases(document string, extra parity.Rules) error {
	deadline := time.Now().Add(ts.ReferenceSettle)
	for {
		err := ts.compareDatabasesOnce(document, extra)
		if err == nil || time.Now().After(deadline) {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (ts *TestSuite) compareDatabasesOnce(document string, extra parity.Rules) error {
	cand, ok1 := ts.Adapter.(DocumentReader)
	ref, ok2 := ts.ReferenceAdapter.(DocumentReader)
	if !ok1 || !ok2 {
		return errors.New("comparing databases needs an Adapter and a ReferenceAdapter that implement DocumentReader")
	}
	refDocs, err := ref.FindJSON(document, nil)
	if err != nil {
		return fmt.Errorf("reference: %w", err)
	}
	candDocs, err := cand.FindJSON(document, nil)
	if err != nil {
		return err
	}
	rules := ts.ParityRules.Merge(extra).Merge(parity.Rules{Unordered: map[string]string{"$": ""}})
	diffs, err := parity.Compare(jsonResponse(refDocs), jsonResponse(candDocs), rules)
	if err != nil {
		return err
	}
	return diffError(fmt.Sprintf("document %q differs between the databases (%d reference, %d candidate items)",
		document, len(refDocs), len(candDocs)), diffs)
}

func jsonResponse(v any) parity.Response {
	return parity.Response{Status: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: mustJSON(v)}
}

func (ts *TestSuite) bothResponsesShouldMatch() error {
	return ts.compareWithReference(ts.ParityRules)
}

func (ts *TestSuite) bothResponsesShouldMatchIgnoring(paths *godog.Table) error {
	rules := ts.ParityRules
	extra := parity.Rules{}
	for _, row := range paths.Rows {
		if len(row.Cells) > 0 {
			extra.Ignore = append(extra.Ignore, row.Cells[0].Value)
		}
	}
	return ts.compareWithReference(rules.Merge(extra))
}

// bothResponsesShouldMatchUnordered compares with the listed arrays as
// multisets, for services that build a list in no particular order. A row's
// second cell, when given, is the key that pairs elements (see
// parity.Rules.Unordered); otherwise whole elements are compared.
func (ts *TestSuite) bothResponsesShouldMatchUnordered(paths *godog.Table) error {
	extra := parity.Rules{Unordered: map[string]string{}}
	for _, row := range paths.Rows {
		if len(row.Cells) == 0 {
			continue
		}
		key := ""
		if len(row.Cells) > 1 {
			key = row.Cells[1].Value
		}
		extra.Unordered[row.Cells[0].Value] = key
	}
	return ts.compareWithReference(ts.ParityRules.Merge(extra))
}

// bothResponsesShouldMatchUnder compares the responses under several kinds
// of rule at once, one per row: "| ignore | $.id |" or
// "| unordered | $.ranks | student.id |" (the key is optional).
func (ts *TestSuite) bothResponsesShouldMatchUnder(table *godog.Table) error {
	extra := parity.Rules{Unordered: map[string]string{}}
	for _, row := range table.Rows {
		if len(row.Cells) < 2 {
			return fmt.Errorf("a rule row needs a kind and a path, got %d cells", len(row.Cells))
		}
		kind, path := row.Cells[0].Value, row.Cells[1].Value
		switch kind {
		case "ignore":
			extra.Ignore = append(extra.Ignore, path)
		case "unordered":
			key := ""
			if len(row.Cells) > 2 {
				key = row.Cells[2].Value
			}
			extra.Unordered[path] = key
		default:
			return fmt.Errorf("unknown rule %q: use ignore or unordered", kind)
		}
	}
	return ts.compareWithReference(ts.ParityRules.Merge(extra))
}

func (ts *TestSuite) compareWithReference(rules parity.Rules) error {
	if ts.refResp == nil || ts.Resp == nil {
		return errors.New("send a request to both services first")
	}
	diffs, err := parity.Compare(*ts.refResp, ts.response(), rules)
	if err != nil {
		return err
	}
	return diffError("reference and candidate differ", diffs)
}

func (ts *TestSuite) response() parity.Response {
	return parity.Response{Status: ts.Resp.StatusCode, Header: ts.Resp.Header, Body: ts.RespBody}
}

func diffError(msg string, diffs []parity.Difference) error {
	if len(diffs) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString(msg)
	for i, d := range diffs {
		if i == 20 {
			fmt.Fprintf(&b, "\n  … %d more", len(diffs)-i)
			break
		}
		fmt.Fprintf(&b, "\n  %s %s", d.Kind, d.Path)
	}
	return errors.New(b.String())
}

// ---- assertions -------------------------------------------------------------

func (ts *TestSuite) requireResponse() error {
	if ts.Resp == nil {
		return errors.New("no response: send a request first")
	}
	return nil
}

func (ts *TestSuite) theResponseBodyShouldBeEmpty() error {
	if ts.Resp == nil {
		return errors.New("no response yet")
	}
	if len(ts.RespBody) != 0 {
		return fmt.Errorf("expected an empty body, got %q", ts.RespBody)
	}
	return nil
}

func (ts *TestSuite) theResponseStatusShouldBe(status int) error {
	if err := ts.requireResponse(); err != nil {
		return err
	}
	if ts.Resp.StatusCode != status {
		return fmt.Errorf("expected status %d, got %d: %s", status, ts.Resp.StatusCode, truncate(ts.RespBody, 300))
	}
	return nil
}

func (ts *TestSuite) theResponseFieldIsStoredAs(field, key string) error {
	v, err := ts.lookup(field)
	if err != nil {
		return err
	}
	s, err := scalarText(v)
	if err != nil {
		return err
	}
	ts.Storage[key] = s
	return nil
}

func (ts *TestSuite) theResponseShouldContainAnItemWith(body *godog.Table) error {
	expected, err := ts.parseDataTableToJSON(body)
	if err != nil {
		return err
	}
	var expectedMap map[string]interface{}
	if err := json.Unmarshal(expected, &expectedMap); err != nil {
		return err
	}
	var actualMap map[string]interface{}
	if err := json.Unmarshal(ts.RespBody, &actualMap); err != nil {
		return fmt.Errorf("response is not a JSON object: %w", err)
	}
	for key, expectedValue := range expectedMap {
		actualValue, ok := actualMap[key]
		if !ok {
			return fmt.Errorf("field %s not found in response", key)
		}
		if fmt.Sprint(expectedValue) != fmt.Sprint(actualValue) {
			return fmt.Errorf("field %s: expected %v, got %v", key, expectedValue, actualValue)
		}
	}
	return nil
}

// Lookup returns the value a path selects in the last response, for use in
// application steps. A path without a leading "$" names a top-level field.
// See parity.Lookup for the syntax.
func (ts *TestSuite) Lookup(path string) (any, error) { return ts.lookup(path) }

func (ts *TestSuite) lookup(path string) (any, error) {
	if err := ts.requireResponse(); err != nil {
		return nil, err
	}
	path = ts.substitute(path)
	if !strings.HasPrefix(path, "$") {
		path = "$." + path
	}
	return parity.Lookup(ts.RespBody, path)
}

func (ts *TestSuite) theResponsePathShouldBe(path, expected string) error {
	v, err := ts.lookup(path)
	if err != nil {
		return err
	}
	expected = ts.substitute(unescapeQuotes(expected))
	switch v.(type) {
	case map[string]any, []any:
		want, ok := decodeLoose(expected)
		if !ok || !reflect.DeepEqual(normaliseNumbers(want), normaliseNumbers(v)) {
			b, _ := json.Marshal(v)
			return fmt.Errorf("%s: expected %s, got %s", path, expected, b)
		}
		return nil
	}
	got, err := scalarText(v)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if got != expected {
		return fmt.Errorf("%s: expected %q, got %q", path, expected, got)
	}
	return nil
}

func (ts *TestSuite) theResponsePathShouldBeNull(path string) error {
	v, err := ts.lookup(path)
	if err != nil {
		return err
	}
	if v != nil {
		return fmt.Errorf("%s: expected null, got %v", path, v)
	}
	return nil
}

func (ts *TestSuite) theResponsePathShouldExist(path string) error {
	_, err := ts.lookup(path)
	return err
}

func (ts *TestSuite) theResponsePathShouldNotExist(path string) error {
	_, err := ts.lookup(path)
	switch {
	case err == nil:
		return fmt.Errorf("%s: expected no value, but it exists", path)
	case errors.Is(err, parity.ErrNotFound):
		return nil
	}
	return err
}

func (ts *TestSuite) theResponsePathShouldHaveItems(path string, n int) error {
	v, err := ts.lookup(path)
	if err != nil {
		return err
	}
	var got int
	switch t := v.(type) {
	case []any:
		got = len(t)
	case map[string]any:
		got = len(t)
	default:
		return fmt.Errorf("%s: expected an array or object, got %T", path, v)
	}
	if got != n {
		return fmt.Errorf("%s: expected %d items, got %d", path, n, got)
	}
	return nil
}

func (ts *TestSuite) theResponseHeaderShouldBe(name, expected string) error {
	if err := ts.requireResponse(); err != nil {
		return err
	}
	if got := ts.Resp.Header.Get(name); got != ts.substitute(expected) {
		return fmt.Errorf("header %s: expected %q, got %q", name, expected, got)
	}
	return nil
}

// ---- snapshots --------------------------------------------------------------

type snapshot struct {
	Status      int             `json:"status"`
	ContentType string          `json:"contentType,omitempty"`
	Body        json.RawMessage `json:"body,omitempty"`
	Text        string          `json:"text,omitempty"`
}

var snapshotName = regexp.MustCompile(`^[A-Za-z0-9_.\-/]+$`)

func (ts *TestSuite) theResponseShouldMatchSnapshot(name string) error {
	if err := ts.requireResponse(); err != nil {
		return err
	}
	if !snapshotName.MatchString(name) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid snapshot name %q", name)
	}
	dir := ts.SnapshotDir
	if dir == "" {
		dir = filepath.Join("testdata", "snapshots")
	}
	file := filepath.Join(dir, name+".json")

	if os.Getenv("UPDATE_SNAPSHOTS") != "" {
		snap := snapshot{Status: ts.Resp.StatusCode, ContentType: ts.Resp.Header.Get("Content-Type")}
		if len(bytes.TrimSpace(ts.RespBody)) > 0 && json.Valid(ts.RespBody) {
			var pretty bytes.Buffer
			_ = json.Indent(&pretty, ts.RespBody, "", "  ")
			snap.Body = pretty.Bytes()
		} else {
			snap.Text = string(ts.RespBody)
		}
		b, err := json.MarshalIndent(snap, "", "  ")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			return err
		}
		return os.WriteFile(file, append(b, '\n'), 0o644)
	}

	b, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("snapshot %s: %w (run with UPDATE_SNAPSHOTS=1 to create it)", name, err)
	}
	var snap snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return fmt.Errorf("snapshot %s: %w", name, err)
	}
	body := []byte(snap.Body)
	if snap.Text != "" {
		body = []byte(snap.Text)
	}
	want := parity.Response{Status: snap.Status, Header: http.Header{"Content-Type": {snap.ContentType}}, Body: body}
	diffs, err := parity.Compare(want, ts.response(), ts.SnapshotRules)
	if err != nil {
		return err
	}
	return diffError("response differs from snapshot "+name, diffs)
}

// ---- helpers ----------------------------------------------------------------

var storageRef = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)

// substitute replaces "{{name}}" with values from Storage; unknown names are
// left as they are.
func (ts *TestSuite) substitute(s string) string {
	return storageRef.ReplaceAllStringFunc(s, func(m string) string {
		if v, ok := ts.Storage[storageRef.FindStringSubmatch(m)[1]]; ok {
			return v
		}
		return m
	})
}

// unescapeQuotes turns \" into " so step arguments can hold JSON.
func unescapeQuotes(s string) string {
	return strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(s)
}

func scalarText(v any) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case json.Number:
		return t.String(), nil
	case bool:
		return strconv.FormatBool(t), nil
	case nil:
		return "", errors.New("value is null")
	}
	return "", fmt.Errorf("value is a %T, not a scalar", v)
}

func decodeLoose(s string) (any, bool) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	return v, true
}

// normaliseNumbers makes 5 and 5.0 compare equal under reflect.DeepEqual.
func normaliseNumbers(v any) any {
	switch t := v.(type) {
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return t.String()
		}
		return f
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = normaliseNumbers(e)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = normaliseNumbers(e)
		}
		return out
	}
	return v
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

func (ts *TestSuite) parseDataTableToJSONs(body *godog.Table) ([]byte, error) {
	if len(body.Rows) < 2 {
		return nil, fmt.Errorf("table must have at least two rows")
	}
	headers := body.Rows[0].Cells
	var data []map[string]interface{}
	for i := 1; i < len(body.Rows); i++ {
		row := body.Rows[i]
		rowData := make(map[string]interface{})
		for j, cell := range row.Cells {
			rowData[headers[j].Value] = ts.substitute(cell.Value)
		}
		data = append(data, rowData)
	}
	return json.Marshal(data)
}

func (ts *TestSuite) parseDataTableToJSON(body *godog.Table) ([]byte, error) {
	if len(body.Rows) < 2 {
		return nil, fmt.Errorf("table must have at least two rows")
	}
	headers := body.Rows[0].Cells
	data := make(map[string]interface{})
	row := body.Rows[1]
	for j, cell := range row.Cells {
		data[headers[j].Value] = ts.substitute(cell.Value)
	}
	return json.Marshal(data)
}

type DBAdapter interface {
	Insert(collection string, doc interface{}) error
	Clear(collection string) error
}

// GenericDBSeeder is a sample DBSeeder that uses reflection to populate structs.
// Users can use this as a starting point for their own seeders.
type GenericDBSeeder struct {
	Constructors map[string]func() interface{}
	Adapter      DBAdapter
}

func NewGenericDBSeeder(adapter DBAdapter) *GenericDBSeeder {
	return &GenericDBSeeder{
		Constructors: make(map[string]func() interface{}),
		Adapter:      adapter,
	}
}

func (gds *GenericDBSeeder) Register(name string, constructor func() interface{}) {
	gds.Constructors[name] = constructor
}

// SeedJSON inserts documents written as JSON through the adapter, which must
// implement JSONInserter.
func (gds *GenericDBSeeder) SeedJSON(document string, data []byte) error {
	ins, ok := gds.Adapter.(JSONInserter)
	if !ok {
		return fmt.Errorf("adapter %T cannot insert JSON documents", gds.Adapter)
	}
	return ins.InsertJSON(document, data)
}

func (gds *GenericDBSeeder) Seed(document string, data *godog.Table) error {
	constructor, ok := gds.Constructors[document]
	if !ok {
		return fmt.Errorf("no constructor registered for document type: %s", document)
	}

	headers := data.Rows[0].Cells
	for i := 1; i < len(data.Rows); i++ {
		row := data.Rows[i]
		docInstance := constructor() // Create a new instance of the document struct

		val := reflect.ValueOf(docInstance).Elem()
		typ := val.Type()

		for j, cell := range row.Cells {
			fieldName := headers[j].Value
			goFieldName := toPascalCase(fieldName)

			field := val.FieldByName(goFieldName)
			if !field.IsValid() {
				for k := 0; k < typ.NumField(); k++ {
					structField := typ.Field(k)
					if jsonTag := strings.Split(structField.Tag.Get("json"), ",")[0]; jsonTag == fieldName {
						field = val.Field(k)
						break
					}
				}
			}

			if !field.IsValid() || !field.CanSet() {
				return fmt.Errorf("could not set field %s for document %s", fieldName, document)
			}
			if err := setField(field, cell.Value); err != nil {
				return fmt.Errorf("field %s: %w", fieldName, err)
			}
		}
		if err := gds.Adapter.Insert(document, docInstance); err != nil {
			return err
		}
	}
	return nil
}

// setField parses a table cell into a struct field. Strings, integers,
// floats and booleans are read directly; anything else (times, slices,
// nested structs) is read as JSON.
func setField(field reflect.Value, s string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if s == "" {
			field.SetInt(0)
			return nil
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return err
		}
		field.SetInt(n)
	case reflect.Float32, reflect.Float64:
		if s == "" {
			field.SetFloat(0)
			return nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
		}
		field.SetFloat(f)
	case reflect.Bool:
		if s == "" {
			field.SetBool(false)
			return nil
		}
		b, err := strconv.ParseBool(s)
		if err != nil {
			return err
		}
		field.SetBool(b)
	default:
		ptr := reflect.New(field.Type())
		if err := json.Unmarshal([]byte(s), ptr.Interface()); err != nil {
			return fmt.Errorf("unsupported value for %s: %w", field.Type(), err)
		}
		field.Set(ptr.Elem())
	}
	return nil
}

func toPascalCase(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (tl *TestLogger) Write(p []byte) (n int, err error) {
	if tl.T != nil {
		tl.T.Logf("%s", p)
	}
	return len(p), nil
}

// TestFeatures runs the suite's features and fails t if any scenario fails.
func TestFeatures(t *testing.T, suite *TestSuite) {
	suite.T = t
	paths := suite.Paths
	if len(paths) == 0 {
		paths = []string{"features"}
	}
	tags := suite.Tags
	if suite.ReferenceURL == "" {
		if tags != "" {
			tags += " && "
		}
		tags += "~@parity"
	}
	opts := godog.Options{
		Format:    "pretty",
		Output:    colors.Colored(&TestLogger{T: t}),
		Paths:     paths,
		Tags:      tags,
		Strict:    true,
		Randomize: 0,
	}

	status := godog.TestSuite{
		Name:                 "ginboot",
		TestSuiteInitializer: suite.InitializeTestSuite,
		ScenarioInitializer:  suite.InitializeScenario,
		Options:              &opts,
	}.Run()
	if status != 0 {
		t.Fail()
	}
}
