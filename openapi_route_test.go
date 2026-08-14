package ginboot

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serverWithOpenAPI builds a server with the specification endpoint configured,
// without going through Start — which would bind a port.
func serverWithOpenAPI(t *testing.T, cfg config.OpenAPIConfig) *Server {
	t.Helper()
	gin.SetMode(gin.TestMode)

	server := New()
	server.config.Ginboot.OpenAPI = cfg
	server.registerOpenAPIEndpoint()
	return server
}

func get(t *testing.T, server *Server, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, request)
	return recorder
}

func TestSpecificationIsServedPubliclyByDefault(t *testing.T) {
	// Empty configuration is a developer running locally, who has nothing to
	// protect and wants the specification to be there.
	server := serverWithOpenAPI(t, config.OpenAPIConfig{})

	recorder := get(t, server, defaultOpenAPIPath, nil)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"openapi"`)
}

func TestDisabledLeavesNothingToFind(t *testing.T) {
	server := serverWithOpenAPI(t, config.OpenAPIConfig{Access: OpenAPIDisabled})

	recorder := get(t, server, defaultOpenAPIPath, nil)

	// 404 rather than 403: an application that has switched this off should not
	// advertise that there is something here to ask for.
	assert.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestTokenAccessRequiresTheToken(t *testing.T) {
	server := serverWithOpenAPI(t, config.OpenAPIConfig{Access: OpenAPIToken, Token: "s3cret"})

	t.Run("with the token", func(t *testing.T) {
		recorder := get(t, server, defaultOpenAPIPath, map[string]string{OpenAPITokenHeader: "s3cret"})
		require.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), `"openapi"`)
	})

	for _, tt := range []struct {
		name    string
		headers map[string]string
	}{
		{"no token", nil},
		{"wrong token", map[string]string{OpenAPITokenHeader: "guess"}},
		{"empty token", map[string]string{OpenAPITokenHeader: ""}},
		{"token in the wrong header", map[string]string{"Authorization": "Bearer s3cret"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := get(t, server, defaultOpenAPIPath, tt.headers)
			assert.Equal(t, http.StatusUnauthorized, recorder.Code)
			assert.NotContains(t, recorder.Body.String(), `"paths"`,
				"the specification leaked to a caller without the token")
		})
	}
}

// A configuration that was started and not finished must not publish the
// specification. The safe reading of "token, but no token" is that someone meant
// to restrict it.
func TestTokenAccessWithoutATokenServesNothing(t *testing.T) {
	server := serverWithOpenAPI(t, config.OpenAPIConfig{Access: OpenAPIToken})

	assert.Equal(t, http.StatusNotFound, get(t, server, defaultOpenAPIPath, nil).Code)
}

func TestAnUnrecognisedAccessValueServesNothing(t *testing.T) {
	// Typos fail closed, for the same reason.
	for _, access := range []string{"pubic", "authenticated", "yes", "true"} {
		server := serverWithOpenAPI(t, config.OpenAPIConfig{Access: access, Token: "s3cret"})
		assert.Equal(t, http.StatusNotFound, get(t, server, defaultOpenAPIPath, nil).Code,
			"access %q should not serve the specification", access)
	}
}

func TestAccessValueIsReadLeniently(t *testing.T) {
	// It arrives from a console field or an environment variable, so casing and
	// stray whitespace should not change what it means.
	for _, access := range []string{"Disabled", " disabled ", "DISABLED"} {
		server := serverWithOpenAPI(t, config.OpenAPIConfig{Access: access})
		assert.Equal(t, http.StatusNotFound, get(t, server, defaultOpenAPIPath, nil).Code,
			"access %q should mean disabled", access)
	}

	server := serverWithOpenAPI(t, config.OpenAPIConfig{Access: " Public "})
	assert.Equal(t, http.StatusOK, get(t, server, defaultOpenAPIPath, nil).Code)
}

func TestSpecificationPathIsConfigurable(t *testing.T) {
	server := serverWithOpenAPI(t, config.OpenAPIConfig{Path: "internal/spec.json"})

	// A path given without a leading slash still means a path.
	assert.Equal(t, http.StatusOK, get(t, server, "/internal/spec.json", nil).Code)
	assert.Equal(t, http.StatusNotFound, get(t, server, defaultOpenAPIPath, nil).Code)
}

func TestEnvironmentOverridesTheConfiguredAccess(t *testing.T) {
	// The platform hosting an application decides this, not a file committed to
	// its repository.
	t.Setenv("GINBOOT_OPENAPI_ACCESS", OpenAPIDisabled)

	cfg := &config.Config{}
	cfg.Ginboot.OpenAPI = config.OpenAPIConfig{Access: OpenAPIPublic}
	cfg.ApplyEnvironmentOverrides()

	assert.Equal(t, OpenAPIDisabled, cfg.Ginboot.OpenAPI.Access)
}

type openAPIProbeRequest struct {
	Name string `json:"name"`
}
type openAPIProbeResponse struct {
	ID string `json:"id"`
}

type openAPIProbeController struct{}

func (openAPIProbeController) Register(g *ControllerGroup) {
	g.POST("", func(ctx *Context, in openAPIProbeRequest) (openAPIProbeResponse, error) {
		return openAPIProbeResponse{ID: "u1"}, nil
	})
}

// The endpoint has to serve the specification the server actually built, not an
// empty shell — which is the whole point of serving it from the running
// application rather than exporting it from a build.
func TestTheServedSpecificationDescribesTheRegisteredRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := New()
	server.SetBasePath("/api/v1")
	server.RegisterController("openapi-probe-users", openAPIProbeController{})

	server.config.Ginboot.OpenAPI = config.OpenAPIConfig{Access: OpenAPIPublic}
	server.registerOpenAPIEndpoint()

	recorder := get(t, server, defaultOpenAPIPath, nil)
	require.Equal(t, http.StatusOK, recorder.Code)

	body := recorder.Body.String()
	assert.Contains(t, body, "/api/v1/openapi-probe-users",
		"the route registered on this server is missing from the specification it served")
	assert.Contains(t, body, "post")
}
