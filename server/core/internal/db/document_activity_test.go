package db

import (
	"context"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/guregu/null/v5"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/activity"
	"github.com/oxynote/oxynote/server/core/internal/document/comment"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepDocumentActivityEntries(t *testing.T, db *DB, count int, fn func(int, *activity.Entry)) []activity.Entry {
	t.Helper()

	res := make([]activity.Entry, count)

	now := timeutil.Now().Truncate(time.Second)

	// entries missing a branch after fn ran share one lazily created
	// branch, so a single call produces entries of the same branch.
	var branch *document.Document

	for i := range count {
		// distinct descending timestamps keep the fetch order the index
		// order.
		at := now.Add(-time.Duration(i) * time.Second)

		e := activity.Entry{
			Actor: activity.Actor{
				Kind: activity.ActorKindUser,
			},
			ID:       xid.NewWithTime(at),
			Type:     activity.TypeBranchRenamed,
			Category: activity.CategoryBranch,
			Metadata: activity.Metadata{
				"from": "old",
				"to":   "new",
			},
			CreatedAt: at,
			UpdatedAt: at,
		}

		if fn != nil {
			fn(i, &e)
		}

		if e.BranchID.IsZero() {
			if branch == nil {
				branch = prepDocumentBranches(t, db, 1, nil)[0]
			}

			e.BranchID = branch.BranchID
			e.DocumentID = branch.ID
			e.OrganizationID = branch.OrganizationID
		}

		res[i] = e

		require.NoError(t, db.InsertDocumentActivityEntry(context.Background(), e))
	}

	return res
}

// fetchStoredDocumentActivityEntries reads the branch's entry rows as
// stored, oldest first.
func fetchStoredDocumentActivityEntries(t *testing.T, db *DB, organizationID string, branchID xid.ID) []activity.Entry {
	t.Helper()

	q, args := db.selectDocumentActivityEntry(db.builder.Select(), organizationID).
		Where(sq.Eq{"fk_branch_id": branchID}).
		OrderBy("created_at ASC", "id ASC").
		MustSql()

	entries := []activity.Entry{}
	require.NoError(t, sqlx.SelectContext(context.Background(), db.sql, &entries, q, args...))

	return entries
}

// fetchFirstDocumentCommentRevision reads the oldest revision of the
// comment.
func fetchFirstDocumentCommentRevision(t *testing.T, db *DB, commentID xid.ID) comment.Revision {
	t.Helper()

	var rev comment.Revision
	require.NoError(t, db.sql.Get(
		&rev,
		"SELECT * FROM document_comment_revisions WHERE fk_document_comment_id = $1 ORDER BY created_at, id LIMIT 1",
		commentID,
	))

	return rev
}

// fetchFirstDocumentCommentReplyRevision reads the oldest revision of the
// reply.
func fetchFirstDocumentCommentReplyRevision(t *testing.T, db *DB, replyID xid.ID) comment.ReplyRevision {
	t.Helper()

	var rev comment.ReplyRevision
	require.NoError(t, db.sql.Get(
		&rev,
		"SELECT * FROM document_comment_reply_revisions WHERE fk_document_comment_reply_id = $1 ORDER BY created_at, id LIMIT 1",
		replyID,
	))

	return rev
}

func Test_agent_InsertDocumentActivityEntry(t *testing.T) {
	t.Parallel()

	db := prepTempDB(t)
	e := prepDocumentActivityEntries(t, db, 1, nil)[0]

	// error
	dup := e
	require.Error(t, db.InsertDocumentActivityEntry(context.Background(), dup))

	// success
	e.ID = xid.New()
	require.NoError(t, db.InsertDocumentActivityEntry(context.Background(), e))
	assert.Len(t, fetchStoredDocumentActivityEntries(t, db, e.OrganizationID, e.BranchID), 2)
}

func Test_agent_insertDocumentActivityEntry(t *testing.T) {
	type tcase struct {
		Entry activity.Entry
		Err   error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Non-existent branch": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				Entry: activity.Entry{
					Actor:          activity.Actor{Kind: activity.ActorKindUser},
					ID:             xid.New(),
					OrganizationID: "non-existent-org-id",
					DocumentID:     xid.New(),
					BranchID:       xid.New(),
					Type:           activity.TypeContentEdited,
					Category:       activity.CategoryContent,
					Metadata:       activity.Metadata{},
					CreatedAt:      timeutil.Now().Truncate(time.Microsecond),
					UpdatedAt:      timeutil.Now().Truncate(time.Microsecond),
				},
				Err: assert.AnError,
			}
		},
		"Successful insert": func(t *testing.T, db *DB) tcase {
			branch := prepDocumentBranches(t, db, 1, nil)[0]
			userID := prepUsers(t, db, 1)[0]
			at := timeutil.Now().Truncate(time.Microsecond)

			return tcase{
				Entry: activity.Entry{
					Actor: activity.Actor{
						Kind:     activity.ActorKindMCP,
						UserID:   null.StringFrom(userID),
						ClientID: null.StringFrom("client-1"),
					},
					Target:         activity.NewTarget(activity.TargetKindReplyRevision, xid.New().String()),
					ID:             xid.New(),
					OrganizationID: branch.OrganizationID,
					DocumentID:     branch.ID,
					BranchID:       branch.BranchID,
					Type:           activity.TypeCommentReplied,
					Category:       activity.CategoryComment,
					Metadata:       activity.Metadata{"count": float64(2)},
					CreatedAt:      at,
					UpdatedAt:      at,
				},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.insertDocumentActivityEntry(context.Background(), db.sql, c.Entry)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, []activity.Entry{c.Entry}, fetchStoredDocumentActivityEntries(t, db, c.Entry.OrganizationID, c.Entry.BranchID))
		})
	}
}

