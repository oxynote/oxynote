package user

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_SpotlightState_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		State SpotlightState
		Err   error
	}{
		"Empty":     {Err: ErrInvalidSpotlightState},
		"Unknown":   {State: "hidden", Err: ErrInvalidSpotlightState},
		"Shown":     {State: SpotlightStateShown},
		"Dismissed": {State: SpotlightStateDismissed},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.State.Validate())
		})
	}
}

func Test_SpotlightState_MarshalText(t *testing.T) {
	t.Parallel()

	text, err := SpotlightStateDismissed.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "dismissed", string(text))
}

func Test_SpotlightState_UnmarshalText(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Text  string
		State SpotlightState
	}{
		"Unknown is kept":   {Text: "hidden", State: "hidden"},
		"Known":             {Text: "shown", State: SpotlightStateShown},
		"Padded upper case": {Text: " DISMISSED ", State: SpotlightStateDismissed},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var ss SpotlightState

			require.NoError(t, ss.UnmarshalText([]byte(c.Text)))
			assert.Equal(t, c.State, ss)
		})
	}
}

func Test_SpotlightsInput_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input SpotlightsInput
		Err   error
	}{
		"Missing star state": {
			Input: SpotlightsInput{},
			Err:   ErrInvalidSpotlightState,
		},
		"Unknown star state": {
			Input: SpotlightsInput{Star: "hidden"},
			Err:   ErrInvalidSpotlightState,
		},
		"Successful validation": {
			Input: SpotlightsInput{Star: SpotlightStateShown},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Input.Validate())
		})
	}
}
