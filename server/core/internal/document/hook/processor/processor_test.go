package processor

import (
	"encoding/json"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_State_MarshalJSON(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Value  any
		Result string
	}{
		"Nil state inside a struct marshals as null": {
			Value: struct {
				State State `json:"state"`
			}{},
			Result: `{"state":null}`,
		},
		"Raw state is kept": {
			Value:  State(`{"a":1}`),
			Result: `{"a":1}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(c.Value)
			require.NoError(t, err)
			assert.Equal(t, c.Result, string(data))
		})
	}
}

func Test_Settings_MarshalJSON(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Value  any
		Result string
	}{
		"Nil settings inside a struct marshal as null": {
			Value: struct {
				Settings Settings `json:"settings"`
			}{},
			Result: `{"settings":null}`,
		},
		"Raw settings are kept": {
			Value:  Settings(`{"a":1}`),
			Result: `{"a":1}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(c.Value)
			require.NoError(t, err)
			assert.Equal(t, c.Result, string(data))
		})
	}
}

func Test_State_Scan(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Src    any
		Result State
		Err    error
	}{
		"String source": {
			Src:    `{"a":1}`,
			Result: State(`{"a":1}`),
		},
		"Byte source is copied": {
			Src:    []byte(`{"a":1}`),
			Result: State(`{"a":1}`),
		},
		"Invalid source type": {
			Src: 42,
			Err: assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var s State

			err := s.Scan(c.Src)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			// the driver may reuse its buffer once Scan returns, so the
			// scanned value must not alias it.
			if src, ok := c.Src.([]byte); ok {
				src[2] = 'b'
			}

			assert.Equal(t, c.Result, s)
		})
	}
}

func Test_Settings_Scan(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Src    any
		Result Settings
		Err    error
	}{
		"String source": {
			Src:    `{"a":1}`,
			Result: Settings(`{"a":1}`),
		},
		"Byte source is copied": {
			Src:    []byte(`{"a":1}`),
			Result: Settings(`{"a":1}`),
		},
		"Invalid source type": {
			Src: 42,
			Err: assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var s Settings

			err := s.Scan(c.Src)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			// the driver may reuse its buffer once Scan returns, so the
			// scanned value must not alias it.
			if src, ok := c.Src.([]byte); ok {
				src[2] = 'b'
			}

			assert.Equal(t, c.Result, s)
		})
	}
}
