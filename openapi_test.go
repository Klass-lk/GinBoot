package ginboot

import (
	"os"
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
	tmpFile := "test_swagger.json"
	t.Cleanup(func() {
		os.Remove(tmpFile)
	})

	// First make sure the route is registered
	handler := func(c *Context) (mockUserResponse, error) {
		return mockUserResponse{}, nil
	}
	registerOpenAPIRoute("GET", "/test", reflect.TypeOf(handler))

	err := exportOpenAPISpec(tmpFile)
	assert.NoError(t, err)

	// Test export failure
	errInvalid := exportOpenAPISpec("/invalid_dir_path_xyz/swagger.json")
	assert.Error(t, errInvalid)
}

type RecursiveStruct struct {
	Name string           `json:"name"`
	Self *RecursiveStruct `json:"self"`
}

type DeletedAt struct{}

type SpecialTypesStruct struct {
	Count      int                    `json:"count"`
	Price      float64                `form:"price"`
	Active     bool                   `json:"active"`
	unexported string                 // unexported field (lowercase)
	Ignored    string                 `json:"-"`
	Untagged   string                 // no tag
	Metadata   map[string]interface{} `json:"metadata"`
	AnyData    interface{}            `json:"any_data"`
	DeletedAt  DeletedAt              `json:"deleted_at"`
	Ch         chan int               `json:"ch"`
}

func TestOpenAPI_BuildSchema_Types(t *testing.T) {
	t.Run("nil type", func(t *testing.T) {
		schema := buildSchema(nil)
		assert.Equal(t, "object", schema.Type)
	})

	t.Run("recursive type", func(t *testing.T) {
		schema := buildSchema(reflect.TypeOf(RecursiveStruct{}))
		assert.Equal(t, "object", schema.Type)
		assert.NotNil(t, schema.Properties["self"])
		assert.Equal(t, "object", schema.Properties["self"].Type)
	})

	t.Run("special types and fields", func(t *testing.T) {
		schema := buildSchema(reflect.TypeOf(SpecialTypesStruct{}))
		assert.Equal(t, "object", schema.Type)
		assert.Equal(t, "integer", schema.Properties["count"].Type)
		assert.Equal(t, "number", schema.Properties["price"].Type)
		assert.Equal(t, "boolean", schema.Properties["active"].Type)
		assert.Equal(t, "string", schema.Properties["untagged"].Type)
		assert.Equal(t, "object", schema.Properties["metadata"].Type)
		assert.Equal(t, "object", schema.Properties["any_data"].Type)
		assert.Equal(t, "string", schema.Properties["deleted_at"].Type)
		assert.Equal(t, "date-time", schema.Properties["deleted_at"].Format)
		assert.Equal(t, "string", schema.Properties["ch"].Type)

		// Check that unexported and ignored fields are omitted
		assert.Nil(t, schema.Properties["unexported"])
		assert.Nil(t, schema.Properties["ignored"])
	})

	t.Run("buildParameters for struct with form tag and ignored field", func(t *testing.T) {
		type QueryReq struct {
			Search  string `form:"q"`
			Page    int    `json:"page"`
			Ignored string `form:"-"`
		}
		params := buildParameters(reflect.TypeOf(QueryReq{}))
		assert.Len(t, params, 2)
		assert.Equal(t, "q", params[0].Name)
		assert.Equal(t, "page", params[1].Name)
	})

	t.Run("buildParameters for non-struct", func(t *testing.T) {
		params := buildParameters(reflect.TypeOf("string"))
		assert.Empty(t, params)
	})
}

func TestOpenAPI_RegisterRouteWithExportEnv(t *testing.T) {
	tmpFile := "test_env_swagger.json"
	t.Setenv("GINBOOT_EXPORT_SWAGGER", tmpFile)
	t.Cleanup(func() {
		os.Remove(tmpFile)
	})

	handler := func(c *Context) (string, error) {
		return "ok", nil
	}
	registerOpenAPIRoute("GET", "/env-test", reflect.TypeOf(handler))

	assert.NotNil(t, openApiSpec.Paths["/env-test"])
}

