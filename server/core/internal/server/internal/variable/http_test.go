package variable

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	variableCore "github.com/oxynote/oxynote/server/core/internal/variable"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

const (
	// _unauthorized is the body every handler answers without a session.
	_unauthorized = `{"code":"account.not_authenticated","message":"not authenticated"}`

	// _invalidJSON is the body every decoding handler answers a malformed
	// payload with.
	_invalidJSON = `{"code":"request.invalid_json","message":"invalid JSON body"}`

	// _internal is the body an unknown error is answered with.
	_internal = `{"code":"general","message":"internal server error"}`

	// _notFound is the body a missing or malformed path id is answered
	// with.
	_notFound = `{"code":"general","message":"not found"}`
)

var (
	// _id is the group or variable the path-addressed tests operate on.
	_id = mustParseID("d3h1n2k0d3h1n2k0d3h0")

	// _groupID is the group the bodies name.
	_groupID = mustParseID("c3h1n2k0c3h1n2k0c3h0")

	// _builtinErr is the body a write to a built-in variable is refused
	// with.
	_builtinErr = `{"code":"variable.builtin","message":"only the default value of a built-in variable can change"}`
)

// mustParseID parses a fixed test id or panics.
func mustParseID(s string) xid.ID {
	id, err := xid.FromString(s)
	if err != nil {
		panic("cannot parse test id: " + err.Error())
	}

	return id
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// request describes one call of a handler.
type request struct {
	// NoSession leaves the session off the request.
	NoSession bool

	// Params are the chi path parameters.
	Params map[string]string

	// Payload is the request body.
	Payload string
}

// serve runs the handler over a request built from the description.
func serve(t *testing.T, fn http.HandlerFunc, req request) *httptest.ResponseRecorder {
	t.Helper()

	r := httptest.NewRequest(http.MethodPost, "http://test.com/", strings.NewReader(req.Payload))

	if !req.NoSession {
		r = r.WithContext(auth.AddSessionToContext(r.Context(), auth.Session{
			UserID:               "u1",
			ActiveOrganizationID: "org1",
		}))
	}

	for k, v := range req.Params {
		r = r.WithContext(testutil.AddChiCtx(r.Context(), k, v))
	}

	rec := httptest.NewRecorder()
	fn(rec, r)

	return rec
}

func Test_NewHandler(t *testing.T) {
	t.Parallel()

	man := &ManagerMock{}

	hdl := NewHandler(slog.New(slog.DiscardHandler), man)
	require.NotNil(t, hdl)
	assert.NotNil(t, hdl.log)
	assert.Same(t, man, hdl.man)
}

func Test_Handler_FetchVariableTree(t *testing.T) {
	t.Parallel()

	tree := variableCore.Tree{{
		ID:             _groupID,
		OrganizationID: "org1",
		GroupName:      "Metrics",
		Color:          "blue",
		Builtin:        true,
		Variables: []variableCore.Entry{{
			ID:             _id,
			OrganizationID: "org1",
			GroupID:        _groupID,
			Key:            "metric_range",
			Name:           "Metric time range",
			Type:           variableCore.TypeTimeRange,
			DefaultValue:   null.StringFrom("last_24_hours"),
			Builtin:        true,
			Value:          null.StringFrom("last_7_days"),
			Options:        []variableCore.Option{},
		}},
	}}

	cc := map[string]struct {
		Man   *ManagerMock
		Req   request
		Code  int
		Body  string
		Calls int
	}{
		"No session in context": {
			Man:  &ManagerMock{},
			Req:  request{NoSession: true},
			Code: http.StatusUnauthorized,
			Body: _unauthorized,
		},
		"Error returned by Manager.FetchTree": {
			Man: &ManagerMock{
				FetchTreeFunc: func(context.Context, string, string) (variableCore.Tree, error) {
					return nil, assert.AnError
				},
			},
			Code:  http.StatusInternalServerError,
			Body:  _internal,
			Calls: 1,
		},
		"Successful fetch": {
			Man: &ManagerMock{
				FetchTreeFunc: func(context.Context, string, string) (variableCore.Tree, error) {
					return tree, nil
				},
			},
			Code: http.StatusOK,
			Body: `[{
				"id": "c3h1n2k0c3h1n2k0c3h0",
				"organizationId": "org1",
				"groupName": "Metrics",
				"color": "blue",
				"sortIndex": 0,
				"builtin": true,
				"createdAt": "0001-01-01T00:00:00Z",
				"createdBy": null,
				"variables": [{
					"id": "d3h1n2k0d3h1n2k0d3h0",
					"organizationId": "org1",
					"groupId": "c3h1n2k0c3h1n2k0c3h0",
					"key": "metric_range",
					"name": "Metric time range",
					"description": null,
					"type": "time-range",
					"defaultValue": "last_24_hours",
					"sortIndex": 0,
					"builtin": true,
					"createdAt": "0001-01-01T00:00:00Z",
					"createdBy": null,
					"value": "last_7_days",
					"options": []
				}]
			}]`,
			Calls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{log: slog.New(slog.DiscardHandler), man: c.Man}

			rec := serve(t, hdl.FetchVariableTree, c.Req)
			testutil.AssertResponse(t, rec, c.Code, c.Body)

			calls := c.Man.FetchTreeCalls()
			require.Len(t, calls, c.Calls)

			if c.Calls == 0 {
				return
			}

			assert.Equal(t, "org1", calls[0].OrganizationID)
			assert.Equal(t, "u1", calls[0].UserID)
		})
	}
}

