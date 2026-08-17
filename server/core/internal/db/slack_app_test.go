package db

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/slack"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepSlackApps(t *testing.T, db *DB, count int, fn func(int, *slack.App)) []slack.App { //nolint:unparam // fixture helpers share a uniform signature
	t.Helper()

	res := make([]slack.App, count)

	for i := range count {
		app := slack.App{
			// the team ID embeds a random ID so repeated fixture
			// calls within one database never collide.
			TeamID: "team-" + xid.New().String(),
			Token:  "token-" + strconv.Itoa(i),
		}

		if fn != nil {
			fn(i, &app)
		}

		res[i] = app

		q, args := db.builder.Insert("slack_apps").
			SetMap(map[string]any{
				"team_id":            app.TeamID,
				"fk_organization_id": app.OrganizationID,
				"token":              app.Token,
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	return res
}

func prepSlackMessages(t *testing.T, db *DB, count int, fn func(int, *slack.Message)) []slack.Message {
	t.Helper()

	res := make([]slack.Message, count)

	now := timeutil.Now().Truncate(time.Second)

	for i := range count {
		msg := slack.Message{
			ID:   xid.New(),
			Text: "Message " + strconv.Itoa(i),
			// distinct timestamps keep the fetch order deterministic.
			CreatedAt: now.Add(-time.Duration(i) * time.Second),
		}

		if fn != nil {
			fn(i, &msg)
		}

		if msg.OrganizationID == "" {
			msg.OrganizationID = prepOrganizations(t, db, 1)[0]
		}

		res[i] = msg

		q, args := db.builder.Insert("slack_messages").
			SetMap(map[string]any{
				"id":                 msg.ID,
				"fk_organization_id": msg.OrganizationID,
				"text":               msg.Text,
				"created_at":         msg.CreatedAt,
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	return res
}

func Test_agent_InsertSlackApp(t *testing.T) {
	type tcase struct {
		App slack.App
		Err error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Non-existent organization": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				App: slack.App{
					TeamID:         "team-1",
					OrganizationID: null.StringFrom("non-existent-org-id"),
					Token:          "token-1",
				},
				Err: assert.AnError,
			}
		},
		"Successful insert": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]

			return tcase{
				App: slack.App{
					TeamID:         "team-1",
					OrganizationID: null.StringFrom(org),
					Token:          "token-1",
				},
			}
		},
		"Successful upsert of an existing team": func(t *testing.T, db *DB) tcase {
			app := prepSlackApps(t, db, 1, nil)[0]
			app.Token = "rotated-token"

			return tcase{
				App: app,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.InsertSlackApp(context.Background(), c.App)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var app slack.App

			q, args := db.selectSlackApp(db.builder.Select()).
				Where(sq.Eq{
					"team_id": c.App.TeamID,
				}).MustSql()

			err = db.sql.Get(&app, q, args...)
			require.NoError(t, err)
			assert.Equal(t, c.App, app)
		})
	}

	t.Run("Reinstall keeps the existing organization", func(t *testing.T) {
		t.Parallel()

		db := prepTempDB(t)
		org := prepOrganizations(t, db, 1)[0]

		app := slack.App{
			TeamID:         "team-reinstall",
			Token:          "token-1",
			OrganizationID: null.StringFrom(org),
		}

		require.NoError(t, db.InsertSlackApp(context.Background(), app))

		// a reinstall carries no organization; it must refresh the token
		// without disconnecting the workspace.
		require.NoError(t, db.InsertSlackApp(context.Background(), slack.App{
			TeamID: app.TeamID,
			Token:  "token-2",
		}))

		res, err := db.FetchSlackAppByTeamID(context.Background(), app.TeamID)
		require.NoError(t, err)
		assert.Equal(t, "token-2", res.Token)
		assert.Equal(t, org, res.OrganizationID.String)
	})
}

func Test_agent_InsertSlackMessage(t *testing.T) {
	type tcase struct {
		Message slack.Message
		Err     error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Duplicate message ID": func(t *testing.T, db *DB) tcase {
			msg := prepSlackMessages(t, db, 1, nil)[0]

			return tcase{
				Message: msg,
				Err:     assert.AnError,
			}
		},
		"Successful insert": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]

			return tcase{
				Message: slack.Message{
					ID:             xid.New(),
					OrganizationID: org,
					Text:           "Hello from Slack",
					CreatedAt:      timeutil.Now().Truncate(time.Second),
				},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.InsertSlackMessage(context.Background(), c.Message)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var msg slack.Message

			q, args := db.selectSlackMessage(db.builder.Select()).
				Where(sq.Eq{
					"id": c.Message.ID,
				}).MustSql()

			err = db.sql.Get(&msg, q, args...)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, c.Message, msg)
		})
	}
}

func Test_agent_FetchSlackMessages(t *testing.T) {
	db := prepTempDB(t)

	// error - cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := db.FetchSlackMessages(ctx, "non-existent-org-id")
	require.Error(t, err)
	assert.Nil(t, res)

	// success - no messages
	res, err = db.FetchSlackMessages(context.Background(), "non-existent-org-id")
	require.NoError(t, err)
	assert.Empty(t, res)

	// success - ordered by creation time, newest first
	org := prepOrganizations(t, db, 1)[0]
	messages := prepSlackMessages(t, db, 3, func(_ int, msg *slack.Message) {
		msg.OrganizationID = org
	})

	res, err = db.FetchSlackMessages(context.Background(), org)
	assert.NoError(t, err)
	testutil.AssertFilterEqual(t, messages, res)
}

