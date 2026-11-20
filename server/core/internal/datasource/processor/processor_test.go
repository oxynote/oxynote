package processor

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"strconv"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/orlangure/gnomock"
	pgDocker "github.com/orlangure/gnomock/preset/postgres"
	"github.com/oxynote/oxynote/server/core/internal/datasource/credentials"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

// _testToken is the token a data source behind tokenHandler accepts.
const _testToken = "good"

// _testCA is a self-signed CA certificate, PEM-encoded, that signed no
// server the tests start.
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

	return pemCertificate(der)
}()

const (
	// _testDB is the database name used in the throwaway containers.
	_testDB = "testdb"

	// _mysqlRootUser is the privileged MariaDB container user.
	_mysqlRootUser = "root"

	// _mysqlRootPass is the privileged MariaDB container password.
	_mysqlRootPass = "rootpass"

	// _pgUser is the privileged PostgreSQL container user.
	_pgUser = "pgtest"

	// _pgPass is the privileged PostgreSQL container password.
	_pgPass = "pgpass"

	// _readerUser is the read-only user created in both containers.
	_readerUser = "reader"

	// _readerPass is the read-only user's password.
	_readerPass = "readerpass"
)

var (
	// _mysqlAddr is the address of the throwaway MariaDB container.
	_mysqlAddr string

	// _pgAddr is the address of the throwaway PostgreSQL container.
	_pgAddr string
)

// startVersions starts a container of each version with start and keeps
// its address in addrs. It returns the containers it started, also when a
// later one fails, so the caller can stop them.
func startVersions(
	versions []string,
	addrs map[string]string,
	start func(version string) (*gnomock.Container, error),
) ([]*gnomock.Container, error) {
	containers := make([]*gnomock.Container, 0, len(versions))

	for _, v := range versions {
		c, err := start(v)
		if err != nil {
			return containers, fmt.Errorf("starting %s: %w", v, err)
		}

		containers = append(containers, c)
		addrs[v] = c.DefaultAddress()
	}

	return containers, nil
}

func TestMain(m *testing.M) {
	// silence the driver's connection noise while the MariaDB container
	// boots.
	if err := mysqlDriver.SetLogger(log.New(io.Discard, "", 0)); err != nil {
		panic("cannot set mysql logger: " + err.Error())
	}

	mysqlContainer, err := gnomock.StartCustom(
		"docker.io/library/mariadb:11-noble",
		gnomock.DefaultTCP(3306),
		gnomock.WithEnv("MARIADB_ROOT_PASSWORD="+_mysqlRootPass),
		gnomock.WithEnv("MARIADB_DATABASE="+_testDB),
		gnomock.WithHealthCheck(func(ctx context.Context, c *gnomock.Container) error {
			db, err := sql.Open("mysql", fmt.Sprintf(
				"%s:%s@tcp(%s)/%s",
				_mysqlRootUser,
				_mysqlRootPass,
				c.Address(gnomock.DefaultPort),
				_testDB,
			))
			if err != nil {
				return err
			}
			defer db.Close() //nolint:errcheck // error provides no meaningful info

			return db.PingContext(ctx)
		}),
	)
	if err != nil {
		panic("cannot set up mariadb: " + err.Error())
	}

	defer func() {
		err = gnomock.Stop(mysqlContainer)
		if err != nil {
			panic("cannot clean up mariadb: " + err.Error())
		}
	}()

	_mysqlAddr = mysqlContainer.Host + ":" + strconv.Itoa(mysqlContainer.DefaultPort())

	mysqlDB, err := sql.Open("mysql", fmt.Sprintf(
		"%s:%s@tcp(%s)/%s",
		_mysqlRootUser,
		_mysqlRootPass,
		_mysqlAddr,
		_testDB,
	))
	if err != nil {
		panic("cannot open mariadb: " + err.Error())
	}

	qq := []string{
		"CREATE TABLE metrics (time BIGINT, host VARCHAR(64), value DOUBLE)",
		"INSERT INTO metrics VALUES (1700000000, 'web-1', 10.5), (1700000060, 'web-2', 20.5)",
		"CREATE TABLE typed_metrics (time BIGINT, code VARCHAR(8), total DECIMAL(10,2))",
		"INSERT INTO typed_metrics VALUES (1700000000, '200', 10.50)",
		fmt.Sprintf("CREATE USER '%s'@'%%' IDENTIFIED BY '%s'", _readerUser, _readerPass),
		fmt.Sprintf("GRANT SELECT ON *.* TO '%s'@'%%'", _readerUser),
	}

	for _, q := range qq {
		if _, err = mysqlDB.Exec(q); err != nil {
			panic("cannot seed mariadb: " + err.Error())
		}
	}

	if err = mysqlDB.Close(); err != nil {
		panic("cannot close mariadb: " + err.Error())
	}

	pgContainer, err := gnomock.Start(pgDocker.Preset(
		pgDocker.WithUser(_pgUser, _pgPass),
		pgDocker.WithDatabase(_testDB),
		pgDocker.WithQueries(
			"CREATE TABLE metrics (time DOUBLE PRECISION, host TEXT, value DOUBLE PRECISION)",
			"INSERT INTO metrics VALUES (1700000000, 'web-1', 10.5), (1700000060, 'web-2', 20.5)",
			"CREATE TABLE typed_metrics (time DOUBLE PRECISION, host TEXT, ratio REAL, total NUMERIC(10,2))",
			"INSERT INTO typed_metrics VALUES (1700000000, 'web-1', 0.25, 10.50)",
			fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD '%s'", _readerUser, _readerPass),
			fmt.Sprintf("ALTER ROLE %s SET default_transaction_read_only = on", _readerUser),
			"GRANT SELECT ON metrics TO "+_readerUser,
			// a schema Supabase keeps for its own services.
			"CREATE SCHEMA auth",
			"CREATE TABLE auth.users (id INT, email TEXT)",
		),
	))
	if err != nil {
		panic("cannot set up postgres: " + err.Error())
	}

	defer func() {
		err = gnomock.Stop(pgContainer)
		if err != nil {
			panic("cannot clean up postgres: " + err.Error())
		}
	}()

	_pgAddr = pgContainer.Host + ":" + strconv.Itoa(pgContainer.DefaultPort())

	// the corpus sits two hours back, so it is old enough to backfill and
	// young enough for the ingesters to keep without a long chunk age.
	_logsWindow = TimeRange{
		From: timeutil.Now().UTC().Truncate(time.Hour).Add(-2 * time.Hour),
	}
	_logsWindow.To = _logsWindow.From.Add(time.Hour)
	_logsCorpus = newLogsCorpus(_logsWindow.From)

	lokiContainers, lerr := startVersions(_lokiVersions, _lokiAddrs, func(v string) (*gnomock.Container, error) {
		return startLoki(v, _logsCorpus)
	})

	defer func() {
		if serr := gnomock.Stop(lokiContainers...); serr != nil {
			panic("cannot clean up loki: " + serr.Error())
		}
	}()

	if lerr != nil {
		panic("cannot set up loki: " + lerr.Error())
	}

	victoriaLogsContainers, verr := startVersions(_victoriaLogsVersions, _victoriaLogsAddrs, func(v string) (*gnomock.Container, error) {
		return startVictoriaLogs(v, _logsCorpus)
	})

	defer func() {
		if serr := gnomock.Stop(victoriaLogsContainers...); serr != nil {
			panic("cannot clean up victorialogs: " + serr.Error())
		}
	}()

	if verr != nil {
		panic("cannot set up victorialogs: " + verr.Error())
	}

	victoriaMetricsContainers, merr := startVersions(_victoriaMetricsVersions, _victoriaMetricsAddrs, startVictoriaMetrics)

	defer func() {
		if serr := gnomock.Stop(victoriaMetricsContainers...); serr != nil {
			panic("cannot clean up victoriametrics: " + serr.Error())
		}
	}()

	if merr != nil {
		panic("cannot set up victoriametrics: " + merr.Error())
	}

	// goleak.VerifyTestMain ends in os.Exit, which skips the deferred
	// container stops. Returning runs them, and the test binary then exits
	// with the code from m.Run.
	ignore := goleak.IgnoreCurrent()

	if m.Run() != 0 {
		return
	}

	if err = goleak.Find(ignore); err != nil {
		panic("goroutines leaked: " + err.Error())
	}
}

