package slackapp

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/purse/util/timeutil"
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

// Value implements the driver.Valuer interface for database storage.
// It converts the UserLinkSettings to JSON for storage in the database.
func (uls UserLinkSettings) Value() (driver.Value, error) {
	return json.Marshal(uls)
}

// Scan implements the sql.Scanner interface for database retrieval.
// It converts JSON data from the database back to UserLinkSettings.
func (uls *UserLinkSettings) Scan(value any) error {
	if value == nil {
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into UserLinkSettings", value)
	}

	return json.Unmarshal(data, uls)
}

// AppAccess  represents access information for a Slack app.
type AppAccess struct {
	// TeamID is the identifier for the Slack team.
	TeamID string `json:"teamId"`

	// AccessToken is the access token for the Slack app.
	AccessToken string `json:"accessToken"`

	// AppID is the identifier for the Slack app.
	AppID string `json:"appId"`
}