func Test_agent_RecordDocumentContentEdit(t *testing.T) {
	type tcase struct {
		Doc     document.Document
		Editors []activity.Actor
		Err     error
		// Check inspects the branch's entries, oldest first.
		Check func(t *testing.T, entries []activity.Entry)
	}

	// prep returns a branch and two users who can edit it.
	prep := func(t *testing.T, db *DB) (document.Document, activity.Actor, activity.Actor) {
		t.Helper()

		branch := prepDocumentBranches(t, db, 1, nil)[0]
		users := prepUsers(t, db, 2)

		return *branch,
			activity.Actor{Kind: activity.ActorKindUser, UserID: null.StringFrom(users[0])},
			activity.Actor{Kind: activity.ActorKindUser, UserID: null.StringFrom(users[1])}
	}

	// prepEdit stores a content edit entry of the editor, made the given
	// time ago.
	prepEdit := func(t *testing.T, db *DB, doc document.Document, ed activity.Actor, ago time.Duration) activity.Entry {
		t.Helper()

		return prepDocumentActivityEntries(t, db, 1, func(_ int, e *activity.Entry) {
			*e = activity.NewContentEdited(ed, doc, timeutil.Now().Add(-ago).Truncate(time.Microsecond))
		})[0]
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Non-existent branch": func(t *testing.T, db *DB) tcase {
			doc, alice, _ := prep(t, db)
			doc.BranchID = xid.New()

			return tcase{
				Doc:     doc,
				Editors: []activity.Actor{alice},
				Err:     assert.AnError,
			}
		},
		"No editors": func(t *testing.T, db *DB) tcase {
			doc, _, _ := prep(t, db)

			return tcase{
				Doc: doc,
				Check: func(t *testing.T, entries []activity.Entry) {
					assert.Empty(t, entries)
				},
			}
		},
		"First edit of an editor": func(t *testing.T, db *DB) tcase {
			doc, alice, _ := prep(t, db)

			return tcase{
				Doc:     doc,
				Editors: []activity.Actor{alice},
				Check: func(t *testing.T, entries []activity.Entry) {
					require.Len(t, entries, 1)
					assert.Equal(t, alice, entries[0].Actor)
					assert.Equal(t, activity.TypeContentEdited, entries[0].Type)
					assert.Equal(t, activity.CategoryContent, entries[0].Category)
					assert.Equal(t, doc.ID, entries[0].DocumentID)
					assert.WithinDuration(t, timeutil.Now(), entries[0].CreatedAt, time.Minute)
				},
			}
		},
		"Edit inside the window grows the open entry": func(t *testing.T, db *DB) tcase {
			doc, alice, _ := prep(t, db)
			open := prepEdit(t, db, doc, alice, 10*time.Minute)

			return tcase{
				Doc:     doc,
				Editors: []activity.Actor{alice},
				Check: func(t *testing.T, entries []activity.Entry) {
					require.Len(t, entries, 1)
					assert.Equal(t, open.ID, entries[0].ID)
					assert.Equal(t, open.CreatedAt, entries[0].CreatedAt)
					assert.WithinDuration(t, timeutil.Now(), entries[0].UpdatedAt, time.Minute)
				},
			}
		},
		"Edit past the window opens a new entry": func(t *testing.T, db *DB) tcase {
			doc, alice, _ := prep(t, db)
			old := prepEdit(t, db, doc, alice, 31*time.Minute)

			return tcase{
				Doc:     doc,
				Editors: []activity.Actor{alice},
				Check: func(t *testing.T, entries []activity.Entry) {
					require.Len(t, entries, 2)
					assert.Equal(t, old, entries[0])
					assert.Equal(t, alice, entries[1].Actor)
				},
			}
		},
		"Each editor gets their own entry": func(t *testing.T, db *DB) tcase {
			doc, alice, bob := prep(t, db)
			open := prepEdit(t, db, doc, alice, time.Minute)

			return tcase{
				Doc:     doc,
				Editors: []activity.Actor{bob},
				Check: func(t *testing.T, entries []activity.Entry) {
					require.Len(t, entries, 2)
					assert.Equal(t, open, entries[0])
					assert.Equal(t, bob, entries[1].Actor)
				},
			}
		},
		"Two editors in one persist": func(t *testing.T, db *DB) tcase {
			doc, alice, bob := prep(t, db)

			return tcase{
				Doc:     doc,
				Editors: []activity.Actor{alice, bob},
				Check: func(t *testing.T, entries []activity.Entry) {
					require.Len(t, entries, 2)
					assert.ElementsMatch(
						t,
						[]activity.Actor{alice, bob},
						[]activity.Actor{entries[0].Actor, entries[1].Actor},
					)
				},
			}
		},
		"A user and their MCP client get separate entries": func(t *testing.T, db *DB) tcase {
			doc, alice, _ := prep(t, db)
			open := prepEdit(t, db, doc, alice, time.Minute)

			mcp := activity.Actor{
				Kind:     activity.ActorKindMCP,
				UserID:   alice.UserID,
				ClientID: null.StringFrom("client-1"),
			}

			return tcase{
				Doc:     doc,
				Editors: []activity.Actor{mcp},
				Check: func(t *testing.T, entries []activity.Entry) {
					require.Len(t, entries, 2)
					assert.Equal(t, open, entries[0])
					assert.Equal(t, mcp, entries[1].Actor)
				},
			}
		},
		"Two MCP clients of one user get separate entries": func(t *testing.T, db *DB) tcase {
			doc, alice, _ := prep(t, db)

			desktop := activity.Actor{
				Kind:     activity.ActorKindMCP,
				UserID:   alice.UserID,
				ClientID: null.StringFrom("desktop"),
			}
			cursor := desktop
			cursor.ClientID = null.StringFrom("cursor")

			open := prepEdit(t, db, doc, desktop, time.Minute)

			return tcase{
				Doc:     doc,
				Editors: []activity.Actor{cursor},
				Check: func(t *testing.T, entries []activity.Entry) {
					require.Len(t, entries, 2)
					assert.Equal(t, open, entries[0])
					assert.Equal(t, cursor, entries[1].Actor)
				},
			}
		},
		"Another type of entry stays closed": func(t *testing.T, db *DB) tcase {
			doc, alice, _ := prep(t, db)
			other := prepDocumentActivityEntries(t, db, 1, func(_ int, e *activity.Entry) {
				e.Actor = alice
				e.BranchID = doc.BranchID
				e.DocumentID = doc.ID
				e.OrganizationID = doc.OrganizationID
			})[0]

			return tcase{
				Doc:     doc,
				Editors: []activity.Actor{alice},
				Check: func(t *testing.T, entries []activity.Entry) {
					require.Len(t, entries, 2)
					assert.Equal(t, other, entries[0])
					assert.Equal(t, activity.TypeContentEdited, entries[1].Type)
				},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.RecordDocumentContentEdit(context.Background(), c.Doc, c.Editors)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			c.Check(t, fetchStoredDocumentActivityEntries(t, db, c.Doc.OrganizationID, c.Doc.BranchID))
		})
	}
}

