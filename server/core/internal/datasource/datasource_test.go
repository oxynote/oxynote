package datasource

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/datasource/demo"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// undecryptableCredentials returns credentials in the state a failed decrypt
// leaves behind, which is the only way they reach it.
func undecryptableCredentials(t *testing.T) processor.Credentials {
	t.Helper()

	keys, err := cryptoutil.ParseKeyring(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	require.NoError(t, err)

	creds := processor.NewCredentials([]byte("ciphertext"))
	require.Error(t, creds.Decrypt(keys, nil))

	return creds
}

func Test_NewDataSource(t *testing.T) {
	t.Parallel()

	creds := processor.NewCredentials([]byte(`{"username":"user","password":"pass"}`))

	cc := map[string]struct {
		Input  CreateInput
		Result *DataSource
		Err    error
	}{
		"Name of white space only": {
			Input: CreateInput{Name: "   "},
			Err:   ErrInvalidName,
		},
		"Invalid credentials": {
			Input: CreateInput{Name: "test-source", Credentials: processor.NewCredentials([]byte(`[1]`))},
			Err:   ErrInvalidCredentials,
		},
		"Successful creation": {
			Input: CreateInput{
				Type:        TypePrometheus,
				Name:        " test-source ",
				URL:         "http://prometheus.test",
				Credentials: creds,
			},
			Result: &DataSource{
				OrganizationID: "org-1",
				Name:           "test-source",
				Type:           TypePrometheus,
				URL:            "http://prometheus.test",
				Credentials:    creds,
				Status:         processor.ConnectionStatusSuccess,
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := NewDataSource(c.Input, "org-1")

			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, res)
				return
			}

			require.NotNil(t, res)
			assert.False(t, res.ID.IsNil())
			assert.WithinDuration(t, timeutil.Now(), res.CreatedAt, time.Second)
			testutil.AssertFilterEqual(t, c.Result, res, xid.ID{}, time.Time{})
		})
	}
}

func Test_NewDemoDataSources(t *testing.T) {
	t.Parallel()

	dd, err := NewDemoDataSources("org-1")
	require.NoError(t, err)
	require.Len(t, dd, 1)

	ds := dd[0]
	assert.False(t, ds.ID.IsNil())
	assert.Equal(t, "org-1", ds.OrganizationID)
	assert.Equal(t, "Demo Metrics", ds.Name)
	assert.Equal(t, TypePrometheus, ds.Type)
	assert.Equal(t, demo.URL, ds.URL)
	assert.Equal(t, processor.NewCredentials([]byte(`{}`)), ds.Credentials)
}

func Test_DataSource_Info(t *testing.T) {
	t.Parallel()

	id := xid.New()

	ds := &DataSource{
		ID:   id,
		Name: "test-source",
		Type: TypePostgreSQL,
	}

	assert.Equal(t, Info{
		ID:   id,
		Name: "test-source",
		Type: TypePostgreSQL,
	}, ds.Info())
}

func Test_CreateInput_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input CreateInput
		Err   error
	}{
		"Empty name": {
			Err: ErrInvalidName,
		},
		"Name of white space only": {
			Input: CreateInput{Name: " \t\n "},
			Err:   ErrInvalidName,
		},
		"Name over the length limit": {
			Input: CreateInput{Name: strings.Repeat("é", _maxNameLength+1)},
			Err:   ErrNameTooLong,
		},
		"Credentials that are not an object": {
			Input: CreateInput{Name: "Metrics", Credentials: processor.NewCredentials([]byte(`[1]`))},
			Err:   ErrInvalidCredentials,
		},
		"Credentials holding a number": {
			Input: CreateInput{Name: "Metrics", Credentials: processor.NewCredentials([]byte(`{"username":1}`))},
			Err:   ErrInvalidCredentials,
		},
		"No credentials": {
			Input: CreateInput{Name: "Metrics"},
		},
		"Name at the limit once trimmed": {
			Input: CreateInput{Name: " " + strings.Repeat("é", _maxNameLength) + " "},
		},
		"Valid credentials": {
			Input: CreateInput{Name: "Metrics", Credentials: processor.NewCredentials([]byte(`{"username":"user","password":"pass"}`))},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Input.Validate())
		})
	}
}

func Test_CreateInput_sanitize(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  CreateInput
		Result CreateInput
	}{
		"Name without outer white space": {
			Input:  CreateInput{Type: TypePrometheus, Name: "Metrics"},
			Result: CreateInput{Type: TypePrometheus, Name: "Metrics"},
		},
		"Name with outer white space": {
			Input:  CreateInput{Type: TypePrometheus, Name: " \tMetrics\n "},
			Result: CreateInput{Type: TypePrometheus, Name: "Metrics"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Input.sanitize())
		})
	}
}

