package db

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/slackapp"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepSlackUserLinks(t *testing.T, db *DB, count int, fn func(int, *slackapp.UserLink)) []slackapp.UserLink { //nolint:unparam // fixture helpers share a uniform signature
	t.Helper()

	res := make([]slackapp.UserLink, count)

	now := timeutil.Now().Truncate(time.Second)

	for i := range count {
		link := slackapp.UserLink{
			SlackUserID: "slack-user-" + strconv.Itoa(i),
			Settings: slackapp.UserLinkSettings{
				Notifications: true,
			},
			CreatedAt: now,
		}

		if fn != nil {
			fn(i, &link)
		}

		if link.TeamID == "" {
			link.TeamID = prepSlackApps(t, db, 1, nil)[0].TeamID
		}

		if link.UserID == "" {
			link.UserID = prepUsers(t, db, 1)[0]
		}

		res[i] = link

		q, args := db.builder.Insert("slack_user_links").
			SetMap(map[string]any{
				"slack_user_id": link.SlackUserID,
				"fk_team_id":    link.TeamID,
				"fk_user_id":    link.UserID,
				"settings":      link.Settings,
				"created_at":    link.CreatedAt,
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	return res
}

func Test_agent_InsertSlackUserLink(t *testing.T) {
	type tcase struct {
		Link slackapp.UserLink
		Err  error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Non-existent team": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 1)

			return tcase{
				Link: slackapp.UserLink{
					SlackUserID: "slack-user-1",
					TeamID:      "non-existent-team-id",
					UserID:      users[0],
					CreatedAt:   timeutil.Now().Truncate(time.Second),
				},
				Err: assert.AnError,
			}
		},
		"Successful insert": func(t *testing.T, db *DB) tcase {
			app := prepSlackApps(t, db, 1, nil)[0]
			users := prepUsers(t, db, 1)

			return tcase{
				Link: slackapp.UserLink{
					SlackUserID: "slack-user-1",
					TeamID:      app.TeamID,
					UserID:      users[0],
					Settings: slackapp.UserLinkSettings{
						Notifications: true,
					},
					CreatedAt: timeutil.Now().Truncate(time.Second),
				},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.InsertSlackUserLink(context.Background(), c.Link)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var link slackapp.UserLink

			q, args := db.selectSlackUserLink(db.builder.Select()).
				Where(sq.Eq{
					"slack_user_id": c.Link.SlackUserID,
					"fk_team_id":    c.Link.TeamID,
				}).MustSql()

			err = db.sql.Get(&link, q, args...)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, c.Link, link)
		})
	}
}

func Test_agent_UpdateSlackUserLink(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Link             slackapp.UserLink
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			link := prepSlackUserLinks(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				Link:             link,
				Err:              assert.AnError,
			}
		},
		"Successful update": func(t *testing.T, db *DB) tcase {
			link := prepSlackUserLinks(t, db, 1, nil)[0]
			link.Settings.Notifications = false

			return tcase{
				Link: link,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.CancelledContext {
				cancel()
			}

			err := db.UpdateSlackUserLink(ctx, c.Link)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var link slackapp.UserLink

			q, args := db.selectSlackUserLink(db.builder.Select()).
				Where(sq.Eq{
					"slack_user_id": c.Link.SlackUserID,
					"fk_team_id":    c.Link.TeamID,
				}).MustSql()

			err = db.sql.Get(&link, q, args...)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, c.Link, link)
		})
	}
}

func Test_agent_FetchSlackUserLink(t *testing.T) {
	db := prepTempDB(t)

	// error - not found
	res, err := db.FetchSlackUserLink(context.Background(), "non-existent-slack-user-id", "non-existent-team-id")
	testutil.AssertEqualError(t, sql.ErrNoRows, err)
	assert.Nil(t, res)

	// success
	link := prepSlackUserLinks(t, db, 1, nil)[0]

	res, err = db.FetchSlackUserLink(context.Background(), link.SlackUserID, link.TeamID)
	assert.NoError(t, err)
	testutil.AssertFilterEqual(t, &link, res)
}

func Test_agent_FetchSlackUserLinkByUserID(t *testing.T) {
	db := prepTempDB(t)

	// error - not found
	res, err := db.FetchSlackUserLinkByUserID(context.Background(), "non-existent-user-id", "non-existent-org-id")
	testutil.AssertEqualError(t, sql.ErrNoRows, err)
	assert.Nil(t, res)

	// success
	org := prepOrganizations(t, db, 1)[0]
	app := prepSlackApps(t, db, 1, func(_ int, app *slackapp.App) {
		app.OrganizationID = null.StringFrom(org)
	})[0]
	link := prepSlackUserLinks(t, db, 1, func(_ int, link *slackapp.UserLink) {
		link.TeamID = app.TeamID
	})[0]

	res, err = db.FetchSlackUserLinkByUserID(context.Background(), link.UserID, org)
	assert.NoError(t, err)
	testutil.AssertFilterEqual(t, &link, res)
}

func Test_agent_DeleteSlackUserLink(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Link             slackapp.UserLink
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			link := prepSlackUserLinks(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				Link:             link,
				Err:              assert.AnError,
			}
		},
		"Successful delete": func(t *testing.T, db *DB) tcase {
			link := prepSlackUserLinks(t, db, 1, nil)[0]

			return tcase{
				Link: link,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.CancelledContext {
				cancel()
			}

			err := db.DeleteSlackUserLink(ctx, c.Link.SlackUserID, c.Link.TeamID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var slackUserID string

			q, args := db.builder.Select("slack_user_id").
				From("slack_user_links").
				Where(sq.Eq{
					"slack_user_id": c.Link.SlackUserID,
					"fk_team_id":    c.Link.TeamID,
				}).MustSql()

			err = db.sql.Get(&slackUserID, q, args...)
			testutil.AssertEqualError(t, sql.ErrNoRows, err)
		})
	}
}
