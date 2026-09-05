package db

import (
	"context"
	"strconv"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/guregu/null/v5"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepTags(t *testing.T, db *DB, count int, fn func(int, *tag.Tag)) []tag.Tag {
	t.Helper()

	res := make([]tag.Tag, count)

	now := timeutil.Now().Truncate(time.Second)

	// tags missing an organization after fn ran share one lazily created
	// organization, so a single call produces sibling tags.
	var organizationID string

	for i := range count {
		tg := tag.Tag{
			ID:        xid.New(),
			TagName:   "Tag " + strconv.Itoa(i),
			Color:     "#22c55e",
			SortIndex: i,
			CreatedAt: now,
		}

		if fn != nil {
			fn(i, &tg)
		}

		if tg.OrganizationID == "" {
			if organizationID == "" {
				organizationID = prepOrganizations(t, db, 1)[0]
			}

			tg.OrganizationID = organizationID
		}

		res[i] = tg

		q, args := db.builder.Insert("tags").
			SetMap(map[string]any{
				"id":                 tg.ID,
				"fk_organization_id": tg.OrganizationID,
				"tag_name":           tg.TagName,
				"color":              tg.Color,
				"sort_index":         tg.SortIndex,
				"created_at":         tg.CreatedAt,
				"fk_created_by":      tg.CreatedBy,
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	return res
}

func prepBranchTags(t *testing.T, db *DB, organizationID string, branchID xid.ID, tagIDs ...xid.ID) {
	t.Helper()

	for _, tagID := range tagIDs {
		q, args := db.builder.Insert("document_branch_tags").
			SetMap(map[string]any{
				"fk_branch_id":       branchID,
				"fk_tag_id":          tagID,
				"fk_organization_id": organizationID,
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}
}

// prepTaggedDocument creates a document in the tag's organization and puts
// its default branch under the tag.
func prepTaggedDocument(t *testing.T, db *DB, tg tag.Tag) *document.Document {
	t.Helper()

	doc := prepDocuments(t, db, 1, func(_ int, doc *document.Document) {
		doc.OrganizationID = tg.OrganizationID
	})[0]

	prepBranchTags(t, db, tg.OrganizationID, doc.BranchID, tg.ID)

	return doc
}

// branchTagIDs reads the tags a branch carries in the tags' display order.
func branchTagIDs(t *testing.T, db *DB, branchID xid.ID) []xid.ID {
	t.Helper()

	q, args := db.builder.Select("fk_tag_id").
		From("document_branch_tags").
		Join("tags ON tags.id = document_branch_tags.fk_tag_id").
		Where(sq.Eq{"fk_branch_id": branchID}).
		OrderBy("tags.sort_index", "tags.id").
		MustSql()

	ids := []xid.ID{}
	require.NoError(t, sqlx.Select(db.sql, &ids, q, args...))

	return ids
}

// tagOrder reads an organization's tag ids in their display order.
func tagOrder(t *testing.T, db *DB, organizationID string) []xid.ID {
	t.Helper()

	q, args := db.builder.Select("id").
		From("tags").
		Where(sq.Eq{"fk_organization_id": organizationID}).
		OrderBy("sort_index", "id").
		MustSql()

	ids := []xid.ID{}
	require.NoError(t, sqlx.Select(db.sql, &ids, q, args...))

	return ids
}

// tagSortIndex reads the stored position of a tag.
func tagSortIndex(t *testing.T, db *DB, id xid.ID) int {
	t.Helper()

	q, args := db.builder.Select("sort_index").
		From("tags").
		Where(sq.Eq{"id": id}).
		MustSql()

	var index int
	require.NoError(t, sqlx.Get(db.sql, &index, q, args...))

	return index
}

// tagSettings reads every user's stored visibility setting for a tag.
func tagSettings(t *testing.T, db *DB, tagID xid.ID) map[string]bool {
	t.Helper()

	q, args := db.builder.Select("fk_user_id", "hidden").
		From("user_tag_settings").
		Where(sq.Eq{"fk_tag_id": tagID}).
		MustSql()

	var rows []struct {
		UserID string `db:"fk_user_id"`
		Hidden bool   `db:"hidden"`
	}

	require.NoError(t, sqlx.Select(db.sql, &rows, q, args...))

	res := map[string]bool{}

	for _, r := range rows {
		res[r.UserID] = r.Hidden
	}

	return res
}

// docSummary builds the tree row FetchDocumentTree produces for a fixture
// document.
func docSummary(doc *document.Document, children ...document.Summary) document.Summary {
	s := document.Summary{
		ID:              doc.ID,
		DocumentName:    doc.DocumentName,
		DefaultBranchID: doc.BranchID,
		Icon:            doc.Icon,
		Protected:       doc.Protected,
	}

	if len(children) > 0 {
		s.Children = children
	}

	return s
}

// tagSummary builds the tree row FetchTagTree produces for a fixture tag.
func tagSummary(tg tag.Tag, hidden bool, docs ...document.Summary) tag.Summary {
	s := tag.Summary{
		ID:        tg.ID,
		TagName:   tg.TagName,
		Color:     tg.Color,
		Hidden:    hidden,
		Documents: document.Summaries{},
	}

	if len(docs) > 0 {
		s.Documents = docs
	}

	return s
}

func Test_agent_InsertTag(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Tag              tag.Tag
		SortIndex        int
		Err              error
	}

	stubTag := func(t *testing.T, db *DB, organizationID, name string) tag.Tag {
		return tag.NewTag(tag.CreateInput{TagName: name, Color: "#3b82f6"}, organizationID, prepUsers(t, db, 1)[0])
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			return tcase{
				CancelledContext: true,
				Tag:              stubTag(t, db, prepOrganizations(t, db, 1)[0], "Release"),
				Err:              assert.AnError,
			}
		},
		"First tag starts at zero": func(t *testing.T, db *DB) tcase {
			return tcase{
				Tag: stubTag(t, db, prepOrganizations(t, db, 1)[0], "Release"),
			}
		},
		"Placed after the highest index": func(t *testing.T, db *DB) tcase {
			existing := prepTags(t, db, 1, func(_ int, tg *tag.Tag) {
				tg.SortIndex = 4
			})[0]

			return tcase{
				Tag:       stubTag(t, db, existing.OrganizationID, "Release"),
				SortIndex: 5,
			}
		},
		"Duplicate name in the organization": func(t *testing.T, db *DB) tcase {
			existing := prepTags(t, db, 1, nil)[0]

			return tcase{
				Tag: stubTag(t, db, existing.OrganizationID, existing.TagName),
				Err: tag.ErrDuplicateTagName,
			}
		},
		"Same name in another organization": func(t *testing.T, db *DB) tcase {
			existing := prepTags(t, db, 1, nil)[0]

			return tcase{
				Tag: stubTag(t, db, prepOrganizations(t, db, 1)[0], existing.TagName),
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

			err := db.InsertTag(ctx, c.Tag)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.SortIndex, tagSortIndex(t, db, c.Tag.ID))
		})
	}
}

func Test_agent_FetchTagTree(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		UserID           string
		Result           tag.Summaries
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			return tcase{
				CancelledContext: true,
				OrganizationID:   prepOrganizations(t, db, 1)[0],
				UserID:           prepUsers(t, db, 1)[0],
				Err:              assert.AnError,
			}
		},
		"Organization without tags": func(t *testing.T, db *DB) tcase {
			return tcase{
				OrganizationID: prepOrganizations(t, db, 1)[0],
				UserID:         prepUsers(t, db, 1)[0],
				Result:         tag.Summaries{},
			}
		},
		"Tags in display order": func(t *testing.T, db *DB) tcase {
			tags := prepTags(t, db, 2, func(i int, tg *tag.Tag) {
				tg.SortIndex = 1 - i
			})

			return tcase{
				OrganizationID: tags[0].OrganizationID,
				UserID:         prepUsers(t, db, 1)[0],
				Result: tag.Summaries{
					tagSummary(tags[1], false),
					tagSummary(tags[0], false),
				},
			}
		},
		"Documents listed under each tag they carry, with their subtrees": func(t *testing.T, db *DB) tcase {
			tags := prepTags(t, db, 2, nil)
			orgID := tags[0].OrganizationID

			roots := prepDocuments(t, db, 2, func(_ int, doc *document.Document) {
				doc.OrganizationID = orgID
			})
			child := prepDocuments(t, db, 1, func(_ int, doc *document.Document) {
				doc.OrganizationID = orgID
				doc.ParentID = null.ValueFrom(roots[0].ID)
			})[0]

			prepBranchTags(t, db, orgID, roots[0].BranchID, tags[0].ID, tags[1].ID)
			prepBranchTags(t, db, orgID, child.BranchID, tags[0].ID)
			prepBranchTags(t, db, orgID, roots[1].BranchID, tags[1].ID)

			return tcase{
				OrganizationID: orgID,
				UserID:         prepUsers(t, db, 1)[0],
				Result: tag.Summaries{
					tagSummary(
						tags[0],
						false,
						docSummary(roots[0], docSummary(child)),
						docSummary(child),
					),
					tagSummary(
						tags[1],
						false,
						docSummary(roots[0], docSummary(child)),
						docSummary(roots[1]),
					),
				},
			}
		},
		"Tag on a non-default branch lists no document": func(t *testing.T, db *DB) tcase {
			tg := prepTags(t, db, 1, nil)[0]

			doc := prepDocuments(t, db, 1, func(_ int, doc *document.Document) {
				doc.OrganizationID = tg.OrganizationID
			})[0]
			branch := prepDocumentBranches(t, db, 1, func(_ int, br *document.Document) {
				br.ID = doc.ID
				br.OrganizationID = doc.OrganizationID
			})[0]

			prepBranchTags(t, db, tg.OrganizationID, branch.BranchID, tg.ID)

			return tcase{
				OrganizationID: tg.OrganizationID,
				UserID:         prepUsers(t, db, 1)[0],
				Result:         tag.Summaries{tagSummary(tg, false)},
			}
		},
		"Another organization's assignment is ignored": func(t *testing.T, db *DB) tcase {
			tg := prepTags(t, db, 1, nil)[0]
			other := prepDocuments(t, db, 1, nil)[0]

			prepBranchTags(t, db, other.OrganizationID, other.BranchID, tg.ID)

			return tcase{
				OrganizationID: tg.OrganizationID,
				UserID:         prepUsers(t, db, 1)[0],
				Result:         tag.Summaries{tagSummary(tg, false)},
			}
		},
		"Hidden reflects the requesting user's own setting": func(t *testing.T, db *DB) tcase {
			tags := prepTags(t, db, 3, nil)
			users := prepUsers(t, db, 2)

			require.NoError(t, db.SetTagVisibility(
				context.Background(),
				tags[0].OrganizationID,
				users[0],
				tags[0].ID,
				tag.VisibilityInput{Hidden: true},
			))
			require.NoError(t, db.SetTagVisibility(
				context.Background(),
				tags[0].OrganizationID,
				users[0],
				tags[1].ID,
				tag.VisibilityInput{Hidden: false},
			))
			require.NoError(t, db.SetTagVisibility(
				context.Background(),
				tags[0].OrganizationID,
				users[1],
				tags[2].ID,
				tag.VisibilityInput{Hidden: true},
			))

			return tcase{
				OrganizationID: tags[0].OrganizationID,
				UserID:         users[0],
				Result: tag.Summaries{
					tagSummary(tags[0], true),
					tagSummary(tags[1], false),
					tagSummary(tags[2], false),
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

			res, err := db.FetchTagTree(ctx, c.OrganizationID, c.UserID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, res)
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_agent_FetchBranchTagIDs(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		DocumentID       xid.ID
		BranchID         xid.ID
		Result           []xid.ID
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				OrganizationID:   doc.OrganizationID,
				DocumentID:       doc.ID,
				BranchID:         doc.BranchID,
				Err:              assert.AnError,
			}
		},
		"Branch without tags": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			return tcase{
				OrganizationID: doc.OrganizationID,
				DocumentID:     doc.ID,
				BranchID:       doc.BranchID,
				Result:         []xid.ID{},
			}
		},
		"Ids in the tags' display order": func(t *testing.T, db *DB) tcase {
			tags := prepTags(t, db, 3, func(i int, tg *tag.Tag) {
				tg.SortIndex = (i + 2) % 3
			})
			doc := prepDocuments(t, db, 1, func(_ int, doc *document.Document) {
				doc.OrganizationID = tags[0].OrganizationID
			})[0]

			prepBranchTags(t, db, doc.OrganizationID, doc.BranchID, tags[0].ID, tags[1].ID, tags[2].ID)

			return tcase{
				OrganizationID: doc.OrganizationID,
				DocumentID:     doc.ID,
				BranchID:       doc.BranchID,
				Result:         []xid.ID{tags[1].ID, tags[2].ID, tags[0].ID},
			}
		},
		"Branch of another document": func(t *testing.T, db *DB) tcase {
			tg := prepTags(t, db, 1, nil)[0]
			doc := prepTaggedDocument(t, db, tg)

			return tcase{
				OrganizationID: doc.OrganizationID,
				DocumentID:     xid.New(),
				BranchID:       doc.BranchID,
				Result:         []xid.ID{},
			}
		},
		"Branch of another organization": func(t *testing.T, db *DB) tcase {
			tg := prepTags(t, db, 1, nil)[0]
			doc := prepTaggedDocument(t, db, tg)

			return tcase{
				OrganizationID: prepOrganizations(t, db, 1)[0],
				DocumentID:     doc.ID,
				BranchID:       doc.BranchID,
				Result:         []xid.ID{},
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

			res, err := db.FetchBranchTagIDs(ctx, c.OrganizationID, c.DocumentID, c.BranchID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, res)
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_agent_UpdateTagTree(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		Tree             tag.Summaries
		Order            []xid.ID
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			tags := prepTags(t, db, 2, nil)

			return tcase{
				CancelledContext: true,
				OrganizationID:   tags[0].OrganizationID,
				Tree:             tag.Summaries{{ID: tags[1].ID}, {ID: tags[0].ID}},
				Order:            []xid.ID{tags[0].ID, tags[1].ID},
				Err:              assert.AnError,
			}
		},
		"Reorders the tags to the given tree": func(t *testing.T, db *DB) tcase {
			tags := prepTags(t, db, 3, nil)

			return tcase{
				OrganizationID: tags[0].OrganizationID,
				Tree:           tag.Summaries{{ID: tags[2].ID}, {ID: tags[0].ID}, {ID: tags[1].ID}},
				Order:          []xid.ID{tags[2].ID, tags[0].ID, tags[1].ID},
			}
		},
		// the rewrite is one transaction, so a foreign tag leaves the
		// order as it was rather than half applied.
		"Tag of another organization": func(t *testing.T, db *DB) tcase {
			tags := prepTags(t, db, 2, nil)
			foreign := prepTags(t, db, 1, nil)[0]

			return tcase{
				OrganizationID: tags[0].OrganizationID,
				Tree:           tag.Summaries{{ID: tags[1].ID}, {ID: foreign.ID}, {ID: tags[0].ID}},
				Order:          []xid.ID{tags[0].ID, tags[1].ID},
				Err:            errutil.ErrNotFound,
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

			err := db.UpdateTagTree(ctx, c.Tree, c.OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			assert.Equal(t, c.Order, tagOrder(t, db, c.OrganizationID))
		})
	}
}

func Test_agent_SetTagVisibility(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		UserID           string
		TagID            xid.ID
		Input            tag.VisibilityInput
		Settings         map[string]bool
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			tg := prepTags(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				OrganizationID:   tg.OrganizationID,
				UserID:           prepUsers(t, db, 1)[0],
				TagID:            tg.ID,
				Input:            tag.VisibilityInput{Hidden: true},
				Settings:         map[string]bool{},
				Err:              assert.AnError,
			}
		},
		"Tag of another organization": func(t *testing.T, db *DB) tcase {
			tg := prepTags(t, db, 1, nil)[0]

			return tcase{
				OrganizationID: prepOrganizations(t, db, 1)[0],
				UserID:         prepUsers(t, db, 1)[0],
				TagID:          tg.ID,
				Input:          tag.VisibilityInput{Hidden: true},
				Settings:       map[string]bool{},
				Err:            errutil.ErrNotFound,
			}
		},
		"Hides the tag for the caller alone": func(t *testing.T, db *DB) tcase {
			tg := prepTags(t, db, 1, nil)[0]
			users := prepUsers(t, db, 2)

			require.NoError(t, db.SetTagVisibility(
				context.Background(),
				tg.OrganizationID,
				users[1],
				tg.ID,
				tag.VisibilityInput{Hidden: false},
			))

			return tcase{
				OrganizationID: tg.OrganizationID,
				UserID:         users[0],
				TagID:          tg.ID,
				Input:          tag.VisibilityInput{Hidden: true},
				Settings:       map[string]bool{users[0]: true, users[1]: false},
			}
		},
		"Overwrites the caller's existing setting": func(t *testing.T, db *DB) tcase {
			tg := prepTags(t, db, 1, nil)[0]
			user := prepUsers(t, db, 1)[0]

			require.NoError(t, db.SetTagVisibility(
				context.Background(),
				tg.OrganizationID,
				user,
				tg.ID,
				tag.VisibilityInput{Hidden: true},
			))

			return tcase{
				OrganizationID: tg.OrganizationID,
				UserID:         user,
				TagID:          tg.ID,
				Input:          tag.VisibilityInput{Hidden: false},
				Settings:       map[string]bool{user: false},
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

			err := db.SetTagVisibility(ctx, c.OrganizationID, c.UserID, c.TagID, c.Input)
			testutil.RequireEqualError(t, c.Err, err)

			assert.Equal(t, c.Settings, tagSettings(t, db, c.TagID))
		})
	}

	t.Run("Setting goes with the user", func(t *testing.T) {
		t.Parallel()

		db := prepTempDB(t)
		tg := prepTags(t, db, 1, nil)[0]
		user := prepUsers(t, db, 1)[0]

		require.NoError(t, db.SetTagVisibility(
			context.Background(),
			tg.OrganizationID,
			user,
			tg.ID,
			tag.VisibilityInput{Hidden: true},
		))

		q, args := db.builder.Delete("users").Where(sq.Eq{"id": user}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)

		assert.Empty(t, tagSettings(t, db, tg.ID))
	})
}

func Test_agent_DeleteTag(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		Tag              tag.Tag
		BranchID         xid.ID
		Remaining        []xid.ID
		Err              error
	}

	// tagged puts a document and a user's setting on the tag, so the
	// delete has something to cascade to.
	tagged := func(t *testing.T, db *DB) (tag.Tag, xid.ID) {
		tg := prepTags(t, db, 1, nil)[0]
		doc := prepTaggedDocument(t, db, tg)

		require.NoError(t, db.SetTagVisibility(
			context.Background(),
			tg.OrganizationID,
			prepUsers(t, db, 1)[0],
			tg.ID,
			tag.VisibilityInput{Hidden: true},
		))

		return tg, doc.BranchID
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			tg, branchID := tagged(t, db)

			return tcase{
				CancelledContext: true,
				OrganizationID:   tg.OrganizationID,
				Tag:              tg,
				BranchID:         branchID,
				Remaining:        []xid.ID{tg.ID},
				Err:              assert.AnError,
			}
		},
		"Tag of another organization": func(t *testing.T, db *DB) tcase {
			tg, branchID := tagged(t, db)

			return tcase{
				OrganizationID: prepOrganizations(t, db, 1)[0],
				Tag:            tg,
				BranchID:       branchID,
				Remaining:      []xid.ID{tg.ID},
				Err:            errutil.ErrNotFound,
			}
		},
		"Removes the tag with its assignments and settings": func(t *testing.T, db *DB) tcase {
			tg, branchID := tagged(t, db)

			return tcase{
				OrganizationID: tg.OrganizationID,
				Tag:            tg,
				BranchID:       branchID,
				Remaining:      []xid.ID{},
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

			err := db.DeleteTag(ctx, c.Tag.ID, c.OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			assert.Equal(t, c.Remaining, tagOrder(t, db, c.Tag.OrganizationID))
			assert.Equal(t, c.Remaining, branchTagIDs(t, db, c.BranchID))
			assert.Len(t, tagSettings(t, db, c.Tag.ID), len(c.Remaining))
		})
	}
}

func Test_agent_AssignBranchTag(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		DocumentID       xid.ID
		BranchID         xid.ID
		TagID            xid.ID
		Result           []xid.ID
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			tg := prepTags(t, db, 1, nil)[0]
			doc := prepDocuments(t, db, 1, func(_ int, doc *document.Document) {
				doc.OrganizationID = tg.OrganizationID
			})[0]

			return tcase{
				CancelledContext: true,
				OrganizationID:   tg.OrganizationID,
				DocumentID:       doc.ID,
				BranchID:         doc.BranchID,
				TagID:            tg.ID,
				Result:           []xid.ID{},
				Err:              assert.AnError,
			}
		},
		"Makes the branch carry the tag": func(t *testing.T, db *DB) tcase {
			tg := prepTags(t, db, 1, nil)[0]
			doc := prepDocuments(t, db, 1, func(_ int, doc *document.Document) {
				doc.OrganizationID = tg.OrganizationID
			})[0]

			return tcase{
				OrganizationID: tg.OrganizationID,
				DocumentID:     doc.ID,
				BranchID:       doc.BranchID,
				TagID:          tg.ID,
				Result:         []xid.ID{tg.ID},
			}
		},
		"Tag the branch already carries changes nothing": func(t *testing.T, db *DB) tcase {
			tg := prepTags(t, db, 1, nil)[0]
			doc := prepTaggedDocument(t, db, tg)

			return tcase{
				OrganizationID: tg.OrganizationID,
				DocumentID:     doc.ID,
				BranchID:       doc.BranchID,
				TagID:          tg.ID,
				Result:         []xid.ID{tg.ID},
			}
		},
		"Tag of another organization": func(t *testing.T, db *DB) tcase {
			tg := prepTags(t, db, 1, nil)[0]
			doc := prepDocuments(t, db, 1, nil)[0]

			return tcase{
				OrganizationID: doc.OrganizationID,
				DocumentID:     doc.ID,
				BranchID:       doc.BranchID,
				TagID:          tg.ID,
				Result:         []xid.ID{},
				Err:            errutil.ErrNotFound,
			}
		},
		"Branch of another organization": func(t *testing.T, db *DB) tcase {
			tg := prepTags(t, db, 1, nil)[0]
			doc := prepDocuments(t, db, 1, nil)[0]

			return tcase{
				OrganizationID: tg.OrganizationID,
				DocumentID:     doc.ID,
				BranchID:       doc.BranchID,
				TagID:          tg.ID,
				Result:         []xid.ID{},
				Err:            errutil.ErrNotFound,
			}
		},
		"Branch of another document": func(t *testing.T, db *DB) tcase {
			tg := prepTags(t, db, 1, nil)[0]
			doc := prepDocuments(t, db, 1, func(_ int, doc *document.Document) {
				doc.OrganizationID = tg.OrganizationID
			})[0]

			return tcase{
				OrganizationID: tg.OrganizationID,
				DocumentID:     xid.New(),
				BranchID:       doc.BranchID,
				TagID:          tg.ID,
				Result:         []xid.ID{},
				Err:            errutil.ErrNotFound,
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

			err := db.AssignBranchTag(ctx, c.OrganizationID, c.DocumentID, c.BranchID, c.TagID)
			testutil.RequireEqualError(t, c.Err, err)

			assert.Equal(t, c.Result, branchTagIDs(t, db, c.BranchID))
		})
	}

	t.Run("Assignment goes with the branch", func(t *testing.T) {
		t.Parallel()

		db := prepTempDB(t)
		tg := prepTags(t, db, 1, nil)[0]
		doc := prepTaggedDocument(t, db, tg)

		require.NoError(t, db.DeleteDocumentBranchByID(context.Background(), doc.BranchID, doc.OrganizationID))

		assert.Empty(t, branchTagIDs(t, db, doc.BranchID))
		assert.Equal(t, []xid.ID{tg.ID}, tagOrder(t, db, tg.OrganizationID))
	})

	t.Run("Assignment goes with the document", func(t *testing.T) {
		t.Parallel()

		db := prepTempDB(t)
		tg := prepTags(t, db, 1, nil)[0]
		doc := prepTaggedDocument(t, db, tg)

		_, err := db.DeleteDocument(context.Background(), doc.ID, doc.OrganizationID)
		require.NoError(t, err)

		assert.Empty(t, branchTagIDs(t, db, doc.BranchID))
		assert.Equal(t, []xid.ID{tg.ID}, tagOrder(t, db, tg.OrganizationID))
	})
}

