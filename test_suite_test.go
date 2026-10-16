package ginboot

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cucumber/godog"
	messages "github.com/cucumber/messages-go/v16"
	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot/parity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memoryAdapter is a DBAdapter and JSONInserter over maps, enough to
// exercise the seeding steps without a database.
type memoryAdapter struct {
	mu   sync.Mutex
	docs map[string][]any
}

func (m *memoryAdapter) Insert(collection string, doc any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.docs[collection] = append(m.docs[collection], doc)
	return nil
}

func (m *memoryAdapter) InsertJSON(collection string, data []byte) error {
	var many []map[string]any
	if err := json.Unmarshal(data, &many); err != nil {
		var one map[string]any
		if err := json.Unmarshal(data, &one); err != nil {
			return err
		}
		many = []map[string]any{one}
	}
	for _, d := range many {
		if err := m.Insert(collection, d); err != nil {
			return err
		}
	}
	return nil
}

// FindJSON returns the collection's documents as JSON values; a filter
// keeps those whose top-level fields equal the filter's.
func (m *memoryAdapter) FindJSON(collection string, filter []byte) ([]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var want map[string]any
	if len(filter) > 0 {
		if err := json.Unmarshal(filter, &want); err != nil {
			return nil, err
		}
	}
	out := []any{}
	for _, d := range m.docs[collection] {
		b, _ := json.Marshal(d)
		var doc map[string]any
		_ = json.Unmarshal(b, &doc)
		keep := true
		for k, v := range want {
			if doc[k] != v {
				keep = false
			}
		}
		if keep {
			out = append(out, doc)
		}
	}
	return out, nil
}

func (m *memoryAdapter) Clear(collection string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.docs, collection)
	return nil
}

type item struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Price  float64 `json:"price"`
	Active bool    `json:"active"`
}

func suiteRouter(db *memoryAdapter, generatedAt, version string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/items/:id", func(c *gin.Context) {
		db.mu.Lock()
		defer db.mu.Unlock()
		for _, d := range db.docs["items"] {
			if it := d.(*item); it.ID == c.Param("id") {
				c.JSON(200, gin.H{"id": it.ID, "name": it.Name, "price": it.Price, "active": it.Active,
					"tags": []string{"x", "y"}, "nothing": nil, "generatedAt": generatedAt})
				return
			}
		}
		c.JSON(404, gin.H{"error": "not found"})
	})
	r.GET("/notes", func(c *gin.Context) {
		db.mu.Lock()
		defer db.mu.Unlock()
		c.JSON(200, gin.H{"count": len(db.docs["notes"])})
	})
	r.POST("/notes", func(c *gin.Context) {
		var body map[string]any
		_ = c.BindJSON(&body)
		db.mu.Lock()
		body["id"] = fmt.Sprintf("note-%d", len(db.docs["notes"])+1)
		db.docs["notes"] = append(db.docs["notes"], body)
		db.mu.Unlock()
		c.JSON(201, body)
	})
	r.GET("/echo/:value", func(c *gin.Context) { c.JSON(200, gin.H{"value": c.Param("value")}) })
	r.POST("/echo", func(c *gin.Context) {
		var body map[string]any
		_ = c.BindJSON(&body)
		c.JSON(200, body)
	})
	r.GET("/whoami", func(c *gin.Context) {
		device, _ := c.Cookie("device")
		c.JSON(200, gin.H{"auth": c.GetHeader("Authorization"), "trace": c.GetHeader("X-Trace"), "device": device})
	})
	r.Any("/raw", func(c *gin.Context) {
		b, _ := io.ReadAll(c.Request.Body)
		if len(b) == 0 && c.Request.Method == http.MethodDelete {
			c.Status(http.StatusOK)
			return
		}
		c.JSON(200, gin.H{"body": string(b), "type": c.GetHeader("Content-Type")})
	})
	r.GET("/drift", func(c *gin.Context) { c.JSON(200, gin.H{"version": version, "same": true}) })
	return r
}

