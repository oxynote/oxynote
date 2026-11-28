package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/comment"
	"github.com/oxynote/oxynote/server/core/internal/document/history"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/internal/user"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_agent_UpsertUserSpotlights(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		UserID           string
		Input            user.SpotlightsInput
		Check            func(t *testing.T, db *DB)
		Result           user.Spotlights
		Err              error
	}

	// prepSpotlights stores the given state for the user before the case
	// runs.
	prepSpotlights := func(t *testing.T, db *DB, userID string, state user.SpotlightState) {
		t.Helper()

		require.NoError(t, db.UpsertUserSpotlights(
			context.Background(),
			userID,
			user.SpotlightsInput{Star: state},
		))
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			return tcase{
				CancelledContext: true,
				UserID:           prepUsers(t, db, 1)[0],
				Input:            user.SpotlightsInput{Star: user.SpotlightStateShown},
				Err:              assert.AnError,
			}
		},
		"Unknown user": func(*testing.T, *DB) tcase {
			return tcase{
				UserID: "unknown",
				Input:  user.SpotlightsInput{Star: user.SpotlightStateShown},
				Err:    assert.AnError,
			}
		},
		"Successful insertion": func(t *testing.T, db *DB) tcase {
			return tcase{
				UserID: prepUsers(t, db, 1)[0],
				Input:  user.SpotlightsInput{Star: user.SpotlightStateShown},
				Result: user.Spotlights{Star: null.ValueFrom(user.SpotlightStateShown)},
			}
		},
		"Dismissed replaces shown": func(t *testing.T, db *DB) tcase {
			userID := prepUsers(t, db, 1)[0]
			prepSpotlights(t, db, userID, user.SpotlightStateShown)

			return tcase{
				UserID: userID,
				Input:  user.SpotlightsInput{Star: user.SpotlightStateDismissed},
				Result: user.Spotlights{Star: null.ValueFrom(user.SpotlightStateDismissed)},
			}
		},
		"Shown does not replace dismissed": func(t *testing.T, db *DB) tcase {
			userID := prepUsers(t, db, 1)[0]
			prepSpotlights(t, db, userID, user.SpotlightStateDismissed)

			return tcase{
				UserID: userID,
				Input:  user.SpotlightsInput{Star: user.SpotlightStateShown},
				Result: user.Spotlights{Star: null.ValueFrom(user.SpotlightStateDismissed)},
			}
		},
		"Other users keep their state": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 2)
			prepSpotlights(t, db, users[1], user.SpotlightStateShown)

			return tcase{
				UserID: users[0],
				Input:  user.SpotlightsInput{Star: user.SpotlightStateDismissed},
				Result: user.Spotlights{Star: null.ValueFrom(user.SpotlightStateDismissed)},
				Check: func(t *testing.T, db *DB) {
					spotlights, err := db.FetchUserSpotlights(context.Background(), users[1])
					require.NoError(t, err)
					assert.Equal(t, user.Spotlights{Star: null.ValueFrom(user.SpotlightStateShown)}, spotlights)
				},
			}
		},
		"State goes with the user": func(t *testing.T, db *DB) tcase {
			userID := prepUsers(t, db, 1)[0]

			return tcase{
				UserID: userID,
				Input:  user.SpotlightsInput{Star: user.SpotlightStateShown},
				Result: user.Spotlights{Star: null.ValueFrom(user.SpotlightStateShown)},
				Check: func(t *testing.T, db *DB) {
					q, args := db.builder.Delete("users").Where(sq.Eq{"id": userID}).MustSql()

					_, err := db.sql.Exec(q, args...)
					require.NoError(t, err)

					var count int

					q, args = db.builder.Select("COUNT(*)").
						From("user_spotlights").
						Where(sq.Eq{"fk_user_id": userID}).
						MustSql()

					require.NoError(t, db.sql.Get(&count, q, args...))
					assert.Zero(t, count)
				},
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

			err := db.UpsertUserSpotlights(ctx, c.UserID, c.Input)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			spotlights, err := db.FetchUserSpotlights(context.Background(), c.UserID)
			require.NoError(t, err)
			assert.Equal(t, c.Result, spotlights)

			if c.Check != nil {
				c.Check(t, db)
			}
		})
	}
}