func Test_Handler_CreateVariableGroup(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Man   *ManagerMock
		Req   request
		Code  int
		Body  string
		Calls int
	}{
		"No session in context": {
			Man:  &ManagerMock{},
			Req:  request{NoSession: true},
			Code: http.StatusUnauthorized,
			Body: _unauthorized,
		},
		"Malformed payload": {
			Man:  &ManagerMock{},
			Req:  request{Payload: `{`},
			Code: http.StatusBadRequest,
			Body: _invalidJSON,
		},
		"Error returned by Manager.CreateGroup": {
			Man: &ManagerMock{
				CreateGroupFunc: func(context.Context, variableCore.GroupCreateInput, string, string) (*variableCore.Group, error) {
					return nil, variableCore.ErrDuplicateGroupName
				},
			},
			Req:   request{Payload: `{"groupName":"Deploys","color":"orange"}`},
			Code:  http.StatusConflict,
			Body:  `{"code":"variable.duplicate_group_name","message":"group name is already in use"}`,
			Calls: 1,
		},
		"Successful creation": {
			Man: &ManagerMock{
				CreateGroupFunc: func(context.Context, variableCore.GroupCreateInput, string, string) (*variableCore.Group, error) {
					return &variableCore.Group{ID: _groupID, OrganizationID: "org1", GroupName: "Deploys", Color: "orange", SortIndex: 1}, nil
				},
			},
			Req:  request{Payload: `{"groupName":"Deploys","color":"orange"}`},
			Code: http.StatusCreated,
			Body: `{
				"id": "c3h1n2k0c3h1n2k0c3h0",
				"organizationId": "org1",
				"groupName": "Deploys",
				"color": "orange",
				"sortIndex": 1,
				"builtin": false,
				"createdAt": "0001-01-01T00:00:00Z",
				"createdBy": null
			}`,
			Calls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{log: slog.New(slog.DiscardHandler), man: c.Man}

			rec := serve(t, hdl.CreateVariableGroup, c.Req)
			testutil.AssertResponse(t, rec, c.Code, c.Body)

			calls := c.Man.CreateGroupCalls()
			require.Len(t, calls, c.Calls)

			if c.Calls == 0 {
				return
			}

			assert.Equal(t, variableCore.GroupCreateInput{GroupName: "Deploys", Color: "orange"}, calls[0].Inp)
			assert.Equal(t, "org1", calls[0].OrganizationID)
			assert.Equal(t, "u1", calls[0].UserID)
		})
	}
}

