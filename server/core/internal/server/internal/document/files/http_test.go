package files

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

	"github.com/oxynote/oxynote/server/core/internal/document/file"
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

// _documentID is the fixed document ID used across test requests.
var _documentID = xid.New()

// addSession stores a test session on the request context.
func addSession(ctx context.Context) context.Context {
	return auth.AddSessionToContext(ctx, auth.Session{
		UserID:               "u1",
		ActiveOrganizationID: "org1",
	})
}

// multipartBody builds a multipart form body carrying a single file field.
func multipartBody(t *testing.T, field, content string) (io.Reader, string) {
	t.Helper()

	var buf bytes.Buffer

	mw := multipart.NewWriter(&buf)

	fw, err := mw.CreateFormFile(field, "file.png")
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

	hdl := NewHandler(slog.New(slog.DiscardHandler), db, storer, "loc/%s/%s")
	require.NotNil(t, hdl)
	assert.NotNil(t, hdl.log)
	assert.Same(t, db, hdl.db)
	assert.Same(t, storer, hdl.storer)
	assert.Equal(t, "loc/%s/%s", hdl.fileLocationFormat)
}

func Test_Handler_UploadDocumentFile(t *testing.T) {
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

			assert.Equal(t, "organizations/org1/documents/"+_documentID.String()+"/files", ff[0].Folder)
			assert.Equal(t, "f1", ff[0].ID)
		}
	}

	wasInsertCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *StorerMock, _ *httptest.ResponseRecorder) {
			ff := db.InsertDocumentFileCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "f1", ff[0].F.ID)
			assert.Equal(t, file.LocationDocument, ff[0].F.Location)
			assert.Equal(t, "organizations/org1/documents/"+_documentID.String()+"/files/f1", ff[0].F.StorageKey)
			assert.Equal(t, _documentID, ff[0].F.DocumentID.V)
			assert.Equal(t, "org1", ff[0].F.OrganizationID.String)
		}
	}

	wasDeleteCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *StorerMock, _ *httptest.ResponseRecorder) {
			ff := db.DeleteDocumentFileCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "f1", ff[0].ID)
		}
	}

	cc := map[string]struct {
		DB        *DBMock
		Storer    *StorerMock
		NoSession bool
		OmitID    bool
		NoFile    bool
		Query     string
		Checks    []check
	}{
		"No session in context": {
			DB:        &DBMock{},
			Storer:    &StorerMock{},
			NoSession: true,
			Query:     "?id=f1&location=document",
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasUploadCalled(0),
			),
		},
		"Missing document ID parameter": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			OmitID: true,
			Query:  "?id=f1&location=document",
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasUploadCalled(0),
			),
		},
		"Document existence check error": {
			DB: &DBMock{
				CheckDocumentExistsFunc: func(context.Context, xid.ID, string) error {
					return errors.New("boom")
				},
			},
			Storer: &StorerMock{},
			Query:  "?id=f1&location=document",
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasUploadCalled(0),
			),
		},
		"Missing file ID query parameter": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			Query:  "?location=document",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_form","message":"invalid form data"}`),
				wasUploadCalled(0),
			),
		},
		"Invalid location query parameter": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			Query:  "?id=f1&location=nowhere",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_form","message":"invalid form data"}`),
				wasUploadCalled(0),
			),
		},
		"Missing multipart file field": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			NoFile: true,
			Query:  "?id=f1&location=document",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_form","message":"invalid form data"}`),
				wasUploadCalled(0),
			),
		},
		"DB insert error": {
			DB: &DBMock{
				InsertDocumentFileFunc: func(context.Context, file.File) error {
					return errors.New("boom")
				},
			},
			Storer: &StorerMock{},
			Query:  "?id=f1&location=document",
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasInsertCalled(1),
				wasUploadCalled(0),
				wasDeleteCalled(0),
			),
		},
		"Storer upload error rolls the row back": {
			DB: &DBMock{},
			Storer: &StorerMock{
				UploadFunc: func(context.Context, string, string, io.Reader) error {
					return errors.New("boom")
				},
			},
			Query: "?id=f1&location=document",
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasInsertCalled(1),
				wasUploadCalled(1),
				wasDeleteCalled(1),
			),
		},
		"Storer upload error with failing rollback": {
			DB: &DBMock{
				DeleteDocumentFileFunc: func(context.Context, string) error {
					return errors.New("cleanup boom")
				},
			},
			Storer: &StorerMock{
				UploadFunc: func(context.Context, string, string, io.Reader) error {
					return errors.New("boom")
				},
			},
			Query: "?id=f1&location=document",
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDeleteCalled(1),
			),
		},
		"Successful upload": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			Query:  "?id=f1&location=comment",
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusCreated, rec.Code)
					assert.Zero(t, rec.Body.Len(), rec.Body.String())
					assert.Equal(t, "loc/"+_documentID.String()+"/f1", rec.Header().Get("Location"))
				},
				wasUploadCalled(1),
				func(t *testing.T, db *DBMock, _ *StorerMock, _ *httptest.ResponseRecorder) {
					ff := db.InsertDocumentFileCalls()
					require.Len(t, ff, 1)
					assert.Equal(t, file.LocationComment, ff[0].F.Location)
				},
				wasDeleteCalled(0),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{
				log:                slog.New(slog.DiscardHandler),
				db:                 c.DB,
				storer:             c.Storer,
				fileLocationFormat: "loc/%s/%s",
			}

			var (
				body        io.Reader = http.NoBody
				contentType string
			)

			if !c.NoFile {
				body, contentType = multipartBody(t, "file", "file-data")
			}

			req := httptest.NewRequest(http.MethodPut, "http://test.com/"+c.Query, body)

			if contentType != "" {
				req.Header.Set("Content-Type", contentType)
			}

			ctx := req.Context()

			if !c.NoSession {
				ctx = addSession(ctx)
			}

			if !c.OmitID {
				ctx = testutil.AddChiCtx(ctx, "documentId", _documentID.String())
			}

			rec := httptest.NewRecorder()

			hdl.UploadDocumentFile(rec, req.WithContext(ctx))

			for _, ch := range c.Checks {
				ch(t, c.DB, c.Storer, rec)
			}
		})
	}
}