func TestSuiteFeatures(t *testing.T) {
	db := &memoryAdapter{docs: map[string][]any{}}
	refDB := &memoryAdapter{docs: map[string][]any{"items": {&item{ID: "1", Name: "Chalk", Price: 2.5, Active: true}}}}
	reference := httptest.NewServer(suiteRouter(refDB, "yesterday", "java"))
	defer reference.Close()

	seeder := NewGenericDBSeeder(db)
	seeder.Register("items", func() any { return &item{} })

	snapshots := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(snapshots, "item-1.json"), []byte(`{
  "status": 200,
  "contentType": "application/json",
  "body": {"id": "1", "name": "Chalk", "price": 2.50, "active": true, "tags": ["x", "y"], "nothing": null, "generatedAt": "today"}
}`), 0o644))

	suite := &TestSuite{
		Router:       suiteRouter(db, "today", "go"),
		Adapter:      db,
		ReferenceURL: reference.URL,
		// The reference here is an in-process test server over refDB, so it
		// may receive writes.
		ReferenceAdapter:     refDB,
		ReferenceAllowWrites: true,
		SnapshotDir:          snapshots,
		Paths:                []string{"testdata/suite/features"},
		// Written by POST /notes, which no scenario seeds.
		ResetCollections: []string{"notes"},
		Principals: map[string]parity.Principal{
			"admin": {Header: http.Header{"Authorization": {"Bearer admin-token"}}},
		},
		PrincipalProvider: func(name string) (parity.Principal, error) {
			return parity.Principal{Header: http.Header{"Authorization": {"Bearer minted-for-" + name}}}, nil
		},
	}
	suite.RegisterDBSeeder("items", seeder)
	TestFeatures(t, suite)
}

func TestSuiteStepsFail(t *testing.T) {
	ts := &TestSuite{Router: suiteRouter(&memoryAdapter{docs: map[string][]any{}}, "", ""), Storage: map[string]string{}}
	ts.header = http.Header{}
	assert.Error(t, ts.theResponseStatusShouldBe(200), "no response yet")
	require.NoError(t, ts.Send("GET", "/items/9", nil))
	assert.ErrorContains(t, ts.theResponseStatusShouldBe(200), "expected status 200, got 404")
	assert.NoError(t, ts.theResponseStatusShouldBe(404))
	assert.Error(t, ts.theResponsePathShouldBe("error", "other"))
	assert.Error(t, ts.theResponsePathShouldExist("$.nope"))
	assert.Error(t, ts.theResponsePathShouldNotExist("error"))
	assert.Error(t, ts.theResponsePathShouldHaveItems("error", 1))
	assert.Error(t, ts.iAmAuthenticatedAs("nobody"))
	assert.Error(t, ts.compareWithReference(parity.Rules{}), "no reference request was sent")
	assert.Error(t, ts.theResponseShouldMatchSnapshot("../escape"))

	ts.SnapshotDir = t.TempDir()
	assert.ErrorContains(t, ts.theResponseShouldMatchSnapshot("missing"), "UPDATE_SNAPSHOTS")
	t.Setenv("UPDATE_SNAPSHOTS", "1")
	require.NoError(t, ts.theResponseShouldMatchSnapshot("created"))
	t.Setenv("UPDATE_SNAPSHOTS", "")
	assert.NoError(t, ts.theResponseShouldMatchSnapshot("created"), "a freshly written snapshot matches")
}

func TestSuiteSkipsParityWithoutReference(t *testing.T) {
	db := &memoryAdapter{docs: map[string][]any{}}
	TestFeatures(t, &TestSuite{Router: suiteRouter(db, "", ""), Adapter: db,
		Paths: []string{"testdata/suite/parity-only"}})
}

func TestSuiteReferenceGuards(t *testing.T) {
	refDB := &memoryAdapter{docs: map[string][]any{}}
	reference := httptest.NewServer(suiteRouter(refDB, "", ""))
	defer reference.Close()
	db := &memoryAdapter{docs: map[string][]any{}}
	ts := &TestSuite{Router: suiteRouter(db, "", ""), Adapter: db, ReferenceURL: reference.URL,
		ReferenceAdapter: refDB, Storage: map[string]string{}, header: http.Header{}}

	assert.ErrorContains(t, ts.sendToBoth("POST", "/notes", []byte(`{"text":"x"}`)), "ReferenceAllowWrites")
	assert.Empty(t, refDB.docs["notes"], "a refused write reaches neither service")
	assert.Empty(t, db.docs["notes"])
	require.NoError(t, ts.sendToBoth("GET", "/notes", nil), "reads need no opt-in")

	ts.ReferenceAllowWrites = true
	require.NoError(t, ts.sendToBoth("POST", "/notes", []byte(`{"text":"x"}`)))
	require.NoError(t, ts.compareDatabases("notes", parity.Rules{}))
	db.docs["notes"] = append(db.docs["notes"], map[string]any{"id": "note-2", "text": "only here"})
	assert.ErrorContains(t, ts.compareDatabases("notes", parity.Rules{}), "1 reference, 2 candidate")

	ts.ReferenceAdapter = &memoryAdapter{docs: map[string][]any{}}
	ts.DbSeeders = map[string]DBSeeder{"items": custom{}}
	assert.ErrorContains(t, ts.documentHasTheFollowingItems("items", &godog.Table{}), "cannot mirror")
}