func Test_Handler_UpdateVariableGroup(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Man   *ManagerMock
		Req   request
		Code  int
		Body  string
		Calls int
	}{
		"No session in context": {
			Man:  &ManagerMock{},
			Req:  request{NoSession: true},
			Code: http.StatusUnauthorized,
			Body: _unauthorized,
		},
		"Missing group id": {
			Man:  &ManagerMock{},
			Code: http.StatusNotFound,
			Body: _notFound,
		},
		"Malformed payload": {
			Man:  &ManagerMock{},
			Req:  request{Params: map[string]string{"groupId": _id.String()}, Payload: `{`},
			Code: http.StatusBadRequest,
			Body: _invalidJSON,
		},
		"Error returned by Manager.UpdateGroup": {
			Man: &ManagerMock{
				UpdateGroupFunc: func(context.Context, xid.ID, variableCore.GroupUpdateInput, string) error {
					return variableCore.ErrBuiltinGroup
				},
			},
			Req:   request{Params: map[string]string{"groupId": _id.String()}, Payload: `{"groupName":"Env"}`},
			Code:  http.StatusForbidden,
			Body:  `{"code":"variable.builtin_group","message":"built-in group cannot be changed"}`,
			Calls: 1,
		},
		"Successful update": {
			Man:   &ManagerMock{},
			Req:   request{Params: map[string]string{"groupId": _id.String()}, Payload: `{"groupName":"Env"}`},
			Code:  http.StatusNoContent,
			Calls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{log: slog.New(slog.DiscardHandler), man: c.Man}

			rec := serve(t, hdl.UpdateVariableGroup, c.Req)
			testutil.AssertResponse(t, rec, c.Code, c.Body)

			calls := c.Man.UpdateGroupCalls()
			require.Len(t, calls, c.Calls)

			if c.Calls == 0 {
				return
			}

			assert.Equal(t, _id, calls[0].ID)
			assert.Equal(t, variableCore.GroupUpdateInput{GroupName: null.StringFrom("Env")}, calls[0].Inp)
			assert.Equal(t, "org1", calls[0].OrganizationID)
		})
	}
}

func Test_Handler_UpdateVariableGroupTree(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Man   *ManagerMock
		Req   request
		Code  int
		Body  string
		Calls int
	}{
		"No session in context": {
			Man:  &ManagerMock{},
			Req:  request{NoSession: true},
			Code: http.StatusUnauthorized,
			Body: _unauthorized,
		},
		"Malformed payload": {
			Man:  &ManagerMock{},
			Req:  request{Payload: `{`},
			Code: http.StatusBadRequest,
			Body: _invalidJSON,
		},
		"Error returned by Manager.MoveGroup": {
			Man: &ManagerMock{
				MoveGroupFunc: func(context.Context, variableCore.GroupMoveInput, string) error {
					return variableCore.ErrInvalidSortIndex
				},
			},
			Req:   request{Payload: `{"id":"d3h1n2k0d3h1n2k0d3h0","sortIndex":2}`},
			Code:  http.StatusBadRequest,
			Body:  `{"code":"variable.invalid_sort_index","message":"sort index is out of range"}`,
			Calls: 1,
		},
		"Successful move": {
			Man:   &ManagerMock{},
			Req:   request{Payload: `{"id":"d3h1n2k0d3h1n2k0d3h0","sortIndex":2}`},
			Code:  http.StatusNoContent,
			Calls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{log: slog.New(slog.DiscardHandler), man: c.Man}

			rec := serve(t, hdl.UpdateVariableGroupTree, c.Req)
			testutil.AssertResponse(t, rec, c.Code, c.Body)

			calls := c.Man.MoveGroupCalls()
			require.Len(t, calls, c.Calls)

			if c.Calls == 0 {
				return
			}

			assert.Equal(t, variableCore.GroupMoveInput{ID: _id, SortIndex: 2}, calls[0].Inp)
			assert.Equal(t, "org1", calls[0].OrganizationID)
		})
	}
}

