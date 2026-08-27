package processor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"testing"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/orlangure/gnomock"
	pgDocker "github.com/orlangure/gnomock/preset/postgres"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// _testSigningKey is a 32-byte AES key used by credentials crypto tests.
const _testSigningKey = "01234567890123456789012345678901"

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
		gnomock.WithHealthCheck(mysqlHealthcheck),
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

	if err = seedMySQL(_mysqlAddr); err != nil {
		panic("cannot seed mariadb: " + err.Error())
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

	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}

// mysqlHealthcheck reports whether the MariaDB container accepts root
// connections.
func mysqlHealthcheck(ctx context.Context, c *gnomock.Container) error {
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
}

// seedMySQL populates the MariaDB container with test data and a
// read-only user.
func seedMySQL(addr string) error {
	db, err := sql.Open("mysql", fmt.Sprintf(
		"%s:%s@tcp(%s)/%s",
		_mysqlRootUser,
		_mysqlRootPass,
		addr,
		_testDB,
	))
	if err != nil {
		return err
	}
	defer db.Close() //nolint:errcheck // error provides no meaningful info

	qq := []string{
		"CREATE TABLE metrics (time BIGINT, host VARCHAR(64), value DOUBLE)",
		"INSERT INTO metrics VALUES (1700000000, 'web-1', 10.5), (1700000060, 'web-2', 20.5)",
		"CREATE TABLE typed_metrics (time BIGINT, code VARCHAR(8), total DECIMAL(10,2))",
		"INSERT INTO typed_metrics VALUES (1700000000, '200', 10.50)",
		fmt.Sprintf("CREATE USER '%s'@'%%' IDENTIFIED BY '%s'", _readerUser, _readerPass),
		fmt.Sprintf("GRANT SELECT ON *.* TO '%s'@'%%'", _readerUser),
	}

	for _, q := range qq {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}

	return nil
}

// mysqlTestURL builds a MariaDB data source URL for the given user.
func mysqlTestURL(user, pass string) string {
	return fmt.Sprintf("mysql://%s:%s@%s/%s", user, pass, _mysqlAddr, _testDB)
}

// pgTestURL builds a PostgreSQL data source URL for the given user.
func pgTestURL(user, pass string) string {
	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", user, pass, _pgAddr, _testDB)
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
		"Invalid signing secret status": {
			Status: ConnectionStatusInvalidSigningSecret,
			Err: errutil.New(
				http.StatusBadRequest,
				"data_source.invalid_signing_secret",
				"The data source credentials cannot be decrypted and must be entered again.",
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

func Test_Credentials_MarshalJSON(t *testing.T) {
	t.Parallel()

	c := NewCredentials([]byte(`{"username":"user"}`))

	data, err := c.MarshalJSON()
	require.NoError(t, err)
	assert.JSONEq(t, `{"username":"user"}`, string(data))

	// empty credentials marshal as null rather than as the zero bytes that
	// make every enclosing struct fail with "unexpected end of JSON input".
	var empty Credentials

	data, err = empty.MarshalJSON()
	require.NoError(t, err)
	assert.JSONEq(t, `null`, string(data))

	// the payload is unexported, so a struct that marshals credentials it
	// should not emits nothing of them rather than the secret itself.
	out, err := json.Marshal(struct {
		Credentials Credentials `json:"credentials"`
	}{Credentials: c})
	require.NoError(t, err)
	assert.JSONEq(t, `{"credentials":{}}`, string(out))
}

func Test_Credentials_IsValid(t *testing.T) {
	t.Parallel()

	// credentials carrying a payload, and empty ones standing for a data
	// source that authenticates through its URL, are both readable.
	c := NewCredentials([]byte(`{"username":"user"}`))
	assert.True(t, c.IsValid())

	var empty Credentials
	assert.True(t, empty.IsValid())

	// only a decrypt that could not make sense of them says otherwise.
	unreadable := NewCredentials([]byte("not-hex-encoded"))
	require.Error(t, unreadable.Decrypt(_testSigningKey))
	assert.False(t, unreadable.IsValid())
}

func Test_Credentials_Scan(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Src    any
		Result Credentials
		Err    error
	}{
		"Bytes straight off the column": {
			Src:    []byte(`{"username":"user"}`),
			Result: NewCredentials([]byte(`{"username":"user"}`)),
		},
		"A driver handing them over as a string": {
			Src:    `{"username":"user"}`,
			Result: NewCredentials([]byte(`{"username":"user"}`)),
		},
		"Anything else": {
			Src: 42,
			Err: assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var creds Credentials

			err := creds.Scan(c.Src)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, creds)
		})
	}
}

