package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	sq "github.com/Masterminds/squirrel"
	"github.com/dchest/uniuri"
	"github.com/guregu/null/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	"github.com/lann/builder"
	"github.com/orlangure/gnomock"
	pgDocker "github.com/orlangure/gnomock/preset/postgres"
	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/db/migrations"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/activity"
	"github.com/oxynote/oxynote/server/core/internal/document/comment"
	"github.com/oxynote/oxynote/server/core/internal/org"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/internal/variable"
	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/ioutil"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

var (
	_pgDSN string

	// _pgContainerID is the Docker ID of the test Postgres container.
	_pgContainerID string
)

const (
	_pgUser string = "pgtest"
	_pgPass string = "pgpass"

	// _dataSourceKey is the base64 credentials encryption key every
	// temporary test database is configured with.
	_dataSourceKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

	// _retiredDataSourceKey is a base64 credentials encryption key no
	// temporary test database is configured with unless a test says so.
	_retiredDataSourceKey = "ZmVkY2JhOTg3NjU0MzIxMGZlZGNiYTk4NzY1NDMyMTA="
)

// _dataSourceKeys is the keyring every temporary test database is
// configured with.
var _dataSourceKeys = mustParseKeys(_dataSourceKey)

// mustParseKeys parses a keyring list or panics.
func mustParseKeys(list string) *cryptoutil.Keyring {
	keys, err := cryptoutil.ParseKeyring(list)
	if err != nil {
		panic("cannot parse test keyring: " + err.Error())
	}

	return keys
}

func TestMain(m *testing.M) {
	container, err := gnomock.Start(pgDocker.Preset(
		pgDocker.WithUser(
			_pgUser,
			_pgPass,
		),
		pgDocker.WithVersion("18.6-alpine"),
	))
	if err != nil {
		panic("cannot set up: " + err.Error())
	}

	defer func() {
		err = gnomock.Stop(container)
		if err != nil {
			panic("cannot clean up: " + err.Error())
		}
	}()

	_pgDSN = container.Host + ":" + strconv.Itoa(container.DefaultPort())
	_pgContainerID = container.DockerID()

	// goleak.VerifyTestMain ends in os.Exit, which skips the deferred
	// container stops. Returning runs them, and the test binary then exits
	// with the code from m.Run.
	ignore := goleak.IgnoreCurrent()

	if m.Run() != 0 {
		return
	}

	if err = goleak.Find(ignore); err != nil {
		panic("goroutines leaked: " + err.Error())
	}
}

func Test_New(t *testing.T) {
	// error - db file not found
	opts := Options{
		DSN: filepath.Join(t.TempDir(), "test", "123"),
	}

	db, err := New(slog.New(slog.DiscardHandler), metricutil.NewFactory("test", nil), opts)
	assert.Error(t, err)
	assert.Nil(t, db)

	// success
	opts = Options{
		DSN: fmt.Sprintf("postgres://%s:%s@%s/postgres?sslmode=disable", _pgUser, _pgPass, _pgDSN),
	}

	db, err = New(slog.New(slog.DiscardHandler), metricutil.NewFactory("test", nil), opts)
	assert.NoError(t, err)
	require.NotNil(t, db)
	assert.NotZero(t, db.log)
	assert.NotNil(t, db.agent)
	assert.NotNil(t, db.closer)
	assert.NotZero(t, db.builder)

	assert.NoError(t, db.closer.Close())
}

func Test_DB_Close(t *testing.T) {
	db := &DB{
		closer: ioutil.CloserFunc(func() error {
			return assert.AnError
		}),
	}

	assert.Error(t, db.Close())

	db.closer = ioutil.CloserFunc(func() error {
		return nil
	})

	assert.NoError(t, db.Close())
}