func Test_Handler_DeleteVariableGroup(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Man   *ManagerMock
		Req   request
		Code  int
		Body  string
		Calls int
	}{
		"No session in context": {
			Man:  &ManagerMock{},
			Req:  request{NoSession: true},
			Code: http.StatusUnauthorized,
			Body: _unauthorized,
		},
		"Missing group id": {
			Man:  &ManagerMock{},
			Code: http.StatusNotFound,
			Body: _notFound,
		},
		"Error returned by Manager.DeleteGroup": {
			Man: &ManagerMock{
				DeleteGroupFunc: func(context.Context, xid.ID, string) error {
					return variableCore.ErrGroupNotEmpty
				},
			},
			Req:   request{Params: map[string]string{"groupId": _id.String()}},
			Code:  http.StatusConflict,
			Body:  `{"code":"variable.group_not_empty","message":"group still has variables"}`,
			Calls: 1,
		},
		"Successful deletion": {
			Man:   &ManagerMock{},
			Req:   request{Params: map[string]string{"groupId": _id.String()}},
			Code:  http.StatusNoContent,
			Calls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{log: slog.New(slog.DiscardHandler), man: c.Man}

			rec := serve(t, hdl.DeleteVariableGroup, c.Req)
			testutil.AssertResponse(t, rec, c.Code, c.Body)

			calls := c.Man.DeleteGroupCalls()
			require.Len(t, calls, c.Calls)

			if c.Calls == 0 {
				return
			}

			assert.Equal(t, _id, calls[0].ID)
			assert.Equal(t, "org1", calls[0].OrganizationID)
		})
	}
}

func Test_Handler_CreateVariable(t *testing.T) {
	t.Parallel()

	payload := `{"groupId":"c3h1n2k0c3h1n2k0c3h0","key":"hotfix_max","name":"Hotfix threshold","type":"number","defaultValue":"2"}`

	cc := map[string]struct {
		Man   *ManagerMock
		Req   request
		Code  int
		Body  string
		Calls int
	}{
		"No session in context": {
			Man:  &ManagerMock{},
			Req:  request{NoSession: true},
			Code: http.StatusUnauthorized,
			Body: _unauthorized,
		},
		"Malformed payload": {
			Man:  &ManagerMock{},
			Req:  request{Payload: `{`},
			Code: http.StatusBadRequest,
			Body: _invalidJSON,
		},
		"Unknown type": {
			Man:  &ManagerMock{},
			Req:  request{Payload: `{"type":"date"}`},
			Code: http.StatusBadRequest,
			Body: _invalidJSON,
		},
		"Error returned by Manager.CreateVariable": {
			Man: &ManagerMock{
				CreateVariableFunc: func(context.Context, variableCore.Input, string, string) (*variableCore.Variable, error) {
					return nil, variableCore.ErrDuplicateKey
				},
			},
			Req:   request{Payload: payload},
			Code:  http.StatusConflict,
			Body:  `{"code":"variable.duplicate_key","message":"key is already in use"}`,
			Calls: 1,
		},
		"Successful creation": {
			Man: &ManagerMock{
				CreateVariableFunc: func(context.Context, variableCore.Input, string, string) (*variableCore.Variable, error) {
					return &variableCore.Variable{
						ID:             _id,
						OrganizationID: "org1",
						GroupID:        _groupID,
						Key:            "hotfix_max",
						Name:           "Hotfix threshold",
						Type:           variableCore.TypeNumber,
						DefaultValue:   null.StringFrom("2"),
						Options:        []variableCore.Option{},
					}, nil
				},
			},
			Req:  request{Payload: payload},
			Code: http.StatusCreated,
			Body: `{
				"id": "d3h1n2k0d3h1n2k0d3h0",
				"organizationId": "org1",
				"groupId": "c3h1n2k0c3h1n2k0c3h0",
				"key": "hotfix_max",
				"name": "Hotfix threshold",
				"description": null,
				"type": "number",
				"defaultValue": "2",
				"sortIndex": 0,
				"builtin": false,
				"createdAt": "0001-01-01T00:00:00Z",
				"createdBy": null,
				"options": []
			}`,
			Calls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{log: slog.New(slog.DiscardHandler), man: c.Man}

			rec := serve(t, hdl.CreateVariable, c.Req)
			testutil.AssertResponse(t, rec, c.Code, c.Body)

			calls := c.Man.CreateVariableCalls()
			require.Len(t, calls, c.Calls)

			if c.Calls == 0 {
				return
			}

			assert.Equal(t, variableCore.Input{
				GroupID:      _groupID,
				Key:          "hotfix_max",
				Name:         "Hotfix threshold",
				Type:         variableCore.TypeNumber,
				DefaultValue: null.StringFrom("2"),
			}, calls[0].Inp)
			assert.Equal(t, "org1", calls[0].OrganizationID)
			assert.Equal(t, "u1", calls[0].UserID)
		})
	}
}

