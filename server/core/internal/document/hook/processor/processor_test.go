package processor

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_State_MarshalJSON(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(struct {
		State State `json:"state"`
	}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(data) != `{"state":null}` {
		t.Fatalf("unexpected result: %s", data)
	}

	data, err = json.Marshal(State(`{"a":1}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(data) != `{"a":1}` {
		t.Fatalf("unexpected result: %s", data)
	}
}

func Test_Settings_MarshalJSON(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(struct {
		Settings Settings `json:"settings"`
	}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(data) != `{"settings":null}` {
		t.Fatalf("unexpected result: %s", data)
	}

	data, err = json.Marshal(Settings(`{"a":1}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(data) != `{"a":1}` {
		t.Fatalf("unexpected result: %s", data)
	}
}

func Test_State_Scan(t *testing.T) {
	t.Parallel()

	var s State

	require.NoError(t, s.Scan(`{"a":1}`))
	assert.Equal(t, State(`{"a":1}`), s)

	// the driver may reuse its buffer once Scan returns, so the scanned
	// value must not alias it.
	src := []byte(`{"a":1}`)

	require.NoError(t, s.Scan(src))

	src[2] = 'b'

	assert.Equal(t, State(`{"a":1}`), s)

	assert.Error(t, s.Scan(42))
}

func Test_Settings_Scan(t *testing.T) {
	t.Parallel()

	var s Settings

	require.NoError(t, s.Scan(`{"a":1}`))
	assert.Equal(t, Settings(`{"a":1}`), s)

	// the driver may reuse its buffer once Scan returns, so the scanned
	// value must not alias it.
	src := []byte(`{"a":1}`)

	require.NoError(t, s.Scan(src))

	src[2] = 'b'

	assert.Equal(t, Settings(`{"a":1}`), s)

	assert.Error(t, s.Scan(42))
}
