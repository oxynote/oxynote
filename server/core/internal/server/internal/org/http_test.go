package org

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/datasource/demo"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/internal/storage"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/internal/variable"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.IgnoreBleveWorkers())
}

// _testPNG is a data prefix carrying the PNG magic bytes so content
// type sniffing detects image/png.
var _testPNG = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{1}, 1024)...)

func Test_NewHandler(t *testing.T) {
	t.Parallel()

	db := &DBMock{}
	storer := &StorerMock{}

	githubMan := &github.Manager{}
	webchangeClient := &webchange.Client{}

	searchQueue := &SearchQueueMock{}

	hdl := NewHandler(
		slog.New(slog.DiscardHandler),
		db,
		storer,
		githubMan,
		webchangeClient,
		searchQueue,
		"loc",
	)
	require.NotNil(t, hdl)
	assert.NotNil(t, hdl.log)
	assert.Same(t, db, hdl.db)
	assert.Same(t, storer, hdl.storer)
	assert.Same(t, githubMan, hdl.githubMan)
	assert.Same(t, webchangeClient, hdl.webchangeClient)
	assert.Same(t, searchQueue, hdl.searchQueue)
	assert.Equal(t, "loc", hdl.publicURL)
}

// assertSeededTags checks that the seeded tags were inserted in their
// display order, each owned by the organization and its first member.
func assertSeededTags(t *testing.T, tx *TxMock, count int) {
	t.Helper()

	ff := tx.InsertTagCalls()
	require.Len(t, ff, count)

	if count == 0 {
		return
	}

	names := make([]string, 0, len(ff))
	colors := make([]string, 0, len(ff))

	for _, f := range ff {
		names = append(names, f.T.TagName)
		colors = append(colors, f.T.Color)
		assert.Equal(t, "org2", f.T.OrganizationID)
		assert.Equal(t, null.StringFrom("member1"), f.T.CreatedBy)
	}

	assert.Equal(t, []string{"Production", "Staging", "Incidents"}[:count], names)
	assert.Equal(t, []string{"green", "orange", "blue"}[:count], colors)
}

// assertWelcomeDocumentTagged checks that the welcome document's default
// branch went under the first seeded tag, and under it alone.
func assertWelcomeDocumentTagged(t *testing.T, tx *TxMock, count int) {
	t.Helper()

	ff := tx.AssignBranchTagCalls()
	require.Len(t, ff, count)

	if count == 0 {
		return
	}

	doc := tx.InsertDocumentCalls()[0].Doc

	assert.Equal(t, "org2", ff[0].OrganizationID)
	assert.Equal(t, doc.ID, ff[0].DocumentID)
	assert.Equal(t, doc.BranchID, ff[0].BranchID)
	assert.Equal(t, tx.InsertTagCalls()[0].T.ID, ff[0].TagID)
}

// assertSeededVariables checks that the built-in variables went into the
// transaction in order, each of the organization and its built-in group.
func assertSeededVariables(t *testing.T, tx *TxMock, count int) {
	t.Helper()

	ff := tx.CreateVariableCalls()
	require.Len(t, ff, count)

	if count == 0 {
		return
	}

	gg := tx.CreateVariableGroupCalls()
	require.NotEmpty(t, gg)

	keys := make([]string, 0, len(ff))

	for _, f := range ff {
		keys = append(keys, f.V.Key)
		assert.Equal(t, gg[0].G.ID, f.V.GroupID)
		assert.Equal(t, "org2", f.V.OrganizationID)
	}

	assert.Equal(t, []string{"metric_range", "metric_refresh"}[:count], keys)
}