func Test_Handler_UpdateVariable(t *testing.T) {
	t.Parallel()

	payload := `{"groupId":"c3h1n2k0c3h1n2k0c3h0","key":"region","name":"Region","type":"list","options":[{"id":"d3h1n2k0d3h1n2k0d3h0","value":"eu-west-1"},{"value":"us-east-1"}]}`

	cc := map[string]struct {
		Man   *ManagerMock
		Req   request
		Code  int
		Body  string
		Calls int
	}{
		"No session in context": {
			Man:  &ManagerMock{},
			Req:  request{NoSession: true},
			Code: http.StatusUnauthorized,
			Body: _unauthorized,
		},
		"Missing variable id": {
			Man:  &ManagerMock{},
			Code: http.StatusNotFound,
			Body: _notFound,
		},
		"Malformed payload": {
			Man:  &ManagerMock{},
			Req:  request{Params: map[string]string{"variableId": _id.String()}, Payload: `{`},
			Code: http.StatusBadRequest,
			Body: _invalidJSON,
		},
		"Error returned by Manager.UpdateVariable": {
			Man: &ManagerMock{
				UpdateVariableFunc: func(context.Context, xid.ID, variableCore.Input, string) error {
					return variableCore.ErrBuiltinVariable
				},
			},
			Req:   request{Params: map[string]string{"variableId": _id.String()}, Payload: payload},
			Code:  http.StatusForbidden,
			Body:  _builtinErr,
			Calls: 1,
		},
		"Successful update": {
			Man:   &ManagerMock{},
			Req:   request{Params: map[string]string{"variableId": _id.String()}, Payload: payload},
			Code:  http.StatusNoContent,
			Calls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{log: slog.New(slog.DiscardHandler), man: c.Man}

			rec := serve(t, hdl.UpdateVariable, c.Req)
			testutil.AssertResponse(t, rec, c.Code, c.Body)

			calls := c.Man.UpdateVariableCalls()
			require.Len(t, calls, c.Calls)

			if c.Calls == 0 {
				return
			}

			assert.Equal(t, _id, calls[0].ID)
			assert.Equal(t, variableCore.Input{
				GroupID: _groupID,
				Key:     "region",
				Name:    "Region",
				Type:    variableCore.TypeList,
				Options: []variableCore.OptionInput{
					{ID: null.ValueFrom(_id), Value: "eu-west-1"},
					{Value: "us-east-1"},
				},
			}, calls[0].Inp)
			assert.Equal(t, "org1", calls[0].OrganizationID)
		})
	}
}

