package user

import (
	"net/http"
	"strings"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// All available SpotlightState constants.
const (
	// SpotlightStateShown means the app has started showing the spotlight.
	SpotlightStateShown SpotlightState = "shown"

	// SpotlightStateDismissed means the user closed the spotlight or acted on
	// it. It is final: a later shown state never replaces it.
	SpotlightStateDismissed SpotlightState = "dismissed"
)

// ErrInvalidSpotlightState is returned when a spotlight state is unknown.
var ErrInvalidSpotlightState = errutil.New(http.StatusBadRequest, "user.invalid_spotlight_state", "invalid spotlight state")

// SpotlightState names how far a user got with a spotlight the app shows once.
type SpotlightState string

// Validate reports whether the state is a known one.
func (ss SpotlightState) Validate() error {
	switch ss {
	case SpotlightStateShown, SpotlightStateDismissed:
		return nil
	default:
		return ErrInvalidSpotlightState
	}
}

// MarshalText returns the state as text.
func (ss SpotlightState) MarshalText() ([]byte, error) {
	return []byte(ss), nil
}

// UnmarshalText parses a state, trimmed and lowercased. An unknown state
// is kept, so Validate refuses it rather than the JSON failing to decode.
func (ss *SpotlightState) UnmarshalText(text []byte) error {
	*ss = SpotlightState(strings.ToLower(strings.TrimSpace(string(text))))

	return nil
}

// Spotlights holds the state of each spotlight the app shows a user once.
type Spotlights struct {
	// Star specifies the state of the spotlight that asks the user to star
	// the project on GitHub, null while it was never shown.
	Star null.Value[SpotlightState] `json:"star" db:"star"`
}

// SpotlightsInput is the input for changing the state of a user's spotlights.
type SpotlightsInput struct {
	// Star specifies the new state of the star spotlight.
	Star SpotlightState `json:"star"`
}

// Validate checks that the input names a known state.
func (inp SpotlightsInput) Validate() error {
	return inp.Star.Validate()
}