func Test_DetectError(t *testing.T) {
	type tcase struct {
		Err    error
		Result error
	}

	cc := map[string]func(*testing.T) tcase{
		"Not found": func(*testing.T) tcase {
			return tcase{
				Err:    sql.ErrNoRows,
				Result: errutil.ErrNotFound,
			}
		},
		"Error pass through": func(*testing.T) tcase {
			return tcase{
				Err:    sql.ErrConnDone,
				Result: sql.ErrConnDone,
			}
		},
		"No error": func(*testing.T) tcase {
			return tcase{}
		},
		"Unmatched pgx constraint": func(*testing.T) tcase {
			return tcase{
				Err: &pgconn.PgError{
					Code: "123",
				},
				Result: &pgconn.PgError{
					Code: "123",
				},
			}
		},
		"Unmatched constraint": func(*testing.T) tcase {
			return tcase{
				Err:    errors.New("b"),
				Result: errors.New("b"),
			}
		},
		"Duplicate branch name": func(t *testing.T) tcase {
			db := prepTempDB(t)

			// renaming a branch onto a sibling's name trips the unique key
			// that DetectError has to recognise.
			branches := prepDocumentBranches(t, db, 2, nil)

			second := branches[1]
			second.BranchName = branches[0].BranchName

			err := db.UpdateDocumentBranchMetadata(context.Background(), *second)
			require.Error(t, err)

			return tcase{
				Err:    err,
				Result: errutil.New(http.StatusConflict, "document_branch.duplicate_name", "branch name is already in use"),
			}
		},
		"Duplicate file id": func(t *testing.T) tcase {
			db := prepTempDB(t)

			f := prepDocumentFiles(t, db, 1, nil)[0]

			err := db.InsertDocumentFile(context.Background(), f)
			require.Error(t, err)

			return tcase{
				Err:    err,
				Result: errutil.New(http.StatusConflict, "document_file.exists", "file id is already in use"),
			}
		},
		"Duplicate tag name": func(t *testing.T) tcase {
			db := prepTempDB(t)

			existing := prepTags(t, db, 1, nil)[0]

			dup := prepNewTag(t, existing.TagName, existing.OrganizationID, prepUsers(t, db, 1)[0])

			err := db.InsertTag(context.Background(), dup)
			require.Error(t, err)

			return tcase{
				Err:    err,
				Result: tag.ErrDuplicateTagName,
			}
		},
		"Duplicate variable group name": func(*testing.T) tcase {
			return tcase{
				Err:    &pgconn.PgError{Code: "23505", ConstraintName: "personal_variable_groups_fk_organization_id_group_name_key"},
				Result: variable.ErrDuplicateGroupName,
			}
		},
		"Duplicate variable key": func(*testing.T) tcase {
			return tcase{
				Err:    &pgconn.PgError{Code: "23505", ConstraintName: "personal_variables_fk_organization_id_variable_key_key"},
				Result: variable.ErrDuplicateKey,
			}
		},
		"Unmatched foreign key": func(*testing.T) tcase {
			return tcase{
				Err:    &pgconn.PgError{Code: "23503", ConstraintName: "tags_fk_organization_id_fkey"},
				Result: &pgconn.PgError{Code: "23503", ConstraintName: "tags_fk_organization_id_fkey"},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			c := cfn(t)

			assert.Equal(t, c.Result, DetectError(c.Err))
		})
	}
}

