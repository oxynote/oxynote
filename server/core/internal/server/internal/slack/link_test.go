package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/slack"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	goslack "github.com/slack-go/slack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Handler_HandleSlashCommand(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB          *DBMock
		Command     string
		RespCode    int
		Hits        int
		AppFetched  int
		LinkFetched int
		Deleted     int
	}{
		"Unknown command sends ephemeral response": {
			DB:       &DBMock{},
			Command:  "/bogus",
			RespCode: http.StatusNoContent,
			Hits:     1,
		},
		"Link with error returned by DB.FetchSlackAppByTeamID": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return nil, assert.AnError
				},
			},
			Command:    _linkCommand,
			RespCode:   http.StatusInternalServerError,
			AppFetched: 1,
		},
		"Link on unconnected workspace": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return &slack.App{TeamID: "team1", Token: "tkn"}, nil
				},
			},
			Command:    _linkCommand,
			RespCode:   http.StatusNoContent,
			Hits:       1,
			AppFetched: 1,
		},
		"Link with error returned by DB.FetchSlackUserLink": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return &slack.App{TeamID: "team1", OrganizationID: null.StringFrom("org1"), Token: "tkn"}, nil
				},
				FetchSlackUserLinkFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return nil, assert.AnError
				},
			},
			Command:     _linkCommand,
			RespCode:    http.StatusInternalServerError,
			AppFetched:  1,
			LinkFetched: 1,
		},
		"Link when already linked": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return &slack.App{TeamID: "team1", OrganizationID: null.StringFrom("org1"), Token: "tkn"}, nil
				},
				FetchSlackUserLinkFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return &slack.UserLink{
						SlackUserID: "slack-u1",
						TeamID:      "team1",
						UserID:      "u1",
						Settings:    slack.UserLinkSettings{Notifications: true},
					}, nil
				},
			},
			Command:     _linkCommand,
			RespCode:    http.StatusNoContent,
			Hits:        1,
			AppFetched:  1,
			LinkFetched: 1,
		},
		"Successful link invitation": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return &slack.App{TeamID: "team1", OrganizationID: null.StringFrom("org1"), Token: "tkn"}, nil
				},
				FetchSlackUserLinkFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return nil, errutil.ErrNotFound
				},
			},
			Command:     _linkCommand,
			RespCode:    http.StatusNoContent,
			Hits:        1,
			AppFetched:  1,
			LinkFetched: 1,
		},
		"Unlink when not linked": {
			DB: &DBMock{
				FetchSlackUserLinkFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return nil, errutil.ErrNotFound
				},
			},
			Command:     _unlinkCommand,
			RespCode:    http.StatusNoContent,
			Hits:        1,
			LinkFetched: 1,
		},
		"Unlink with error returned by DB.FetchSlackUserLink": {
			DB: &DBMock{
				FetchSlackUserLinkFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return nil, assert.AnError
				},
			},
			Command:     _unlinkCommand,
			RespCode:    http.StatusInternalServerError,
			LinkFetched: 1,
		},
		"Unlink with error returned by DB.DeleteSlackUserLink": {
			DB: &DBMock{
				FetchSlackUserLinkFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return &slack.UserLink{
						SlackUserID: "slack-u1",
						TeamID:      "team1",
						UserID:      "u1",
						Settings:    slack.UserLinkSettings{Notifications: true},
					}, nil
				},
				DeleteSlackUserLinkFunc: func(context.Context, string, string) error {
					return assert.AnError
				},
			},
			Command:     _unlinkCommand,
			RespCode:    http.StatusInternalServerError,
			LinkFetched: 1,
			Deleted:     1,
		},
		"Successful unlink": {
			DB: &DBMock{
				FetchSlackUserLinkFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return &slack.UserLink{
						SlackUserID: "slack-u1",
						TeamID:      "team1",
						UserID:      "u1",
						Settings:    slack.UserLinkSettings{Notifications: true},
					}, nil
				},
			},
			Command:     _unlinkCommand,
			RespCode:    http.StatusNoContent,
			Hits:        1,
			LinkFetched: 1,
			Deleted:     1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var hits int

			// a real server rather than a mock transport, because the link
			// message is posted through http.DefaultClient.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Content-Type") != "application/json" {
					w.WriteHeader(http.StatusBadRequest)

					return
				}

				var body struct {
					ResponseType string `json:"response_type"` //nolint:tagliatelle // slack wire format
				}

				if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
					body.ResponseType != goslack.ResponseTypeEphemeral {
					w.WriteHeader(http.StatusBadRequest)

					return
				}

				hits++

				w.WriteHeader(http.StatusOK)
			}))

			t.Cleanup(func() {
				srv.Close()
				http.DefaultClient.CloseIdleConnections()
			})

			hdl := newMockedHandler(t, true, c.DB, srv.Client())

			form := url.Values{
				"command":      {c.Command},
				"user_id":      {"slack-u1"},
				"team_id":      {"team1"},
				"response_url": {srv.URL},
			}

			req := httptest.NewRequest(http.MethodPost, "http://test.com/", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			rec := httptest.NewRecorder()

			hdl.HandleSlashCommand(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Equal(t, c.Hits, hits)
			assert.Len(t, c.DB.FetchSlackAppByTeamIDCalls(), c.AppFetched)
			assert.Len(t, c.DB.FetchSlackUserLinkCalls(), c.LinkFetched)
			assert.Len(t, c.DB.DeleteSlackUserLinkCalls(), c.Deleted)
		})
	}
}

