package processor

import (
	"context"
	"database/sql"
	"encoding/base64"
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
	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// _testKeys is the keyring used by credentials crypto tests.
var _testKeys = func() *cryptoutil.Keyring {
	keys, err := cryptoutil.ParseKeyring(base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901")))
	if err != nil {
		panic("cannot parse test keyring: " + err.Error())
	}

	return keys
}()

// _testAAD is the associated data credentials crypto tests seal with.
var _testAAD = []byte("data-source-id")

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
	cc := map[string]struct {
		Creds Credentials

		// ByValue marshals the credentials as a field held by value in
		// another struct.
		ByValue bool
		Result  string
	}{
		"Credentials with a payload": {
			Creds:  NewCredentials([]byte(`{"username":"user"}`)),
			Result: `{"username":"user"}`,
		},
		// empty credentials marshal as null rather than as the zero bytes
		// that make every enclosing struct fail with "unexpected end of JSON
		// input".
		"Empty credentials": {
			Result: `null`,
		},
		// the payload is unexported, so a struct that marshals credentials
		// it should not emits nothing of them rather than the secret
		// itself.
		"Credentials held by value": {
			Creds:   NewCredentials([]byte(`{"username":"user"}`)),
			ByValue: true,
			Result:  `{"credentials":{}}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var (
				data []byte
				err  error
			)

			if c.ByValue {
				data, err = json.Marshal(struct {
					Credentials Credentials `json:"credentials"`
				}{Credentials: c.Creds})
			} else {
				data, err = c.Creds.MarshalJSON()
			}

			require.NoError(t, err)
			assert.JSONEq(t, c.Result, string(data))
		})
	}
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
	unreadable := NewCredentials([]byte("not-sealed"))
	require.Error(t, unreadable.Decrypt(_testKeys, _testAAD))
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
	type tcase struct {
		Creds Credentials
		Err   error
	}

	cc := map[string]tcase{
		// credentials a decrypt could not read are refused, so the
		// ciphertext they came from is never overwritten by an encryption
		// of nothing.
		"Unreadable credentials": func() tcase {
			creds := NewCredentials([]byte("not-sealed"))
			require.Error(t, creds.Decrypt(_testKeys, _testAAD))

			return tcase{
				Creds: creds,
				Err:   assert.AnError,
			}
		}(),
		"Successful encryption": {
			Creds: NewCredentials([]byte(`{"username":"user"}`)),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			data, err := c.Creds.Encrypt(_testKeys, _testAAD)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			require.NotEmpty(t, data)

			decrypted := NewCredentials(data)
			require.NoError(t, decrypted.Decrypt(_testKeys, _testAAD))
			assert.Equal(t, c.Creds, decrypted)
		})
	}
}

func Test_Credentials_Decrypt(t *testing.T) {
	original := NewCredentials([]byte(`{"username":"user"}`))

	sealed, err := original.Encrypt(_testKeys, _testAAD)
	require.NoError(t, err)

	cc := map[string]struct {
		Data   []byte
		AAD    []byte
		Result Credentials
		Err    error
	}{
		// what it could not read is dropped rather than left as ciphertext
		// nobody can use, and says so to every later reader.
		"Error returned by Keyring.Decrypt": {
			Data:   []byte("not-sealed"),
			AAD:    _testAAD,
			Result: Credentials{unreadable: true},
			Err:    assert.AnError,
		},
		"Credentials sealed for another row": {
			Data:   sealed,
			AAD:    []byte("other-id"),
			Result: Credentials{unreadable: true},
			Err:    assert.AnError,
		},
		"Successful decryption": {
			Data:   sealed,
			AAD:    _testAAD,
			Result: original,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			creds := NewCredentials(c.Data)

			err := creds.Decrypt(_testKeys, c.AAD)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, err == nil, creds.IsValid())
			assert.Equal(t, c.Result, creds)
		})
	}
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
		"Error returned by json.Unmarshal of credentials": {
			Creds: NewCredentials([]byte(`{`)),
			Inp:   CredentialsUpdateInput(`{"username":"user"}`),
			Err:   assert.AnError,
		},
		"Error returned by json.Unmarshal of update input": {
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