func Test_agent_FetchDocumentActivityEntries(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		BranchID         xid.ID
		OrganizationID   string
		Query            activity.Query
		Result           []activity.Entry
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Error returned by the entry read": func(t *testing.T, db *DB) tcase {
			e := prepDocumentActivityEntries(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				BranchID:         e.BranchID,
				OrganizationID:   e.OrganizationID,
				Query:            activity.Query{Limit: 10},
				Err:              assert.AnError,
			}
		},
		"No entries": func(t *testing.T, db *DB) tcase {
			branch := prepDocumentBranches(t, db, 1, nil)[0]

			return tcase{
				BranchID:       branch.BranchID,
				OrganizationID: branch.OrganizationID,
				Query:          activity.Query{Limit: 10},
				Result:         []activity.Entry{},
			}
		},
		"Newest entries up to the limit": func(t *testing.T, db *DB) tcase {
			entries := prepDocumentActivityEntries(t, db, 3, nil)

			return tcase{
				BranchID:       entries[0].BranchID,
				OrganizationID: entries[0].OrganizationID,
				Query:          activity.Query{Limit: 2},
				Result:         entries[:2],
			}
		},
		"Entries after the cursor": func(t *testing.T, db *DB) tcase {
			entries := prepDocumentActivityEntries(t, db, 3, nil)

			return tcase{
				BranchID:       entries[0].BranchID,
				OrganizationID: entries[0].OrganizationID,
				Query:          activity.Query{Before: entries[0].ID, Limit: 10},
				Result:         entries[1:],
			}
		},
		"Entries with the same time are ordered by id": func(t *testing.T, db *DB) tcase {
			at := timeutil.Now().Truncate(time.Microsecond)
			entries := prepDocumentActivityEntries(t, db, 3, func(_ int, e *activity.Entry) {
				e.CreatedAt = at
				e.UpdatedAt = at
			})

			// the ids still carry descending times, so they order the
			// entries as before.
			return tcase{
				BranchID:       entries[0].BranchID,
				OrganizationID: entries[0].OrganizationID,
				Query:          activity.Query{Before: entries[0].ID, Limit: 10},
				Result:         entries[1:],
			}
		},
		"Cursor from another branch": func(t *testing.T, db *DB) tcase {
			entries := prepDocumentActivityEntries(t, db, 2, nil)
			other := prepDocumentActivityEntries(t, db, 1, nil)[0]

			return tcase{
				BranchID:       entries[0].BranchID,
				OrganizationID: entries[0].OrganizationID,
				Query:          activity.Query{Before: other.ID, Limit: 10},
				Result:         []activity.Entry{},
			}
		},
		"Entries of the given categories": func(t *testing.T, db *DB) tcase {
			entries := prepDocumentActivityEntries(t, db, 3, func(i int, e *activity.Entry) {
				tp := []activity.Type{
					activity.TypeTagAssigned,
					activity.TypeBranchRenamed,
					activity.TypeReviewApproved,
				}[i]

				e.Type = tp
				e.Category = tp.Category()
			})

			return tcase{
				BranchID:       entries[0].BranchID,
				OrganizationID: entries[0].OrganizationID,
				Query: activity.Query{
					Categories: []activity.Category{activity.CategoryTag, activity.CategoryReview},
					Limit:      10,
				},
				Result: []activity.Entry{entries[0], entries[2]},
			}
		},
		"Entries of another branch are left out": func(t *testing.T, db *DB) tcase {
			entries := prepDocumentActivityEntries(t, db, 1, nil)
			prepDocumentActivityEntries(t, db, 1, nil)

			return tcase{
				BranchID:       entries[0].BranchID,
				OrganizationID: entries[0].OrganizationID,
				Query:          activity.Query{Limit: 10},
				Result:         entries,
			}
		},
		"Another organization": func(t *testing.T, db *DB) tcase {
			e := prepDocumentActivityEntries(t, db, 1, nil)[0]

			return tcase{
				BranchID:       e.BranchID,
				OrganizationID: prepOrganizations(t, db, 1)[0],
				Query:          activity.Query{Limit: 10},
				Result:         []activity.Entry{},
			}
		},
		"Comment entries carry the text they point at": func(t *testing.T, db *DB) tcase {
			c := prepDocumentComments(t, db, 1, nil)[0]
			r := prepDocumentCommentReplies(t, db, 1, func(_ int, r *comment.Reply) {
				r.CommentID = c.ID
				r.OrganizationID = c.OrganizationID
			})[0]

			rev := fetchFirstDocumentCommentRevision(t, db, c.ID)
			replyRev := fetchFirstDocumentCommentReplyRevision(t, db, r.ID)

			entries := prepDocumentActivityEntries(t, db, 2, func(i int, e *activity.Entry) {
				e.BranchID = c.BranchID
				e.DocumentID = c.DocumentID
				e.OrganizationID = c.OrganizationID
				e.Type = activity.TypeCommentReplied
				e.Category = activity.CategoryComment
				e.Target = activity.NewTarget(activity.TargetKindReplyRevision, replyRev.ID.String())

				if i == 1 {
					e.Type = activity.TypeCommentCreated
					e.Target = activity.NewTarget(activity.TargetKindCommentRevision, rev.ID.String())
				}
			})

			// an edit and a delete leave the revisions as they were.
			edited := c
			edited.Content = comment.Content{"text": "edited"}
			edited.UpdatedAt = null.TimeFrom(c.CreatedAt.Add(time.Minute))
			require.NoError(t, db.UpdateDocumentComment(context.Background(), edited))
			require.NoError(t, db.DeleteDocumentComment(context.Background(), c.ID, c.DocumentID, c.OrganizationID))

			entries[0].Subject = replyRev
			entries[1].Subject = rev

			return tcase{
				BranchID:       c.BranchID,
				OrganizationID: c.OrganizationID,
				Query:          activity.Query{Limit: 10},
				Result:         entries,
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

			res, err := db.FetchDocumentActivityEntries(ctx, c.BranchID, c.OrganizationID, c.Query)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_agent_attachDocumentActivitySubjects(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Entries          []activity.Entry
		OrganizationID   string
		Result           []activity.Entry
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Error returned by the subject read": func(t *testing.T, db *DB) tcase {
			c := prepDocumentComments(t, db, 1, nil)[0]
			rev := fetchFirstDocumentCommentRevision(t, db, c.ID)

			return tcase{
				CancelledContext: true,
				Entries:          []activity.Entry{{Target: activity.NewTarget(activity.TargetKindCommentRevision, rev.ID.String())}},
				OrganizationID:   c.OrganizationID,
				Err:              assert.AnError,
			}
		},
		"Entries without a subject": func(t *testing.T, db *DB) tcase {
			entries := []activity.Entry{
				{Type: activity.TypeDocumentCreated},
				{Target: activity.NewTarget(activity.TargetKindCommentRevision, xid.New().String())},
			}

			return tcase{
				Entries:        entries,
				OrganizationID: prepOrganizations(t, db, 1)[0],
				Result:         entries,
			}
		},
		"Successful attach": func(t *testing.T, db *DB) tcase {
			r := prepDocumentCommentReplies(t, db, 1, nil)[0]
			rev := fetchFirstDocumentCommentRevision(t, db, r.CommentID)
			replyRev := fetchFirstDocumentCommentReplyRevision(t, db, r.ID)

			res := []activity.Entry{
				{Target: activity.NewTarget(activity.TargetKindCommentRevision, rev.ID.String())},
				{Target: activity.NewTarget(activity.TargetKindReplyRevision, replyRev.ID.String())},
			}
			res[0].Subject = rev
			res[1].Subject = replyRev

			return tcase{
				Entries: []activity.Entry{
					{Target: activity.NewTarget(activity.TargetKindCommentRevision, rev.ID.String())},
					{Target: activity.NewTarget(activity.TargetKindReplyRevision, replyRev.ID.String())},
				},
				OrganizationID: r.OrganizationID,
				Result:         res,
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

			err := db.attachDocumentActivitySubjects(ctx, c.Entries, c.OrganizationID)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, c.Entries)
		})
	}
}