func Test_agent_UnassignBranchTag(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		DocumentID       xid.ID
		BranchID         xid.ID
		TagID            xid.ID
		Result           []xid.ID
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			tg := prepTags(t, db, 1, nil)[0]
			doc := prepTaggedDocument(t, db, tg)

			return tcase{
				CancelledContext: true,
				OrganizationID:   tg.OrganizationID,
				DocumentID:       doc.ID,
				BranchID:         doc.BranchID,
				TagID:            tg.ID,
				Result:           []xid.ID{tg.ID},
				Err:              assert.AnError,
			}
		},
		"Stops the branch carrying the tag": func(t *testing.T, db *DB) tcase {
			tags := prepTags(t, db, 2, nil)
			doc := prepTaggedDocument(t, db, tags[0])

			prepBranchTags(t, db, doc.OrganizationID, doc.BranchID, tags[1].ID)

			return tcase{
				OrganizationID: doc.OrganizationID,
				DocumentID:     doc.ID,
				BranchID:       doc.BranchID,
				TagID:          tags[0].ID,
				Result:         []xid.ID{tags[1].ID},
			}
		},
		"Tag the branch does not carry changes nothing": func(t *testing.T, db *DB) tcase {
			tags := prepTags(t, db, 2, nil)
			doc := prepTaggedDocument(t, db, tags[0])

			return tcase{
				OrganizationID: doc.OrganizationID,
				DocumentID:     doc.ID,
				BranchID:       doc.BranchID,
				TagID:          tags[1].ID,
				Result:         []xid.ID{tags[0].ID},
			}
		},
		"Branch of another document is left alone": func(t *testing.T, db *DB) tcase {
			tg := prepTags(t, db, 1, nil)[0]
			doc := prepTaggedDocument(t, db, tg)

			return tcase{
				OrganizationID: doc.OrganizationID,
				DocumentID:     xid.New(),
				BranchID:       doc.BranchID,
				TagID:          tg.ID,
				Result:         []xid.ID{tg.ID},
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

			err := db.UnassignBranchTag(ctx, c.OrganizationID, c.DocumentID, c.BranchID, c.TagID)
			testutil.RequireEqualError(t, c.Err, err)

			assert.Equal(t, c.Result, branchTagIDs(t, db, c.BranchID))
		})
	}
}