func Test_Handler_FetchUserLink(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		RespCode  int
		RespJSON  string
		Fetched   int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Error returned by DB.FetchSlackUserLinkByUserID": {
			DB: &DBMock{
				FetchSlackUserLinkByUserIDFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return nil, assert.AnError
				},
			},
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchSlackUserLinkByUserIDFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return &slack.UserLink{
						SlackUserID: "slack-u1",
						TeamID:      "team1",
						UserID:      "u1",
						Settings:    slack.UserLinkSettings{Notifications: true},
					}, nil
				},
			},
			RespCode: http.StatusOK,
			RespJSON: `{"notifications":true}`,
			Fetched:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, true, c.DB, nil)

			req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)

			if !c.NoSession {
				req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				}))
			}

			rec := httptest.NewRecorder()

			hdl.FetchUserLink(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespJSON != "" {
				assert.JSONEq(t, c.RespJSON, rec.Body.String())
			}

			assert.Len(t, c.DB.FetchSlackUserLinkByUserIDCalls(), c.Fetched)
		})
	}
}

func Test_Handler_UpdateUserLink(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		Body      string
		RespCode  int
		Fetched   int
		Updated   int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Body:      `{"notifications":false}`,
			RespCode:  http.StatusUnauthorized,
		},
		"Invalid JSON body": {
			DB:       &DBMock{},
			Body:     "{",
			RespCode: http.StatusBadRequest,
		},
		"Error returned by DB.FetchSlackUserLinkByUserID": {
			DB: &DBMock{
				FetchSlackUserLinkByUserIDFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return nil, assert.AnError
				},
			},
			Body:     `{"notifications":false}`,
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
		},
		"Error returned by DB.UpdateSlackUserLink": {
			DB: &DBMock{
				FetchSlackUserLinkByUserIDFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return &slack.UserLink{
						SlackUserID: "slack-u1",
						TeamID:      "team1",
						UserID:      "u1",
						Settings:    slack.UserLinkSettings{Notifications: true},
					}, nil
				},
				UpdateSlackUserLinkFunc: func(context.Context, slack.UserLink) error {
					return assert.AnError
				},
			},
			Body:     `{"notifications":false}`,
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
			Updated:  1,
		},
		"Successful update": {
			DB: &DBMock{
				FetchSlackUserLinkByUserIDFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return &slack.UserLink{
						SlackUserID: "slack-u1",
						TeamID:      "team1",
						UserID:      "u1",
						Settings:    slack.UserLinkSettings{Notifications: true},
					}, nil
				},
			},
			Body:     `{"notifications":false}`,
			RespCode: http.StatusOK,
			Fetched:  1,
			Updated:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, true, c.DB, nil)

			req := httptest.NewRequest(http.MethodPut, "http://test.com/", strings.NewReader(c.Body))

			if !c.NoSession {
				req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				}))
			}

			rec := httptest.NewRecorder()

			hdl.UpdateUserLink(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchSlackUserLinkByUserIDCalls(), c.Fetched)
			assert.Len(t, c.DB.UpdateSlackUserLinkCalls(), c.Updated)

			if c.Updated > 0 {
				assert.False(t, c.DB.UpdateSlackUserLinkCalls()[0].Link.Settings.Notifications)
			}
		})
	}
}

func Test_Handler_DeleteUserLink(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		RespCode  int
		Fetched   int
		Deleted   int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Link not found": {
			DB: &DBMock{
				FetchSlackUserLinkByUserIDFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return nil, errutil.ErrNotFound
				},
			},
			RespCode: http.StatusBadRequest,
			Fetched:  1,
		},
		"Error returned by DB.FetchSlackUserLinkByUserID": {
			DB: &DBMock{
				FetchSlackUserLinkByUserIDFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return nil, assert.AnError
				},
			},
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
		},
		"Error returned by DB.DeleteSlackUserLink": {
			DB: &DBMock{
				FetchSlackUserLinkByUserIDFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return &slack.UserLink{
						SlackUserID: "slack-u1",
						TeamID:      "team1",
						UserID:      "u1",
						Settings:    slack.UserLinkSettings{Notifications: true},
					}, nil
				},
				DeleteSlackUserLinkFunc: func(context.Context, string, string) error {
					return assert.AnError
				},
			},
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
			Deleted:  1,
		},
		"Successful deletion": {
			DB: &DBMock{
				FetchSlackUserLinkByUserIDFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return &slack.UserLink{
						SlackUserID: "slack-u1",
						TeamID:      "team1",
						UserID:      "u1",
						Settings:    slack.UserLinkSettings{Notifications: true},
					}, nil
				},
			},
			RespCode: http.StatusNoContent,
			Fetched:  1,
			Deleted:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, true, c.DB, nil)

			req := httptest.NewRequest(http.MethodDelete, "http://test.com/", http.NoBody)

			if !c.NoSession {
				req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				}))
			}

			rec := httptest.NewRecorder()

			hdl.DeleteUserLink(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchSlackUserLinkByUserIDCalls(), c.Fetched)
			assert.Len(t, c.DB.DeleteSlackUserLinkCalls(), c.Deleted)
		})
	}
}