func Test_DB_migrateUp(t *testing.T) {
	db := prepTempDB(t)

	organizationID := prepOrganizations(t, db, 1)[0]

	// rolling back to before the variables migration (008) and up again
	// runs its backfill over an organization that existed before it, and
	// the palette name migration (010) over tags stored as hex.
	_, err := db.migrations.DownTo(context.Background(), 7)
	require.NoError(t, err)

	// the member roles migration (012) turns every owner into an admin and
	// leaves every other role alone.
	users := prepUsers(t, db, 2)

	for i, role := range []string{"owner", "editor"} {
		q, args := db.builder.Insert("organization_members").
			SetMap(map[string]any{
				"id":                 xid.New().String(),
				"fk_organization_id": organizationID,
				"fk_user_id":         users[i],
				"role":               role,
				"created_at":         timeutil.Now().Truncate(time.Second),
			}).MustSql()

		_, err = db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	q, args := db.builder.Insert("organization_invitations").
		SetMap(map[string]any{
			"id":                 "inv-1",
			"fk_organization_id": organizationID,
			"email":              "a@b.c",
			"role":               "owner",
			"status":             "pending",
			"expires_at":         timeutil.Now().Add(time.Hour).Truncate(time.Second),
			"created_at":         timeutil.Now().Truncate(time.Second),
			"fk_inviter_id":      users[0],
		}).MustSql()

	_, err = db.sql.Exec(q, args...)
	require.NoError(t, err)

	// the credentials kind migration (013) adds the basic kind to
	// credentials stored before it.
	source := prepDataSources(t, db, 1, func(_ int, ds *datasource.DataSource) {
		ds.OrganizationID = organizationID
		ds.Credentials = stubCredentials(`{"username":"user","password":"pass"}`)
	})[0]

	hexes := []string{"#f54a00", "#e7010c", "#155DFC", "rgb(1, 2, 3)"}

	prepTags(t, db, len(hexes), func(i int, tg *tag.Tag) {
		tg.OrganizationID = organizationID
		tg.Color = hexes[i]
	})

	require.NoError(t, db.migrateUp())

	tags, err := db.FetchTagTree(context.Background(), organizationID, "")
	require.NoError(t, err)

	colors := make([]string, 0, len(tags))

	for _, s := range tags {
		colors = append(colors, s.Color)
	}

	assert.Equal(t, []string{"orange", "red", "blue", "green"}, colors)

	tree, err := db.FetchVariableTree(context.Background(), organizationID, "")
	require.NoError(t, err)
	require.Len(t, tree, 1)
	require.Len(t, tree[0].Variables, 2)

	exp := variable.SeedBuiltins(organizationID)

	// option ids are random on both sides, so the list default is checked
	// against each side's own first option and then set aside.
	for _, g := range []variable.GroupTree{exp, tree[0]} {
		refresh := g.Variables[1]
		assert.Equal(t, null.StringFrom(refresh.Options[0].ID.String()), refresh.DefaultValue)

		g.Variables[1].DefaultValue = null.String{}
	}

	testutil.AssertFilterEqual(t, exp, tree[0], xid.ID{}, time.Time{})

	for i, role := range []org.Role{org.RoleAdmin, org.RoleEditor} {
		res, err := db.FetchOrganizationMemberRole(context.Background(), organizationID, users[i])
		require.NoError(t, err)
		assert.Equal(t, role, res)
	}

	var invitationRole string

	require.NoError(t, db.sql.Get(&invitationRole, "SELECT role FROM organization_invitations WHERE id = 'inv-1'"))
	assert.Equal(t, "admin", invitationRole)

	assert.JSONEq(t, `{"kind":"basic","username":"user","password":"pass"}`, decryptDataSourceCredentials(t, db, _dataSourceKeys, source.ID))
}

func Test_migrateDataSourceCredentialsKind(t *testing.T) {
	// the keyring holds a retired key besides the newest one, so a row
	// sealed under the retired key is still read.
	keys := mustParseKeys(_dataSourceKey + "," + _retiredDataSourceKey)

	type tcase struct {
		CancelledContext bool
		Down             bool
		Err              error
		// Check inspects the rows the migration left behind.
		Check func(t *testing.T, db *DB)
	}

	// prep stores one data source holding the given credentials JSON,
	// sealed under the database's keys.
	prep := func(t *testing.T, db *DB, creds string) *datasource.DataSource {
		t.Helper()

		return prepDataSources(t, db, 1, func(_ int, ds *datasource.DataSource) {
			ds.Credentials = stubCredentials(creds)
		})[0]
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			prep(t, db, `{"username":"user"}`)

			return tcase{
				CancelledContext: true,
				Err:              assert.AnError,
			}
		},
		"Basic credentials gain the basic kind": func(t *testing.T, db *DB) tcase {
			ds := prep(t, db, `{"username":"user","password":"pass","caCertificate":"pem"}`)

			return tcase{
				Check: func(t *testing.T, db *DB) {
					assert.JSONEq(t,
						`{"kind":"basic","username":"user","password":"pass","caCertificate":"pem"}`,
						decryptDataSourceCredentials(t, db, keys, ds.ID),
					)
				},
			}
		},
		"Credentials under a retired key move to the newest key": func(t *testing.T, db *DB) tcase {
			db.opts.DataSourceCredentialsKeys = mustParseKeys(_retiredDataSourceKey)
			ds := prep(t, db, `{"username":"user"}`)

			return tcase{
				Check: func(t *testing.T, db *DB) {
					assert.True(t, keys.IsCurrent(fetchDataSourceCredentials(t, db, ds.ID)))
					assert.JSONEq(t, `{"kind":"basic","username":"user"}`, decryptDataSourceCredentials(t, db, keys, ds.ID))
				},
			}
		},
		"Credentials with a kind are left alone": func(t *testing.T, db *DB) tcase {
			ds := prep(t, db, `{"kind":"token","token":"secret"}`)
			before := fetchDataSourceCredentials(t, db, ds.ID)

			return tcase{
				Check: func(t *testing.T, db *DB) {
					assert.Equal(t, before, fetchDataSourceCredentials(t, db, ds.ID))
				},
			}
		},
		"Unreadable credentials are reset": func(t *testing.T, db *DB) tcase {
			ds := prep(t, db, `{"username":"user"}`)
			poisonDataSourceCredentials(t, db, ds.ID)

			return tcase{
				Check: func(t *testing.T, db *DB) {
					assert.Empty(t, decryptDataSourceCredentials(t, db, keys, ds.ID))
				},
			}
		},
		"Empty credentials are left alone": func(t *testing.T, db *DB) tcase {
			ds := prep(t, db, ``)
			before := fetchDataSourceCredentials(t, db, ds.ID)

			return tcase{
				Check: func(t *testing.T, db *DB) {
					assert.Equal(t, before, fetchDataSourceCredentials(t, db, ds.ID))
				},
			}
		},
		"Credentials that are not an object are reset": func(t *testing.T, db *DB) tcase {
			array := prep(t, db, `[1]`)
			nullCreds := prep(t, db, `null`)

			return tcase{
				Check: func(t *testing.T, db *DB) {
					assert.Empty(t, decryptDataSourceCredentials(t, db, keys, array.ID))
					assert.Empty(t, decryptDataSourceCredentials(t, db, keys, nullCreds.ID))
				},
			}
		},
		"Down removes the basic kind": func(t *testing.T, db *DB) tcase {
			ds := prep(t, db, `{"kind":"basic","username":"user","password":"pass"}`)

			return tcase{
				Down: true,
				Check: func(t *testing.T, db *DB) {
					assert.JSONEq(t, `{"username":"user","password":"pass"}`, decryptDataSourceCredentials(t, db, keys, ds.ID))
				},
			}
		},
		"Down removes the token and keeps the CA certificate": func(t *testing.T, db *DB) tcase {
			ds := prep(t, db, `{"kind":"token","token":"secret","caCertificate":"pem"}`)

			return tcase{
				Down: true,
				Check: func(t *testing.T, db *DB) {
					assert.JSONEq(t, `{"caCertificate":"pem"}`, decryptDataSourceCredentials(t, db, keys, ds.ID))
				},
			}
		},
		"Down leaves credentials without a kind alone": func(t *testing.T, db *DB) tcase {
			ds := prep(t, db, `{"username":"user"}`)
			before := fetchDataSourceCredentials(t, db, ds.ID)

			return tcase{
				Down: true,
				Check: func(t *testing.T, db *DB) {
					assert.Equal(t, before, fetchDataSourceCredentials(t, db, ds.ID))
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

			tx, err := db.sql.BeginTx(ctx, nil)
			require.NoError(t, err)

			t.Cleanup(func() {
				tx.Rollback() //nolint:errcheck,gosec // the transaction is committed or the test failed
			})

			if c.CancelledContext {
				cancel()
			}

			m := migrations.Go(migrations.Deps{DataSourceKeys: keys})[0]
			require.Equal(t, int64(13), m.Version)

			run := m.UpFnContext
			if c.Down {
				run = m.DownFnContext
			}

			err = run(ctx, tx)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			require.NoError(t, tx.Commit())

			c.Check(t, db)
		})
	}
}

func Test_backfillDocumentCommentRevisions(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Down             bool
		Err              error
		// Check inspects the rows the migration left behind.
		Check func(t *testing.T, db *DB)
	}

	// prep stores a comment with a reply and drops the revisions the
	// write recorded, so the rows look as they did before migration 015.
	prep := func(t *testing.T, db *DB) (comment.Comment, comment.Reply) {
		t.Helper()

		c := prepDocumentComments(t, db, 1, func(_ int, c *comment.Comment) {
			c.UpdatedAt = null.TimeFrom(c.CreatedAt.Add(time.Minute))
		})[0]
		r := prepDocumentCommentReplies(t, db, 1, func(_ int, r *comment.Reply) {
			r.CommentID = c.ID
			r.OrganizationID = c.OrganizationID
		})[0]

		_, err := db.sql.Exec("DELETE FROM document_comment_revisions")
		require.NoError(t, err)

		_, err = db.sql.Exec("DELETE FROM document_comment_reply_revisions")
		require.NoError(t, err)

		return c, r
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			prep(t, db)

			return tcase{
				CancelledContext: true,
				Err:              assert.AnError,
			}
		},
		"Every comment and reply gains a revision of its text": func(t *testing.T, db *DB) tcase {
			c, r := prep(t, db)

			return tcase{
				Check: func(t *testing.T, db *DB) {
					var revs []comment.Revision

					require.NoError(t, db.sql.Select(&revs, "SELECT * FROM document_comment_revisions"))
					require.Len(t, revs, 1)
					assert.Equal(t, c.ID, revs[0].CommentID)
					assert.Equal(t, c.OrganizationID, revs[0].OrganizationID)
					assert.Equal(t, c.Content, revs[0].Content)
					assert.Equal(t, c.UserID, revs[0].UserID)
					assert.Equal(t, c.UpdatedAt.Time, revs[0].CreatedAt)

					var replyRevs []comment.ReplyRevision

					require.NoError(t, db.sql.Select(&replyRevs, "SELECT * FROM document_comment_reply_revisions"))
					require.Len(t, replyRevs, 1)
					assert.Equal(t, r.ID, replyRevs[0].ReplyID)
					assert.Equal(t, r.Content, replyRevs[0].Content)
					assert.Equal(t, r.CreatedAt, replyRevs[0].CreatedAt)

					// the ids are real xids, so they carry the time they
					// were made.
					assert.WithinDuration(t, timeutil.Now(), revs[0].ID.Time(), time.Minute)
				},
			}
		},
		"Deleted comments and replies gain no revision": func(t *testing.T, db *DB) tcase {
			c, r := prep(t, db)
			require.NoError(t, db.DeleteDocumentCommentReply(context.Background(), r.ID, r.CommentID, r.OrganizationID))
			require.NoError(t, db.DeleteDocumentComment(context.Background(), c.ID, c.DocumentID, c.OrganizationID))

			return tcase{
				Check: func(t *testing.T, db *DB) {
					var n int

					require.NoError(t, db.sql.Get(&n, `SELECT
						(SELECT count(*) FROM document_comment_revisions) +
						(SELECT count(*) FROM document_comment_reply_revisions)`))
					assert.Zero(t, n)
				},
			}
		},
		"No comments": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				Check: func(t *testing.T, db *DB) {
					var n int

					require.NoError(t, db.sql.Get(&n, "SELECT count(*) FROM document_comment_revisions"))
					assert.Zero(t, n)
				},
			}
		},
		"Down removes every revision": func(t *testing.T, db *DB) tcase {
			prepDocumentCommentReplies(t, db, 1, nil)

			return tcase{
				Down: true,
				Check: func(t *testing.T, db *DB) {
					var n int

					require.NoError(t, db.sql.Get(&n, `SELECT
						(SELECT count(*) FROM document_comment_revisions) +
						(SELECT count(*) FROM document_comment_reply_revisions)`))
					assert.Zero(t, n)
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

			tx, err := db.sql.BeginTx(ctx, nil)
			require.NoError(t, err)

			t.Cleanup(func() {
				tx.Rollback() //nolint:errcheck,gosec // the transaction is committed or the test failed
			})

			if c.CancelledContext {
				cancel()
			}

			m := migrations.Go(migrations.Deps{})[1]
			require.Equal(t, int64(15), m.Version)

			run := m.UpFnContext
			if c.Down {
				run = m.DownFnContext
			}

			err = run(ctx, tx)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			require.NoError(t, tx.Commit())

			c.Check(t, db)
		})
	}
}