// prepBranchPair creates two branches of one document in a fresh
// organization, carrying the given tags out of the two the organization
// gets.
func prepBranchPair(t *testing.T, db *DB, fromTags, toTags func([]tag.Tag) []xid.ID) ([]tag.Tag, *document.Document, *document.Document) {
	t.Helper()

	tags := prepTags(t, db, 2, nil)
	from := prepDocuments(t, db, 1, func(_ int, doc *document.Document) {
		doc.OrganizationID = tags[0].OrganizationID
	})[0]
	to := prepDocumentBranches(t, db, 1, func(_ int, br *document.Document) {
		br.ID = from.ID
		br.OrganizationID = from.OrganizationID
	})[0]

	prepBranchTags(t, db, from.OrganizationID, from.BranchID, fromTags(tags)...)
	prepBranchTags(t, db, from.OrganizationID, to.BranchID, toTags(tags)...)

	return tags, from, to
}

func Test_agent_CopyBranchTags(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		FromBranchID     xid.ID
		ToBranchID       xid.ID
		Result           []xid.ID
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			tags, from, to := prepBranchPair(t, db,
				func(tags []tag.Tag) []xid.ID { return []xid.ID{tags[0].ID} },
				func([]tag.Tag) []xid.ID { return nil },
			)

			return tcase{
				CancelledContext: true,
				OrganizationID:   tags[0].OrganizationID,
				FromBranchID:     from.BranchID,
				ToBranchID:       to.BranchID,
				Result:           []xid.ID{},
				Err:              assert.AnError,
			}
		},
		"Adds the source's tags to the target's own": func(t *testing.T, db *DB) tcase {
			tags, from, to := prepBranchPair(t, db,
				func(tags []tag.Tag) []xid.ID { return []xid.ID{tags[1].ID} },
				func(tags []tag.Tag) []xid.ID { return []xid.ID{tags[0].ID} },
			)

			return tcase{
				OrganizationID: tags[0].OrganizationID,
				FromBranchID:   from.BranchID,
				ToBranchID:     to.BranchID,
				Result:         []xid.ID{tags[0].ID, tags[1].ID},
			}
		},
		"Tag both branches carry is kept once": func(t *testing.T, db *DB) tcase {
			tags, from, to := prepBranchPair(t, db,
				func(tags []tag.Tag) []xid.ID { return []xid.ID{tags[0].ID} },
				func(tags []tag.Tag) []xid.ID { return []xid.ID{tags[0].ID} },
			)

			return tcase{
				OrganizationID: tags[0].OrganizationID,
				FromBranchID:   from.BranchID,
				ToBranchID:     to.BranchID,
				Result:         []xid.ID{tags[0].ID},
			}
		},
		"Source without tags leaves the target as it is": func(t *testing.T, db *DB) tcase {
			tags, from, to := prepBranchPair(t, db,
				func([]tag.Tag) []xid.ID { return nil },
				func(tags []tag.Tag) []xid.ID { return []xid.ID{tags[0].ID} },
			)

			return tcase{
				OrganizationID: tags[0].OrganizationID,
				FromBranchID:   from.BranchID,
				ToBranchID:     to.BranchID,
				Result:         []xid.ID{tags[0].ID},
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

			err := db.CopyBranchTags(ctx, c.OrganizationID, c.FromBranchID, c.ToBranchID)
			testutil.RequireEqualError(t, c.Err, err)

			assert.Equal(t, c.Result, branchTagIDs(t, db, c.ToBranchID))
		})
	}
}