// custom is a DBSeeder the suite cannot mirror into a reference database.
type custom struct{}

func (custom) Seed(string, *godog.Table) error { return nil }

func TestContainsJSON(t *testing.T) {
	got := map[string]any{"a": 1.0, "b": map[string]any{"c": "x", "d": []any{1.0, 2.0}}, "e": nil}
	for _, tc := range []struct {
		want string
		ok   bool
		path string
	}{
		{`{"a": 1}`, true, ""},
		{`{"b": {"c": "x"}}`, true, ""},
		{`{"b": {"d": [1, 2]}}`, true, ""},
		{`{"a": "<any>", "b": "<any>"}`, true, ""},
		{`{"e": "<any>"}`, false, "$.e"},
		{`{"b": {"d": [1]}}`, false, "$.b.d"},
		{`{"b": {"c": "y"}}`, false, "$.b.c"},
		{`{"z": 1}`, false, "$.z"},
	} {
		var want any
		require.NoError(t, json.Unmarshal([]byte(tc.want), &want))
		path, ok := containsJSON("$", want, got)
		assert.Equal(t, tc.ok, ok, tc.want)
		if !tc.ok {
			assert.Equal(t, tc.path, path, tc.want)
		}
	}
}

// failingAdapter fails every call; plainAdapter only inserts and clears, so
// the optional JSONInserter and DocumentReader steps have nothing to use.
type failingAdapter struct{}

func (failingAdapter) Insert(string, any) error               { return errors.New("insert failed") }
func (failingAdapter) Clear(string) error                     { return errors.New("clear failed") }
func (failingAdapter) InsertJSON(string, []byte) error        { return errors.New("insert failed") }
func (failingAdapter) FindJSON(string, []byte) ([]any, error) { return nil, errors.New("find failed") }

type plainAdapter struct{}

func (plainAdapter) Insert(string, any) error { return nil }
func (plainAdapter) Clear(string) error       { return nil }

func doc(s string) *godog.DocString { return &godog.DocString{Content: s} }

func table(rows ...[]string) *godog.Table {
	t := &godog.Table{}
	for _, r := range rows {
		row := &messages.PickleTableRow{}
		for _, c := range r {
			row.Cells = append(row.Cells, &messages.PickleTableCell{Value: c})
		}
		t.Rows = append(t.Rows, row)
	}
	return t
}

