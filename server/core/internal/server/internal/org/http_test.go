package org

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/searchgw"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/internal/storage"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// addSession stores a test session on the request context.
func addSession(ctx context.Context) context.Context {
	return auth.AddSessionToContext(ctx, auth.Session{
		UserID:               "u1",
		ActiveOrganizationID: "org1",
	})
}

// withTx wires the DB mock's BeginTx to hand out the provided Tx mock.
func withTx(db *DBMock, tx *TxMock, err error) *DBMock {
	db.BeginTxFunc = func(_ context.Context, dest any) error {
		if err != nil {
			return err
		}

		*(dest.(*Tx)) = tx

		return nil
	}

	return db
}

// multipartBody builds a multipart form body carrying a single file field.
func multipartBody(t *testing.T, field, content string) (io.Reader, string) {
	t.Helper()

	var buf bytes.Buffer

	mw := multipart.NewWriter(&buf)

	fw, err := mw.CreateFormFile(field, "logo.png")
	require.NoError(t, err)

	_, err = fw.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	return &buf, mw.FormDataContentType()
}

func Test_NewHandler(t *testing.T) {
	t.Parallel()

	db := &DBMock{}
	storer := &StorerMock{}

	hdl := NewHandler(slog.New(slog.DiscardHandler), db, storer, "loc", "http://prom.test")
	require.NotNil(t, hdl)
	assert.NotNil(t, hdl.log)
	assert.Same(t, db, hdl.db)
	assert.Same(t, storer, hdl.storer)
	assert.Equal(t, "loc", hdl.logoLocation)
	assert.Equal(t, "http://prom.test", hdl.demoPrometheusURL)
}