func Test_backfillDocumentActivityEntries(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Down             bool
		Err              error
		// Check inspects the rows the migration left behind.
		Check func(t *testing.T, db *DB)
	}

	// countEntries returns how many activity entries are stored.
	countEntries := func(t *testing.T, db *DB) int {
		t.Helper()

		var n int
		require.NoError(t, db.sql.Get(&n, "SELECT count(*) FROM document_activity_entries"))

		return n
	}

	// user returns the actor of the user.
	user := func(userID string) activity.Actor {
		return activity.Actor{
			Kind:   activity.ActorKindUser,
			UserID: null.StringFrom(userID),
		}
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			prepDocuments(t, db, 1, nil)

			return tcase{
				CancelledContext: true,
				Err:              assert.AnError,
			}
		},
		"No rows": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				Check: func(t *testing.T, db *DB) {
					assert.Zero(t, countEntries(t, db))
				},
			}
		},
		"Existing rows gain the entries they can tell": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 2)
			alice, bob := users[0], users[1]

			doc := prepDocuments(t, db, 1, func(_ int, d *document.Document) {
				d.CreatedBy = null.StringFrom(alice)
			})[0]
			fork := prepDocumentBranches(t, db, 1, func(_ int, d *document.Document) {
				d.ID = doc.ID
				d.OrganizationID = doc.OrganizationID
				d.CreatedBy = null.StringFrom(alice)
				d.CreatedAt = doc.CreatedAt.Add(time.Minute)
			})[0]
			c := prepDocumentComments(t, db, 1, func(_ int, c *comment.Comment) {
				c.BranchID = fork.BranchID
				c.DocumentID = fork.ID
				c.OrganizationID = fork.OrganizationID
				c.UserID = null.StringFrom(bob)
				c.CreatedAt = fork.CreatedAt.Add(time.Minute)
				c.Resolved = true
				c.ResolvedBy = null.StringFrom(alice)
				c.ResolvedAt = null.TimeFrom(c.CreatedAt.Add(2 * time.Minute))
			})[0]
			r := prepDocumentCommentReplies(t, db, 1, func(_ int, r *comment.Reply) {
				r.CommentID = c.ID
				r.OrganizationID = c.OrganizationID
				r.UserID = null.StringFrom(alice)
				r.CreatedAt = c.CreatedAt.Add(time.Minute)
			})[0]

			rev := fetchFirstDocumentCommentRevision(t, db, c.ID)
			replyRev := fetchFirstDocumentCommentReplyRevision(t, db, r.ID)

			return tcase{
				Check: func(t *testing.T, db *DB) {
					main := fetchStoredDocumentActivityEntries(t, db, doc.OrganizationID, doc.BranchID)
					require.Len(t, main, 1)
					assert.Equal(t, user(alice), main[0].Actor)
					assert.Equal(t, activity.TypeDocumentCreated, main[0].Type)
					assert.Equal(t, activity.CategoryDocument, main[0].Category)
					assert.Equal(t, activity.Target{}, main[0].Target)
					assert.Equal(t, activity.Metadata{}, main[0].Metadata)
					assert.Equal(t, doc.ID, main[0].DocumentID)
					assert.Equal(t, doc.OrganizationID, main[0].OrganizationID)
					assert.Equal(t, doc.CreatedAt, main[0].CreatedAt)
					assert.Equal(t, doc.CreatedAt, main[0].UpdatedAt)

					// the ids are real xids, so they carry the time they
					// were made.
					assert.WithinDuration(t, timeutil.Now(), main[0].ID.Time(), time.Minute)

					entries := fetchStoredDocumentActivityEntries(t, db, fork.OrganizationID, fork.BranchID)
					require.Len(t, entries, 4)

					assert.Equal(t, user(alice), entries[0].Actor)
					assert.Equal(t, activity.TypeBranchForked, entries[0].Type)
					assert.Equal(t, activity.CategoryBranch, entries[0].Category)
					assert.Equal(t, activity.Metadata{activity.MetaKeyToBranchID: fork.BranchID.String()}, entries[0].Metadata)
					assert.Equal(t, fork.CreatedAt, entries[0].CreatedAt)

					assert.Equal(t, user(bob), entries[1].Actor)
					assert.Equal(t, activity.TypeCommentCreated, entries[1].Type)
					assert.Equal(t, activity.CategoryComment, entries[1].Category)
					assert.Equal(t, activity.NewTarget(activity.TargetKindCommentRevision, rev.ID.String()), entries[1].Target)
					assert.Equal(t, activity.Metadata{activity.MetaKeyCommentID: c.ID.String()}, entries[1].Metadata)
					assert.Equal(t, c.CreatedAt, entries[1].CreatedAt)

					assert.Equal(t, user(alice), entries[2].Actor)
					assert.Equal(t, activity.TypeCommentReplied, entries[2].Type)
					assert.Equal(t, activity.NewTarget(activity.TargetKindReplyRevision, replyRev.ID.String()), entries[2].Target)
					assert.Equal(t, activity.Metadata{
						activity.MetaKeyCommentID: c.ID.String(),
						activity.MetaKeyReplyID:   r.ID.String(),
					}, entries[2].Metadata)
					assert.Equal(t, r.CreatedAt, entries[2].CreatedAt)

					assert.Equal(t, user(alice), entries[3].Actor)
					assert.Equal(t, activity.TypeCommentResolved, entries[3].Type)
					assert.Equal(t, activity.NewTarget(activity.TargetKindCommentRevision, rev.ID.String()), entries[3].Target)
					assert.Equal(t, c.ResolvedAt.Time, entries[3].CreatedAt)
				},
			}
		},
		"A comment's creation points at its oldest revision and its resolution at its newest": func(t *testing.T, db *DB) tcase {
			c := prepDocumentComments(t, db, 1, nil)[0]
			first := fetchFirstDocumentCommentRevision(t, db, c.ID)

			edited := c
			edited.Content = comment.Content{"text": "edited"}
			edited.UpdatedAt = null.TimeFrom(c.CreatedAt.Add(time.Minute))
			_, err := db.UpdateDocumentComment(context.Background(), edited)
			require.NoError(t, err)

			edited.Resolved = true
			edited.ResolvedAt = null.TimeFrom(c.CreatedAt.Add(2 * time.Minute))
			_, err = db.UpdateDocumentCommentStatus(context.Background(), edited)
			require.NoError(t, err)

			var newest xid.ID
			require.NoError(t, db.sql.Get(
				&newest,
				"SELECT id FROM document_comment_revisions WHERE fk_document_comment_id = $1 ORDER BY created_at DESC LIMIT 1",
				c.ID,
			))

			return tcase{
				Check: func(t *testing.T, db *DB) {
					// the comment's branch is a fork, so its first entry is
					// the fork.
					entries := fetchStoredDocumentActivityEntries(t, db, c.OrganizationID, c.BranchID)
					require.Len(t, entries, 3)
					assert.Equal(t, activity.TypeCommentCreated, entries[1].Type)
					assert.Equal(t, activity.NewTarget(activity.TargetKindCommentRevision, first.ID.String()), entries[1].Target)
					assert.Equal(t, activity.TypeCommentResolved, entries[2].Type)
					assert.Equal(t, activity.NewTarget(activity.TargetKindCommentRevision, newest.String()), entries[2].Target)
				},
			}
		},
		"Deleted comments and replies gain no entries": func(t *testing.T, db *DB) tcase {
			r := prepDocumentCommentReplies(t, db, 1, nil)[0]
			c := fetchStoredDocumentComment(t, db, r.CommentID)
			require.NoError(t, db.DeleteDocumentCommentReply(context.Background(), r.ID, r.CommentID, r.OrganizationID))
			require.NoError(t, db.DeleteDocumentComment(context.Background(), c.ID, c.DocumentID, c.OrganizationID))

			return tcase{
				Check: func(t *testing.T, db *DB) {
					var n int

					require.NoError(t, db.sql.Get(&n, "SELECT count(*) FROM document_activity_entries WHERE category = 'comment'"))
					assert.Zero(t, n)
				},
			}
		},
		"Down removes every entry": func(t *testing.T, db *DB) tcase {
			prepDocumentActivityEntries(t, db, 2, nil)

			return tcase{
				Down: true,
				Check: func(t *testing.T, db *DB) {
					assert.Zero(t, countEntries(t, db))
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

			tx, err := db.sql.BeginTx(ctx, nil)
			require.NoError(t, err)

			t.Cleanup(func() {
				tx.Rollback() //nolint:errcheck,gosec // the transaction is committed or the test failed
			})

			if c.CancelledContext {
				cancel()
			}

			m := migrations.Go(migrations.Deps{})[2]
			require.Equal(t, int64(17), m.Version)

			run := m.UpFnContext
			if c.Down {
				run = m.DownFnContext
			}

			err = run(ctx, tx)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			require.NoError(t, tx.Commit())

			c.Check(t, db)
		})
	}
}