func TestSuiteDataStepErrors(t *testing.T) {
	mem := func() *memoryAdapter { return &memoryAdapter{docs: map[string][]any{}} }
	items := table([]string{"id", "name"}, []string{"1", "Chalk"})

	ts := &TestSuite{Storage: map[string]string{}}
	assert.ErrorContains(t, ts.documentHasTheFollowingItems("items", items), "no seeder registered")

	// A reference without the constructor for the rows.
	seeder := NewGenericDBSeeder(mem())
	ts.DbSeeders = map[string]DBSeeder{"items": seeder}
	ts.ReferenceAdapter = mem()
	assert.ErrorContains(t, ts.documentHasTheFollowingItems("items", items), "reference:")

	// JSON seeding: the reference must insert JSON, and its errors surface.
	ts.ReferenceAdapter = plainAdapter{}
	assert.ErrorContains(t, ts.documentHasTheFollowingJSON("notes", doc(`{}`)), "JSONInserter")
	ts.ReferenceAdapter = failingAdapter{}
	assert.ErrorContains(t, ts.documentHasTheFollowingJSON("notes", doc(`{}`)), "reference: insert failed")

	// Without a reference: a registered JSONSeeder, then the Adapter, then nothing.
	ts.ReferenceAdapter = nil
	seeded := mem()
	ts.DbSeeders = map[string]DBSeeder{"notes": NewGenericDBSeeder(seeded)}
	require.NoError(t, ts.documentHasTheFollowingJSON("notes", doc(`{"_id":"n1"}`)))
	assert.Len(t, seeded.docs["notes"], 1)
	assert.ErrorContains(t, NewGenericDBSeeder(plainAdapter{}).SeedJSON("notes", []byte(`{}`)), "cannot insert JSON")
	ts.DbSeeders, ts.Adapter = nil, plainAdapter{}
	assert.ErrorContains(t, ts.documentHasTheFollowingJSON("notes", doc(`{}`)), "no JSON seeder")

	// Reading stored documents.
	assert.ErrorContains(t, ts.documentShouldHaveItems("notes", 0), "DocumentReader")
	assert.ErrorContains(t, ts.documentShouldContainAnItemWith("notes", doc(`{}`)), "DocumentReader")
	ts.Adapter = failingAdapter{}
	assert.ErrorContains(t, ts.documentShouldHaveItems("notes", 0), "find failed")
	assert.ErrorContains(t, ts.documentShouldContainAnItemWith("notes", doc(`{}`)), "find failed")
	assert.ErrorContains(t, ts.documentShouldContainAnItemWith("notes", doc(`{`)), "invalid JSON")
	stored := mem()
	ts.Adapter = stored
	assert.ErrorContains(t, ts.documentShouldContainAnItemWith("notes", doc(`{"a":1}`)), "is empty")
	stored.docs["notes"] = []any{map[string]any{"a": 2.0}, map[string]any{"a": 3.0, "b": "x"}}
	assert.ErrorContains(t, ts.documentShouldHaveItems("notes", 1), "has 2 matching items, want 1")
	assert.ErrorContains(t, ts.documentShouldContainAnItemWith("notes", doc(`{"a":1}`)), "no item of \"notes\" contains")

	// clearData tolerates adapters that fail to clear.
	ts.Adapter, ts.ReferenceAdapter, ts.ResetCollections = failingAdapter{}, failingAdapter{}, []string{"notes"}
	ts.collectionsToClear = []string{"notes"}
	ts.clearData()
	assert.Empty(t, ts.collectionsToClear)
}

func TestSuiteReferenceStepErrors(t *testing.T) {
	refDB := &memoryAdapter{docs: map[string][]any{}}
	reference := httptest.NewServer(suiteRouter(refDB, "", ""))
	defer reference.Close()
	db := &memoryAdapter{docs: map[string][]any{}}
	ts := &TestSuite{Router: suiteRouter(db, "", ""), Adapter: db, Storage: map[string]string{}, header: http.Header{}}

	assert.ErrorContains(t, ts.iSendARequestToBothServices("GET", "/notes"), "no reference service")
	ts.ReferenceURL, ts.ReferenceAdapter, ts.ReferenceAllowWrites = reference.URL, refDB, true
	assert.ErrorContains(t, ts.iSendARequestWithJSONToBothServices("POST", "/notes", doc(`{`)), "not valid JSON")
	require.NoError(t, ts.iSendARequestWithRawBodyToBothServices("POST", "/notes", doc(`{"text":"raw"}`)))
	require.NoError(t, ts.bothResponsesShouldMatch())
	require.NoError(t, ts.documentShouldMatchInBothDatabasesIgnoring("notes", table([]string{"$[*].id"}, []string{})))

	// Comparing databases needs readers on both sides, and their errors surface.
	ts.ReferenceAdapter = plainAdapter{}
	assert.ErrorContains(t, ts.documentShouldMatchInBothDatabases("notes"), "DocumentReader")
	ts.ReferenceAdapter = failingAdapter{}
	assert.ErrorContains(t, ts.documentShouldMatchInBothDatabases("notes"), "reference: find failed")
	ts.Adapter, ts.ReferenceAdapter = failingAdapter{}, refDB
	assert.ErrorContains(t, ts.documentShouldMatchInBothDatabases("notes"), "find failed")
	ts.ParityRules = parity.Rules{Ignore: []string{"no-dollar"}}
	ts.Adapter = db
	assert.Error(t, ts.documentShouldMatchInBothDatabases("notes"), "a bad rule is reported")
	ts.ParityRules = parity.Rules{}

	// A reference that cannot be reached.
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	ts.ReferenceURL = closed.URL
	assert.ErrorContains(t, ts.iSendARequestToBothServices("GET", "/notes"), "reference:")

	// The empty-body assertion.
	ts = &TestSuite{Router: suiteRouter(db, "", ""), Storage: map[string]string{}, header: http.Header{}}
	assert.ErrorContains(t, ts.theResponseBodyShouldBeEmpty(), "no response yet")
	require.NoError(t, ts.Send("GET", "/notes", nil))
	assert.ErrorContains(t, ts.theResponseBodyShouldBeEmpty(), "expected an empty body")
}

