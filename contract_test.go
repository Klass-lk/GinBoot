package ginboot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- what a shared contract package contains -------------------------------
//
// In real use everything between here and MemberContract lives in its own Go
// module (say github.com/acme/user-service/api), imported by the owning
// service and by every consumer.

type MemberSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type ListMembersRequest struct {
	Role string `form:"role,omitempty" json:"role,omitempty"`
	Page int    `form:"page" json:"page"`
}

type GetMemberRequest struct {
	ID string `uri:"id" json:"id"`
}

type UpdateMemberRoleRequest struct {
	ID   string `uri:"id" json:"-"`
	Role string `json:"role"`
}

// MemberService is the contract's Go face. Consumers depend on this; whether
// the value behind it is a network client or the real implementation is a
// wiring decision, not a code change.
type MemberService interface {
	ListMembers(ctx context.Context, req ListMembersRequest) ([]MemberSummary, error)
	GetMember(ctx context.Context, req GetMemberRequest) (MemberSummary, error)
	UpdateMemberRole(ctx context.Context, req UpdateMemberRoleRequest) (MemberSummary, error)
	WhoAmI(ctx context.Context) (MemberSummary, error)
}

// MemberContract binds the interface to the wire. One table, both sides.
var MemberContract = Contract{
	Service:  "user-service",
	BasePath: "/api/v1/members",
	Routes: map[string]Route{
		"ListMembers":      {Method: http.MethodGet, Path: ""},
		"GetMember":        {Method: http.MethodGet, Path: "/:id"},
		"UpdateMemberRole": {Method: http.MethodPut, Path: "/:id/role"},
		"WhoAmI":           {Method: http.MethodGet, Path: "/me"},
	},
}

// NewMemberClient returns the remote implementation of MemberService. This is
// the part worth generating once there is more than one contract; each method
// is mechanical.
func NewMemberClient(client service.ServiceClient) MemberService {
	return &memberClient{client: client}
}

type memberClient struct{ client service.ServiceClient }

func (c *memberClient) ListMembers(ctx context.Context, req ListMembersRequest) ([]MemberSummary, error) {
	return Invoke[[]MemberSummary](ctx, c.client, MemberContract, "ListMembers", req)
}

func (c *memberClient) GetMember(ctx context.Context, req GetMemberRequest) (MemberSummary, error) {
	return Invoke[MemberSummary](ctx, c.client, MemberContract, "GetMember", req)
}

func (c *memberClient) UpdateMemberRole(ctx context.Context, req UpdateMemberRoleRequest) (MemberSummary, error) {
	return Invoke[MemberSummary](ctx, c.client, MemberContract, "UpdateMemberRole", req)
}

func (c *memberClient) WhoAmI(ctx context.Context) (MemberSummary, error) {
	return Invoke[MemberSummary](ctx, c.client, MemberContract, "WhoAmI", NoRequest{})
}

// --- what the owning service contains --------------------------------------

type memberController struct {
	members  map[string]MemberSummary
	lastPage int
	lastRole string
}

// The compile-time check that replaces "hope the routes still match".
var _ MemberService = (*memberController)(nil)

func (c *memberController) ListMembers(ctx context.Context, req ListMembersRequest) ([]MemberSummary, error) {
	c.lastPage, c.lastRole = req.Page, req.Role
	out := []MemberSummary{}
	for _, m := range c.members {
		if req.Role == "" || m.Role == req.Role {
			out = append(out, m)
		}
	}
	return out, nil
}

func (c *memberController) GetMember(ctx context.Context, req GetMemberRequest) (MemberSummary, error) {
	member, ok := c.members[req.ID]
	if !ok {
		return MemberSummary{}, NewApiError(http.StatusNotFound, "member not found")
	}
	return member, nil
}

func (c *memberController) UpdateMemberRole(ctx context.Context, req UpdateMemberRoleRequest) (MemberSummary, error) {
	member, ok := c.members[req.ID]
	if !ok {
		return MemberSummary{}, NewApiError(http.StatusNotFound, "member not found")
	}
	member.Role = req.Role
	c.members[req.ID] = member
	return member, nil
}

