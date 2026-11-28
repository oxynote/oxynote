package user

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
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/internal/storage"
	userCore "github.com/oxynote/oxynote/server/core/internal/user"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// _testPNG is a data prefix carrying the PNG magic bytes so content
// type sniffing detects image/png.
var _testPNG = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{1}, 1024)...)

func Test_NewHandler(t *testing.T) {
	t.Parallel()

	db := &DBMock{}
	storer := &StorerMock{}

	hdl := NewHandler(slog.New(slog.DiscardHandler), db, storer, "loc")
	require.NotNil(t, hdl)
	assert.NotNil(t, hdl.log)
	assert.Same(t, db, hdl.db)
	assert.Same(t, storer, hdl.storer)
	assert.Equal(t, "loc", hdl.publicURL)
}

func Test_Handler_RetrieveUserImage(t *testing.T) {
	type check func(*testing.T, *StorerMock, *httptest.ResponseRecorder)

	checks := func(cc ...check) []check { return cc }

	hasResp := func(code int, body string) check {
		return func(t *testing.T, _ *StorerMock, rec *httptest.ResponseRecorder) {
			testutil.AssertResponse(t, rec, code, body)
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
		"Error returned by Storer.Retrieve": {
			Storer: &StorerMock{
				RetrieveFunc: func(context.Context, string, string) (*storage.ObjectInfo, bool, error) {
					return nil, false, assert.AnError
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
					assert.JSONEq(t, `{"code":"general","message":"not found"}`, rec.Body.String())
				},
				wasRetrieveCalled(1),
			),
		},
		"Image not modified": {
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
				ctx = auth.AddSessionToContext(ctx, auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				})
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

			assert.Equal(t, "users/images", ff[0].Folder)
			assert.Equal(t, "u1", ff[0].ID)
			assert.Equal(t, _testPNG, ff[0].Data)
			assert.Equal(t, "image/png", ff[0].ContentType)
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
			assert.Regexp(t, `^loc/api/users/u1/image\?v=\d{14}$`, ff[0].Image)
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
				wasUpdateUserImageCalled(0),
				wasDeleteCalled(0),
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
				wasUpdateUserImageCalled(0),
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
				wasUpdateUserImageCalled(0),
				wasDeleteCalled(0),
			),
		},
		"Error returned by DB.UpdateUserImage": {
			DB: &DBMock{
				UpdateUserImageFunc: func(context.Context, string, string) error {
					return assert.AnError
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
		"Error returned by Storer.Delete": {
			DB: &DBMock{
				UpdateUserImageFunc: func(context.Context, string, string) error {
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
					assert.Regexp(t, `^loc/api/users/u1/image\?v=\d{14}$`, rec.Header().Get("Location"))
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

				body, contentType = testutil.MultipartBody(t, "image", "img.png", content)
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

			hdl.UploadUserImage(rec, req)

			for _, ch := range c.Checks {
				ch(t, c.DB, c.Storer, rec)
			}
		})
	}
}

func Test_Handler_FetchUserSpotlights(t *testing.T) {
	type check func(*testing.T, *DBMock, *httptest.ResponseRecorder)

	checks := func(cc ...check) []check { return cc }

	hasResp := func(code int, body string) check {
		return func(t *testing.T, _ *DBMock, rec *httptest.ResponseRecorder) {
			testutil.AssertResponse(t, rec, code, body)
		}
	}

	wasFetchUserSpotlightsCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *httptest.ResponseRecorder) {
			ff := db.FetchUserSpotlightsCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "u1", ff[0].UserID)
		}
	}

	wasFetchUserSpotlightActivityCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *httptest.ResponseRecorder) {
			ff := db.FetchUserSpotlightActivityCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "u1", ff[0].UserID)
		}
	}

	wasUpsertUserSpotlightsCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *httptest.ResponseRecorder) {
			ff := db.UpsertUserSpotlightsCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "u1", ff[0].UserID)
			assert.Equal(t, userCore.SpotlightsInput{Star: userCore.SpotlightStateShown}, ff[0].Inp)
		}
	}

	// earned is an activity that earns the star spotlight at any time.
	earned := userCore.SpotlightActivity{CreatedDocuments: 100}

	stubDB := func(
		spotlights userCore.Spotlights,
		activity userCore.SpotlightActivity,
		fetchErr, activityErr, upsertErr error,
	) *DBMock {
		return &DBMock{
			FetchUserSpotlightsFunc: func(context.Context, string) (userCore.Spotlights, error) {
				return spotlights, fetchErr
			},
			FetchUserSpotlightActivityFunc: func(context.Context, string) (userCore.SpotlightActivity, error) {
				return activity, activityErr
			},
			UpsertUserSpotlightsFunc: func(context.Context, string, userCore.SpotlightsInput) error {
				return upsertErr
			},
		}
	}

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		Checks    []check
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasFetchUserSpotlightsCalled(0),
				wasFetchUserSpotlightActivityCalled(0),
				wasUpsertUserSpotlightsCalled(0),
			),
		},
		"Error returned by DB.FetchUserSpotlights": {
			DB: stubDB(userCore.Spotlights{}, earned, assert.AnError, nil, nil),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchUserSpotlightsCalled(1),
				wasFetchUserSpotlightActivityCalled(0),
				wasUpsertUserSpotlightsCalled(0),
			),
		},
		"Error returned by DB.FetchUserSpotlightActivity": {
			DB: stubDB(userCore.Spotlights{}, earned, nil, assert.AnError, nil),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchUserSpotlightsCalled(1),
				wasFetchUserSpotlightActivityCalled(1),
				wasUpsertUserSpotlightsCalled(0),
			),
		},
		"Error returned by DB.UpsertUserSpotlights": {
			DB: stubDB(userCore.Spotlights{}, earned, nil, nil, assert.AnError),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchUserSpotlightsCalled(1),
				wasFetchUserSpotlightActivityCalled(1),
				wasUpsertUserSpotlightsCalled(1),
			),
		},
		"Successful retrieval of an unset state that is not earned": {
			DB: stubDB(userCore.Spotlights{}, userCore.SpotlightActivity{}, nil, nil, nil),
			Checks: checks(
				hasResp(http.StatusOK, `{"star":null}`),
				wasFetchUserSpotlightsCalled(1),
				wasFetchUserSpotlightActivityCalled(1),
				wasUpsertUserSpotlightsCalled(0),
			),
		},
		"Successful retrieval of an unset state that is earned": {
			DB: stubDB(userCore.Spotlights{}, earned, nil, nil, nil),
			Checks: checks(
				hasResp(http.StatusOK, `{"star":"shown"}`),
				wasFetchUserSpotlightsCalled(1),
				wasFetchUserSpotlightActivityCalled(1),
				wasUpsertUserSpotlightsCalled(1),
			),
		},
		"Successful retrieval of a shown state": {
			DB: stubDB(userCore.Spotlights{Star: null.ValueFrom(userCore.SpotlightStateShown)}, earned, nil, nil, nil),
			Checks: checks(
				hasResp(http.StatusOK, `{"star":"shown"}`),
				wasFetchUserSpotlightsCalled(1),
				wasFetchUserSpotlightActivityCalled(0),
				wasUpsertUserSpotlightsCalled(0),
			),
		},
		"Successful retrieval of a dismissed state": {
			DB: stubDB(userCore.Spotlights{Star: null.ValueFrom(userCore.SpotlightStateDismissed)}, earned, nil, nil, nil),
			Checks: checks(
				hasResp(http.StatusOK, `{"star":"dismissed"}`),
				wasFetchUserSpotlightsCalled(1),
				wasFetchUserSpotlightActivityCalled(0),
				wasUpsertUserSpotlightsCalled(0),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{
				log: slog.New(slog.DiscardHandler),
				db:  c.DB,
			}

			req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)

			if !c.NoSession {
				req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				}))
			}

			rec := httptest.NewRecorder()

			hdl.FetchUserSpotlights(rec, req)

			for _, ch := range c.Checks {
				ch(t, c.DB, rec)
			}
		})
	}
}