func Test_agent_fetchDocumentActivitySubjects(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Kind             activity.TargetKind
		IDs              []string
		OrganizationID   string
		Result           map[activity.Target]any
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Error returned by the comment revision read": func(t *testing.T, db *DB) tcase {
			rev := fetchFirstDocumentCommentRevision(t, db, prepDocumentComments(t, db, 1, nil)[0].ID)

			return tcase{
				CancelledContext: true,
				Kind:             activity.TargetKindCommentRevision,
				IDs:              []string{rev.ID.String()},
				OrganizationID:   rev.OrganizationID,
				Err:              assert.AnError,
			}
		},
		"Error returned by the reply revision read": func(t *testing.T, db *DB) tcase {
			rev := fetchFirstDocumentCommentReplyRevision(t, db, prepDocumentCommentReplies(t, db, 1, nil)[0].ID)

			return tcase{
				CancelledContext: true,
				Kind:             activity.TargetKindReplyRevision,
				IDs:              []string{rev.ID.String()},
				OrganizationID:   rev.OrganizationID,
				Err:              assert.AnError,
			}
		},
		"Kind that loads nothing": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				CancelledContext: true,
				Kind:             "hook",
				IDs:              []string{xid.New().String()},
				OrganizationID:   "org-id",
				Result:           map[activity.Target]any{},
			}
		},
		"Comment revisions": func(t *testing.T, db *DB) tcase {
			rev := fetchFirstDocumentCommentRevision(t, db, prepDocumentComments(t, db, 1, nil)[0].ID)

			return tcase{
				Kind:           activity.TargetKindCommentRevision,
				IDs:            []string{rev.ID.String(), xid.New().String()},
				OrganizationID: rev.OrganizationID,
				Result: map[activity.Target]any{
					activity.NewTarget(activity.TargetKindCommentRevision, rev.ID.String()): rev,
				},
			}
		},
		"Reply revisions": func(t *testing.T, db *DB) tcase {
			rev := fetchFirstDocumentCommentReplyRevision(t, db, prepDocumentCommentReplies(t, db, 1, nil)[0].ID)

			return tcase{
				Kind:           activity.TargetKindReplyRevision,
				IDs:            []string{rev.ID.String()},
				OrganizationID: rev.OrganizationID,
				Result: map[activity.Target]any{
					activity.NewTarget(activity.TargetKindReplyRevision, rev.ID.String()): rev,
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

			res, err := db.fetchDocumentActivitySubjects(ctx, c.Kind, c.IDs, c.OrganizationID)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}
