package datasource

import (
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
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

func Test_NewDataSource(t *testing.T) {
	t.Parallel()

	ds := NewDataSource(CreateInput{
		Type:        TypePrometheus,
		Name:        "test-source",
		URL:         "http://prometheus.test",
		Credentials: processor.Credentials(`{"username":"user","password":"pass"}`),
	}, "org-1")

	require.NotNil(t, ds)
	assert.False(t, ds.ID.IsNil())
	assert.Equal(t, "org-1", ds.OrganizationID)
	assert.Equal(t, "test-source", ds.Name)
	assert.Equal(t, TypePrometheus, ds.Type)
	assert.Equal(t, "http://prometheus.test", ds.URL)
	assert.Equal(t, processor.Credentials(`{"username":"user","password":"pass"}`), ds.Credentials)
	assert.Equal(t, processor.ConnectionStatusSuccess, ds.Status)
	assert.WithinDuration(t, timeutil.Now(), ds.CreatedAt, time.Second)
	assert.False(t, ds.UpdatedAt.Valid)
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

func Test_DataSource_ApplyUpdate(t *testing.T) {
	cc := map[string]struct {
		DataSource *DataSource
		Inp        UpdateInput
		Name       string
		URL        string
		Creds      processor.Credentials
		Err        error
	}{
		"Error returned by updateCredentials": {
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
				Name: null.StringFrom("new"),
				URL:  null.StringFrom("http://new.test"),
			},
			Name: "new",
			URL:  "http://new.test",
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
			Creds: processor.Credentials(`{"username":"user","password":""}`),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := c.DataSource.ApplyUpdate(c.Inp)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Name, c.DataSource.Name)
			assert.Equal(t, c.URL, c.DataSource.URL)
			assert.Equal(t, c.Creds, c.DataSource.Credentials)
			require.True(t, c.DataSource.UpdatedAt.Valid)
			assert.WithinDuration(t, timeutil.Now(), c.DataSource.UpdatedAt.Time, time.Second)
		})
	}
}

func Test_DataSource_updateCredentials(t *testing.T) {
	cc := map[string]struct {
		DataSource *DataSource
		Inp        null.Value[processor.CredentialsUpdateInput]
		Creds      processor.Credentials
		Err        error
	}{
		"Skipped update without valid input": {
			DataSource: &DataSource{
				Type:        TypePrometheus,
				Credentials: processor.Credentials(`{"username":"old","password":""}`),
			},
			Creds: processor.Credentials(`{"username":"old","password":""}`),
		},
		"Error returned by processor.UpdatePrometheusCredentials": {
			DataSource: &DataSource{Type: TypePrometheus},
			Inp:        null.ValueFrom(processor.CredentialsUpdateInput(`{`)),
			Err:        assert.AnError,
		},
		"Successful prometheus update": {
			DataSource: &DataSource{Type: TypePrometheus},
			Inp:        null.ValueFrom(processor.CredentialsUpdateInput(`{"username":"user"}`)),
			Creds:      processor.Credentials(`{"username":"user","password":""}`),
		},
		"Error returned by processor.UpdatePostgreSQLCredentials": {
			DataSource: &DataSource{Type: TypePostgreSQL},
			Inp:        null.ValueFrom(processor.CredentialsUpdateInput(`{`)),
			Err:        assert.AnError,
		},
		"Successful postgresql update": {
			DataSource: &DataSource{Type: TypePostgreSQL},
			Inp:        null.ValueFrom(processor.CredentialsUpdateInput(`{"username":"user"}`)),
			Creds:      processor.Credentials(`{"username":"user","password":""}`),
		},
		"Error returned by processor.UpdateMySQLCredentials": {
			DataSource: &DataSource{Type: TypeMariaDB},
			Inp:        null.ValueFrom(processor.CredentialsUpdateInput(`{`)),
			Err:        assert.AnError,
		},
		"Successful mariadb update": {
			DataSource: &DataSource{Type: TypeMariaDB},
			Inp:        null.ValueFrom(processor.CredentialsUpdateInput(`{"username":"user"}`)),
			Creds:      processor.Credentials(`{"username":"user","password":""}`),
		},
		"Successful mysql update": {
			DataSource: &DataSource{Type: TypeMySQL},
			Inp:        null.ValueFrom(processor.CredentialsUpdateInput(`{"username":"user"}`)),
			Creds:      processor.Credentials(`{"username":"user","password":""}`),
		},
		"Invalid data source type": {
			DataSource: &DataSource{Type: Type("bogus")},
			Inp:        null.ValueFrom(processor.CredentialsUpdateInput(`{"username":"user"}`)),
			Err:        assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := c.DataSource.updateCredentials(c.Inp)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Creds, c.DataSource.Credentials)
		})
	}
}