func Test_ConnectionStatus_Error(t *testing.T) {
	cc := map[string]struct {
		Status ConnectionStatus
		Err    error
	}{
		"Unauthorized status": {
			Status: ConnectionStatusUnauthorized,
			Err:    errutil.New(http.StatusUnauthorized, "data_source.unauthorized", "Unauthorized access to the data source."),
		},
		"Unreachable status": {
			Status: ConnectionStatusUnreachable,
			Err:    errutil.New(http.StatusBadRequest, "data_source.unreachable", "The data source is unreachable."),
		},
		"Version not supported status": {
			Status: ConnectionStatusVersionNotSupported,
			Err:    errutil.New(http.StatusBadRequest, "data_source.version_not_supported", "The data source version is not supported."),
		},
		"Not read-only status": {
			Status: ConnectionStatusNotReadOnly,
			Err:    errutil.New(http.StatusBadRequest, "data_source.not_read_only", "The data source connection must be read-only."),
		},
		"Invalid encryption key status": {
			Status: ConnectionStatusInvalidEncryptionKey,
			Err: errutil.New(
				http.StatusBadRequest,
				"data_source.invalid_encryption_key",
				"The data source credentials cannot be decrypted with the configured keys and must be entered again.",
			),
		},
		"Invalid CA certificate status": {
			Status: ConnectionStatusInvalidCACertificate,
			Err: errutil.New(
				http.StatusBadRequest,
				"data_source.invalid_ca_certificate",
				"The CA certificate must be a PEM certificate, and the data source must connect over TLS.",
			),
		},
		"Success status": {
			Status: ConnectionStatusSuccess,
		},
		"Unknown status": {
			Status: ConnectionStatus("bogus"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Err, c.Status.Error())
		})
	}
}

// containerRequest sends a request to a test container and returns the
// body. It fails unless the container answers 2xx.
func containerRequest(ctx context.Context, method, target, contentType string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck // error provides no meaningful info

	res, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("%s answered %d: %s", target, resp.StatusCode, res)
	}

	return res, nil
}

// tokenHandler answers 401 to a request that does not carry _testToken as
// its bearer token, and passes every other request to next.
func tokenHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+_testToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// pemCertificate encodes a DER certificate as PEM.
func pemCertificate(der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: der,
	}))
}

// caCredentials returns credentials carrying only the CA certificate.
func caCredentials(ca string) credentials.Credentials {
	data, err := json.Marshal(credentials.Basic{CACertificate: ca})
	if err != nil {
		panic("cannot marshal test credentials: " + err.Error())
	}

	return stubCredentials(string(data))
}

// stubCredentials returns credentials holding the given JSON.
func stubCredentials(data string) credentials.Credentials {
	var creds credentials.Credentials
	if err := creds.UnmarshalJSON([]byte(data)); err != nil {
		panic("cannot set test credentials: " + err.Error())
	}

	return creds
}