func Test_Handler_UpdateVariableTree(t *testing.T) {
	t.Parallel()

	payload := `{"id":"d3h1n2k0d3h1n2k0d3h0","groupId":"c3h1n2k0c3h1n2k0c3h0","sortIndex":1}`

	cc := map[string]struct {
		Man   *ManagerMock
		Req   request
		Code  int
		Body  string
		Calls int
	}{
		"No session in context": {
			Man:  &ManagerMock{},
			Req:  request{NoSession: true},
			Code: http.StatusUnauthorized,
			Body: _unauthorized,
		},
		"Malformed payload": {
			Man:  &ManagerMock{},
			Req:  request{Payload: `{`},
			Code: http.StatusBadRequest,
			Body: _invalidJSON,
		},
		"Error returned by Manager.MoveVariable": {
			Man: &ManagerMock{
				MoveVariableFunc: func(context.Context, variableCore.MoveInput, string) error {
					return variableCore.ErrBuiltinGroup
				},
			},
			Req:   request{Payload: payload},
			Code:  http.StatusForbidden,
			Body:  `{"code":"variable.builtin_group","message":"built-in group cannot be changed"}`,
			Calls: 1,
		},
		"Successful move": {
			Man:   &ManagerMock{},
			Req:   request{Payload: payload},
			Code:  http.StatusNoContent,
			Calls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{log: slog.New(slog.DiscardHandler), man: c.Man}

			rec := serve(t, hdl.UpdateVariableTree, c.Req)
			testutil.AssertResponse(t, rec, c.Code, c.Body)

			calls := c.Man.MoveVariableCalls()
			require.Len(t, calls, c.Calls)

			if c.Calls == 0 {
				return
			}

			assert.Equal(t, variableCore.MoveInput{ID: _id, GroupID: _groupID, SortIndex: 1}, calls[0].Inp)
			assert.Equal(t, "org1", calls[0].OrganizationID)
		})
	}
}

func Test_Handler_DeleteVariable(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Man   *ManagerMock
		Req   request
		Code  int
		Body  string
		Calls int
	}{
		"No session in context": {
			Man:  &ManagerMock{},
			Req:  request{NoSession: true},
			Code: http.StatusUnauthorized,
			Body: _unauthorized,
		},
		"Missing variable id": {
			Man:  &ManagerMock{},
			Code: http.StatusNotFound,
			Body: _notFound,
		},
		"Error returned by Manager.DeleteVariable": {
			Man: &ManagerMock{
				DeleteVariableFunc: func(context.Context, xid.ID, string) error {
					return variableCore.ErrBuiltinVariable
				},
			},
			Req:   request{Params: map[string]string{"variableId": _id.String()}},
			Code:  http.StatusForbidden,
			Body:  _builtinErr,
			Calls: 1,
		},
		"Successful deletion": {
			Man:   &ManagerMock{},
			Req:   request{Params: map[string]string{"variableId": _id.String()}},
			Code:  http.StatusNoContent,
			Calls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{log: slog.New(slog.DiscardHandler), man: c.Man}

			rec := serve(t, hdl.DeleteVariable, c.Req)
			testutil.AssertResponse(t, rec, c.Code, c.Body)

			calls := c.Man.DeleteVariableCalls()
			require.Len(t, calls, c.Calls)

			if c.Calls == 0 {
				return
			}

			assert.Equal(t, _id, calls[0].ID)
			assert.Equal(t, "org1", calls[0].OrganizationID)
		})
	}
}

func Test_Handler_SetVariableDefault(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Man   *ManagerMock
		Req   request
		Code  int
		Body  string
		Calls int
	}{
		"No session in context": {
			Man:  &ManagerMock{},
			Req:  request{NoSession: true},
			Code: http.StatusUnauthorized,
			Body: _unauthorized,
		},
		"Missing variable id": {
			Man:  &ManagerMock{},
			Code: http.StatusNotFound,
			Body: _notFound,
		},
		"Malformed payload": {
			Man:  &ManagerMock{},
			Req:  request{Params: map[string]string{"variableId": _id.String()}, Payload: `{`},
			Code: http.StatusBadRequest,
			Body: _invalidJSON,
		},
		"Error returned by Manager.SetDefaultValue": {
			Man: &ManagerMock{
				SetDefaultValueFunc: func(context.Context, xid.ID, string, string) error {
					return variableCore.ErrInvalidTimeRange
				},
			},
			Req:   request{Params: map[string]string{"variableId": _id.String()}, Payload: `{"value":"last_7_days"}`},
			Code:  http.StatusBadRequest,
			Body:  `{"code":"variable.invalid_time_range","message":"value must be a time range preset"}`,
			Calls: 1,
		},
		"Successful set": {
			Man:   &ManagerMock{},
			Req:   request{Params: map[string]string{"variableId": _id.String()}, Payload: `{"value":"last_7_days"}`},
			Code:  http.StatusNoContent,
			Calls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{log: slog.New(slog.DiscardHandler), man: c.Man}

			rec := serve(t, hdl.SetVariableDefault, c.Req)
			testutil.AssertResponse(t, rec, c.Code, c.Body)

			calls := c.Man.SetDefaultValueCalls()
			require.Len(t, calls, c.Calls)

			if c.Calls == 0 {
				return
			}

			assert.Equal(t, _id, calls[0].ID)
			assert.Equal(t, "last_7_days", calls[0].Value)
			assert.Equal(t, "org1", calls[0].OrganizationID)
		})
	}
}