func Test_DataSource_ApplyUpdate(t *testing.T) {
	cc := map[string]struct {
		DataSource *DataSource
		Inp        UpdateInput
		Name       string
		URL        string
		Creds      processor.Credentials
		Err        error
	}{
		"Name of white space only": {
			DataSource: &DataSource{
				Type: TypePrometheus,
				Name: "old",
				URL:  "http://old.test",
			},
			Inp: UpdateInput{
				Name: null.StringFrom("   "),
			},
			Err: ErrInvalidName,
		},
		"Error returned by updatedCredentials": {
			DataSource: &DataSource{
				Type: Type("bogus"),
				Name: "old",
				URL:  "http://old.test",
			},
			Inp: UpdateInput{
				Credentials: null.ValueFrom(processor.CredentialsUpdateInput(`{"username":"user"}`)),
			},
			Err: assert.AnError,
		},
		"Successful update without any fields set": {
			DataSource: &DataSource{
				Type: TypePrometheus,
				Name: "old",
				URL:  "http://old.test",
			},
			Name: "old",
			URL:  "http://old.test",
		},
		"Successful update of name and url": {
			DataSource: &DataSource{
				Type: TypePrometheus,
				Name: "old",
				URL:  "http://old.test",
			},
			Inp: UpdateInput{
				Name: null.StringFrom(" new "),
				URL:  null.StringFrom("http://new.test"),
			},
			Name: "new",
			URL:  "http://new.test",
		},
		// the stored credentials were entered for the old endpoint.
		"Changed url clears credentials": {
			DataSource: &DataSource{
				Type:        TypePrometheus,
				Name:        "old",
				URL:         "http://old.test",
				Credentials: processor.NewCredentials([]byte(`{"username":"user","password":"pass"}`)),
			},
			Inp: UpdateInput{
				URL: null.StringFrom("http://new.test"),
			},
			Name: "old",
			URL:  "http://new.test",
		},
		"Changed url keeps only the credentials sent with it": {
			DataSource: &DataSource{
				Type:        TypePrometheus,
				Name:        "old",
				URL:         "http://old.test",
				Credentials: processor.NewCredentials([]byte(`{"username":"user","password":"pass"}`)),
			},
			Inp: UpdateInput{
				URL:         null.StringFrom("http://new.test"),
				Credentials: null.ValueFrom(processor.CredentialsUpdateInput(`{"password":"new"}`)),
			},
			Name:  "old",
			URL:   "http://new.test",
			Creds: processor.NewCredentials([]byte(`{"username":"","password":"new"}`)),
		},
		"Unchanged url keeps credentials": {
			DataSource: &DataSource{
				Type:        TypePrometheus,
				Name:        "old",
				URL:         "http://old.test",
				Credentials: processor.NewCredentials([]byte(`{"username":"user","password":"pass"}`)),
			},
			Inp: UpdateInput{
				URL: null.StringFrom("http://old.test"),
			},
			Name:  "old",
			URL:   "http://old.test",
			Creds: processor.NewCredentials([]byte(`{"username":"user","password":"pass"}`)),
		},
		"Successful update of credentials": {
			DataSource: &DataSource{
				Type: TypePrometheus,
				Name: "old",
				URL:  "http://old.test",
			},
			Inp: UpdateInput{
				Credentials: null.ValueFrom(processor.CredentialsUpdateInput(`{"username":"user"}`)),
			},
			Name:  "old",
			URL:   "http://old.test",
			Creds: processor.NewCredentials([]byte(`{"username":"user","password":""}`)),
		},
		"Successful update of undecryptable credentials": {
			DataSource: &DataSource{
				Type:        TypePrometheus,
				Name:        "old",
				URL:         "http://old.test",
				Credentials: undecryptableCredentials(t),
			},
			Inp: UpdateInput{
				Credentials: null.ValueFrom(processor.CredentialsUpdateInput(`{"username":"user","password":"pass"}`)),
			},
			Name:  "old",
			URL:   "http://old.test",
			Creds: processor.NewCredentials([]byte(`{"username":"user","password":"pass"}`)),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := c.DataSource.ApplyUpdate(c.Inp)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Name, res.Name)
			assert.Equal(t, c.URL, res.URL)
			assert.Equal(t, c.Creds, res.Credentials)
			require.True(t, res.UpdatedAt.Valid)
			assert.WithinDuration(t, timeutil.Now(), res.UpdatedAt.Time, time.Second)
			assert.Equal(t, "old", c.DataSource.Name)
			assert.False(t, c.DataSource.UpdatedAt.Valid)
		})
	}
}