func Test_DB_moveSQLMigrateHistory(t *testing.T) {
	// sql-migrate recorded these ids before goose replaced it, so its
	// history can never list anything newer. A database with the full
	// history is therefore at version 12, and later migrations still run.
	ids := []string{
		"001_initial.sql",
		"002_history_entry_snapshot.sql",
		"003_document_hook_cross_branch_id.sql",
		"004_history_entry_updated_at.sql",
		"005_hook_status.sql",
		"006_document_branch_views.sql",
		"007_document_branch_view_from.sql",
		"008_personal_variables.sql",
		"009_drop_search_jobs.sql",
		"010_palette_color_names.sql",
		"011_hook_triggered.sql",
		"012_member_roles.sql",
	}

	type tcase struct {
		// DB, when set, replaces the database the case was given.
		DB       *DB
		Versions []int64
		Err      error
		// Check, when set, inspects the database after a failed move.
		Check func(t *testing.T, db *DB)
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"No sql-migrate table": func(_ *testing.T, db *DB) tcase {
			return tcase{
				Versions: allVersions(db),
			}
		},
		"Full history": func(t *testing.T, db *DB) tcase {
			_, err := db.migrations.DownTo(context.Background(), 12)
			require.NoError(t, err)

			_, err = db.sql.Exec("DROP TABLE goose_db_version")
			require.NoError(t, err)

			prepSQLMigrateHistory(t, db, ids...)

			return tcase{
				Versions: []int64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			}
		},
		"History behind the migrations": func(t *testing.T, db *DB) tcase {
			_, err := db.migrations.DownTo(context.Background(), 10)
			require.NoError(t, err)

			_, err = db.sql.Exec("DROP TABLE goose_db_version")
			require.NoError(t, err)

			prepSQLMigrateHistory(t, db, ids[:10]...)

			return tcase{
				Versions: []int64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			}
		},
		"History outside the public schema": func(t *testing.T, db *DB) tcase {
			_, err := db.migrations.DownTo(context.Background(), 12)
			require.NoError(t, err)

			_, err = db.sql.Exec("DROP TABLE goose_db_version")
			require.NoError(t, err)

			prepSQLMigrateHistory(t, db, ids...)

			_, err = db.sql.Exec("ALTER SCHEMA public RENAME TO oxynote")
			require.NoError(t, err)

			var name string

			require.NoError(t, db.sql.Get(&name, "SELECT current_database()"))

			// New moves the history itself, so the move below finds nothing
			// left to do. New fails when the move misses the history,
			// because 001_initial runs again over the existing tables.
			sdb, err := New(slog.New(slog.DiscardHandler), metricutil.NewFactory("test", nil), Options{
				DSN:                       fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable&search_path=oxynote", _pgUser, _pgPass, _pgDSN, name),
				DataSourceCredentialsKeys: _dataSourceKeys,
			})
			require.NoError(t, err)

			t.Cleanup(func() {
				sdb.Close() //nolint:errcheck,gosec // error provides no meaningful info
			})

			return tcase{
				DB:       sdb,
				Versions: allVersions(sdb),
			}
		},
		"History already in the goose table": func(t *testing.T, db *DB) tcase {
			prepSQLMigrateHistory(t, db, ids...)

			return tcase{
				Versions: allVersions(db),
			}
		},
		"Error returned by BeginTx": func(t *testing.T, db *DB) tcase {
			require.NoError(t, db.Close())

			return tcase{
				Err: assert.AnError,
			}
		},
		"Id without a version": func(t *testing.T, db *DB) tcase {
			_, err := db.sql.Exec("DROP TABLE goose_db_version")
			require.NoError(t, err)

			prepSQLMigrateHistory(t, db, "initial.sql")

			return tcase{
				Err: assert.AnError,
				Check: func(t *testing.T, db *DB) {
					t.Helper()

					var hasHistory, hasVersionTable bool

					require.NoError(t, db.sql.QueryRow(
						"SELECT to_regclass('migrations') IS NOT NULL, to_regclass('goose_db_version') IS NOT NULL",
					).Scan(&hasHistory, &hasVersionTable))
					assert.True(t, hasHistory)
					assert.False(t, hasVersionTable)
				},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			if c.DB != nil {
				db = c.DB
			}

			err := db.moveSQLMigrateHistory(context.Background())
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				if c.Check != nil {
					c.Check(t, db)
				}

				return
			}

			var hasHistory bool

			require.NoError(t, db.sql.Get(&hasHistory, "SELECT to_regclass('migrations') IS NOT NULL"))
			assert.False(t, hasHistory)
			assert.Equal(t, c.Versions, recordedVersions(t, db))

			// the first migration fails on its existing tables, so a clean
			// run proves that no recorded migration runs again.
			require.NoError(t, db.migrateUp())
			assert.Equal(t, allVersions(db), recordedVersions(t, db))
		})
	}
}