// stubFileDB returns a db mock whose file record belongs to the given
// document, which is what the ownership check compares the url against.
func stubFileDB(documentID xid.ID) *DBMock {
	return &DBMock{
		FetchDocumentFileFunc: func(_ context.Context, id, organizationID string) (*file.File, error) {
			f := file.NewFile(id, file.LocationDocument, "key", documentID, organizationID)

			return &f, nil
		},
	}
}

func Test_Handler_RetrieveDocumentFile(t *testing.T) {
	type check func(*testing.T, *DBMock, *StorerMock, *httptest.ResponseRecorder)

	checks := func(cc ...check) []check { return cc }

	wasFetchCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *StorerMock, _ *httptest.ResponseRecorder) {
			ff := db.FetchDocumentFileCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "f1", ff[0].BlockID)
			assert.Equal(t, "org1", ff[0].OrganizationID)
		}
	}

	wasRetrieveCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, storer *StorerMock, _ *httptest.ResponseRecorder) {
			ff := storer.RetrieveCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "organizations/org1/documents/"+_documentID.String()+"/files", ff[0].Folder)
			assert.Equal(t, "f1", ff[0].ID)
		}
	}

	cc := map[string]struct {
		DB         *DBMock
		Storer     *StorerMock
		NoSession  bool
		OmitDocID  bool
		OmitFileID bool
		NoneMatch  string
		Checks     []check
	}{
		"No session in context": {
			DB:        &DBMock{},
			Storer:    &StorerMock{},
			NoSession: true,
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusUnauthorized, rec.Code)
				},
				wasRetrieveCalled(0),
			),
		},
		"Missing document ID parameter": {
			DB:        &DBMock{},
			Storer:    &StorerMock{},
			OmitDocID: true,
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusNotFound, rec.Code)
					assert.JSONEq(t, `{"code":"general","message":"not found"}`, rec.Body.String())
				},
				wasRetrieveCalled(0),
			),
		},
		"Missing file ID parameter": {
			DB:         &DBMock{},
			Storer:     &StorerMock{},
			OmitFileID: true,
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusNotFound, rec.Code)
					assert.JSONEq(t, `{"code":"general","message":"not found"}`, rec.Body.String())
				},
				wasFetchCalled(0),
				wasRetrieveCalled(0),
			),
		},
		"File record fetch error": {
			DB: &DBMock{
				FetchDocumentFileFunc: func(context.Context, string, string) (*file.File, error) {
					return nil, errors.New("boom")
				},
			},
			Storer: &StorerMock{},
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusInternalServerError, rec.Code)
				},
				wasFetchCalled(1),
				wasRetrieveCalled(0),
			),
		},
		"File belongs to another document": {
			DB:     stubFileDB(xid.New()),
			Storer: &StorerMock{},
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusNotFound, rec.Code)
					assert.JSONEq(t, `{"code":"general","message":"not found"}`, rec.Body.String())
				},
				wasFetchCalled(1),
				wasRetrieveCalled(0),
			),
		},
		"Storer retrieval error": {
			DB: stubFileDB(_documentID),
			Storer: &StorerMock{
				RetrieveFunc: func(context.Context, string, string) (*storage.ObjectInfo, bool, error) {
					return nil, false, errors.New("boom")
				},
			},
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusInternalServerError, rec.Code)
				},
				wasRetrieveCalled(1),
			),
		},
		"File object not found": {
			DB: stubFileDB(_documentID),
			Storer: &StorerMock{
				RetrieveFunc: func(context.Context, string, string) (*storage.ObjectInfo, bool, error) {
					return nil, false, nil
				},
			},
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusNotFound, rec.Code)
				},
				wasRetrieveCalled(1),
			),
		},
		"Matching If-None-Match": {
			DB: stubFileDB(_documentID),
			Storer: &StorerMock{
				RetrieveFunc: func(context.Context, string, string) (*storage.ObjectInfo, bool, error) {
					return &storage.ObjectInfo{
						Body: io.NopCloser(strings.NewReader("file-data")),
						ETag: "etag1",
					}, true, nil
				},
			},
			NoneMatch: "etag1",
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusNotModified, rec.Code)
					assert.Zero(t, rec.Body.Len(), rec.Body.String())
				},
				wasRetrieveCalled(1),
			),
		},
		"Successful retrieval": {
			DB: stubFileDB(_documentID),
			Storer: &StorerMock{
				RetrieveFunc: func(context.Context, string, string) (*storage.ObjectInfo, bool, error) {
					return &storage.ObjectInfo{
						Body:        io.NopCloser(strings.NewReader("file-data")),
						ETag:        "etag1",
						ContentType: "image/png",
					}, true, nil
				},
			},
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusOK, rec.Code)
					assert.Equal(t, "file-data", rec.Body.String())
					assert.Equal(t, "etag1", rec.Header().Get("ETag"))
					assert.Equal(t, "image/png", rec.Header().Get("Content-Type"))
				},
				wasFetchCalled(1),
				wasRetrieveCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{
				log:    slog.New(slog.DiscardHandler),
				db:     c.DB,
				storer: c.Storer,
			}

			req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)

			ctx := req.Context()

			if !c.NoSession {
				ctx = addSession(ctx)
			}

			if !c.OmitDocID {
				ctx = testutil.AddChiCtx(ctx, "documentId", _documentID.String())
			}

			if !c.OmitFileID {
				ctx = testutil.AddChiCtx(ctx, "id", "f1")
			}

			if c.NoneMatch != "" {
				req.Header.Set("If-None-Match", c.NoneMatch)
			}

			rec := httptest.NewRecorder()

			hdl.RetrieveDocumentFile(rec, req.WithContext(ctx))

			for _, ch := range c.Checks {
				ch(t, c.DB, c.Storer, rec)
			}
		})
	}
}
