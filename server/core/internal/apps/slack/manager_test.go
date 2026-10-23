package slack

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/jarcoal/httpmock"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/internal/notification/interpreter"
	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// _testSigningSecret is a 32-byte AES key for installation and link state
// tests.
var _testSigningSecret = strings.Repeat("k", 32)

// _testRedirectURL is the redirect URL configured on test managers.
const _testRedirectURL = "http://localhost/slack/redirect"

// _testSignatureSecret is the signature secret configured on test managers.
const _testSignatureSecret = "sig"

// recordingReceiver captures the subscribed callback and counts
// subscribe and unsubscribe calls.
type recordingReceiver struct {
	fn           func(context.Context, notification.Notification)
	subscribed   int
	unsubscribed int
}

func (rr *recordingReceiver) OnNotification(fn func(context.Context, notification.Notification)) notification.Unsubscribe {
	rr.fn = fn
	rr.subscribed++

	return func() {
		rr.unsubscribed++
	}
}

// newDisabledManager creates an unconfigured Manager for tests.
func newDisabledManager(t *testing.T) *Manager {
	t.Helper()

	man, err := NewManager(
		slog.New(slog.DiscardHandler),
		nil,
		nil,
		nil,
		nil,
		Options{},
	)
	require.NoError(t, err)

	return man
}

// newTestManager creates a configured Manager with the given collaborators
// and the test signing secrets.
func newTestManager(t *testing.T, db DB, interp Interpreter) *Manager {
	t.Helper()

	man, err := NewManager(
		slog.New(slog.DiscardHandler),
		db,
		interp,
		&recordingReceiver{},
		nil,
		Options{
			ClientID:                  "id",
			ClientSecret:              "secret",
			SignatureSecret:           _testSignatureSecret,
			RedirectURL:               _testRedirectURL,
			InstallationSigningSecret: _testSigningSecret,
		},
	)
	require.NoError(t, err)

	return man
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Options_Validate(t *testing.T) {
	t.Parallel()

	valid := Options{
		ClientID:                  "id",
		ClientSecret:              "secret",
		SignatureSecret:           "sig",
		RedirectURL:               _testRedirectURL,
		InstallationSigningSecret: _testSigningSecret,
	}

	cc := map[string]struct {
		Mutate func(o *Options)
		Err    error
	}{
		"Full options are valid": {
			Mutate: func(*Options) {},
		},
		"Missing client ID fails": {
			Mutate: func(o *Options) {
				o.ClientID = ""
			},
			Err: errors.New("client id is required"),
		},
		"Missing client secret fails": {
			Mutate: func(o *Options) {
				o.ClientSecret = ""
			},
			Err: errors.New("client secret is required"),
		},
		"Missing signature secret fails": {
			Mutate: func(o *Options) {
				o.SignatureSecret = ""
			},
			Err: errors.New("signature secret is required"),
		},
		"Missing redirect URL fails": {
			Mutate: func(o *Options) {
				o.RedirectURL = ""
			},
			Err: errors.New("redirect URL is required"),
		},
		"Missing installation signing secret fails": {
			Mutate: func(o *Options) {
				o.InstallationSigningSecret = ""
			},
			Err: errors.New("installation signing secret is required"),
		},
		// the secret is an AES key: a wrong-length one used to boot fine and
		// then fail every install, link and verify call.
		"Wrong-length installation signing secret fails": {
			Mutate: func(o *Options) {
				o.InstallationSigningSecret = "too-short"
			},
			Err: fmt.Errorf("installation signing secret: %w", cryptoutil.ErrInvalidKeySize),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			opt := valid
			c.Mutate(&opt)

			testutil.AssertEqualError(t, c.Err, opt.Validate())
		})
	}
}

func Test_NewManager(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Opt            Options
		Subscriptions  int
		WantConfigured bool
		Err            error
	}{
		"Empty client ID creates an unconfigured manager": {
			Opt: Options{},
		},
		"Client ID with missing client secret fails": {
			Opt: Options{
				ClientID: "id",
			},
			Err: assert.AnError,
		},
		"Full options create a configured manager": {
			Opt: Options{
				ClientID:                  "id",
				ClientSecret:              "secret",
				SignatureSecret:           "sig",
				RedirectURL:               _testRedirectURL,
				InstallationSigningSecret: _testSigningSecret,
			},
			Subscriptions:  1,
			WantConfigured: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			rr := &recordingReceiver{}

			man, err := NewManager(
				slog.New(slog.DiscardHandler),
				nil,
				nil,
				rr,
				nil,
				c.Opt,
			)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Subscriptions, rr.subscribed)

			if err != nil {
				return
			}

			assert.Equal(t, c.WantConfigured, man.Configured())
		})
	}
}