func Test_Handler_UpdateUserSpotlights(t *testing.T) {
	type check func(*testing.T, *DBMock, *httptest.ResponseRecorder)

	checks := func(cc ...check) []check { return cc }

	hasResp := func(code int, body string) check {
		return func(t *testing.T, _ *DBMock, rec *httptest.ResponseRecorder) {
			testutil.AssertResponse(t, rec, code, body)
		}
	}

	wasUpsertUserSpotlightsCalled := func(count int, inp userCore.SpotlightsInput) check {
		return func(t *testing.T, db *DBMock, _ *httptest.ResponseRecorder) {
			ff := db.UpsertUserSpotlightsCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "u1", ff[0].UserID)
			assert.Equal(t, inp, ff[0].Inp)
		}
	}

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		JSON      string
		Checks    []check
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			JSON:      `{"star":"shown"}`,
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasUpsertUserSpotlightsCalled(0, userCore.SpotlightsInput{}),
			),
		},
		"Invalid JSON": {
			DB:   &DBMock{},
			JSON: `{"star":`,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_json","message":"invalid JSON body"}`),
				wasUpsertUserSpotlightsCalled(0, userCore.SpotlightsInput{}),
			),
		},
		"Invalid star state": {
			DB:   &DBMock{},
			JSON: `{"star":"hidden"}`,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"user.invalid_spotlight_state","message":"invalid spotlight state"}`),
				wasUpsertUserSpotlightsCalled(0, userCore.SpotlightsInput{}),
			),
		},
		"Error returned by DB.UpsertUserSpotlights": {
			DB: &DBMock{
				UpsertUserSpotlightsFunc: func(context.Context, string, userCore.SpotlightsInput) error {
					return assert.AnError
				},
			},
			JSON: `{"star":"shown"}`,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasUpsertUserSpotlightsCalled(1, userCore.SpotlightsInput{Star: userCore.SpotlightStateShown}),
			),
		},
		"Successful update": {
			DB:   &DBMock{},
			JSON: `{"star":"dismissed"}`,
			Checks: checks(
				hasResp(http.StatusNoContent, ""),
				wasUpsertUserSpotlightsCalled(1, userCore.SpotlightsInput{Star: userCore.SpotlightStateDismissed}),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{
				log: slog.New(slog.DiscardHandler),
				db:  c.DB,
			}

			req := httptest.NewRequest(http.MethodPut, "http://test.com/", strings.NewReader(c.JSON))

			if !c.NoSession {
				req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				}))
			}

			rec := httptest.NewRecorder()

			hdl.UpdateUserSpotlights(rec, req)

			for _, ch := range c.Checks {
				ch(t, c.DB, rec)
			}
		})
	}
}
