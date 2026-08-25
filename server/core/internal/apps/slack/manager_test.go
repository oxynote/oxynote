package slack

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/apps/state"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/internal/notification/interpreter"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _testSigningSecret is a 32-byte AES key for installation and link state
// tests.
var _testSigningSecret = strings.Repeat("k", 32)

// _testRedirectURL is the redirect URL configured on test managers.
const _testRedirectURL = "http://localhost/slack/redirect"

// _testSignatureSecret is the signature secret configured on test managers.
const _testSignatureSecret = "sig"

// fakeReceiver is a no-op notification receiver for tests.
type fakeReceiver struct{}

func (fakeReceiver) OnNotification(_ func(context.Context, notification.Notification)) notification.Unsubscribe {
	return func() {}
}

// recordingReceiver captures the subscribed callback and counts
// unsubscribe calls.
type recordingReceiver struct {
	fn           func(context.Context, notification.Notification)
	unsubscribed int
}

func (rr *recordingReceiver) OnNotification(fn func(context.Context, notification.Notification)) notification.Unsubscribe {
	rr.fn = fn

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
		fakeReceiver{},
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

// encryptState encrypts the given payload as a state string minted for the
// given purpose using the test signing secret.
func encryptState[T state.Stamped](t *testing.T, purpose string, payload T) string {
	t.Helper()

	token, err := state.Encode(payload, purpose, _testSigningSecret)
	require.NoError(t, err)

	return token
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

	tests := map[string]struct {
		Mutate      func(o *Options)
		ExpectedErr string
	}{
		"Full options are valid": {
			Mutate: func(*Options) {},
		},
		"Missing client ID fails": {
			Mutate: func(o *Options) {
				o.ClientID = ""
			},
			ExpectedErr: "client id is required",
		},
		"Missing client secret fails": {
			Mutate: func(o *Options) {
				o.ClientSecret = ""
			},
			ExpectedErr: "client secret is required",
		},
		"Missing signature secret fails": {
			Mutate: func(o *Options) {
				o.SignatureSecret = ""
			},
			ExpectedErr: "signature secret is required",
		},
		"Missing redirect URL fails": {
			Mutate: func(o *Options) {
				o.RedirectURL = ""
			},
			ExpectedErr: "redirect URL is required",
		},
		"Missing installation signing secret fails": {
			Mutate: func(o *Options) {
				o.InstallationSigningSecret = ""
			},
			ExpectedErr: "installation signing secret is required",
		},
		// the secret is an AES key: a wrong-length one used to boot fine and
		// then fail every install, link and verify call.
		"Wrong-length installation signing secret fails": {
			Mutate: func(o *Options) {
				o.InstallationSigningSecret = "too-short"
			},
			ExpectedErr: "installation signing secret: key must be 32 bytes long",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			opt := valid
			tc.Mutate(&opt)

			err := opt.Validate()

			if tc.ExpectedErr != "" {
				assert.EqualError(t, err, tc.ExpectedErr)

				return
			}

			assert.NoError(t, err)
		})
	}
}

func Test_NewManager(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Opt            Options
		ExpectErr      bool
		WantConfigured bool
	}{
		"Empty client ID creates an unconfigured manager": {
			Opt: Options{},
		},
		"Client ID with missing client secret fails": {
			Opt: Options{
				ClientID: "id",
			},
			ExpectErr: true,
		},
		"Full options create a configured manager": {
			Opt: Options{
				ClientID:                  "id",
				ClientSecret:              "secret",
				SignatureSecret:           "sig",
				RedirectURL:               _testRedirectURL,
				InstallationSigningSecret: _testSigningSecret,
			},
			WantConfigured: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			man, err := NewManager(
				slog.New(slog.DiscardHandler),
				nil,
				nil,
				fakeReceiver{},
				tc.Opt,
			)

			if tc.ExpectErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.WantConfigured, man.Configured())
		})
	}
}

func Test_Manager_Close(t *testing.T) {
	t.Parallel()

	t.Run("Unconfigured manager has no subscription to clean up", func(t *testing.T) {
		t.Parallel()

		rr := &recordingReceiver{}

		man, err := NewManager(slog.New(slog.DiscardHandler), nil, nil, rr, Options{})
		require.NoError(t, err)

		assert.Nil(t, rr.fn)
		require.NoError(t, man.Close())
		assert.Zero(t, rr.unsubscribed)
	})

	t.Run("Configured manager subscribes and unsubscribes", func(t *testing.T) {
		t.Parallel()

		rr := &recordingReceiver{}

		man, err := NewManager(
			slog.New(slog.DiscardHandler),
			nil,
			nil,
			rr,
			Options{
				ClientID:                  "id",
				ClientSecret:              "secret",
				SignatureSecret:           "sig",
				RedirectURL:               _testRedirectURL,
				InstallationSigningSecret: _testSigningSecret,
			},
		)
		require.NoError(t, err)

		assert.NotNil(t, rr.fn)
		require.NoError(t, man.Close())
		assert.Equal(t, 1, rr.unsubscribed)
	})
}