func Test_Manager_Close(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Opt          Options
		Subscribed   bool
		Unsubscribed int
	}{
		"Unconfigured manager has no subscription to clean up": {
			Opt: Options{},
		},
		"Configured manager subscribes and unsubscribes": {
			Opt: Options{
				ClientID:                  "id",
				ClientSecret:              "secret",
				SignatureSecret:           "sig",
				RedirectURL:               _testRedirectURL,
				InstallationSigningSecret: _testSigningSecret,
			},
			Subscribed:   true,
			Unsubscribed: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			rr := &recordingReceiver{}

			man, err := NewManager(slog.New(slog.DiscardHandler), nil, nil, rr, nil, c.Opt)
			require.NoError(t, err)

			assert.Equal(t, c.Subscribed, rr.fn != nil)
			require.NoError(t, man.Close())
			assert.Equal(t, c.Unsubscribed, rr.unsubscribed)
		})
	}
}

func Test_Manager_ExchangeCode(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Unconfigured  bool
		Resp          httpmock.Responder
		ExchangeCalls int
		Result        *AppAccess
		Err           error
	}{
		// the responder is registered so that a stray exchange call would be
		// counted. Without one the transport does not count the call.
		"Unconfigured manager fails": {
			Unconfigured: true,
			Resp: httpmock.NewStringResponder(
				http.StatusOK,
				`{"ok":true,"access_token":"token","app_id":"A1","team":{"id":"T1"}}`,
			),
			Err: ErrNotConfigured,
		},
		"Error returned by the exchange": {
			Resp:          httpmock.NewStringResponder(http.StatusOK, `{"ok":false,"error":"invalid_code"}`),
			ExchangeCalls: 1,
			Err:           assert.AnError,
		},
		"Successful exchange": {
			Resp: httpmock.NewStringResponder(
				http.StatusOK,
				`{"ok":true,"access_token":"token","app_id":"A1","team":{"id":"T1"}}`,
			),
			ExchangeCalls: 1,
			Result: &AppAccess{
				TeamID:      "T1",
				AccessToken: "token",
				AppID:       "A1",
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			man := newTestManager(t, nil, nil)

			if c.Unconfigured {
				man = newDisabledManager(t)
			}

			client, mt := testutil.MockHTTP()
			mt.RegisterResponder(http.MethodPost, "https://slack.com/api/oauth.v2.access", c.Resp)

			man.client = client

			res, err := man.ExchangeCode(context.Background(), "code")
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.ExchangeCalls, mt.GetTotalCallCount())

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_Manager_GetClient(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Configured bool
		DB         *DBMock
		FetchCalls int
		Err        error
	}{
		"Unconfigured manager fails": {
			DB:  &DBMock{},
			Err: ErrNotConfigured,
		},
		"Missing app fails": {
			Configured: true,
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*App, error) {
					return nil, sql.ErrNoRows
				},
			},
			FetchCalls: 1,
			Err:        ErrAppNotFound,
		},
		"Error returned by DB.FetchSlackAppByTeamID": {
			Configured: true,
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*App, error) {
					return nil, assert.AnError
				},
			},
			FetchCalls: 1,
			Err:        assert.AnError,
		},
		"Existing app returns a client": {
			Configured: true,
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*App, error) {
					return &App{TeamID: "team-1", Token: "token"}, nil
				},
			},
			FetchCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			man := newTestManager(t, c.DB, nil)

			if !c.Configured {
				man = newDisabledManager(t)
				man.db = c.DB
			}

			client, err := man.GetClient(context.Background(), "team-1")
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.DB.FetchSlackAppByTeamIDCalls()
			require.Len(t, ff, c.FetchCalls)

			if err != nil {
				return
			}

			assert.NotNil(t, client)
			assert.Equal(t, "team-1", ff[0].TeamID)
		})
	}
}