func Test_Credentials_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	var c Credentials

	require.NoError(t, c.UnmarshalJSON([]byte(`{"username":"user"}`)))
	assert.Equal(t, NewCredentials([]byte(`{"username":"user"}`)), c)
}

func Test_Credentials_Encrypt(t *testing.T) {
	t.Parallel()

	c := NewCredentials([]byte(`{"username":"user"}`))

	// error
	_, err := c.Encrypt("short-key")
	assert.Error(t, err)

	// success
	data, err := c.Encrypt(_testSigningKey)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	decrypted := NewCredentials(data)
	require.NoError(t, decrypted.Decrypt(_testSigningKey))
	assert.Equal(t, c, decrypted)

	// credentials a decrypt could not read are refused, so the ciphertext
	// they came from is never overwritten by an encryption of nothing.
	unreadable := NewCredentials([]byte("not-hex-encoded"))
	require.Error(t, unreadable.Decrypt(_testSigningKey))

	_, err = unreadable.Encrypt(_testSigningKey)
	assert.Error(t, err)
}

func Test_Credentials_Decrypt(t *testing.T) {
	t.Parallel()

	// error
	c := NewCredentials([]byte("not-hex-encoded"))
	assert.Error(t, c.Decrypt(_testSigningKey))

	// what it could not read is dropped rather than left as ciphertext
	// nobody can use, and says so to every later reader.
	assert.False(t, c.IsValid())
	assert.Equal(t, Credentials{unreadable: true}, c)

	// success
	original := NewCredentials([]byte(`{"username":"user"}`))

	data, err := original.Encrypt(_testSigningKey)
	require.NoError(t, err)

	c = NewCredentials(data)
	require.NoError(t, c.Decrypt(_testSigningKey))
	assert.Equal(t, original, c)
}

func Test_CredentialsUpdateInput_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	var cui CredentialsUpdateInput

	require.NoError(t, cui.UnmarshalJSON([]byte(`{"username":"user"}`)))
	assert.Equal(t, CredentialsUpdateInput(`{"username":"user"}`), cui)
}

func Test_UpdateBasicCredentials(t *testing.T) {
	cc := map[string]struct {
		Creds  Credentials
		Inp    CredentialsUpdateInput
		Result Credentials
		Err    error
	}{
		"Error returned by unmarshaling credentials": {
			Creds: NewCredentials([]byte(`{`)),
			Inp:   CredentialsUpdateInput(`{"username":"user"}`),
			Err:   assert.AnError,
		},
		"Error returned by unmarshaling update input": {
			Inp: CredentialsUpdateInput(`{`),
			Err: assert.AnError,
		},
		"Updated username retains password": {
			Creds:  NewCredentials([]byte(`{"username":"old","password":"secret"}`)),
			Inp:    CredentialsUpdateInput(`{"username":"new"}`),
			Result: NewCredentials([]byte(`{"username":"new","password":"secret"}`)),
		},
		"Updated password retains username": {
			Creds:  NewCredentials([]byte(`{"username":"user","password":"old"}`)),
			Inp:    CredentialsUpdateInput(`{"password":"new"}`),
			Result: NewCredentials([]byte(`{"username":"user","password":"new"}`)),
		},
		"Cleared credentials": {
			Creds: NewCredentials([]byte(`{"username":"user","password":"pass"}`)),
			Inp:   CredentialsUpdateInput(`{"username":"","password":""}`),
		},
		"Created credentials from scratch": {
			Inp:    CredentialsUpdateInput(`{"username":"user","password":"pass"}`),
			Result: NewCredentials([]byte(`{"username":"user","password":"pass"}`)),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			creds, err := UpdateBasicCredentials(c.Creds, c.Inp)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, creds)
		})
	}
}