func TestSuiteSmallBranches(t *testing.T) {
	// Tags combine with the @parity filter when there is no reference.
	db := &memoryAdapter{docs: map[string][]any{}}
	TestFeatures(t, &TestSuite{Router: suiteRouter(db, "", ""), Adapter: db,
		Paths: []string{"testdata/suite/parity-only"}, Tags: "@parity"})

	// clearData skips a missing adapter.
	ts := &TestSuite{ResetCollections: []string{"notes"}, Adapter: db}
	db.docs["notes"] = []any{map[string]any{}}
	ts.clearData()
	assert.Empty(t, db.docs["notes"])

	// A request that cannot be built is reported, for either service.
	ts = &TestSuite{Router: suiteRouter(db, "", ""), Storage: map[string]string{}, header: http.Header{},
		ReferenceURL: "http://127.0.0.1:1"}
	assert.Error(t, ts.Send("BAD METHOD", "/notes", nil))
	assert.Error(t, ts.iSendARequestToBothServices("GET", "/%zz"))
}

func TestCompareDatabasesSettles(t *testing.T) {
	ref := &memoryAdapter{docs: map[string][]any{"log": {map[string]any{"a": 1.0}}}}
	cand := &memoryAdapter{docs: map[string][]any{}}
	ts := &TestSuite{Adapter: cand, ReferenceAdapter: ref, ReferenceSettle: 2 * time.Second}
	go func() {
		time.Sleep(200 * time.Millisecond)
		cand.mu.Lock()
		cand.docs["log"] = []any{map[string]any{"a": 1.0}}
		cand.mu.Unlock()
	}()
	assert.NoError(t, ts.compareDatabases("log", parity.Rules{}), "a late write is waited for")
	ts.ReferenceSettle = 0
	ref.docs["log"] = append(ref.docs["log"], map[string]any{"b": 2.0})
	assert.Error(t, ts.compareDatabases("log", parity.Rules{}), "without settling, compared once")
}

func TestBothResponsesMatchWithListsInAnyOrder(t *testing.T) {
	ts := &TestSuite{}
	ts.refResp = &parity.Response{Status: 200, Header: http.Header{"Content-Type": {"application/json"}},
		Body: []byte(`{"students":[{"id":"a","n":1},{"id":"b","n":2}],"tags":["x","y"]}`)}
	ts.Resp = &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}}
	ts.RespBody = []byte(`{"students":[{"id":"b","n":2},{"id":"a","n":1}],"tags":["y","x"]}`)
	assert.Error(t, ts.bothResponsesShouldMatch(), "order matters by default")
	assert.NoError(t, ts.bothResponsesShouldMatchUnordered(table([]string{"$.students", "id"}, []string{"$.tags"})))
	assert.Error(t, ts.bothResponsesShouldMatchUnordered(table([]string{"$.tags"})), "only the listed paths")
}

func TestSeedingClearsLeftoversFirst(t *testing.T) {
	cand, ref := &memoryAdapter{docs: map[string][]any{}}, &memoryAdapter{docs: map[string][]any{}}
	require.NoError(t, cand.InsertJSON("notes", []byte(`{"_id":"left-over"}`)))
	require.NoError(t, ref.InsertJSON("notes", []byte(`{"_id":"left-over"}`)))
	ts := &TestSuite{Adapter: cand, ReferenceAdapter: ref}

	require.NoError(t, ts.documentHasTheFollowingJSON("notes", doc(`{"_id":"n1"}`)))
	require.NoError(t, ts.documentHasTheFollowingJSON("notes", doc(`{"_id":"n2"}`)))
	assert.Len(t, cand.docs["notes"], 2, "leftovers cleared once; a second seed adds")
	assert.Len(t, ref.docs["notes"], 2)

	ts.clearSeeded()
	assert.Empty(t, cand.docs["notes"])
	assert.ErrorContains(t, (&TestSuite{Adapter: cand, ReferenceAdapter: failingAdapter{}}).
		documentHasTheFollowingJSON("notes", doc(`{}`)), "reference: clearing notes: clear failed")
}
