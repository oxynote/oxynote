package user

import (
	"testing"
	"time"

	"github.com/guregu/null/v5"
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

func Test_SpotlightActivity_QualifiesForStar(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.November, 28, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour

	// aged returns an activity whose account has the given age.
	aged := func(age time.Duration, fn func(*SpotlightActivity)) SpotlightActivity {
		sa := SpotlightActivity{AccountCreatedAt: now.Add(-age)}

		if fn != nil {
			fn(&sa)
		}

		return sa
	}

	cc := map[string]struct {
		Activity SpotlightActivity
		Result   bool
	}{
		"Account below the minimum age with every signal": {
			Activity: aged(3*day-time.Second, func(sa *SpotlightActivity) {
				sa.LastViewedAt = null.TimeFrom(now)
				sa.CreatedDocuments = 100
				sa.EditedDocuments = 100
				sa.AcceptedInvitations = 100
				sa.Approvals = 100
				sa.Comments = 100
				sa.MCPClients = 100
			}),
		},
		"No signal": {
			Activity: aged(10*day, nil),
		},
		"Every count one below its threshold": {
			Activity: aged(10*day, func(sa *SpotlightActivity) {
				sa.LastViewedAt = null.TimeFrom(now)
				sa.CreatedDocuments = 4
				sa.EditedDocuments = 9
				sa.AcceptedInvitations = 1
				sa.Comments = 4
			}),
		},
		"Old account that never viewed a document": {
			Activity: aged(14*day, nil),
		},
		"Old account whose last view is not recent": {
			Activity: aged(14*day, func(sa *SpotlightActivity) {
				sa.LastViewedAt = null.TimeFrom(now.Add(-3*day - time.Second))
			}),
		},
		"Old account with a recent view": {
			Activity: aged(14*day, func(sa *SpotlightActivity) {
				sa.LastViewedAt = null.TimeFrom(now.Add(-3 * day))
			}),
			Result: true,
		},
		"Enough created documents": {
			Activity: aged(3*day, func(sa *SpotlightActivity) {
				sa.CreatedDocuments = 5
			}),
			Result: true,
		},
		"Enough edited documents": {
			Activity: aged(3*day, func(sa *SpotlightActivity) {
				sa.EditedDocuments = 10
			}),
			Result: true,
		},
		"Enough accepted invitations": {
			Activity: aged(3*day, func(sa *SpotlightActivity) {
				sa.AcceptedInvitations = 2
			}),
			Result: true,
		},
		"Enough comments": {
			Activity: aged(3*day, func(sa *SpotlightActivity) {
				sa.Comments = 5
			}),
			Result: true,
		},
		"An approval": {
			Activity: aged(3*day, func(sa *SpotlightActivity) {
				sa.Approvals = 1
			}),
			Result: true,
		},
		"A connected MCP client": {
			Activity: aged(3*day, func(sa *SpotlightActivity) {
				sa.MCPClients = 1
			}),
			Result: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Activity.QualifiesForStar(now))
		})
	}
}