func Test_DB_BeginTx(t *testing.T) {
	cc := map[string]struct {
		CancelledContext bool
		Err              error
	}{
		"Cancelled context": {
			CancelledContext: true,
			Err:              assert.AnError,
		},
		"Successful begin": {},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.CancelledContext {
				cancel()
			}

			tx, err := db.BeginTx(ctx)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.IsType(t, &Tx{}, tx)
			assert.NoError(t, tx.Commit())
		})
	}
}

func Test_Tx_SQLTx(t *testing.T) {
	db := prepTempDB(t)

	ntx, err := sqlutil.BeginTx[*Tx](context.Background(), db)
	require.NoError(t, err)

	defer ntx.Rollback() //nolint:errcheck // error provides no meaningful info

	stx := ntx.SQLTx()
	require.NotNil(t, stx)
	assert.Same(t, ntx.sql.Tx, stx)
}

func Test_Tx_Commit(t *testing.T) {
	db := prepTempDB(t)

	ntx, err := db.BeginTx(context.Background())
	require.NoError(t, err)
	require.NoError(t, ntx.Commit())
	require.Error(t, ntx.Commit())
}

func Test_Tx_Rollback(t *testing.T) {
	db := prepTempDB(t)

	ntx, err := db.BeginTx(context.Background())
	require.NoError(t, err)
	require.NoError(t, ntx.Rollback())
	require.Error(t, ntx.Rollback())
}