func Test_Handler_SetVariableValue(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Man   *ManagerMock
		Req   request
		Code  int
		Body  string
		Calls int
	}{
		"No session in context": {
			Man:  &ManagerMock{},
			Req:  request{NoSession: true},
			Code: http.StatusUnauthorized,
			Body: _unauthorized,
		},
		"Missing variable id": {
			Man:  &ManagerMock{},
			Code: http.StatusNotFound,
			Body: _notFound,
		},
		"Malformed payload": {
			Man:  &ManagerMock{},
			Req:  request{Params: map[string]string{"variableId": _id.String()}, Payload: `{`},
			Code: http.StatusBadRequest,
			Body: _invalidJSON,
		},
		"Error returned by Manager.SetValue": {
			Man: &ManagerMock{
				SetValueFunc: func(context.Context, xid.ID, string, string, string) error {
					return variableCore.ErrInvalidNumber
				},
			},
			Req:   request{Params: map[string]string{"variableId": _id.String()}, Payload: `{"value":"3"}`},
			Code:  http.StatusBadRequest,
			Body:  `{"code":"variable.invalid_number","message":"value must be a decimal number"}`,
			Calls: 1,
		},
		"Successful set": {
			Man:   &ManagerMock{},
			Req:   request{Params: map[string]string{"variableId": _id.String()}, Payload: `{"value":"3"}`},
			Code:  http.StatusNoContent,
			Calls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{log: slog.New(slog.DiscardHandler), man: c.Man}

			rec := serve(t, hdl.SetVariableValue, c.Req)
			testutil.AssertResponse(t, rec, c.Code, c.Body)

			calls := c.Man.SetValueCalls()
			require.Len(t, calls, c.Calls)

			if c.Calls == 0 {
				return
			}

			assert.Equal(t, _id, calls[0].ID)
			assert.Equal(t, "3", calls[0].Value)
			assert.Equal(t, "org1", calls[0].OrganizationID)
			assert.Equal(t, "u1", calls[0].UserID)
		})
	}
}

func Test_Handler_ResetVariableValue(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Man   *ManagerMock
		Req   request
		Code  int
		Body  string
		Calls int
	}{
		"No session in context": {
			Man:  &ManagerMock{},
			Req:  request{NoSession: true},
			Code: http.StatusUnauthorized,
			Body: _unauthorized,
		},
		"Missing variable id": {
			Man:  &ManagerMock{},
			Code: http.StatusNotFound,
			Body: _notFound,
		},
		"Error returned by Manager.ResetValue": {
			Man: &ManagerMock{
				ResetValueFunc: func(context.Context, xid.ID, string, string) error {
					return assert.AnError
				},
			},
			Req:   request{Params: map[string]string{"variableId": _id.String()}},
			Code:  http.StatusInternalServerError,
			Body:  _internal,
			Calls: 1,
		},
		"Successful reset": {
			Man:   &ManagerMock{},
			Req:   request{Params: map[string]string{"variableId": _id.String()}},
			Code:  http.StatusNoContent,
			Calls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{log: slog.New(slog.DiscardHandler), man: c.Man}

			rec := serve(t, hdl.ResetVariableValue, c.Req)
			testutil.AssertResponse(t, rec, c.Code, c.Body)

			calls := c.Man.ResetValueCalls()
			require.Len(t, calls, c.Calls)

			if c.Calls == 0 {
				return
			}

			assert.Equal(t, _id, calls[0].ID)
			assert.Equal(t, "org1", calls[0].OrganizationID)
			assert.Equal(t, "u1", calls[0].UserID)
		})
	}
}
