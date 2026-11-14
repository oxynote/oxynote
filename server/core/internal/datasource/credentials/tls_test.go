package credentials

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _testCA is a self-signed CA certificate, PEM-encoded.
var _testCA = func() string {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic("cannot generate test ca key: " + err.Error())
	}

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "oxynote test ca"},
		NotBefore:             timeutil.Now().Add(-time.Hour),
		NotAfter:              timeutil.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		panic("cannot create test ca certificate: " + err.Error())
	}

	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}()

func Test_TLS_HasCACertificate(t *testing.T) {
	cc := map[string]struct {
		TLS    TLS
		Result bool
	}{
		"No CA certificate": {},
		"CA certificate set": {
			TLS:    TLS{CACertificate: "pem"},
			Result: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.TLS.HasCACertificate())
		})
	}
}

func Test_TLS_RootCAs(t *testing.T) {
	cc := map[string]struct {
		CACertificate string
		Err           error
	}{
		"Certificate that is not PEM": {
			CACertificate: "not a certificate",
			Err:           ErrInvalidCACertificate,
		},
		"Successful pool": {
			CACertificate: _testCA,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			pool, err := TLS{CACertificate: c.CACertificate}.RootCAs()
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			// the pool trusts the CA, so the CA verifies against it.
			block, _ := pem.Decode([]byte(c.CACertificate))
			require.NotNil(t, block)

			cert, err := x509.ParseCertificate(block.Bytes)
			require.NoError(t, err)

			_, err = cert.Verify(x509.VerifyOptions{Roots: pool})
			assert.NoError(t, err)
		})
	}
}

func Test_TLS_configured(t *testing.T) {
	cc := map[string]struct {
		TLS    TLS
		Result map[string]bool
	}{
		"No CA certificate": {
			Result: map[string]bool{},
		},
		"CA certificate set": {
			TLS:    TLS{CACertificate: "pem"},
			Result: map[string]bool{"caCertificate": true},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.TLS.configured())
		})
	}
}
