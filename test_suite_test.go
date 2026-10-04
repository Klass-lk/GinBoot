package ginboot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

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
		SnapshotDir:  snapshots,
		Paths:        []string{"testdata/suite/features"},
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