func Test_agent_ReplaceBranchTags(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		FromBranchID     xid.ID
		ToBranchID       xid.ID
		Result           []xid.ID
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			tags, from, to := prepBranchPair(t, db,
				func(tags []tag.Tag) []xid.ID { return []xid.ID{tags[1].ID} },
				func(tags []tag.Tag) []xid.ID { return []xid.ID{tags[0].ID} },
			)

			return tcase{
				CancelledContext: true,
				OrganizationID:   tags[0].OrganizationID,
				FromBranchID:     from.BranchID,
				ToBranchID:       to.BranchID,
				Result:           []xid.ID{tags[0].ID},
				Err:              assert.AnError,
			}
		},
		"Target carries exactly the source's tags": func(t *testing.T, db *DB) tcase {
			tags, from, to := prepBranchPair(t, db,
				func(tags []tag.Tag) []xid.ID { return []xid.ID{tags[1].ID} },
				func(tags []tag.Tag) []xid.ID { return []xid.ID{tags[0].ID} },
			)

			return tcase{
				OrganizationID: tags[0].OrganizationID,
				FromBranchID:   from.BranchID,
				ToBranchID:     to.BranchID,
				Result:         []xid.ID{tags[1].ID},
			}
		},
		"Source without tags empties the target": func(t *testing.T, db *DB) tcase {
			tags, from, to := prepBranchPair(t, db,
				func([]tag.Tag) []xid.ID { return nil },
				func(tags []tag.Tag) []xid.ID { return []xid.ID{tags[0].ID, tags[1].ID} },
			)

			return tcase{
				OrganizationID: tags[0].OrganizationID,
				FromBranchID:   from.BranchID,
				ToBranchID:     to.BranchID,
				Result:         []xid.ID{},
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

			err := db.ReplaceBranchTags(ctx, c.OrganizationID, c.FromBranchID, c.ToBranchID)
			testutil.RequireEqualError(t, c.Err, err)

			assert.Equal(t, c.Result, branchTagIDs(t, db, c.ToBranchID))
		})
	}
}
