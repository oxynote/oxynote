// Package slack manages the Slack App integration and its API access.
package slack

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
)

// App represents a Slack application with its team ID.
type App struct {
	// TeamID is the identifier for the Slack team associated with the app.
	TeamID string `json:"teamId" db:"team_id"`

	// OrganizationID is the identifier for the organization associated with the app.
	OrganizationID null.String `json:"organizationId" db:"fk_organization_id"`

	// Token is the access token for the Slack app.
	Token string `json:"token" db:"token"`
}

// Message represents a message in the Slack app.
type Message struct {
	// ID is the unique identifier for the message.
	ID xid.ID `json:"id" db:"id"`

	// OrganizationID is the identifier for the organization associated with the message.
	OrganizationID string `json:"organizationId" db:"fk_organization_id"`

	// Text is the content of the message.
	Text string `json:"text" db:"text"`

	// CreatedAt is the timestamp when the message was created.
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}

// NewMessage creates a new message instance with the given organization ID and text.
func NewMessage(organizationID, text string) *Message {
	return &Message{
		ID:             xid.New(),
		OrganizationID: organizationID,
		Text:           text,
		CreatedAt:      timeutil.Now(),
	}
}

// UserLink represents a link between a Slack user and internal app user.
type UserLink struct {
	// SlackUserID is the Slack user identifier.
	SlackUserID string `json:"slackUserId" db:"slack_user_id"`

	// TeamID is the Slack team identifier.
	TeamID string `json:"teamId" db:"fk_team_id"`

	// UserID is the internal user identifier.
	UserID string `json:"userId" db:"fk_user_id"`

	// Settings contains the settings for the user link.
	Settings UserLinkSettings `json:"settings" db:"settings"`

	// CreatedAt is the timestamp when the link was created.
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}

// NewUserLink creates a new user link instance.
func NewUserLink(slackUserID, teamID, userID string) *UserLink {
	return &UserLink{
		SlackUserID: slackUserID,
		TeamID:      teamID,
		UserID:      userID,
		Settings: UserLinkSettings{
			Notifications: true,
		},
		CreatedAt: timeutil.Now(),
	}
}

// UserLinkSettings represents settings for a Slack user link.
type UserLinkSettings struct {
	// Notifications indicates whether notifications are enabled for the user link.
	Notifications bool `json:"notifications"`
}

// Value transforms the user link settings into a database entry.
func (uls UserLinkSettings) Value() (driver.Value, error) {
	// NOCOV: error case cannot happen since the data
	// is already validated.
	return json.Marshal(uls)
}

// Scan transforms a database entry into a user link settings type.
func (uls *UserLinkSettings) Scan(src any) error {
	var pv []byte

	switch v := src.(type) {
	case []byte:
		pv = v
	case string:
		pv = []byte(v)
	default:
		return errors.New("invalid user link settings type")
	}

	data := &UserLinkSettings{}
	if err := json.Unmarshal(pv, data); err != nil {
		return err
	}

	*uls = *data

	return nil
}

// AppAccess represents access information for a Slack app.
type AppAccess struct {
	// TeamID is the identifier for the Slack team.
	TeamID string `json:"teamId"`

	// AccessToken is the access token for the Slack app.
	AccessToken string `json:"accessToken"`

	// AppID is the identifier for the Slack app.
	AppID string `json:"appId"`
}