func Test_Handler_InitializeOrganization(t *testing.T) {
	type check func(*testing.T, *TxMock, *httptest.ResponseRecorder)

	checks := func(cc ...check) []check { return cc }

	hasResp := func(code int, body string) check {
		return func(t *testing.T, _ *TxMock, rec *httptest.ResponseRecorder) {
			assert.Equal(t, code, rec.Code)

			if body == "" {
				assert.Zero(t, rec.Body.Len(), rec.Body.String())
				return
			}

			assert.JSONEq(t, body, rec.Body.String())
		}
	}

	wasInsertDataSourceCalled := func(count int) check {
		return func(t *testing.T, tx *TxMock, _ *httptest.ResponseRecorder) {
			ff := tx.InsertDataSourceCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "Demo", ff[0].Ds.Name)
			assert.Equal(t, datasource.TypePrometheus, ff[0].Ds.Type)
			assert.Equal(t, "http://prom.test", ff[0].Ds.URL)
			assert.Equal(t, "org2", ff[0].Ds.OrganizationID)
		}
	}

	wasInsertDocumentCalled := func(count int) check {
		return func(t *testing.T, tx *TxMock, _ *httptest.ResponseRecorder) {
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
		return func(t *testing.T, tx *TxMock, _ *httptest.ResponseRecorder) {
			ff := tx.UpsertDocumentMaintainersCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "org2", ff[0].OrganizationID)
			assert.Equal(t, []string{"member1"}, ff[0].MaintainerIDs)
		}
	}

	wasSearchJobInserted := func(count int) check {
		return func(t *testing.T, tx *TxMock, _ *httptest.ResponseRecorder) {
			assert.Len(t, tx.InsertDocumentSearchJobCalls(), count)
		}
	}

	wasCommitCalled := func(count int) check {
		return func(t *testing.T, tx *TxMock, _ *httptest.ResponseRecorder) {
			assert.Len(t, tx.CommitCalls(), count)
			assert.NotEmpty(t, tx.RollbackCalls())
		}
	}

	cc := map[string]struct {
		Tx       *TxMock
		BeginErr error
		NoProm   bool
		OmitID   bool
		Checks   []check
	}{
		"Missing organization ID parameter": {
			Tx:     &TxMock{},
			OmitID: true,
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasInsertDocumentCalled(0),
			),
		},
		"No demo Prometheus URL configured": {
			Tx:     &TxMock{},
			NoProm: true,
			Checks: checks(
				hasResp(http.StatusOK, ""),
				wasInsertDataSourceCalled(0),
				wasInsertDocumentCalled(0),
			),
		},
		"Transaction start error": {
			Tx:       &TxMock{},
			BeginErr: errors.New("boom"),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasInsertDocumentCalled(0),
			),
		},
		"Member fetch error": {
			Tx: &TxMock{
				FetchOrganizationMembersFunc: func(context.Context, string) ([]string, error) {
					return nil, errors.New("boom")
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasInsertDataSourceCalled(0),
				wasCommitCalled(0),
			),
		},
		"No organization members": {
			Tx: &TxMock{
				FetchOrganizationMembersFunc: func(context.Context, string) ([]string, error) {
					return nil, nil
				},
			},
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"organization.no_members","message":"organization has no members"}`),
				wasInsertDataSourceCalled(0),
				wasCommitCalled(0),
			),
		},
		"Data source insertion error is tolerated": {
			Tx: &TxMock{
				FetchOrganizationMembersFunc: func(context.Context, string) ([]string, error) {
					return []string{"member1"}, nil
				},
				InsertDataSourceFunc: func(context.Context, *datasource.DataSource) error {
					return errors.New("boom")
				},
			},
			Checks: checks(
				func(t *testing.T, _ *TxMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusCreated, rec.Code)
				},
				wasInsertDataSourceCalled(1),
				wasInsertDocumentCalled(1),
				wasCommitCalled(1),
			),
		},
		"Document insertion error": {
			Tx: &TxMock{
				FetchOrganizationMembersFunc: func(context.Context, string) ([]string, error) {
					return []string{"member1"}, nil
				},
				InsertDocumentFunc: func(context.Context, document.Document) error {
					return errors.New("boom")
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasInsertDocumentCalled(1),
				wasCommitCalled(0),
			),
		},
		"Maintainer upsert error": {
			Tx: &TxMock{
				FetchOrganizationMembersFunc: func(context.Context, string) ([]string, error) {
					return []string{"member1"}, nil
				},
				UpsertDocumentMaintainersFunc: func(context.Context, xid.ID, string, []string) error {
					return errors.New("boom")
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasUpsertMaintainersCalled(1),
				wasCommitCalled(0),
			),
		},
		"Search job insertion error": {
			Tx: &TxMock{
				FetchOrganizationMembersFunc: func(context.Context, string) ([]string, error) {
					return []string{"member1"}, nil
				},
				InsertDocumentSearchJobFunc: func(context.Context, searchgw.BlocksDifference) error {
					return errors.New("boom")
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasSearchJobInserted(1),
				wasCommitCalled(0),
			),
		},
		"Commit error": {
			Tx: &TxMock{
				FetchOrganizationMembersFunc: func(context.Context, string) ([]string, error) {
					return []string{"member1"}, nil
				},
				CommitFunc: func() error {
					return errors.New("boom")
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasCommitCalled(1),
			),
		},
		"Successful initialization": {
			Tx: &TxMock{
				FetchOrganizationMembersFunc: func(context.Context, string) ([]string, error) {
					return []string{"member1", "member2"}, nil
				},
			},
			Checks: checks(
				func(t *testing.T, _ *TxMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusCreated, rec.Code)
					assert.Contains(t, rec.Body.String(), "Welcome to Oxynote!")
				},
				wasInsertDataSourceCalled(1),
				wasInsertDocumentCalled(1),
				wasUpsertMaintainersCalled(1),
				wasSearchJobInserted(1),
				wasCommitCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			promURL := "http://prom.test"

			if c.NoProm {
				promURL = ""
			}

			hdl := Handler{
				log:               slog.New(slog.DiscardHandler),
				db:                withTx(&DBMock{}, c.Tx, c.BeginErr),
				demoPrometheusURL: promURL,
			}

			req := httptest.NewRequest(http.MethodPost, "http://test.com/", http.NoBody)

			ctx := req.Context()

			if !c.OmitID {
				ctx = testutil.AddChiCtx(ctx, "organizationId", "org2")
			}

			rec := httptest.NewRecorder()

			hdl.InitializeOrganization(rec, req.WithContext(ctx))

			for _, ch := range c.Checks {
				ch(t, c.Tx, rec)
			}
		})
	}
}

func Test_Handler_UploadOrganizationLogo(t *testing.T) {
	type check func(*testing.T, *DBMock, *StorerMock, *httptest.ResponseRecorder)

	checks := func(cc ...check) []check { return cc }

	hasResp := func(code int, body string) check {
		return func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
			assert.Equal(t, code, rec.Code)

			if body == "" {
				assert.Zero(t, rec.Body.Len(), rec.Body.String())
				return
			}

			assert.JSONEq(t, body, rec.Body.String())
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
			assert.Regexp(t, `^loc\?v=\d{14}$`, ff[0].Logo)
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
		Checks    []check
	}{
		"No session in context": {
			DB:        &DBMock{},
			Storer:    &StorerMock{},
			NoSession: true,
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasUploadCalled(0),
			),
		},
		"Missing multipart logo field": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			NoFile: true,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasUploadCalled(0),
			),
		},
		"Storer upload error": {
			DB: &DBMock{},
			Storer: &StorerMock{
				UploadFunc: func(context.Context, string, string, io.Reader) error {
					return errors.New("boom")
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasUploadCalled(1),
				wasUpdateLogoCalled(0),
			),
		},
		"DB update error triggers cleanup": {
			DB: &DBMock{
				UpdateOrganizationLogoFunc: func(context.Context, string, string) error {
					return errors.New("boom")
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
		"DB update error with failing cleanup": {
			DB: &DBMock{
				UpdateOrganizationLogoFunc: func(context.Context, string, string) error {
					return errors.New("boom")
				},
			},
			Storer: &StorerMock{
				DeleteFunc: func(context.Context, string, string) error {
					return errors.New("cleanup boom")
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
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
					assert.Regexp(t, `^loc\?v=\d{14}$`, rec.Header().Get("Location"))
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
				log:          slog.New(slog.DiscardHandler),
				db:           c.DB,
				storer:       c.Storer,
				logoLocation: "loc",
			}

			var (
				body        io.Reader = http.NoBody
				contentType string
			)

			if !c.NoFile {
				body, contentType = multipartBody(t, "logo", "logo-data")
			}

			req := httptest.NewRequest(http.MethodPut, "http://test.com/", body)

			if contentType != "" {
				req.Header.Set("Content-Type", contentType)
			}

			if !c.NoSession {
				req = req.WithContext(addSession(req.Context()))
			}

			rec := httptest.NewRecorder()

			hdl.UploadOrganizationLogo(rec, req)

			for _, ch := range c.Checks {
				ch(t, c.DB, c.Storer, rec)
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
		"Storer retrieval error": {
			Storer: &StorerMock{
				RetrieveFunc: func(context.Context, string, string) (*storage.ObjectInfo, bool, error) {
					return nil, false, errors.New("boom")
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
				req = req.WithContext(addSession(req.Context()))
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