// prepMockDB creates an agent backed by sqlmock for reaching error
// branches that a real database cannot produce (e.g. failures in the
// middle of a multi-statement transaction).
func prepMockDB(t *testing.T) (*agent, sqlmock.Sqlmock) {
	t.Helper()

	mockDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)

	t.Cleanup(func() {
		mockDB.Close() //nolint:errcheck,gosec // error provides no meaningful info
		require.NoError(t, mock.ExpectationsWereMet())
	})

	return &agent{
		sql:     sqlx.NewDb(mockDB, "sqlmock"),
		builder: sq.StatementBuilderType(builder.EmptyBuilder).PlaceholderFormat(sq.Dollar),
	}, mock
}

func prepTempDB(t *testing.T) *DB {
	name := uniuri.NewLenChars(10, []byte("abcdefghijklmnopqrstuvwxyz"))

	opts := Options{
		DSN:                       fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", _pgUser, _pgPass, _pgDSN, name),
		DataSourceCredentialsKeys: _dataSourceKeys,
	}

	tmpDB, err := sqlx.Connect(
		"pgx",
		fmt.Sprintf("postgres://%s:%s@%s/postgres?sslmode=disable", _pgUser, _pgPass, _pgDSN),
	)
	require.NoError(t, err)

	_, err = tmpDB.Exec("CREATE DATABASE " + name)
	require.NoError(t, err)
	require.NoError(t, tmpDB.Close())

	db, err := New(slog.New(slog.DiscardHandler), metricutil.NewFactory("test", nil), opts)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close() //nolint:errcheck,gosec // error provides no meaningful info
	})

	return db
}

// prepSQLMigrateHistory creates the migrations table that rubenv/sql-migrate
// kept and records the given ids in it.
func prepSQLMigrateHistory(t *testing.T, db *DB, ids ...string) {
	t.Helper()

	_, err := db.sql.Exec("CREATE TABLE migrations (id text PRIMARY KEY, applied_at timestamptz)")
	require.NoError(t, err)

	for _, id := range ids {
		_, err = db.sql.Exec("INSERT INTO migrations (id, applied_at) VALUES ($1, now())", id)
		require.NoError(t, err)
	}
}

// recordedVersions returns the versions in the goose version table, in
// ascending order and with any duplicate kept.
func recordedVersions(t *testing.T, db *DB) []int64 {
	t.Helper()

	var versions []int64

	require.NoError(t, db.sql.Select(&versions, "SELECT version_id FROM goose_db_version ORDER BY version_id"))

	return versions
}

// allVersions returns version 0 followed by the version of every migration
// goose knows, which is what the version table holds once all are applied.
func allVersions(db *DB) []int64 {
	sources := db.migrations.ListSources()
	versions := make([]int64, 1, 1+len(sources))

	for _, s := range sources {
		versions = append(versions, s.Version)
	}

	return versions
}
