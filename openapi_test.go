package ginboot

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type mockUserResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"createdAt"`
	Tags      []string  `json:"tags"`
}

func TestOpenAPI_Init(t *testing.T) {
	assert.NotNil(t, openApiSpec)
	assert.Equal(t, "3.0.0", openApiSpec.OpenAPI)
	assert.Equal(t, "Ginboot Application API", openApiSpec.Info.Title)
	assert.NotNil(t, openApiSpec.Paths)
}

func TestOpenAPI_SchemaForType(t *testing.T) {
	schema := buildSchema(reflect.TypeOf(mockUserRequest{}))
	assert.NotNil(t, schema)
	assert.Equal(t, "object", schema.Type)
	assert.NotNil(t, schema.Properties["name"])
	assert.Equal(t, "string", schema.Properties["name"].Type)

	resSchema := buildSchema(reflect.TypeOf(mockUserResponse{}))
	assert.Equal(t, "object", resSchema.Type)
	assert.NotNil(t, resSchema.Properties["tags"])
	assert.Equal(t, "array", resSchema.Properties["tags"].Type)
	assert.Equal(t, "string", resSchema.Properties["tags"].Items.Type)

	assert.NotNil(t, resSchema.Properties["createdAt"])
	assert.Equal(t, "string", resSchema.Properties["createdAt"].Type)
	assert.Equal(t, "date-time", resSchema.Properties["createdAt"].Format)
}

func TestOpenAPI_AddRoute(t *testing.T) {
	// Our handler signature needs to match the types we are testing
	handler := func(c *Context, req mockUserRequest) (mockUserResponse, error) {
		return mockUserResponse{}, nil
	}

	registerOpenAPIRoute("POST", "/api/users", reflect.TypeOf(handler))

	assert.NotNil(t, openApiSpec.Paths["/api/users"])
	assert.NotNil(t, openApiSpec.Paths["/api/users"]["post"])

	postOp := openApiSpec.Paths["/api/users"]["post"]
	assert.NotNil(t, postOp.RequestBody)
	assert.NotNil(t, postOp.Responses["200"])

	// Test GET method which builds parameters
	registerOpenAPIRoute("GET", "/api/users", reflect.TypeOf(handler))
	getOp := openApiSpec.Paths["/api/users"]["get"]
	assert.NotNil(t, getOp.Parameters)
	assert.Len(t, getOp.Parameters, 2)
	assert.Equal(t, "name", getOp.Parameters[0].Name)
}

func TestOpenAPI_Export(t *testing.T) {
	// First make sure the route is registered
	handler := func(c *Context) (mockUserResponse, error) {
		return mockUserResponse{}, nil
	}
	registerOpenAPIRoute("GET", "/test", reflect.TypeOf(handler))

	err := exportOpenAPISpec("test_swagger.json")
	assert.NoError(t, err)
}
