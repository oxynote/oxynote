package state

import (
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// _testSecret is the signing secret used across the state tests.
const _testSecret = "01234567890123456789012345678912"

// testState is a state payload used to exercise the codec.
type testState struct {
	// OrganizationID is the organization the state belongs to.
	OrganizationID string `json:"organizationId"`

	// CreatedAt is the time the state was issued at.
	CreatedAt time.Time `json:"createdAt"`
}

// Created reports when the test state was issued.
func (ts testState) Created() time.Time {
	return ts.CreatedAt
}

// unmarshalableState is a state payload that cannot be marshaled.
type unmarshalableState struct {
	// Fn is a function field, which encoding/json refuses to marshal.
	Fn func() `json:"fn"`
}

// Created reports when the unmarshalable state was issued.
func (us unmarshalableState) Created() time.Time {
	return time.Time{}
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Encode(t *testing.T) {
	t.Parallel()

	// marshaling error
	_, err := Encode(unmarshalableState{Fn: func() {}}, _testSecret)
	assert.Error(t, err)

	// encryption error
	_, err = Encode(testState{}, "short-secret")
	assert.Error(t, err)

	// success
	token, err := Encode(testState{OrganizationID: "org-1"}, _testSecret)
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func Test_Decode(t *testing.T) {
	t.Parallel()

	// tcase carries a decode scenario; cases that need setup build it
	// through an immediately-invoked closure.
	type tcase struct {
		Token  string
		Secret string
		TTL    time.Duration
		Result testState
		Err    error
	}

	stub := func(createdAt time.Time) string {
		t.Helper()

		token, err := Encode(testState{
			OrganizationID: "org-1",
			CreatedAt:      createdAt,
		}, _testSecret)
		require.NoError(t, err)

		return token
	}

	cc := map[string]tcase{
		"Undecryptable token": {
			Token:  "not-a-token",
			Secret: _testSecret,
			TTL:    time.Minute,
			Err:    ErrInvalid,
		},
		"Token encrypted with another secret": {
			Token:  stub(time.Now()),
			Secret: "99999999999999999999999999999999",
			TTL:    time.Minute,
			Err:    ErrInvalid,
		},
		"Token carrying invalid JSON": func() tcase {
			token, err := cryptoutil.EncryptText("not json", []byte(_testSecret))
			require.NoError(t, err)

			return tcase{
				Token:  token,
				Secret: _testSecret,
				TTL:    time.Minute,
				Err:    ErrInvalid,
			}
		}(),
		"Expired token": {
			Token:  stub(time.Now().Add(-time.Hour)),
			Secret: _testSecret,
			TTL:    time.Minute,
			Err:    ErrExpired,
		},
		"Valid token": {
			Token:  stub(time.Now()),
			Secret: _testSecret,
			TTL:    time.Minute,
			Result: testState{OrganizationID: "org-1"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := Decode[testState](c.Token, c.Secret, c.TTL)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result.OrganizationID, res.OrganizationID)
		})
	}
}