func Test_DataSource_updatedCredentials(t *testing.T) {
	cc := map[string]struct {
		DataSource *DataSource
		Inp        null.Value[processor.CredentialsUpdateInput]
		Creds      processor.Credentials
		Err        error
	}{
		"Skipped update without valid input": {
			DataSource: &DataSource{
				Type:        TypePrometheus,
				Credentials: processor.NewCredentials([]byte(`{"username":"old","password":""}`)),
			},
			Creds: processor.NewCredentials([]byte(`{"username":"old","password":""}`)),
		},
		"Error returned by processor.UpdateBasicCredentials for prometheus": {
			DataSource: &DataSource{Type: TypePrometheus},
			Inp:        null.ValueFrom(processor.CredentialsUpdateInput(`{`)),
			Err:        assert.AnError,
		},
		"Successful prometheus update": {
			DataSource: &DataSource{Type: TypePrometheus},
			Inp:        null.ValueFrom(processor.CredentialsUpdateInput(`{"username":"user"}`)),
			Creds:      processor.NewCredentials([]byte(`{"username":"user","password":""}`)),
		},
		"Error returned by processor.UpdateBasicCredentials for postgresql": {
			DataSource: &DataSource{Type: TypePostgreSQL},
			Inp:        null.ValueFrom(processor.CredentialsUpdateInput(`{`)),
			Err:        assert.AnError,
		},
		"Successful postgresql update": {
			DataSource: &DataSource{Type: TypePostgreSQL},
			Inp:        null.ValueFrom(processor.CredentialsUpdateInput(`{"username":"user"}`)),
			Creds:      processor.NewCredentials([]byte(`{"username":"user","password":""}`)),
		},
		"Successful supabase update": {
			DataSource: &DataSource{Type: TypeSupabase},
			Inp:        null.ValueFrom(processor.CredentialsUpdateInput(`{"username":"user"}`)),
			Creds:      processor.NewCredentials([]byte(`{"username":"user","password":""}`)),
		},
		"Error returned by processor.UpdateBasicCredentials for mariadb": {
			DataSource: &DataSource{Type: TypeMariaDB},
			Inp:        null.ValueFrom(processor.CredentialsUpdateInput(`{`)),
			Err:        assert.AnError,
		},
		"Successful mariadb update": {
			DataSource: &DataSource{Type: TypeMariaDB},
			Inp:        null.ValueFrom(processor.CredentialsUpdateInput(`{"username":"user"}`)),
			Creds:      processor.NewCredentials([]byte(`{"username":"user","password":""}`)),
		},
		"Successful mysql update": {
			DataSource: &DataSource{Type: TypeMySQL},
			Inp:        null.ValueFrom(processor.CredentialsUpdateInput(`{"username":"user"}`)),
			Creds:      processor.NewCredentials([]byte(`{"username":"user","password":""}`)),
		},
		"Successful loki update": {
			DataSource: &DataSource{Type: TypeLoki},
			Inp:        null.ValueFrom(processor.CredentialsUpdateInput(`{"username":"user"}`)),
			Creds:      processor.NewCredentials([]byte(`{"username":"user","password":""}`)),
		},
		"Invalid data source type": {
			DataSource: &DataSource{Type: Type("bogus")},
			Inp:        null.ValueFrom(processor.CredentialsUpdateInput(`{"username":"user"}`)),
			Err:        assert.AnError,
		},
		"Skipped update leaves undecryptable credentials as they are": {
			DataSource: &DataSource{
				Type:        TypePrometheus,
				Credentials: undecryptableCredentials(t),
			},
			Creds: undecryptableCredentials(t),
		},
		"Successful update replaces undecryptable credentials wholesale": {
			DataSource: &DataSource{
				Type:        TypePrometheus,
				Credentials: undecryptableCredentials(t),
			},
			Inp:   null.ValueFrom(processor.CredentialsUpdateInput(`{"username":"user"}`)),
			Creds: processor.NewCredentials([]byte(`{"username":"user","password":""}`)),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := c.DataSource.updatedCredentials(c.Inp)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Creds, res)
		})
	}
}

func Test_UpdateInput_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input UpdateInput
		Err   error
	}{
		"Name of white space only": {
			Input: UpdateInput{Name: null.StringFrom("   ")},
			Err:   ErrInvalidName,
		},
		"Name over the length limit": {
			Input: UpdateInput{Name: null.StringFrom(strings.Repeat("a", _maxNameLength+1))},
			Err:   ErrNameTooLong,
		},
		"Name unset": {
			Input: UpdateInput{URL: null.StringFrom("http://new.test")},
		},
		"Name with outer white space": {
			Input: UpdateInput{Name: null.StringFrom(" Metrics ")},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Input.Validate())
		})
	}
}

func Test_UpdateInput_sanitize(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  UpdateInput
		Result UpdateInput
	}{
		"Name unset": {
			Input:  UpdateInput{URL: null.StringFrom("http://new.test")},
			Result: UpdateInput{URL: null.StringFrom("http://new.test")},
		},
		"Name with outer white space": {
			Input:  UpdateInput{Name: null.StringFrom(" Metrics\t")},
			Result: UpdateInput{Name: null.StringFrom("Metrics")},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Input.sanitize())
		})
	}
}

func Test_validateName(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Name string
		Err  error
	}{
		"Empty name": {
			Err: ErrInvalidName,
		},
		"Name over the length limit": {
			Name: strings.Repeat("a", _maxNameLength+1),
			Err:  ErrNameTooLong,
		},
		"Name at the length limit": {
			Name: strings.Repeat("a", _maxNameLength),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, validateName(c.Name))
		})
	}
}