func Test_Manager_VerifyMiddleware(t *testing.T) {
	t.Parallel()

	// signRequest builds a request carrying the Slack signature headers
	// for the given body, signed with the given secret.
	signRequest := func(t *testing.T, body, secret string) *http.Request {
		t.Helper()

		r := httptest.NewRequest(http.MethodPost, "/slack/events", bytes.NewBufferString(body))

		ts := strconv.FormatInt(time.Now().Unix(), 10)

		mac := hmac.New(sha256.New, []byte(secret))

		_, err := fmt.Fprintf(mac, "v0:%s:%s", ts, body)
		require.NoError(t, err)

		r.Header.Set("X-Slack-Request-Timestamp", ts)
		r.Header.Set("X-Slack-Signature", "v0="+hex.EncodeToString(mac.Sum(nil)))

		return r
	}

	cc := map[string]struct {
		Request      func(t *testing.T) *http.Request
		Unconfigured bool
		Body         string
		Err          error
	}{
		"Correctly signed request passes and keeps its body readable": {
			Request: func(t *testing.T) *http.Request {
				return signRequest(t, `{"type": "event_callback"}`, _testSignatureSecret)
			},
			Body: `{"type": "event_callback"}`,
		},
		"Request signed with another secret fails": {
			Request: func(t *testing.T) *http.Request {
				return signRequest(t, `{}`, "other-secret")
			},
			Err: assert.AnError,
		},
		"Request without signature headers fails": {
			Request: func(_ *testing.T) *http.Request {
				return httptest.NewRequest(http.MethodPost, "/slack/events", http.NoBody)
			},
			Err: assert.AnError,
		},
		// a request signed with the empty secret is exactly what a forger
		// could produce against an unconfigured deployment.
		"Unconfigured manager refuses verification": {
			Request: func(t *testing.T) *http.Request {
				return signRequest(t, `{}`, "")
			},
			Unconfigured: true,
			Err:          ErrNotConfigured,
		},
		// a transport failure is not malformed JSON, and these payloads are
		// form-encoded in the first place.
		"Body read failure is a form error": {
			Request: func(_ *testing.T) *http.Request {
				r := httptest.NewRequest(http.MethodPost, "/slack/events", iotest.ErrReader(assert.AnError))
				r.Header.Set("X-Slack-Request-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
				r.Header.Set("X-Slack-Signature", "v0=deadbeef")

				return r
			},
			Err: httpserver.ErrInvalidForm,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			man := newTestManager(t, nil, nil)

			if c.Unconfigured {
				man = newDisabledManager(t)
			}

			r := c.Request(t)

			err := man.VerifyMiddleware(r)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			body, rerr := io.ReadAll(r.Body)
			require.NoError(t, rerr)
			assert.JSONEq(t, c.Body, string(body))
		})
	}
}

func Test_Manager_ProcessNotification(t *testing.T) {
	t.Parallel()

	type check func(*testing.T, *DBMock, *InterpreterMock)

	checks := func(cc ...check) []check { return cc }

	wasDBFetchUserLinkCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *InterpreterMock) {
			require.Len(t, db.FetchSlackUserLinkByUserIDCalls(), count)
		}
	}

	wasDBFetchAppCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *InterpreterMock) {
			require.Len(t, db.FetchSlackAppByTeamIDCalls(), count)
		}
	}

	wasInterpretCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, interp *InterpreterMock) {
			require.Len(t, interp.InterpretNotificationCalls(), count)
		}
	}

	cc := map[string]struct {
		LinkErr   error
		Link      *UserLink
		AppErr    error
		InterpErr error
		Checks    []check
	}{
		"Missing user link is ignored": {
			LinkErr: sql.ErrNoRows,
			Checks: checks(
				wasDBFetchUserLinkCalled(1),
				wasDBFetchAppCalled(0),
				wasInterpretCalled(0),
			),
		},
		"Error returned by DB.FetchSlackUserLinkByUserID": {
			LinkErr: assert.AnError,
			Checks: checks(
				wasDBFetchUserLinkCalled(1),
				wasDBFetchAppCalled(0),
				wasInterpretCalled(0),
			),
		},
		"Disabled notifications short-circuit": {
			Link: &UserLink{
				SlackUserID: "slack-user",
				TeamID:      "team-1",
				UserID:      "user-1",
				Settings:    UserLinkSettings{Notifications: false},
			},
			Checks: checks(
				wasDBFetchUserLinkCalled(1),
				wasDBFetchAppCalled(0),
				wasInterpretCalled(0),
			),
		},
		"Missing app is logged and ignored": {
			Link: &UserLink{
				SlackUserID: "slack-user",
				TeamID:      "team-1",
				UserID:      "user-1",
				Settings:    UserLinkSettings{Notifications: true},
			},
			AppErr: sql.ErrNoRows,
			Checks: checks(
				wasDBFetchUserLinkCalled(1),
				wasDBFetchAppCalled(1),
				wasInterpretCalled(0),
			),
		},
		"Error returned by Interpreter.InterpretNotification": {
			Link: &UserLink{
				SlackUserID: "slack-user",
				TeamID:      "team-1",
				UserID:      "user-1",
				Settings:    UserLinkSettings{Notifications: true},
			},
			InterpErr: assert.AnError,
			Checks: checks(
				wasDBFetchUserLinkCalled(1),
				wasDBFetchAppCalled(1),
				wasInterpretCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := &DBMock{
				FetchSlackUserLinkByUserIDFunc: func(context.Context, string, string) (*UserLink, error) {
					return c.Link, c.LinkErr
				},
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*App, error) {
					return &App{TeamID: "team-1", Token: "token"}, c.AppErr
				},
			}

			interp := &InterpreterMock{
				InterpretNotificationFunc: func(context.Context, notification.Notification) (*interpreter.Message, error) {
					return &interpreter.Message{Text: "hi"}, c.InterpErr
				},
			}

			man := newTestManager(t, db, interp)

			man.ProcessNotification(context.Background(), notification.Notification{
				UserID:         "user-1",
				OrganizationID: "org-1",
			})

			for _, ch := range c.Checks {
				ch(t, db, interp)
			}
		})
	}
}
