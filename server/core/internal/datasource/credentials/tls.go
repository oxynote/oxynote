package credentials

import "crypto/x509"

// TLS holds what credentials of every kind share: how the data source's
// server is verified.
type TLS struct {
	// CACertificate is the PEM certificate the server's TLS certificate
	// must chain to. When set, it is the only one trusted, and the data
	// source always connects over TLS.
	CACertificate string `json:"caCertificate,omitempty"`
}

// HasCACertificate reports whether the server is verified with a CA
// certificate of its own.
func (t TLS) HasCACertificate() bool {
	return t.CACertificate != ""
}

// RootCAs returns the pool holding the CA certificate.
func (t TLS) RootCAs() (*x509.CertPool, error) {
	pool := x509.NewCertPool()

	if !pool.AppendCertsFromPEM([]byte(t.CACertificate)) {
		return nil, ErrInvalidCACertificate
	}

	return pool, nil
}

// configured reports whether the CA certificate is set.
func (t TLS) configured() map[string]bool {
	configured := make(map[string]bool)

	if t.HasCACertificate() {
		configured["caCertificate"] = true
	}

	return configured
}