func Test_Handler_InitializeOrganization(t *testing.T) {
	type check func(*testing.T, *DBMock, *TxMock, *SearchQueueMock, *httptest.ResponseRecorder)

	checks := func(cc ...check) []check { return cc }

	hasResp := func(code int, body string) check {
		return func(t *testing.T, _ *DBMock, _ *TxMock, _ *SearchQueueMock, rec *httptest.ResponseRecorder) {
			testutil.AssertResponse(t, rec, code, body)
		}
	}

	wasFetchMembersCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *TxMock, _ *SearchQueueMock, _ *httptest.ResponseRecorder) {
			assert.Len(t, db.FetchOrganizationMembersCalls(), count)
		}
	}

	wasBeginTxCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *TxMock, _ *SearchQueueMock, _ *httptest.ResponseRecorder) {
			assert.Len(t, db.BeginTxCalls(), count)
		}
	}

	wasInsertDataSourceCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *TxMock, _ *SearchQueueMock, _ *httptest.ResponseRecorder) {
			ff := db.InsertDataSourceCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "Demo", ff[0].Ds.Name)
			assert.Equal(t, datasource.TypePrometheus, ff[0].Ds.Type)
			assert.Equal(t, demo.URL, ff[0].Ds.URL)
			assert.Equal(t, "org2", ff[0].Ds.OrganizationID)
		}
	}

	wasInsertDocumentCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, tx *TxMock, _ *SearchQueueMock, _ *httptest.ResponseRecorder) {
			ff := tx.InsertDocumentCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "Welcome to Oxynote!", ff[0].Doc.DocumentName)
			assert.Equal(t, "org2", ff[0].Doc.OrganizationID)
			assert.NotEmpty(t, ff[0].Doc.Content)
		}
	}

	wasUpsertMaintainersCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, tx *TxMock, _ *SearchQueueMock, _ *httptest.ResponseRecorder) {
			ff := tx.UpsertDocumentMaintainersCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "org2", ff[0].OrganizationID)
			assert.Equal(t, []string{"member1"}, ff[0].MaintainerIDs)
		}
	}

	wasInsertTagCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, tx *TxMock, _ *SearchQueueMock, _ *httptest.ResponseRecorder) {
			assertSeededTags(t, tx, count)
		}
	}

	wasAssignBranchTagCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, tx *TxMock, _ *SearchQueueMock, _ *httptest.ResponseRecorder) {
			assertWelcomeDocumentTagged(t, tx, count)
		}
	}

	wasCreateVariableGroupCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, tx *TxMock, _ *SearchQueueMock, _ *httptest.ResponseRecorder) {
			ff := tx.CreateVariableGroupCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "Metrics", ff[0].G.GroupName)
			assert.Equal(t, "org2", ff[0].G.OrganizationID)
			assert.True(t, ff[0].G.Builtin)
		}
	}

	wasCreateVariableCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, tx *TxMock, _ *SearchQueueMock, _ *httptest.ResponseRecorder) {
			assertSeededVariables(t, tx, count)
		}
	}

	wasEnqueueCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, tx *TxMock, queue *SearchQueueMock, _ *httptest.ResponseRecorder) {
			ff := queue.EnqueueCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Same(t, tx, ff[0].Tx)
		}
	}

	wasCommitCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, tx *TxMock, _ *SearchQueueMock, _ *httptest.ResponseRecorder) {
			assert.Len(t, tx.CommitCalls(), count)
		}
	}

	wasRollbackCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, tx *TxMock, _ *SearchQueueMock, _ *httptest.ResponseRecorder) {
			assert.Len(t, tx.RollbackCalls(), count)
		}
	}

	cc := map[string]struct {
		DB         *DBMock
		Tx         *TxMock
		Queue      *SearchQueueMock
		Members    []string
		MembersErr error
		BeginErr   error
		OmitID     bool
		Checks     []check
	}{
		"Missing organization ID parameter": {
			Tx:     &TxMock{},
			OmitID: true,
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasFetchMembersCalled(0),
				wasInsertDataSourceCalled(0),
				wasBeginTxCalled(0),
				wasInsertDocumentCalled(0),
				wasUpsertMaintainersCalled(0),
				wasInsertTagCalled(0),
				wasAssignBranchTagCalled(0),
				wasCreateVariableGroupCalled(0),
				wasCreateVariableCalled(0),
				wasEnqueueCalled(0),
				wasCommitCalled(0),
				wasRollbackCalled(0),
			),
		},
		"Error returned by DB.BeginTx": {
			Tx:       &TxMock{},
			Members:  []string{"member1"},
			BeginErr: assert.AnError,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchMembersCalled(1),
				wasInsertDataSourceCalled(1),
				wasBeginTxCalled(1),
				wasInsertDocumentCalled(0),
				wasUpsertMaintainersCalled(0),
				wasInsertTagCalled(0),
				wasAssignBranchTagCalled(0),
				wasCreateVariableGroupCalled(0),
				wasCreateVariableCalled(0),
				wasEnqueueCalled(0),
				wasCommitCalled(0),
				wasRollbackCalled(0),
			),
		},
		// both of these fail before anything is written, so no data source
		// is created and no transaction is opened.
		"Error returned by DB.FetchOrganizationMembers": {
			Tx:         &TxMock{},
			MembersErr: assert.AnError,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchMembersCalled(1),
				wasInsertDataSourceCalled(0),
				wasBeginTxCalled(0),
				wasInsertDocumentCalled(0),
				wasUpsertMaintainersCalled(0),
				wasInsertTagCalled(0),
				wasAssignBranchTagCalled(0),
				wasCreateVariableGroupCalled(0),
				wasCreateVariableCalled(0),
				wasEnqueueCalled(0),
				wasCommitCalled(0),
				wasRollbackCalled(0),
			),
		},
		"No organization members": {
			Tx: &TxMock{},
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"organization.no_members","message":"organization has no members"}`),
				wasFetchMembersCalled(1),
				wasInsertDataSourceCalled(0),
				wasBeginTxCalled(0),
				wasInsertDocumentCalled(0),
				wasUpsertMaintainersCalled(0),
				wasInsertTagCalled(0),
				wasAssignBranchTagCalled(0),
				wasCreateVariableGroupCalled(0),
				wasCreateVariableCalled(0),
				wasEnqueueCalled(0),
				wasCommitCalled(0),
				wasRollbackCalled(0),
			),
		},
		"Error returned by DB.InsertDataSource is tolerated": {
			Members: []string{"member1"},
			DB: &DBMock{
				InsertDataSourceFunc: func(context.Context, *datasource.DataSource) error {
					return assert.AnError
				},
			},
			Tx: &TxMock{},
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *TxMock, _ *SearchQueueMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusCreated, rec.Code)
				},
				wasFetchMembersCalled(1),
				wasInsertDataSourceCalled(1),
				wasBeginTxCalled(1),
				wasInsertDocumentCalled(1),
				wasUpsertMaintainersCalled(1),
				wasInsertTagCalled(3),
				wasAssignBranchTagCalled(1),
				wasCreateVariableGroupCalled(1),
				wasCreateVariableCalled(2),
				wasEnqueueCalled(1),
				wasCommitCalled(1),
				wasRollbackCalled(1),
				// the charts have no data source to read, so the welcome
				// document ships without them rather than with a dangling
				// reference.
				func(t *testing.T, _ *DBMock, tx *TxMock, _ *SearchQueueMock, _ *httptest.ResponseRecorder) {
					ff := tx.InsertDocumentCalls()
					require.NotEmpty(t, ff)

					for _, b := range ff[0].Doc.Content.Content {
						assert.NotEqual(t, document.BlockNodeMetricBlock, b.Type)
					}
				},
			),
		},
		"Error returned by Tx.InsertDocument": {
			Members: []string{"member1"},
			Tx: &TxMock{
				InsertDocumentFunc: func(context.Context, document.Document) error {
					return assert.AnError
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchMembersCalled(1),
				wasInsertDataSourceCalled(1),
				wasBeginTxCalled(1),
				wasInsertDocumentCalled(1),
				wasUpsertMaintainersCalled(0),
				wasInsertTagCalled(0),
				wasAssignBranchTagCalled(0),
				wasCreateVariableGroupCalled(0),
				wasCreateVariableCalled(0),
				wasEnqueueCalled(0),
				wasCommitCalled(0),
				wasRollbackCalled(1),
			),
		},
		"Error returned by Tx.UpsertDocumentMaintainers": {
			Members: []string{"member1"},
			Tx: &TxMock{
				UpsertDocumentMaintainersFunc: func(context.Context, xid.ID, string, []string) error {
					return assert.AnError
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchMembersCalled(1),
				wasInsertDataSourceCalled(1),
				wasBeginTxCalled(1),
				wasInsertDocumentCalled(1),
				wasUpsertMaintainersCalled(1),
				wasInsertTagCalled(0),
				wasAssignBranchTagCalled(0),
				wasCreateVariableGroupCalled(0),
				wasCreateVariableCalled(0),
				wasEnqueueCalled(0),
				wasCommitCalled(0),
				wasRollbackCalled(1),
			),
		},
		"Error returned by Tx.InsertTag": {
			Members: []string{"member1"},
			Tx: &TxMock{
				InsertTagFunc: func(context.Context, tag.Tag) error {
					return assert.AnError
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchMembersCalled(1),
				wasInsertDataSourceCalled(1),
				wasBeginTxCalled(1),
				wasInsertDocumentCalled(1),
				wasUpsertMaintainersCalled(1),
				wasInsertTagCalled(1),
				wasAssignBranchTagCalled(0),
				wasCreateVariableGroupCalled(0),
				wasCreateVariableCalled(0),
				wasEnqueueCalled(0),
				wasCommitCalled(0),
				wasRollbackCalled(1),
			),
		},
		"Error returned by Tx.AssignBranchTag": {
			Members: []string{"member1"},
			Tx: &TxMock{
				AssignBranchTagFunc: func(context.Context, string, xid.ID, xid.ID, xid.ID) error {
					return assert.AnError
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchMembersCalled(1),
				wasInsertDataSourceCalled(1),
				wasBeginTxCalled(1),
				wasInsertDocumentCalled(1),
				wasUpsertMaintainersCalled(1),
				wasInsertTagCalled(3),
				wasAssignBranchTagCalled(1),
				wasCreateVariableGroupCalled(0),
				wasCreateVariableCalled(0),
				wasEnqueueCalled(0),
				wasCommitCalled(0),
				wasRollbackCalled(1),
			),
		},
		"Error returned by Tx.CreateVariableGroup": {
			Members: []string{"member1"},
			Tx: &TxMock{
				CreateVariableGroupFunc: func(context.Context, variable.Group) error {
					return assert.AnError
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchMembersCalled(1),
				wasInsertDataSourceCalled(1),
				wasBeginTxCalled(1),
				wasInsertDocumentCalled(1),
				wasUpsertMaintainersCalled(1),
				wasInsertTagCalled(3),
				wasAssignBranchTagCalled(1),
				wasCreateVariableGroupCalled(1),
				wasCreateVariableCalled(0),
				wasEnqueueCalled(0),
				wasCommitCalled(0),
				wasRollbackCalled(1),
			),
		},
		"Error returned by Tx.CreateVariable": {
			Members: []string{"member1"},
			Tx: &TxMock{
				CreateVariableFunc: func(context.Context, variable.Variable) error {
					return assert.AnError
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchMembersCalled(1),
				wasInsertDataSourceCalled(1),
				wasBeginTxCalled(1),
				wasInsertDocumentCalled(1),
				wasUpsertMaintainersCalled(1),
				wasInsertTagCalled(3),
				wasAssignBranchTagCalled(1),
				wasCreateVariableGroupCalled(1),
				wasCreateVariableCalled(1),
				wasEnqueueCalled(0),
				wasCommitCalled(0),
				wasRollbackCalled(1),
			),
		},
		"Error returned by SearchQueue.Enqueue": {
			Members: []string{"member1"},
			Tx:      &TxMock{},
			Queue: &SearchQueueMock{
				EnqueueFunc: func(context.Context, sqlutil.Tx, search.Job) error {
					return assert.AnError
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchMembersCalled(1),
				wasInsertDataSourceCalled(1),
				wasBeginTxCalled(1),
				wasInsertDocumentCalled(1),
				wasUpsertMaintainersCalled(1),
				wasInsertTagCalled(3),
				wasAssignBranchTagCalled(1),
				wasCreateVariableGroupCalled(1),
				wasCreateVariableCalled(2),
				wasEnqueueCalled(1),
				wasCommitCalled(0),
				wasRollbackCalled(1),
			),
		},
		"Error returned by Tx.Commit": {
			Members: []string{"member1"},
			Tx: &TxMock{
				CommitFunc: func() error {
					return assert.AnError
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchMembersCalled(1),
				wasInsertDataSourceCalled(1),
				wasBeginTxCalled(1),
				wasInsertDocumentCalled(1),
				wasUpsertMaintainersCalled(1),
				wasInsertTagCalled(3),
				wasAssignBranchTagCalled(1),
				wasCreateVariableGroupCalled(1),
				wasCreateVariableCalled(2),
				wasEnqueueCalled(1),
				wasCommitCalled(1),
				wasRollbackCalled(1),
			),
		},
		"Successful initialization": {
			Members: []string{"member1", "member2"},
			Tx:      &TxMock{},
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *TxMock, _ *SearchQueueMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusCreated, rec.Code)
					assert.Contains(t, rec.Body.String(), "Welcome to Oxynote!")
				},
				wasFetchMembersCalled(1),
				wasInsertDataSourceCalled(1),
				wasBeginTxCalled(1),
				wasInsertDocumentCalled(1),
				wasUpsertMaintainersCalled(1),
				wasInsertTagCalled(3),
				wasAssignBranchTagCalled(1),
				wasCreateVariableGroupCalled(1),
				wasCreateVariableCalled(2),
				wasEnqueueCalled(1),
				wasCommitCalled(1),
				wasRollbackCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := c.DB
			if db == nil {
				db = &DBMock{}
			}

			db.FetchOrganizationMembersFunc = func(context.Context, string) ([]string, error) {
				return c.Members, c.MembersErr
			}
			db.BeginTxFunc = testutil.StubBeginTx(c.Tx, c.BeginErr)

			queue := c.Queue
			if queue == nil {
				queue = &SearchQueueMock{}
			}

			hdl := Handler{
				log:         slog.New(slog.DiscardHandler),
				db:          db,
				searchQueue: queue,
				publicURL:   "loc",
			}

			req := httptest.NewRequest(http.MethodPost, "http://test.com/", http.NoBody)

			ctx := req.Context()

			if !c.OmitID {
				ctx = testutil.AddChiCtx(ctx, "organizationId", "org2")
			}

			rec := httptest.NewRecorder()

			hdl.InitializeOrganization(rec, req.WithContext(ctx))

			for _, ch := range c.Checks {
				ch(t, db, c.Tx, queue, rec)
			}
		})
	}
}

func Test_Handler_UploadOrganizationLogo(t *testing.T) {
	type check func(*testing.T, *DBMock, *StorerMock, *httptest.ResponseRecorder)

	checks := func(cc ...check) []check { return cc }

	hasResp := func(code int, body string) check {
		return func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
			testutil.AssertResponse(t, rec, code, body)
		}
	}

	wasUploadCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, storer *StorerMock, _ *httptest.ResponseRecorder) {
			ff := storer.UploadCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "organizations/org1/logo", ff[0].Folder)
			assert.Equal(t, "org1", ff[0].ID)
			assert.Equal(t, _testPNG, ff[0].Data)
			assert.Equal(t, "image/png", ff[0].ContentType)
		}
	}

	wasUpdateLogoCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *StorerMock, _ *httptest.ResponseRecorder) {
			ff := db.UpdateOrganizationLogoCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "org1", ff[0].OrganizationID)
			assert.Regexp(t, `^loc/api/organizations/logo\?v=\d{14}$`, ff[0].Logo)
		}
	}

	wasDeleteCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, storer *StorerMock, _ *httptest.ResponseRecorder) {
			assert.Len(t, storer.DeleteCalls(), count)
		}
	}

	cc := map[string]struct {
		DB        *DBMock
		Storer    *StorerMock
		NoSession bool
		NoFile    bool
		// Content is the multipart file's bytes; nil sends a PNG.
		Content []byte
		Checks  []check
	}{
		"No session in context": {
			DB:        &DBMock{},
			Storer:    &StorerMock{},
			NoSession: true,
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasUploadCalled(0),
				wasUpdateLogoCalled(0),
				wasDeleteCalled(0),
			),
		},
		"Missing multipart logo field": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			NoFile: true,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_form","message":"invalid form data"}`),
				wasUploadCalled(0),
				wasUpdateLogoCalled(0),
				wasDeleteCalled(0),
			),
		},
		"Error returned by storage.ReadObject": {
			DB:      &DBMock{},
			Storer:  &StorerMock{},
			Content: []byte("plain text data"),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"storage.invalid_content_type","message":"invalid content type"}`),
				wasUploadCalled(0),
				wasUpdateLogoCalled(0),
				wasDeleteCalled(0),
			),
		},
		"Error returned by Storer.Upload": {
			DB: &DBMock{},
			Storer: &StorerMock{
				UploadFunc: func(context.Context, string, string, []byte, string) error {
					return assert.AnError
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasUploadCalled(1),
				wasUpdateLogoCalled(0),
				wasDeleteCalled(0),
			),
		},
		"Error returned by DB.UpdateOrganizationLogo": {
			DB: &DBMock{
				UpdateOrganizationLogoFunc: func(context.Context, string, string) error {
					return assert.AnError
				},
			},
			Storer: &StorerMock{},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasUploadCalled(1),
				wasUpdateLogoCalled(1),
				wasDeleteCalled(1),
			),
		},
		"Error returned by Storer.Delete after DB.UpdateOrganizationLogo": {
			DB: &DBMock{
				UpdateOrganizationLogoFunc: func(context.Context, string, string) error {
					return assert.AnError
				},
			},
			Storer: &StorerMock{
				DeleteFunc: func(context.Context, string, string) error {
					return assert.AnError
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasUploadCalled(1),
				wasUpdateLogoCalled(1),
				wasDeleteCalled(1),
			),
		},
		"Successful upload": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusCreated, rec.Code)
					assert.Zero(t, rec.Body.Len(), rec.Body.String())
					assert.Regexp(t, `^loc/api/organizations/logo\?v=\d{14}$`, rec.Header().Get("Location"))
				},
				wasUploadCalled(1),
				wasUpdateLogoCalled(1),
				wasDeleteCalled(0),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{
				log:       slog.New(slog.DiscardHandler),
				db:        c.DB,
				storer:    c.Storer,
				publicURL: "loc",
			}

			var (
				body        io.Reader = http.NoBody
				contentType string
			)

			if !c.NoFile {
				content := c.Content
				if content == nil {
					content = _testPNG
				}

				body, contentType = testutil.MultipartBody(t, "logo", "logo.png", content)
			}

			req := httptest.NewRequest(http.MethodPut, "http://test.com/", body)

			if contentType != "" {
				req.Header.Set("Content-Type", contentType)
			}

			if !c.NoSession {
				req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				}))
			}

			rec := httptest.NewRecorder()

			hdl.UploadOrganizationLogo(rec, req)

			for _, ch := range c.Checks {
				ch(t, c.DB, c.Storer, rec)
			}
		})
	}
}

func Test_Handler_storeLogo(t *testing.T) {
	type check func(*testing.T, *DBMock, *StorerMock, string, *bytes.Buffer)

	checks := func(cc ...check) []check { return cc }

	hasLocation := func(pattern string) check {
		return func(t *testing.T, _ *DBMock, _ *StorerMock, location string, _ *bytes.Buffer) {
			assert.Regexp(t, pattern, location)
		}
	}

	hasLog := func(line string) check {
		return func(t *testing.T, _ *DBMock, _ *StorerMock, _ string, buf *bytes.Buffer) {
			if line == "" {
				assert.Empty(t, buf.String())
				return
			}

			assert.Contains(t, buf.String(), line)
		}
	}

	wasUploadCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, storer *StorerMock, _ string, _ *bytes.Buffer) {
			ff := storer.UploadCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "organizations/org1/logo", ff[0].Folder)
			assert.Equal(t, "org1", ff[0].ID)
			assert.Equal(t, _testPNG, ff[0].Data)
			assert.Equal(t, "image/png", ff[0].ContentType)
		}
	}

	wasUpdateLogoCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *StorerMock, _ string, _ *bytes.Buffer) {
			ff := db.UpdateOrganizationLogoCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "org1", ff[0].OrganizationID)
			assert.Regexp(t, `^loc/api/organizations/logo\?v=\d{14}$`, ff[0].Logo)
		}
	}

	wasDeleteCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, storer *StorerMock, _ string, _ *bytes.Buffer) {
			ff := storer.DeleteCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "organizations/org1/logo", ff[0].Folder)
			assert.Equal(t, "org1", ff[0].ID)
		}
	}

	cc := map[string]struct {
		DB     *DBMock
		Storer *StorerMock
		Err    error
		Checks []check
	}{
		"Error returned by Storer.Upload": {
			DB: &DBMock{},
			Storer: &StorerMock{
				UploadFunc: func(context.Context, string, string, []byte, string) error {
					return assert.AnError
				},
			},
			Err: assert.AnError,
			Checks: checks(
				wasUploadCalled(1),
				wasUpdateLogoCalled(0),
				wasDeleteCalled(0),
			),
		},
		"Error returned by DB.UpdateOrganizationLogo": {
			DB: &DBMock{
				UpdateOrganizationLogoFunc: func(context.Context, string, string) error {
					return assert.AnError
				},
			},
			Storer: &StorerMock{},
			Err:    assert.AnError,
			Checks: checks(
				hasLog(""),
				wasUploadCalled(1),
				wasUpdateLogoCalled(1),
				wasDeleteCalled(1),
			),
		},
		"Error returned by Storer.Delete after DB.UpdateOrganizationLogo": {
			DB: &DBMock{
				UpdateOrganizationLogoFunc: func(context.Context, string, string) error {
					return assert.AnError
				},
			},
			Storer: &StorerMock{
				DeleteFunc: func(context.Context, string, string) error {
					return assert.AnError
				},
			},
			Err: assert.AnError,
			Checks: checks(
				hasLog("deleting object after DB failure"),
				wasUploadCalled(1),
				wasUpdateLogoCalled(1),
				wasDeleteCalled(1),
			),
		},
		"Successful store": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			Checks: checks(
				hasLocation(`^loc/api/organizations/logo\?v=\d{14}$`),
				hasLog(""),
				wasUploadCalled(1),
				wasUpdateLogoCalled(1),
				wasDeleteCalled(0),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			hdl := Handler{
				log:       slog.New(slog.NewTextHandler(&buf, nil)),
				db:        c.DB,
				storer:    c.Storer,
				publicURL: "loc",
			}

			location, err := hdl.storeLogo(context.Background(), "org1", _testPNG, "image/png")
			testutil.AssertEqualError(t, c.Err, err)

			for _, ch := range c.Checks {
				ch(t, c.DB, c.Storer, location, &buf)
			}
		})
	}
}

func Test_Handler_RetrieveOrganizationLogo(t *testing.T) {
	type check func(*testing.T, *StorerMock, *httptest.ResponseRecorder)

	checks := func(cc ...check) []check { return cc }

	wasRetrieveCalled := func(count int) check {
		return func(t *testing.T, storer *StorerMock, _ *httptest.ResponseRecorder) {
			ff := storer.RetrieveCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "organizations/org1/logo", ff[0].Folder)
			assert.Equal(t, "org1", ff[0].ID)
		}
	}

	cc := map[string]struct {
		Storer    *StorerMock
		NoSession bool
		NoneMatch string
		Checks    []check
	}{
		"No session in context": {
			Storer:    &StorerMock{},
			NoSession: true,
			Checks: checks(
				func(t *testing.T, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusUnauthorized, rec.Code)
					assert.JSONEq(t, `{"code":"account.not_authenticated","message":"not authenticated"}`, rec.Body.String())
				},
				wasRetrieveCalled(0),
			),
		},
		"Error returned by Storer.Retrieve": {
			Storer: &StorerMock{
				RetrieveFunc: func(context.Context, string, string) (*storage.ObjectInfo, bool, error) {
					return nil, false, assert.AnError
				},
			},
			Checks: checks(
				func(t *testing.T, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusInternalServerError, rec.Code)
				},
				wasRetrieveCalled(1),
			),
		},
		"Logo not found": {
			Storer: &StorerMock{
				RetrieveFunc: func(context.Context, string, string) (*storage.ObjectInfo, bool, error) {
					return nil, false, nil
				},
			},
			Checks: checks(
				func(t *testing.T, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusNotFound, rec.Code)
				},
				wasRetrieveCalled(1),
			),
		},
		"Matching If-None-Match": {
			Storer: &StorerMock{
				RetrieveFunc: func(context.Context, string, string) (*storage.ObjectInfo, bool, error) {
					return &storage.ObjectInfo{
						Body: io.NopCloser(strings.NewReader("logo-data")),
						ETag: "etag1",
					}, true, nil
				},
			},
			NoneMatch: "etag1",
			Checks: checks(
				func(t *testing.T, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusNotModified, rec.Code)
					assert.Zero(t, rec.Body.Len(), rec.Body.String())
				},
				wasRetrieveCalled(1),
			),
		},
		"Successful retrieval": {
			Storer: &StorerMock{
				RetrieveFunc: func(context.Context, string, string) (*storage.ObjectInfo, bool, error) {
					return &storage.ObjectInfo{
						Body:        io.NopCloser(strings.NewReader("logo-data")),
						ETag:        "etag1",
						ContentType: "image/png",
					}, true, nil
				},
			},
			Checks: checks(
				func(t *testing.T, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusOK, rec.Code)
					assert.Equal(t, "logo-data", rec.Body.String())
					assert.Equal(t, "etag1", rec.Header().Get("ETag"))
					assert.Equal(t, "image/png", rec.Header().Get("Content-Type"))
				},
				wasRetrieveCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{
				log:    slog.New(slog.DiscardHandler),
				storer: c.Storer,
			}

			req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)

			if !c.NoSession {
				req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				}))
			}

			if c.NoneMatch != "" {
				req.Header.Set("If-None-Match", c.NoneMatch)
			}

			rec := httptest.NewRecorder()

			hdl.RetrieveOrganizationLogo(rec, req)

			for _, ch := range c.Checks {
				ch(t, c.Storer, rec)
			}
		})
	}
}

func Test_Handler_TeardownOrganization(t *testing.T) {
	type check func(*testing.T, *DBMock, *TxMock, *SearchQueueMock, *StorerMock, *httptest.ResponseRecorder)

	checks := func(cc ...check) []check { return cc }

	hasResp := func(code int, body string) check {
		return func(t *testing.T, _ *DBMock, _ *TxMock, _ *SearchQueueMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
			testutil.AssertResponse(t, rec, code, body)
		}
	}

	wasFetchHooksCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *TxMock, _ *SearchQueueMock, _ *StorerMock, _ *httptest.ResponseRecorder) {
			ff := db.FetchDocumentHooksByOrganizationIDCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "org2", ff[0].OrganizationID)
		}
	}

	wasBeginTxCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *TxMock, _ *SearchQueueMock, _ *StorerMock, _ *httptest.ResponseRecorder) {
			assert.Len(t, db.BeginTxCalls(), count)
		}
	}

	wasEnqueueCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, tx *TxMock, queue *SearchQueueMock, _ *StorerMock, _ *httptest.ResponseRecorder) {
			ff := queue.EnqueueCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Same(t, tx, ff[0].Tx)
			assert.Equal(t, search.OrganizationScope("org2"), ff[0].J)
		}
	}

	wasDeleteSlackAppsCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, tx *TxMock, _ *SearchQueueMock, _ *StorerMock, _ *httptest.ResponseRecorder) {
			ff := tx.DeleteSlackAppsByOrganizationIDCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "org2", ff[0].OrganizationID)
		}
	}

	wasDeleteGithubInstallationsCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, tx *TxMock, _ *SearchQueueMock, _ *StorerMock, _ *httptest.ResponseRecorder) {
			ff := tx.DeleteGithubInstallationsByOrganizationIDCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "org2", ff[0].OrganizationID)
		}
	}

	wasDeleteCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, _ *TxMock, _ *SearchQueueMock, storer *StorerMock, _ *httptest.ResponseRecorder) {
			ff := storer.DeleteCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "organizations/org2/logo", ff[0].Folder)
			assert.Equal(t, "org2", ff[0].ID)
		}
	}

	wasCommitCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, tx *TxMock, _ *SearchQueueMock, _ *StorerMock, _ *httptest.ResponseRecorder) {
			assert.Len(t, tx.CommitCalls(), count)
		}
	}

	wasRollbackCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, tx *TxMock, _ *SearchQueueMock, _ *StorerMock, _ *httptest.ResponseRecorder) {
			assert.Len(t, tx.RollbackCalls(), count)
		}
	}

	cc := map[string]struct {
		DB       *DBMock
		Tx       *TxMock
		Queue    *SearchQueueMock
		Storer   *StorerMock
		BeginErr error
		OmitID   bool
		Checks   []check
	}{
		"Missing organization ID parameter": {
			DB:     &DBMock{},
			Tx:     &TxMock{},
			OmitID: true,
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasFetchHooksCalled(0),
				wasBeginTxCalled(0),
				wasEnqueueCalled(0),
				wasDeleteSlackAppsCalled(0),
				wasDeleteGithubInstallationsCalled(0),
				wasCommitCalled(0),
				wasRollbackCalled(0),
				wasDeleteCalled(0),
			),
		},
		"Error returned by DB.FetchDocumentHooksByOrganizationID": {
			DB: &DBMock{
				FetchDocumentHooksByOrganizationIDFunc: func(context.Context, string) ([]hook.Hook, error) {
					return nil, assert.AnError
				},
			},
			Tx: &TxMock{},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchHooksCalled(1),
				wasBeginTxCalled(0),
				wasEnqueueCalled(0),
				wasDeleteSlackAppsCalled(0),
				wasDeleteGithubInstallationsCalled(0),
				wasCommitCalled(0),
				wasRollbackCalled(0),
				wasDeleteCalled(0),
			),
		},
		// the teardown is refused rather than letting the organization go
		// away with a watcher nothing can reach any more.
		"Error returned by Hook.Delete": {
			DB: &DBMock{
				FetchDocumentHooksByOrganizationIDFunc: func(context.Context, string) ([]hook.Hook, error) {
					return []hook.Hook{{
						ID:             xid.New(),
						Type:           "bogus",
						DocumentID:     null.ValueFrom(xid.New()),
						OrganizationID: null.StringFrom("org2"),
						BranchID:       null.ValueFrom(xid.New()),
						Settings:       processor.Settings(`{"scale":"linear"}`),
						State:          null.ValueFrom(processor.State(`{}`)),
						Status:         processor.StatusActive,
					}}, nil
				},
			},
			Tx: &TxMock{},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchHooksCalled(1),
				wasBeginTxCalled(0),
				wasEnqueueCalled(0),
				wasDeleteSlackAppsCalled(0),
				wasDeleteGithubInstallationsCalled(0),
				wasCommitCalled(0),
				wasRollbackCalled(0),
				wasDeleteCalled(0),
			),
		},
		"Error returned by DB.BeginTx": {
			DB:       &DBMock{},
			Tx:       &TxMock{},
			BeginErr: assert.AnError,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchHooksCalled(1),
				wasBeginTxCalled(1),
				wasEnqueueCalled(0),
				wasDeleteSlackAppsCalled(0),
				wasDeleteGithubInstallationsCalled(0),
				wasCommitCalled(0),
				wasRollbackCalled(0),
				wasDeleteCalled(0),
			),
		},
		"Error returned by SearchQueue.Enqueue": {
			DB: &DBMock{},
			Tx: &TxMock{},
			Queue: &SearchQueueMock{
				EnqueueFunc: func(context.Context, sqlutil.Tx, search.Job) error {
					return assert.AnError
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchHooksCalled(1),
				wasBeginTxCalled(1),
				wasEnqueueCalled(1),
				wasDeleteSlackAppsCalled(0),
				wasDeleteGithubInstallationsCalled(0),
				wasCommitCalled(0),
				wasRollbackCalled(1),
				wasDeleteCalled(0),
			),
		},
		"Error returned by Tx.DeleteSlackAppsByOrganizationID": {
			DB: &DBMock{},
			Tx: &TxMock{
				DeleteSlackAppsByOrganizationIDFunc: func(context.Context, string) error {
					return assert.AnError
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchHooksCalled(1),
				wasBeginTxCalled(1),
				wasEnqueueCalled(1),
				wasDeleteSlackAppsCalled(1),
				wasDeleteGithubInstallationsCalled(0),
				wasCommitCalled(0),
				wasRollbackCalled(1),
				wasDeleteCalled(0),
			),
		},
		"Error returned by Tx.DeleteGithubInstallationsByOrganizationID": {
			DB: &DBMock{},
			Tx: &TxMock{
				DeleteGithubInstallationsByOrganizationIDFunc: func(context.Context, string) error {
					return assert.AnError
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchHooksCalled(1),
				wasBeginTxCalled(1),
				wasEnqueueCalled(1),
				wasDeleteSlackAppsCalled(1),
				wasDeleteGithubInstallationsCalled(1),
				wasCommitCalled(0),
				wasRollbackCalled(1),
				wasDeleteCalled(0),
			),
		},
		"Error returned by Tx.Commit": {
			DB: &DBMock{},
			Tx: &TxMock{
				CommitFunc: func() error {
					return assert.AnError
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchHooksCalled(1),
				wasBeginTxCalled(1),
				wasEnqueueCalled(1),
				wasDeleteSlackAppsCalled(1),
				wasDeleteGithubInstallationsCalled(1),
				wasCommitCalled(1),
				wasRollbackCalled(1),
				wasDeleteCalled(0),
			),
		},
		"Error returned by Storer.Delete": {
			DB: &DBMock{},
			Tx: &TxMock{},
			Storer: &StorerMock{
				DeleteFunc: func(context.Context, string, string) error {
					return assert.AnError
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchHooksCalled(1),
				wasBeginTxCalled(1),
				wasEnqueueCalled(1),
				wasDeleteSlackAppsCalled(1),
				wasDeleteGithubInstallationsCalled(1),
				wasCommitCalled(1),
				wasRollbackCalled(1),
				wasDeleteCalled(1),
			),
		},
		"Successful teardown": {
			DB: &DBMock{
				FetchDocumentHooksByOrganizationIDFunc: func(context.Context, string) ([]hook.Hook, error) {
					return []hook.Hook{{
						ID:             xid.New(),
						Type:           hook.TypeScheduledReminder,
						DocumentID:     null.ValueFrom(xid.New()),
						OrganizationID: null.StringFrom("org2"),
						BranchID:       null.ValueFrom(xid.New()),
						Settings:       processor.Settings(`{"scale":"linear"}`),
						State:          null.ValueFrom(processor.State(`{}`)),
						Status:         processor.StatusActive,
					}}, nil
				},
			},
			Tx: &TxMock{},
			Checks: checks(
				hasResp(http.StatusNoContent, ""),
				wasFetchHooksCalled(1),
				wasBeginTxCalled(1),
				wasEnqueueCalled(1),
				wasDeleteSlackAppsCalled(1),
				wasDeleteGithubInstallationsCalled(1),
				wasCommitCalled(1),
				wasRollbackCalled(1),
				wasDeleteCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			storer := c.Storer
			if storer == nil {
				storer = &StorerMock{}
			}

			queue := c.Queue
			if queue == nil {
				queue = &SearchQueueMock{}
			}

			c.DB.BeginTxFunc = testutil.StubBeginTx(c.Tx, c.BeginErr)

			hdl := Handler{
				log:         slog.New(slog.DiscardHandler),
				db:          c.DB,
				storer:      storer,
				searchQueue: queue,
			}

			req := httptest.NewRequest(http.MethodPost, "http://test.com/", http.NoBody)

			ctx := req.Context()

			if !c.OmitID {
				ctx = testutil.AddChiCtx(ctx, "organizationId", "org2")
			}

			rec := httptest.NewRecorder()

			hdl.TeardownOrganization(rec, req.WithContext(ctx))

			for _, ch := range c.Checks {
				ch(t, c.DB, c.Tx, queue, storer, rec)
			}
		})
	}
}
