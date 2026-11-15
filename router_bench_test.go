package ginboot

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type benchItem struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Tags  []string `json:"tags"`
	Price float64  `json:"price"`
}

type benchController struct{}

func (benchController) Register(group *ControllerGroup) {
	items := make([]benchItem, 50)
	for i := range items {
		items[i] = benchItem{ID: "id", Name: "a <name> & more", Tags: []string{"x", "y"}, Price: 12.5}
	}
	group.GET("/items", func() (any, error) { return items, nil })
}

// BenchmarkHandlerJSONResponse measures a handler answering a 50-item list
// through the wrapper (marshal, then write).
func BenchmarkHandlerJSONResponse(b *testing.B) {
	gin.SetMode(gin.TestMode)
	server := New()
	server.SetBasePath("/api")
	server.RegisterController("b", benchController{})
	req := httptest.NewRequest(http.MethodGet, "/api/b/items", nil)
	b.ReportAllocs()
	for b.Loop() {
		server.Engine().ServeHTTP(httptest.NewRecorder(), req)
	}
}
