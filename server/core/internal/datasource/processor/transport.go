package processor

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	// _tlsHandshakeTimeout bounds the TLS handshake of a data source's own
	// transport, as the shared transports do.
	_tlsHandshakeTimeout = 10 * time.Second

	// _idleConnTimeout is how long a data source's own transport keeps an
	// idle connection open.
	_idleConnTimeout = 90 * time.Second
)

// ownTransport keeps the HTTP transport of a data source whose credentials
// carry a CA certificate. That transport trusts the CA alone, so it cannot
// be the one every other client shares.
type ownTransport struct {
	mu        sync.Mutex
	transport *http.Transport
}

// roundTripper returns the transport requests to rawURL go through: the
// shared one, or the data source's own when the credentials carry a CA
// certificate, built on first use.
func (ot *ownTransport) roundTripper(
	shared http.RoundTripper,
	rawURL string,
	creds BasicCredentials,
) (http.RoundTripper, error) {
	if creds.CACertificate == "" {
		return shared, nil
	}

	ot.mu.Lock()
	defer ot.mu.Unlock()

	if ot.transport != nil {
		return ot.transport, nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("error parsing url: %w", err)
	}

	// a CA certificate verifies a TLS server. Over plain HTTP it would
	// verify nothing while the data source looked protected.
	if u.Scheme != "https" {
		return nil, _errInvalidCACertificate
	}

	pool, err := creds.rootCAs()
	if err != nil {
		return nil, err
	}

	ot.transport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			RootCAs:    pool,
			MinVersion: tls.VersionTLS12,
		},
		TLSHandshakeTimeout: _tlsHandshakeTimeout,
		IdleConnTimeout:     _idleConnTimeout,
		ForceAttemptHTTP2:   true,
	}

	return ot.transport, nil
}

// close closes the idle connections of the data source's own transport.
func (ot *ownTransport) close() {
	ot.mu.Lock()
	defer ot.mu.Unlock()

	if ot.transport != nil {
		ot.transport.CloseIdleConnections()
	}
}