func Test_agent_FetchUserSpotlights(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		UserID           string
		Result           user.Spotlights
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			return tcase{
				CancelledContext: true,
				UserID:           prepUsers(t, db, 1)[0],
				Err:              assert.AnError,
			}
		},
		"Unknown user": func(*testing.T, *DB) tcase {
			return tcase{
				UserID: "unknown",
				Err:    sql.ErrNoRows,
			}
		},
		"User without a stored state": func(t *testing.T, db *DB) tcase {
			return tcase{
				UserID: prepUsers(t, db, 1)[0],
			}
		},
		"User with a stored state": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 2)

			for i, state := range []user.SpotlightState{user.SpotlightStateDismissed, user.SpotlightStateShown} {
				require.NoError(t, db.UpsertUserSpotlights(
					context.Background(),
					users[i],
					user.SpotlightsInput{Star: state},
				))
			}

			return tcase{
				UserID: users[0],
				Result: user.Spotlights{Star: null.ValueFrom(user.SpotlightStateDismissed)},
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

			spotlights, err := db.FetchUserSpotlights(ctx, c.UserID)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, spotlights)
		})
	}
}

func Test_agent_FetchUserSpotlightActivity(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		UserID           string
		Result           user.SpotlightActivity
		Err              error
	}

	now := timeutil.Now().Truncate(time.Second)
	createdAt := now.Add(-30 * 24 * time.Hour)

	// prepAccount creates a user whose account dates from createdAt.
	prepAccount := func(t *testing.T, db *DB) string {
		t.Helper()

		userID := prepUsers(t, db, 1)[0]

		q, args := db.builder.Update("users").
			Set("created_at", createdAt).
			Where(sq.Eq{"id": userID}).
			MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)

		return userID
	}

	// prepInvitation stores an invitation the user sent.
	prepInvitation := func(t *testing.T, db *DB, organizationID, inviterID, status string) {
		t.Helper()

		id := xid.New().String()

		q, args := db.builder.Insert("organization_invitations").
			SetMap(map[string]any{
				"id":                 id,
				"fk_organization_id": organizationID,
				"email":              id + "@test.test",
				"status":             status,
				"expires_at":         now,
				"created_at":         now,
				"fk_inviter_id":      inviterID,
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	// prepConsent stores the user's consent to an MCP client of its own.
	prepConsent := func(t *testing.T, db *DB, userID string) {
		t.Helper()

		id := xid.New().String()

		q, args := db.builder.Insert("oauth_clients").
			SetMap(map[string]any{
				"id":            id,
				"client_id":     id,
				"redirect_uris": "http://test.test",
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)

		q, args = db.builder.Insert("oauth_consents").
			SetMap(map[string]any{
				"id":         id,
				"client_id":  id,
				"fk_user_id": userID,
				"scopes":     "documents:read",
			}).MustSql()

		_, err = db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			return tcase{
				CancelledContext: true,
				UserID:           prepUsers(t, db, 1)[0],
				Err:              assert.AnError,
			}
		},
		"Unknown user": func(*testing.T, *DB) tcase {
			return tcase{
				UserID: "unknown",
				Err:    sql.ErrNoRows,
			}
		},
		"User without activity": func(t *testing.T, db *DB) tcase {
			return tcase{
				UserID: prepAccount(t, db),
				Result: user.SpotlightActivity{
					AccountCreatedAt: createdAt,
				},
			}
		},
		"User with activity next to another user's": func(t *testing.T, db *DB) tcase {
			userID := prepAccount(t, db)
			otherID := prepUsers(t, db, 1)[0]
			organizationID := prepOrganizations(t, db, 1)[0]

			// the user created the first two documents, the other user
			// the third.
			docs := prepDocuments(t, db, 3, func(i int, d *document.Document) {
				d.OrganizationID = organizationID
				d.CreatedBy = null.StringFrom(userID)

				if i == 2 {
					d.CreatedBy = null.StringFrom(otherID)
				}
			})

			// two edits of one document count once.
			for i, e := range []struct {
				Doc *document.Document
				By  string
			}{
				{Doc: docs[0], By: userID},
				{Doc: docs[0], By: userID},
				{Doc: docs[2], By: userID},
				{Doc: docs[1], By: otherID},
			} {
				entry := history.NewEntry(
					*e.Doc,
					now.Add(-time.Duration(i+1)*time.Hour),
					null.StringFrom(e.By),
					history.Hooks{},
					false,
				)

				insertHistoryEntry(t, db, entry)
			}

			prepInvitation(t, db, organizationID, userID, "accepted")
			prepInvitation(t, db, organizationID, userID, "accepted")
			prepInvitation(t, db, organizationID, userID, "pending")
			prepInvitation(t, db, organizationID, otherID, "accepted")

			// an approval the user gave, one a branch of theirs got, and
			// two reviews without an approval.
			prepBranchReviewers(t, db, 4, func(i int, r *document.BranchReviewer) {
				r.OrganizationID = organizationID

				switch i {
				case 0:
					r.BranchID = docs[2].BranchID
					r.UserID = userID
					r.CurrentlyApproved = true
				case 1:
					r.BranchID = docs[0].BranchID
					r.UserID = otherID
					r.PreviouslyApproved = true
				case 2:
					r.BranchID = docs[1].BranchID
					r.UserID = otherID
				case 3:
					r.BranchID = docs[1].BranchID
					r.UserID = userID
				}
			})

			comments := prepDocumentComments(t, db, 3, func(i int, c *comment.Comment) {
				c.BranchID = docs[0].BranchID
				c.DocumentID = docs[0].ID
				c.OrganizationID = organizationID
				c.UserID = null.StringFrom(userID)

				if i == 2 {
					c.UserID = null.StringFrom(otherID)
				}
			})

			prepDocumentCommentReplies(t, db, 2, func(i int, r *comment.Reply) {
				r.CommentID = comments[0].ID
				r.OrganizationID = organizationID
				r.UserID = null.StringFrom(userID)

				if i == 1 {
					r.UserID = null.StringFrom(otherID)
				}
			})

			prepConsent(t, db, userID)
			prepConsent(t, db, otherID)

			for _, v := range []struct {
				UserID string
				Doc    *document.Document
				At     time.Time
			}{
				{UserID: userID, Doc: docs[0], At: now.Add(-2 * time.Hour)},
				{UserID: userID, Doc: docs[1], At: now.Add(-time.Hour)},
				{UserID: otherID, Doc: docs[1], At: now},
			} {
				require.NoError(t, db.UpsertDocumentBranchView(
					context.Background(),
					v.UserID,
					organizationID,
					v.Doc.BranchID,
					search.ViewFromAll,
					v.At,
				))
			}

			return tcase{
				UserID: userID,
				Result: user.SpotlightActivity{
					AccountCreatedAt:    createdAt,
					LastViewedAt:        null.TimeFrom(now.Add(-time.Hour)),
					CreatedDocuments:    2,
					EditedDocuments:     2,
					AcceptedInvitations: 2,
					Approvals:           2,
					Comments:            3,
					MCPClients:          1,
				},
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

			activity, err := db.FetchUserSpotlightActivity(ctx, c.UserID)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			// the account time comes back in the session's time zone.
			activity.AccountCreatedAt = activity.AccountCreatedAt.UTC()

			assert.Equal(t, c.Result, activity)
		})
	}
}