func Test_agent_UpdateSlackAppOrganizationID(t *testing.T) {
	type tcase struct {
		TeamID         string
		OrganizationID string
		Err            error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Non-existent organization": func(t *testing.T, db *DB) tcase {
			app := prepSlackApps(t, db, 1, nil)[0]

			return tcase{
				TeamID:         app.TeamID,
				OrganizationID: "non-existent-org-id",
				Err:            assert.AnError,
			}
		},
		"Successful update": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			app := prepSlackApps(t, db, 1, nil)[0]

			return tcase{
				TeamID:         app.TeamID,
				OrganizationID: org,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.UpdateSlackAppOrganizationID(context.Background(), c.TeamID, c.OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var organizationID null.String

			q, args := db.builder.Select("fk_organization_id").
				From("slack_apps").
				Where(sq.Eq{
					"team_id": c.TeamID,
				}).MustSql()

			err = db.sql.Get(&organizationID, q, args...)
			require.NoError(t, err)
			assert.Equal(t, null.StringFrom(c.OrganizationID), organizationID)
		})
	}
}

func Test_agent_FetchSlackAppByTeamID(t *testing.T) {
	db := prepTempDB(t)

	// error - not found
	res, err := db.FetchSlackAppByTeamID(context.Background(), "non-existent-team-id")
	testutil.AssertEqualError(t, sql.ErrNoRows, err)
	assert.Nil(t, res)

	// success
	app := prepSlackApps(t, db, 1, nil)[0]

	res, err = db.FetchSlackAppByTeamID(context.Background(), app.TeamID)
	assert.NoError(t, err)
	assert.Equal(t, &app, res)
}

func Test_agent_FetchSlackAppByOrganizationID(t *testing.T) {
	db := prepTempDB(t)

	// error - not found
	res, err := db.FetchSlackAppByOrganizationID(context.Background(), "non-existent-org-id")
	testutil.AssertEqualError(t, sql.ErrNoRows, err)
	assert.Nil(t, res)

	// success
	org := prepOrganizations(t, db, 1)[0]
	app := prepSlackApps(t, db, 1, func(_ int, app *slack.App) {
		app.OrganizationID = null.StringFrom(org)
	})[0]

	res, err = db.FetchSlackAppByOrganizationID(context.Background(), org)
	assert.NoError(t, err)
	assert.Equal(t, &app, res)
}

func Test_agent_DeleteSlackApp(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		TeamID           string
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			app := prepSlackApps(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				TeamID:           app.TeamID,
				Err:              assert.AnError,
			}
		},
		"Successful delete": func(t *testing.T, db *DB) tcase {
			app := prepSlackApps(t, db, 1, nil)[0]

			return tcase{
				TeamID: app.TeamID,
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

			err := db.DeleteSlackApp(ctx, c.TeamID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var teamID string

			q, args := db.builder.Select("team_id").
				From("slack_apps").
				Where(sq.Eq{
					"team_id": c.TeamID,
				}).MustSql()

			err = db.sql.Get(&teamID, q, args...)
			testutil.AssertEqualError(t, sql.ErrNoRows, err)
		})
	}
}

func Test_agent_UnassignSlackAppOrganization(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		TeamID           string
		OrganizationID   string
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			app := prepSlackApps(t, db, 1, func(_ int, app *slack.App) {
				app.OrganizationID = null.StringFrom(org)
			})[0]

			return tcase{
				CancelledContext: true,
				TeamID:           app.TeamID,
				OrganizationID:   org,
				Err:              assert.AnError,
			}
		},
		"Successful unassign": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			app := prepSlackApps(t, db, 1, func(_ int, app *slack.App) {
				app.OrganizationID = null.StringFrom(org)
			})[0]

			return tcase{
				TeamID:         app.TeamID,
				OrganizationID: org,
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

			err := db.UnassignSlackAppOrganization(ctx, c.OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var organizationID null.String

			q, args := db.builder.Select("fk_organization_id").
				From("slack_apps").
				Where(sq.Eq{
					"team_id": c.TeamID,
				}).MustSql()

			err = db.sql.Get(&organizationID, q, args...)
			require.NoError(t, err)
			assert.False(t, organizationID.Valid)
		})
	}
}

func Test_agent_DeleteSlackAppsByOrganizationID(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		KeptTeamID       string
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			prepSlackApps(t, db, 1, func(_ int, app *slack.App) {
				app.OrganizationID = null.StringFrom(org)
			})

			return tcase{
				CancelledContext: true,
				OrganizationID:   org,
				Err:              assert.AnError,
			}
		},
		"Only the organization's apps go": func(t *testing.T, db *DB) tcase {
			orgs := prepOrganizations(t, db, 2)

			prepSlackApps(t, db, 1, func(_ int, app *slack.App) {
				app.OrganizationID = null.StringFrom(orgs[0])
			})

			kept := prepSlackApps(t, db, 1, func(_ int, app *slack.App) {
				app.OrganizationID = null.StringFrom(orgs[1])
			})[0]

			return tcase{
				OrganizationID: orgs[0],
				KeptTeamID:     kept.TeamID,
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

			err := db.DeleteSlackAppsByOrganizationID(ctx, c.OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var teamIDs []string

			q, args := db.builder.Select("team_id").From("slack_apps").MustSql()

			require.NoError(t, db.sql.Select(&teamIDs, q, args...))
			assert.Equal(t, []string{c.KeptTeamID}, teamIDs)
		})
	}
}