func Test_Manager_ExchangeCode(t *testing.T) {
	t.Parallel()

	_, err := newDisabledManager(t).ExchangeCode(context.Background(), "code")
	assert.ErrorIs(t, err, ErrNotConfigured)
}

func Test_Manager_GetClient(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Configured  bool
		DB          *DBMock
		ExpectedErr error
	}{
		"Unconfigured manager fails": {
			ExpectedErr: ErrNotConfigured,
		},
		"Missing app fails": {
			Configured: true,
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*App, error) {
					return nil, sql.ErrNoRows
				},
			},
			ExpectedErr: ErrAppNotFound,
		},
		"Database error is propagated": {
			Configured: true,
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*App, error) {
					return nil, assert.AnError
				},
			},
			ExpectedErr: assert.AnError,
		},
		"Existing app returns a client": {
			Configured: true,
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*App, error) {
					return &App{TeamID: "team-1", Token: "token"}, nil
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			man := newTestManager(t, tc.DB, nil)

			if !tc.Configured {
				man = newDisabledManager(t)
			}

			client, err := man.GetClient(context.Background(), "team-1")

			if tc.ExpectedErr != nil {
				assert.Equal(t, tc.ExpectedErr, err)

				return
			}

			require.NoError(t, err)
			assert.NotNil(t, client)

			ff := tc.DB.FetchSlackAppByTeamIDCalls()
			require.Len(t, ff, 1)
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

	man := newTestManager(t, nil, nil)

	t.Run("Correctly signed request passes and keeps its body readable", func(t *testing.T) {
		t.Parallel()

		r := signRequest(t, `{"type": "event_callback"}`, _testSignatureSecret)

		require.NoError(t, man.VerifyMiddleware(r))

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"type": "event_callback"}`, string(body))
	})

	t.Run("Request signed with another secret fails", func(t *testing.T) {
		t.Parallel()

		r := signRequest(t, `{}`, "other-secret")

		assert.Error(t, man.VerifyMiddleware(r))
	})

	t.Run("Request without signature headers fails", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodPost, "/slack/events", http.NoBody)

		assert.Error(t, man.VerifyMiddleware(r))
	})

	t.Run("Body read failure is a form error", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodPost, "/slack/events", &failingReader{})
		r.Header.Set("X-Slack-Request-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
		r.Header.Set("X-Slack-Signature", "v0=deadbeef")

		// a transport failure is not malformed JSON, and these payloads are
		// form-encoded in the first place.
		assert.Equal(t, httpserver.ErrInvalidForm, man.VerifyMiddleware(r))
	})
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

	// userLink builds a user link with notifications toggled.
	userLink := func(notifications bool) *UserLink {
		return &UserLink{
			SlackUserID: "slack-user",
			TeamID:      "team-1",
			UserID:      "user-1",
			Settings: UserLinkSettings{
				Notifications: notifications,
			},
		}
	}

	tests := map[string]struct {
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
		"User link fetch failure is logged and ignored": {
			LinkErr: assert.AnError,
			Checks: checks(
				wasDBFetchUserLinkCalled(1),
				wasDBFetchAppCalled(0),
				wasInterpretCalled(0),
			),
		},
		"Disabled notifications short-circuit": {
			Link: userLink(false),
			Checks: checks(
				wasDBFetchUserLinkCalled(1),
				wasDBFetchAppCalled(0),
				wasInterpretCalled(0),
			),
		},
		"Missing app is logged and ignored": {
			Link:   userLink(true),
			AppErr: sql.ErrNoRows,
			Checks: checks(
				wasDBFetchUserLinkCalled(1),
				wasDBFetchAppCalled(1),
				wasInterpretCalled(0),
			),
		},
		"Interpreter failure is logged and ignored": {
			Link:      userLink(true),
			InterpErr: assert.AnError,
			Checks: checks(
				wasDBFetchUserLinkCalled(1),
				wasDBFetchAppCalled(1),
				wasInterpretCalled(1),
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			db := &DBMock{
				FetchSlackUserLinkByUserIDFunc: func(context.Context, string, string) (*UserLink, error) {
					return tc.Link, tc.LinkErr
				},
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*App, error) {
					return &App{TeamID: "team-1", Token: "token"}, tc.AppErr
				},
			}

			interp := &InterpreterMock{
				InterpretNotificationFunc: func(context.Context, notification.Notification) (*interpreter.Message, error) {
					return &interpreter.Message{Text: "hi"}, tc.InterpErr
				},
			}

			man := newTestManager(t, db, interp)

			man.ProcessNotification(context.Background(), notification.Notification{
				UserID:         "user-1",
				OrganizationID: "org-1",
			})

			for _, ch := range tc.Checks {
				ch(t, db, interp)
			}
		})
	}
}

// failingReader always fails, standing in for a request body that dies
// mid-transport.
type failingReader struct{}

// Read always returns an error.
func (failingReader) Read([]byte) (int, error) {
	return 0, assert.AnError
}