// WhoAmI reads request state that only exists when the call arrived over HTTP.
func (c *memberController) WhoAmI(ctx context.Context) (MemberSummary, error) {
	auth, ok := AuthFrom(ctx)
	if !ok {
		return MemberSummary{}, NewApiError(http.StatusUnauthorized, "not authenticated")
	}
	return c.members[auth.UserID], nil
}

// --- test plumbing ---------------------------------------------------------

type staticResolver struct{ target string }

func (r staticResolver) ResolveEndpoint(string) (service.ServiceEndpoint, error) {
	return service.ServiceEndpoint{Protocol: "http", Target: r.target, Timeout: 5 * time.Second}, nil
}

func newContractFixture(t *testing.T) (*memberController, MemberService, *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	impl := &memberController{members: map[string]MemberSummary{
		"u1": {ID: "u1", Name: "Ada", Role: "admin"},
		"u2": {ID: "u2", Name: "Linus", Role: "developer"},
	}}

	server := &Server{engine: gin.New(), logger: NewSlogLogger(nil)}
	group := server.Group("")
	group.Use(func(c *gin.Context) { c.Set("user_id", "u1"); c.Next() })
	group.RegisterContract(MemberContract, impl)

	httpServer := httptest.NewServer(server.Engine())
	t.Cleanup(httpServer.Close)

	client := NewMemberClient(service.NewServiceClient(staticResolver{target: httpServer.URL}))
	return impl, client, httpServer
}

func TestContractRoundTrip(t *testing.T) {
	impl, client, _ := newContractFixture(t)
	ctx := context.Background()

	t.Run("path parameter", func(t *testing.T) {
		member, err := client.GetMember(ctx, GetMemberRequest{ID: "u2"})
		require.NoError(t, err)
		assert.Equal(t, MemberSummary{ID: "u2", Name: "Linus", Role: "developer"}, member)
	})

	t.Run("query parameters", func(t *testing.T) {
		members, err := client.ListMembers(ctx, ListMembersRequest{Role: "admin", Page: 2})
		require.NoError(t, err)
		require.Len(t, members, 1)
		assert.Equal(t, "Ada", members[0].Name)
		assert.Equal(t, 2, impl.lastPage, "page should survive the query string")
		assert.Equal(t, "admin", impl.lastRole)
	})

	t.Run("path parameter and body together", func(t *testing.T) {
		member, err := client.UpdateMemberRole(ctx, UpdateMemberRoleRequest{ID: "u2", Role: "admin"})
		require.NoError(t, err)
		assert.Equal(t, "admin", member.Role)
		assert.Equal(t, "admin", impl.members["u2"].Role)
	})

	t.Run("method with no request", func(t *testing.T) {
		member, err := client.WhoAmI(ctx)
		require.NoError(t, err)
		assert.Equal(t, "Ada", member.Name)
	})
}

func TestContractRemoteErrorKeepsTheCause(t *testing.T) {
	_, client, _ := newContractFixture(t)

	_, err := client.GetMember(context.Background(), GetMemberRequest{ID: "missing"})
	require.Error(t, err)

	var remote *RemoteError
	require.True(t, errors.As(err, &remote), "should surface as a RemoteError")
	assert.Equal(t, "user-service", remote.Service)
	assert.Equal(t, http.StatusNotFound, remote.StatusCode)

	// The remote ApiError survives the wire, so callers can branch on it.
	var apiErr ApiError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, "404", apiErr.ErrorCode)
	assert.Equal(t, "member not found", apiErr.Message)
}

func TestRemoteErrorIsNotForwardedByDefault(t *testing.T) {
	_, client, _ := newContractFixture(t)
	gin.SetMode(gin.TestMode)

	callerErr := func() error {
		_, err := client.GetMember(context.Background(), GetMemberRequest{ID: "missing"})
		return err
	}()

	t.Run("defaults to 502", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		NewContext(c, nil, nil, nil).SendError(callerErr)

		assert.Equal(t, http.StatusBadGateway, w.Code)
		assert.Contains(t, w.Body.String(), "UPSTREAM_ERROR")
		assert.NotContains(t, w.Body.String(), "member not found", "upstream detail should not leak")
	})

	t.Run("forwards when the caller asks for it", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		NewContext(c, nil, nil, nil).SendError(Propagate(callerErr))

		assert.Equal(t, http.StatusNotFound, w.Code)
		var body ErrorResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "member not found", body.Message)
	})
}

