package processor

import (
	"cmp"
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ownTransport_roundTripper(t *testing.T) {
	srv, srvCA := prepTLSServer(t, nil, nil)

	cc := map[string]struct {
		URL           string
		CACertificate string
		Shared        bool
		Err           error

		// RequestErr indicates whether a request through the transport
		// fails.
		RequestErr bool
	}{
		"Credentials without a CA certificate": {
			URL:    srv.URL,
			Shared: true,
		},
		"Error returned by url.Parse": {
			URL:           "://",
			CACertificate: srvCA,
			Err:           assert.AnError,
		},
		"URL without TLS": {
			URL:           "http://127.0.0.1",
			CACertificate: srvCA,
			Err:           _errInvalidCACertificate,
		},
		"Error returned by BasicCredentials.rootCAs": {
			URL:           srv.URL,
			CACertificate: "not a certificate",
			Err:           _errInvalidCACertificate,
		},
		"Server the CA did not sign": {
			URL:           srv.URL,
			CACertificate: _testCA,
			RequestErr:    true,
		},
		"Server the CA signed": {
			URL:           srv.URL,
			CACertificate: srvCA,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var ot ownTransport

			t.Cleanup(ot.close)

			shared := &http.Transport{}
			creds := BasicCredentials{CACertificate: c.CACertificate}

			rt, err := ot.roundTripper(shared, c.URL, creds)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, ot.transport)
				return
			}

			if c.Shared {
				assert.Same(t, shared, rt)
				assert.Nil(t, ot.transport)

				return
			}

			// the transport is built once and handed to every later call.
			again, err := ot.roundTripper(shared, c.URL, creds)
			require.NoError(t, err)
			assert.Same(t, rt, again)

			err = roundTrip(rt, srv.URL)
			if c.RequestErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
		})
	}
}

func Test_ownTransport_close(t *testing.T) {
	cc := map[string]struct {
		// Used indicates whether a request built the transport before the
		// close.
		Used bool
	}{
		"Transport never built": {},
		"Transport in use":      {Used: true},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var conns atomic.Int32

			srv, srvCA := prepTLSServer(t, nil, func(_ net.Conn, state http.ConnState) {
				if state == http.StateNew {
					conns.Add(1)
				}
			})

			var ot ownTransport

			t.Cleanup(ot.close)

			creds := BasicCredentials{CACertificate: srvCA}

			if !c.Used {
				ot.close()
				assert.Nil(t, ot.transport)

				return
			}

			rt, err := ot.roundTripper(nil, srv.URL, creds)
			require.NoError(t, err)
			require.NoError(t, roundTrip(rt, srv.URL))

			ot.close()

			// the idle connection is gone, so the next request opens another.
			require.NoError(t, roundTrip(rt, srv.URL))
			assert.Equal(t, int32(2), conns.Load())
		})
	}
}

// prepTLSServer starts a TLS server for h, or one that answers every
// request with 204 when h is nil, and returns it with its certificate,
// PEM-encoded, to trust as a CA. A handshake the client refuses is not
// logged.
func prepTLSServer(t *testing.T, h http.Handler, connState func(net.Conn, http.ConnState)) (*httptest.Server, string) {
	t.Helper()

	if h == nil {
		h = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
	}

	srv := httptest.NewUnstartedServer(h)
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.Config.ConnState = connState
	srv.StartTLS()

	t.Cleanup(srv.Close)

	return srv, pemCertificate(srv.Certificate().Raw)
}

// roundTrip sends a GET to the URL through rt.
func roundTrip(rt http.RoundTripper, url string) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}

	resp, err := rt.RoundTrip(req)
	if err != nil {
		return err
	}

	return resp.Body.Close()
}

// prepTLSInput starts a TLS server for h and returns the input of a data
// source at it, whose credentials carry ca as their CA certificate, or the
// server's own certificate when ca is empty.
func prepTLSInput(t *testing.T, h http.Handler, ca string) *InputMock {
	t.Helper()

	srv, srvCA := prepTLSServer(t, h, nil)
	creds := caCredentials(cmp.Or(ca, srvCA))

	return &InputMock{
		URLFunc:         func() string { return srv.URL },
		CredentialsFunc: func() Credentials { return creds },
	}
}