func Test_Handler_LinkUser(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB             *DBMock
		NoSession      bool
		RawState       string
		StateOrg       string
		RespCode       int
		AppFetched     int
		MembersFetched int
		Inserted       int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			StateOrg:  "org1",
			RespCode:  http.StatusUnauthorized,
		},
		"Missing link state": {
			DB:       &DBMock{},
			RawState: "-",
			RespCode: http.StatusBadRequest,
		},
		"Invalid link state": {
			DB:       &DBMock{},
			RawState: "garbage",
			RespCode: http.StatusBadRequest,
		},
		"Error returned by DB.FetchSlackAppByTeamID": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return nil, assert.AnError
				},
			},
			StateOrg:   "org1",
			RespCode:   http.StatusInternalServerError,
			AppFetched: 1,
		},
		"Workspace connected to another organization": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return &slack.App{TeamID: "team1", OrganizationID: null.StringFrom("org9"), Token: "tkn"}, nil
				},
			},
			StateOrg:   "org1",
			RespCode:   http.StatusBadRequest,
			AppFetched: 1,
		},
		"Error returned by DB.FetchOrganizationMembers": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return &slack.App{TeamID: "team1", OrganizationID: null.StringFrom("org1"), Token: "tkn"}, nil
				},
				FetchOrganizationMembersFunc: func(context.Context, string) ([]string, error) {
					return nil, assert.AnError
				},
			},
			StateOrg:       "org1",
			RespCode:       http.StatusInternalServerError,
			AppFetched:     1,
			MembersFetched: 1,
		},
		"User not an organization member": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return &slack.App{TeamID: "team1", OrganizationID: null.StringFrom("org1"), Token: "tkn"}, nil
				},
				FetchOrganizationMembersFunc: func(context.Context, string) ([]string, error) {
					return []string{"u2"}, nil
				},
			},
			StateOrg:       "org1",
			RespCode:       http.StatusForbidden,
			AppFetched:     1,
			MembersFetched: 1,
		},
		"Error returned by DB.InsertSlackUserLink": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return &slack.App{TeamID: "team1", OrganizationID: null.StringFrom("org1"), Token: "tkn"}, nil
				},
				FetchOrganizationMembersFunc: func(context.Context, string) ([]string, error) {
					return []string{"u1"}, nil
				},
				InsertSlackUserLinkFunc: func(context.Context, slack.UserLink) error {
					return assert.AnError
				},
			},
			StateOrg:       "org1",
			RespCode:       http.StatusInternalServerError,
			AppFetched:     1,
			MembersFetched: 1,
			Inserted:       1,
		},
		"Successful link with unavailable confirmation client": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return &slack.App{TeamID: "team1", OrganizationID: null.StringFrom("org1"), Token: "tkn"}, nil
				},
				FetchOrganizationMembersFunc: func(context.Context, string) ([]string, error) {
					return []string{"u1"}, nil
				},
			},
			StateOrg:       "org1",
			RespCode:       http.StatusOK,
			AppFetched:     1,
			MembersFetched: 1,
			Inserted:       1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, true, c.DB, nil)

			state := c.RawState

			switch state {
			case "":
				linkURL, err := hdl.man.CreateLinkURL("slack-u1", "team1", c.StateOrg)
				require.NoError(t, err)

				u, err := url.Parse(linkURL)
				require.NoError(t, err)

				state = u.Query().Get("state")
			case "-":
				state = ""
			}

			q := url.Values{}

			if state != "" {
				q.Set("state", state)
			}

			req := httptest.NewRequest(http.MethodGet, "http://test.com/?"+q.Encode(), http.NoBody)

			if !c.NoSession {
				req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				}))
			}

			rec := httptest.NewRecorder()

			hdl.LinkUser(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchSlackAppByTeamIDCalls(), c.AppFetched)
			assert.Len(t, c.DB.FetchOrganizationMembersCalls(), c.MembersFetched)
			assert.Len(t, c.DB.InsertSlackUserLinkCalls(), c.Inserted)

			if c.RespCode == http.StatusOK {
				assert.JSONEq(t, `{"linked":true}`, rec.Body.String())
			}
		})
	}
}