// TestContractInProcess is the payoff: the consumer holds MemberService, so
// co-deploying the two services is an injection change and nothing else. No
// HTTP server, no serialization, no client.
func TestContractInProcess(t *testing.T) {
	impl := &memberController{members: map[string]MemberSummary{
		"u1": {ID: "u1", Name: "Ada", Role: "admin"},
	}}

	var members MemberService = impl // instead of NewMemberClient(...)

	member, err := members.GetMember(context.Background(), GetMemberRequest{ID: "u1"})
	require.NoError(t, err)
	assert.Equal(t, "Ada", member.Name)

	_, err = members.GetMember(context.Background(), GetMemberRequest{ID: "nope"})
	var apiErr ApiError
	require.True(t, errors.As(err, &apiErr), "the same error type surfaces either way")
	assert.Equal(t, "404", apiErr.ErrorCode)
}

func TestRegisterContractFailsFastOnDrift(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("missing method", func(t *testing.T) {
		drifted := Contract{
			Service:  "user-service",
			BasePath: "/api/v1/members",
			Routes:   map[string]Route{"DeleteMember": {Method: http.MethodDelete, Path: "/:id"}},
		}
		server := &Server{engine: gin.New()}
		assert.PanicsWithValue(t,
			"ginboot: contract \"user-service\" requires method DeleteMember on *ginboot.memberController, which does not implement it",
			func() { server.RegisterContract(drifted, &memberController{}) })
	})

	t.Run("middleware for an unknown method", func(t *testing.T) {
		server := &Server{engine: gin.New()}
		assert.Panics(t, func() {
			server.RegisterContract(MemberContract, &memberController{},
				RouteMiddleware{"DeleteMember": {func(c *gin.Context) {}}})
		})
	})
}

func TestRegisterContractAppliesPerMethodMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var guarded []string
	impl := &memberController{members: map[string]MemberSummary{"u1": {ID: "u1", Name: "Ada"}}}
	server := &Server{engine: gin.New(), logger: NewSlogLogger(nil)}
	server.RegisterContract(MemberContract, impl, RouteMiddleware{
		"UpdateMemberRole": {func(c *gin.Context) { guarded = append(guarded, "UpdateMemberRole"); c.Next() }},
	})

	httpServer := httptest.NewServer(server.Engine())
	defer httpServer.Close()
	client := NewMemberClient(service.NewServiceClient(staticResolver{target: httpServer.URL}))

	_, err := client.GetMember(context.Background(), GetMemberRequest{ID: "u1"})
	require.NoError(t, err)
	assert.Empty(t, guarded, "middleware must not leak onto other methods")

	_, err = client.UpdateMemberRole(context.Background(), UpdateMemberRoleRequest{ID: "u1", Role: "admin"})
	require.NoError(t, err)
	assert.Equal(t, []string{"UpdateMemberRole"}, guarded)
}

func TestRegisterContractFeedsOpenAPISpec(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := &Server{engine: gin.New(), basePath: "/api"}
	server.RegisterContract(Contract{
		Service:  "spec-service",
		BasePath: "/v9/members",
		Routes: map[string]Route{
			"GetMember":   {Method: http.MethodGet, Path: "/:id"},
			"ListMembers": {Method: http.MethodGet, Path: ""},
			"WhoAmI":      {Method: http.MethodGet, Path: "/me"},
		},
	}, &memberController{})

	spec := openApiSpec.Paths["/api/v9/members/:id"]
	require.NotNil(t, spec, "contract routes should appear in the exported spec")
	require.Contains(t, spec, "get")
	assert.NotNil(t, spec["get"].Responses["200"].Content, "response schema derived from the method signature")

	list := openApiSpec.Paths["/api/v9/members"]
	require.NotNil(t, list)
	assert.NotEmpty(t, list["get"].Parameters, "form-tagged request fields become query parameters")

	// A method taking only a context must not be mistaken for one taking a request.
	me := openApiSpec.Paths["/api/v9/members/me"]
	require.NotNil(t, me)
	assert.Empty(t, me["get"].Parameters)
	assert.Nil(t, me["get"].RequestBody)
}
