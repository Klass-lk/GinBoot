package ginboot

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
)

type OpenAPI struct {
	OpenAPI string                 `json:"openapi"`
	Info    OpenAPIInfo            `json:"info"`
	Paths   map[string]OpenAPIPath `json:"paths"`
}

type OpenAPIInfo struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

type OpenAPIPath map[string]OpenAPIOperation

type OpenAPIOperation struct {
	Summary     string                     `json:"summary,omitempty"`
	Parameters  []OpenAPIParameter         `json:"parameters,omitempty"`
	RequestBody *OpenAPIRequestBody        `json:"requestBody,omitempty"`
	Responses   map[string]OpenAPIResponse `json:"responses"`
}

type OpenAPIParameter struct {
	Name     string         `json:"name"`
	In       string         `json:"in"`
	Required bool           `json:"required"`
	Schema   *OpenAPISchema `json:"schema"`
}

type OpenAPIRequestBody struct {
	Content map[string]OpenAPIMediaType `json:"content"`
}

type OpenAPIResponse struct {
	Description string                      `json:"description"`
	Content     map[string]OpenAPIMediaType `json:"content,omitempty"`
}

type OpenAPIMediaType struct {
	Schema *OpenAPISchema `json:"schema"`
}

type OpenAPISchema struct {
	Type       string                    `json:"type,omitempty"`
	Properties map[string]*OpenAPISchema `json:"properties,omitempty"`
	Items      *OpenAPISchema            `json:"items,omitempty"`
	Format     string                    `json:"format,omitempty"`
}

var openApiSpec = OpenAPI{
	OpenAPI: "3.0.0",
	Info: OpenAPIInfo{
		Title:   "Ginboot Application API",
		Version: "1.0.0",
	},
	Paths: make(map[string]OpenAPIPath),
}

func registerOpenAPIRoute(method, path string, handlerType reflect.Type) {
	if openApiSpec.Paths[path] == nil {
		openApiSpec.Paths[path] = make(OpenAPIPath)
	}

	operation := OpenAPIOperation{
		Responses: map[string]OpenAPIResponse{
			"200": {
				Description: "Successful operation",
			},
		},
	}

	// Parse Request (Argument 0 or 1 depending on context)
	numIn := handlerType.NumIn()
	var reqType reflect.Type
	if numIn == 1 && handlerType.In(0) != reflect.TypeOf(&Context{}) && handlerType.In(0) != reflect.TypeOf(&gin.Context{}) {
		reqType = handlerType.In(0)
	} else if numIn == 2 {
		reqType = handlerType.In(1)
	}

	if reqType != nil {
		if reqType.Kind() == reflect.Ptr {
			reqType = reqType.Elem()
		}

		if method == "GET" || method == "DELETE" {
			operation.Parameters = buildParameters(reqType)
		} else {
			operation.RequestBody = &OpenAPIRequestBody{
				Content: map[string]OpenAPIMediaType{
					"application/json": {
						Schema: buildSchema(reqType),
					},
				},
			}
		}
	}

	// Parse Response
	numOut := handlerType.NumOut()
	if numOut >= 1 {
		resType := handlerType.Out(0)
		if resType.Kind() == reflect.Ptr {
			resType = resType.Elem()
		}
		if resType.Kind() != reflect.Invalid {
			operation.Responses["200"] = OpenAPIResponse{
				Description: "Successful operation",
				Content: map[string]OpenAPIMediaType{
					"application/json": {
						Schema: buildSchema(resType),
					},
				},
			}
		}
	}

	openApiSpec.Paths[path][strings.ToLower(method)] = operation

	if exportPath := os.Getenv("GINBOOT_EXPORT_SWAGGER"); exportPath != "" {
		_ = exportOpenAPISpec(exportPath)
	}
}

func buildParameters(t reflect.Type) []OpenAPIParameter {
	var params []OpenAPIParameter
	if t.Kind() != reflect.Struct {
		return params
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name := field.Tag.Get("form")
		if name == "" {
			name = field.Tag.Get("json")
		}
		if name == "" {
			name = strings.ToLower(field.Name)
		}
		name = strings.Split(name, ",")[0]
		if name == "-" {
			continue
		}

		params = append(params, OpenAPIParameter{
			Name:     name,
			In:       "query",
			Required: false,
			Schema:   buildSchema(field.Type),
		})
	}
	return params
}

func buildSchema(t reflect.Type) *OpenAPISchema {
	if t == nil {
		return nil
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	schema := &OpenAPISchema{}

	switch t.Kind() {
	case reflect.String:
		schema.Type = "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		schema.Type = "integer"
	case reflect.Float32, reflect.Float64:
		schema.Type = "number"
	case reflect.Bool:
		schema.Type = "boolean"
	case reflect.Slice, reflect.Array:
		schema.Type = "array"
		schema.Items = buildSchema(t.Elem())
	case reflect.Struct:
		// Convert time.Time to string format
		if t.String() == "time.Time" {
			schema.Type = "string"
			schema.Format = "date-time"
			return schema
		}
		schema.Type = "object"
		schema.Properties = make(map[string]*OpenAPISchema)
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			name := field.Tag.Get("json")
			if name == "" {
				name = strings.ToLower(field.Name)
			}
			name = strings.Split(name, ",")[0]
			if name == "-" {
				continue
			}
			schema.Properties[name] = buildSchema(field.Type)
		}
	case reflect.Map:
		schema.Type = "object"
	default:
		schema.Type = "string"
	}

	return schema
}

func exportOpenAPISpec(filepath string) error {
	data, err := json.MarshalIndent(openApiSpec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath, data, 0644)
}
