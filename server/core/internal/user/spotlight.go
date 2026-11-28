package user

import (
	"net/http"
	"strings"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

const (
	// _starMinAccountAge is the account age below which the star spotlight
	// is never earned, so a new user is not asked in their first days.
	_starMinAccountAge = 3 * 24 * time.Hour

	// _starReturningAccountAge is the account age from which a recent view
	// alone earns the star spotlight.
	_starReturningAccountAge = 14 * 24 * time.Hour

	// _starRecentView is how far back the last view may lie for the user
	// to count as still returning.
	_starRecentView = 3 * 24 * time.Hour

	// _starCreatedDocuments is the count of created documents that earns
	// the star spotlight.
	_starCreatedDocuments = 5

	// _starEditedDocuments is the count of edited documents that earns the
	// star spotlight.
	_starEditedDocuments = 10

	// _starAcceptedInvitations is the count of accepted invitations that
	// earns the star spotlight.
	_starAcceptedInvitations = 2

	// _starComments is the count of comments and replies that earns the
	// star spotlight.
	_starComments = 5
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

// SpotlightActivity holds what a user has done so far. It decides which
// spotlights the user has earned.
type SpotlightActivity struct {
	// AccountCreatedAt specifies when the user's account was created.
	AccountCreatedAt time.Time `db:"account_created_at"`

	// LastViewedAt specifies when the user last opened a document, null
	// when they never did.
	LastViewedAt null.Time `db:"last_viewed_at"`

	// CreatedDocuments specifies how many documents the user created.
	CreatedDocuments int `db:"created_documents"`

	// EditedDocuments specifies how many documents the user edited.
	EditedDocuments int `db:"edited_documents"`

	// AcceptedInvitations specifies how many of the invitations the user
	// sent were accepted.
	AcceptedInvitations int `db:"accepted_invitations"`

	// Approvals specifies how many branch approvals the user gave, plus
	// those given to branches the user created.
	Approvals int `db:"approvals"`

	// Comments specifies how many comments and replies the user wrote.
	Comments int `db:"comments"`

	// MCPClients specifies how many MCP clients the user connected.
	MCPClients int `db:"mcp_clients"`
}

// QualifiesForStar reports whether the activity is enough to show the star
// spotlight at the given time. One signal is enough, but never before the
// account reaches its minimum age.
func (sa SpotlightActivity) QualifiesForStar(now time.Time) bool {
	age := now.Sub(sa.AccountCreatedAt)
	if age < _starMinAccountAge {
		return false
	}

	returning := age >= _starReturningAccountAge &&
		sa.LastViewedAt.Valid &&
		now.Sub(sa.LastViewedAt.Time) <= _starRecentView

	return returning ||
		sa.CreatedDocuments >= _starCreatedDocuments ||
		sa.EditedDocuments >= _starEditedDocuments ||
		sa.AcceptedInvitations >= _starAcceptedInvitations ||
		sa.Comments >= _starComments ||
		sa.Approvals > 0 ||
		sa.MCPClients > 0
}
