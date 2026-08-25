package user

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

	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/internal/storage"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
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

// multipartBody builds a multipart form body carrying a single file field.
func multipartBody(t *testing.T, field, content string) (io.Reader, string) {
	t.Helper()

	var buf bytes.Buffer

	mw := multipart.NewWriter(&buf)

	fw, err := mw.CreateFormFile(field, "img.png")
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

	hdl := NewHandler(slog.New(slog.DiscardHandler), db, storer, "loc/%s")
	require.NotNil(t, hdl)
	assert.NotNil(t, hdl.log)
	assert.Same(t, db, hdl.db)
	assert.Same(t, storer, hdl.storer)
	assert.Equal(t, "loc/%s", hdl.imageLocationFormat)
}

func Test_Handler_RetrieveUserImage(t *testing.T) {
	type check func(*testing.T, *StorerMock, *httptest.ResponseRecorder)

	checks := func(cc ...check) []check { return cc }

	hasResp := func(code int, body string) check {
		return func(t *testing.T, _ *StorerMock, rec *httptest.ResponseRecorder) {
			assert.Equal(t, code, rec.Code)

			if body == "" {
				assert.Zero(t, rec.Body.Len(), rec.Body.String())
				return
			}

			assert.JSONEq(t, body, rec.Body.String())
		}
	}

	wasRetrieveCalled := func(count int) check {
		return func(t *testing.T, storer *StorerMock, _ *httptest.ResponseRecorder) {
			ff := storer.RetrieveCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "users/images", ff[0].Folder)
			assert.Equal(t, "u2", ff[0].ID)
		}
	}

	cc := map[string]struct {
		Storer    *StorerMock
		NoSession bool
		OmitID    bool
		NoneMatch string
		Checks    []check
	}{
		"No session in context": {
			Storer:    &StorerMock{},
			NoSession: true,
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasRetrieveCalled(0),
			),
		},
		"Missing user ID parameter": {
			Storer: &StorerMock{},
			OmitID: true,
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
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
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasRetrieveCalled(1),
			),
		},
		"Image not found": {
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
						Body: io.NopCloser(strings.NewReader("img-data")),
						ETag: "etag1",
					}, true, nil
				},
			},
			NoneMatch: "etag1",
			Checks: checks(
				hasResp(http.StatusNotModified, ""),
				wasRetrieveCalled(1),
			),
		},
		"Successful retrieval": {
			Storer: &StorerMock{
				RetrieveFunc: func(context.Context, string, string) (*storage.ObjectInfo, bool, error) {
					return &storage.ObjectInfo{
						Body:        io.NopCloser(strings.NewReader("img-data")),
						ETag:        "etag1",
						ContentType: "image/png",
					}, true, nil
				},
			},
			Checks: checks(
				func(t *testing.T, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusOK, rec.Code)
					assert.Equal(t, "img-data", rec.Body.String())
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

			ctx := req.Context()

			if !c.NoSession {
				ctx = addSession(ctx)
			}

			if !c.OmitID {
				ctx = testutil.AddChiCtx(ctx, "userId", "u2")
			}

			if c.NoneMatch != "" {
				req.Header.Set("If-None-Match", c.NoneMatch)
			}

			rec := httptest.NewRecorder()

			hdl.RetrieveUserImage(rec, req.WithContext(ctx))

			for _, ch := range c.Checks {
				ch(t, c.Storer, rec)
			}
		})
	}
}

func Test_Handler_UploadUserImage(t *testing.T) {
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

			assert.Equal(t, "users/images", ff[0].Folder)
			assert.Equal(t, "u1", ff[0].ID)
		}
	}

	wasUpdateUserImageCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *StorerMock, _ *httptest.ResponseRecorder) {
			ff := db.UpdateUserImageCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "u1", ff[0].UserID)
			assert.Regexp(t, `^loc/u1\?v=\d{14}$`, ff[0].Image)
		}
	}

	wasDeleteCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, storer *StorerMock, _ *httptest.ResponseRecorder) {
			ff := storer.DeleteCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "users/images", ff[0].Folder)
			assert.Equal(t, "u1", ff[0].ID)
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
				wasUpdateUserImageCalled(0),
			),
		},
		"Missing multipart image field": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			NoFile: true,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_form","message":"invalid form data"}`),
				wasUploadCalled(0),
				wasUpdateUserImageCalled(0),
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
				wasUpdateUserImageCalled(0),
				wasDeleteCalled(0),
			),
		},
		"DB update error triggers cleanup": {
			DB: &DBMock{
				UpdateUserImageFunc: func(context.Context, string, string) error {
					return errors.New("boom")
				},
			},
			Storer: &StorerMock{},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasUploadCalled(1),
				wasUpdateUserImageCalled(1),
				wasDeleteCalled(1),
			),
		},
		"DB update error with failing cleanup": {
			DB: &DBMock{
				UpdateUserImageFunc: func(context.Context, string, string) error {
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
				wasUploadCalled(1),
				wasUpdateUserImageCalled(1),
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
					assert.Regexp(t, `^loc/u1\?v=\d{14}$`, rec.Header().Get("Location"))
				},
				wasUploadCalled(1),
				wasUpdateUserImageCalled(1),
				wasDeleteCalled(0),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{
				log:                 slog.New(slog.DiscardHandler),
				db:                  c.DB,
				storer:              c.Storer,
				imageLocationFormat: "loc/%s",
			}

			var (
				body        io.Reader = http.NoBody
				contentType string
			)

			if !c.NoFile {
				body, contentType = multipartBody(t, "image", "img-data")
			}

			req := httptest.NewRequest(http.MethodPut, "http://test.com/", body)

			if contentType != "" {
				req.Header.Set("Content-Type", contentType)
			}

			if !c.NoSession {
				req = req.WithContext(addSession(req.Context()))
			}

			rec := httptest.NewRecorder()

			hdl.UploadUserImage(rec, req)

			for _, ch := range c.Checks {
				ch(t, c.DB, c.Storer, rec)
			}
		})
	}
}
