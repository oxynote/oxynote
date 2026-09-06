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
	"slices"
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

// _testPNG is a data prefix carrying the PNG magic bytes so content
// type sniffing detects image/png.
var _testPNG = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{1}, 1024)...)

// multipartBody builds a multipart form body carrying a single file field
// under the given file name; an empty name sends "file.png" and nil
// content sends a PNG.
func multipartBody(t *testing.T, field, name string, content []byte) (io.Reader, string) {
	t.Helper()

	if name == "" {
		name = "file.png"
	}

	if content == nil {
		content = _testPNG
	}

	var buf bytes.Buffer

	mw := multipart.NewWriter(&buf)

	fw, err := mw.CreateFormFile(field, name)
	require.NoError(t, err)

	_, err = fw.Write(content)
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	return &buf, mw.FormDataContentType()
}

func Test_NewHandler(t *testing.T) {
	t.Parallel()

	db := &DBMock{}
	storer := &StorerMock{}

	hdl := NewHandler(slog.New(slog.DiscardHandler), db, storer, "https://app.test/core")
	require.NotNil(t, hdl)
	assert.NotNil(t, hdl.log)
	assert.Same(t, db, hdl.db)
	assert.Same(t, storer, hdl.storer)
	assert.Equal(t, "https://app.test/core", hdl.publicURL)
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

			assert.Equal(t, "organizations/org1/documents/"+_documentID.String()+"/files/", ff[0].Folder)
			assert.Equal(t, "f1xxxxxxxxxxxxxxxxxxx", ff[0].ID)
		}
	}

	wasUploadCalledWith := func(data []byte, contentType string) check {
		return func(t *testing.T, _ *DBMock, storer *StorerMock, _ *httptest.ResponseRecorder) {
			ff := storer.UploadCalls()
			require.Len(t, ff, 1)

			assert.Equal(t, data, ff[0].Data)
			assert.Equal(t, contentType, ff[0].ContentType)
		}
	}

	wasInsertCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *StorerMock, _ *httptest.ResponseRecorder) {
			ff := db.InsertDocumentFileCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "f1xxxxxxxxxxxxxxxxxxx", ff[0].F.ID)
			assert.Equal(t, file.LocationDocument, ff[0].F.Location)
			assert.Equal(t, _documentID, ff[0].F.DocumentID.V)
			assert.Equal(t, "org1", ff[0].F.OrganizationID.String)
		}
	}

	wasInsertCalledWith := func(name string, size int64, contentType string) check {
		return func(t *testing.T, db *DBMock, _ *StorerMock, _ *httptest.ResponseRecorder) {
			ff := db.InsertDocumentFileCalls()
			require.Len(t, ff, 1)

			assert.Equal(t, file.Key("org1", _documentID, "f1xxxxxxxxxxxxxxxxxxx"), ff[0].F.StorageKey)
			assert.Equal(t, name, ff[0].F.Name)
			assert.Equal(t, size, ff[0].F.Size)
			assert.Equal(t, contentType, ff[0].F.ContentType)
		}
	}

	wasDeleteCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *StorerMock, _ *httptest.ResponseRecorder) {
			ff := db.DeleteDocumentFileCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "f1xxxxxxxxxxxxxxxxxxx", ff[0].ID)
		}
	}

	cc := map[string]struct {
		DB        *DBMock
		Storer    *StorerMock
		NoSession bool
		OmitID    bool
		NoFile    bool
		// FileName is the multipart part's file name; empty sends
		// "file.png".
		FileName string
		// Content is the multipart file's bytes; nil sends a PNG.
		Content []byte
		Query   string
		Checks  []check
	}{
		"No session in context": {
			DB:        &DBMock{},
			Storer:    &StorerMock{},
			NoSession: true,
			Query:     "?id=f1xxxxxxxxxxxxxxxxxxx&location=document&kind=image",
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasUploadCalled(0),
			),
		},
		"Missing document ID parameter": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			OmitID: true,
			Query:  "?id=f1xxxxxxxxxxxxxxxxxxx&location=document&kind=image",
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
			Query:  "?id=f1xxxxxxxxxxxxxxxxxxx&location=document&kind=image",
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasUploadCalled(0),
			),
		},
		"Missing file ID query parameter": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			Query:  "?location=document&kind=image",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_form","message":"invalid form data"}`),
				wasUploadCalled(0),
			),
		},
		// path.Join would clean the dot segments straight out of the
		// storage folder, so the id shape check is what stands between
		// a request and another organization's object keys.
		"File ID with path traversal": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			Query:  "?id=../../../org2/documents/d2/files/f9&location=document&kind=image",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_form","message":"invalid form data"}`),
				wasInsertCalled(0),
				wasUploadCalled(0),
			),
		},
		"Invalid location query parameter": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			Query:  "?id=f1xxxxxxxxxxxxxxxxxxx&location=nowhere&kind=image",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_form","message":"invalid form data"}`),
				wasUploadCalled(0),
			),
		},
		"Missing kind query parameter": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			Query:  "?id=f1xxxxxxxxxxxxxxxxxxx&location=document",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_form","message":"invalid form data"}`),
				wasInsertCalled(0),
				wasUploadCalled(0),
			),
		},
		"Invalid kind query parameter": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			Query:  "?id=f1xxxxxxxxxxxxxxxxxxx&location=document&kind=video",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_form","message":"invalid form data"}`),
				wasInsertCalled(0),
				wasUploadCalled(0),
			),
		},
		// every editor mints 21-character nanoids, and the retrieval route
		// cuts the id off the front of the segment by that length.
		"File ID of the wrong length": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			Query:  "?id=short&location=document&kind=image",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_form","message":"invalid form data"}`),
				wasInsertCalled(0),
				wasUploadCalled(0),
			),
		},
		"Missing multipart file field": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			NoFile: true,
			Query:  "?id=f1xxxxxxxxxxxxxxxxxxx&location=document&kind=image",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_form","message":"invalid form data"}`),
				wasUploadCalled(0),
			),
		},
		// the image kind keeps the image-only rule, and a rejected body
		// never reaches the row.
		"Image kind rejects a non-image": {
			DB:      &DBMock{},
			Storer:  &StorerMock{},
			Content: []byte("plain text data"),
			Query:   "?id=f1xxxxxxxxxxxxxxxxxxx&location=document&kind=image",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"storage.invalid_content_type","message":"invalid content type"}`),
				wasInsertCalled(0),
				wasUploadCalled(0),
			),
		},
		"Image kind rejects a body over the image size limit": {
			DB:      &DBMock{},
			Storer:  &StorerMock{},
			Content: append(slices.Clone(_testPNG), bytes.Repeat([]byte{1}, int(storage.ImagePolicy.MaxSize))...),
			Query:   "?id=f1xxxxxxxxxxxxxxxxxxx&location=document&kind=image",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"storage.size_limit_exceeded","message":"file size exceeds limit"}`),
				wasInsertCalled(0),
				wasUploadCalled(0),
			),
		},
		"File kind rejects a body over the file size limit": {
			DB:      &DBMock{},
			Storer:  &StorerMock{},
			Content: bytes.Repeat([]byte{'x'}, int(storage.FilePolicy.MaxSize)+1),
			Query:   "?id=f1xxxxxxxxxxxxxxxxxxx&location=document&kind=file",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"storage.size_limit_exceeded","message":"file size exceeds limit"}`),
				wasInsertCalled(0),
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
			Query:  "?id=f1xxxxxxxxxxxxxxxxxxx&location=document&kind=image",
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
				UploadFunc: func(context.Context, string, string, []byte, string) error {
					return errors.New("boom")
				},
			},
			Query: "?id=f1xxxxxxxxxxxxxxxxxxx&location=document&kind=image",
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
				UploadFunc: func(context.Context, string, string, []byte, string) error {
					return errors.New("boom")
				},
			},
			Query: "?id=f1xxxxxxxxxxxxxxxxxxx&location=document&kind=image",
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDeleteCalled(1),
			),
		},
		"Successful image upload": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			Query:  "?id=f1xxxxxxxxxxxxxxxxxxx&location=comment&kind=image",
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusCreated, rec.Code)
					assert.JSONEq(
						t,
						`{"id":"f1xxxxxxxxxxxxxxxxxxx","name":"file.png","size":1032,"contentType":"image/png"}`,
						rec.Body.String(),
					)
					assert.Equal(t, "https://app.test/core/api/documents/"+_documentID.String()+"/files/f1xxxxxxxxxxxxxxxxxxx-file.png", rec.Header().Get("Location"))
				},
				wasUploadCalled(1),
				wasUploadCalledWith(_testPNG, "image/png"),
				wasInsertCalledWith("file.png", 1032, "image/png"),
				func(t *testing.T, db *DBMock, _ *StorerMock, _ *httptest.ResponseRecorder) {
					ff := db.InsertDocumentFileCalls()
					require.Len(t, ff, 1)
					assert.Equal(t, file.LocationComment, ff[0].F.Location)
				},
				wasDeleteCalled(0),
			),
		},
		// the type is what the bytes say, whatever the part's name
		// claims, and the file kind takes any of them.
		"Successful file upload records the name, size and detected type": {
			DB:       &DBMock{},
			Storer:   &StorerMock{},
			FileName: "notes.zip",
			Content:  []byte("plain text data"),
			Query:    "?id=f1xxxxxxxxxxxxxxxxxxx&location=document&kind=file",
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusCreated, rec.Code)
					assert.JSONEq(
						t,
						`{"id":"f1xxxxxxxxxxxxxxxxxxx","name":"notes.zip","size":15,"contentType":"text/plain; charset=utf-8"}`,
						rec.Body.String(),
					)
					assert.Equal(t, "https://app.test/core/api/documents/"+_documentID.String()+"/files/f1xxxxxxxxxxxxxxxxxxx-notes.zip", rec.Header().Get("Location"))
				},
				wasInsertCalled(1),
				wasInsertCalledWith("notes.zip", 15, "text/plain; charset=utf-8"),
				func(t *testing.T, _ *DBMock, storer *StorerMock, _ *httptest.ResponseRecorder) {
					ff := storer.UploadCalls()
					require.Len(t, ff, 1)
					assert.Equal(t, "f1xxxxxxxxxxxxxxxxxxx", ff[0].ID)
				},
				wasUploadCalledWith([]byte("plain text data"), "text/plain; charset=utf-8"),
				wasDeleteCalled(0),
			),
		},
		// a spreadsheet whose first cell happens to spell a bitmap's magic
		// bytes is still the text it is.
		"Successful file upload of a CSV starting like a bitmap": {
			DB:       &DBMock{},
			Storer:   &StorerMock{},
			FileName: "Portfolio - sheet.csv",
			Content:  []byte("BMW,Model,Price\n3,320i,40000\n"),
			Query:    "?id=f1xxxxxxxxxxxxxxxxxxx&location=document&kind=file",
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusCreated, rec.Code)
					assert.Equal(t, "https://app.test/core/api/documents/"+_documentID.String()+"/files/f1xxxxxxxxxxxxxxxxxxx-Portfolio%20-%20sheet.csv", rec.Header().Get("Location"))
				},
				wasInsertCalledWith("Portfolio - sheet.csv", 29, "text/plain; charset=utf-8"),
			),
		},
		"Successful file upload at the file size limit": {
			DB:      &DBMock{},
			Storer:  &StorerMock{},
			Content: bytes.Repeat([]byte{'x'}, int(storage.FilePolicy.MaxSize)),
			Query:   "?id=f1xxxxxxxxxxxxxxxxxxx&location=document&kind=file",
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusCreated, rec.Code)
				},
				wasInsertCalledWith("file.png", storage.FilePolicy.MaxSize, "text/plain; charset=utf-8"),
				wasUploadCalled(1),
			),
		},
		"Successful file upload records a path's base name": {
			DB:       &DBMock{},
			Storer:   &StorerMock{},
			FileName: `C:\Users\me\Documents\notes.zip`,
			Content:  []byte("plain text data"),
			Query:    "?id=f1xxxxxxxxxxxxxxxxxxx&location=document&kind=file",
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusCreated, rec.Code)
				},
				wasInsertCalledWith("notes.zip", 15, "text/plain; charset=utf-8"),
			),
		},
		"Successful file upload without a usable name": {
			DB:       &DBMock{},
			Storer:   &StorerMock{},
			FileName: ".",
			Content:  []byte("plain text data"),
			Query:    "?id=f1xxxxxxxxxxxxxxxxxxx&location=document&kind=file",
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusCreated, rec.Code)
				},
				wasInsertCalledWith("file", 15, "text/plain; charset=utf-8"),
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
				publicURL: "https://app.test/core",
			}

			var (
				body        io.Reader = http.NoBody
				contentType string
			)

			if !c.NoFile {
				body, contentType = multipartBody(t, "file", c.FileName, c.Content)
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
// document, which is what the ownership check compares the url against,
// and carries the given name and content type.
func stubFileDB(documentID xid.ID, name, contentType string) *DBMock {
	return &DBMock{
		FetchDocumentFileFunc: func(_ context.Context, id, organizationID string) (*file.File, error) {
			f := file.NewFile(id, file.LocationDocument, file.Key(organizationID, documentID, id), documentID, organizationID, name, 9, contentType)

			return &f, nil
		},
	}
}

// stubObjectStorer returns a storer mock serving one object of the given
// content type.
func stubObjectStorer(contentType string) *StorerMock {
	return &StorerMock{
		RetrieveFunc: func(context.Context, string, string) (*storage.ObjectInfo, bool, error) {
			return &storage.ObjectInfo{
				Body:        io.NopCloser(strings.NewReader("file-data")),
				ETag:        "etag1",
				ContentType: contentType,
			}, true, nil
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

			assert.Equal(t, "f1xxxxxxxxxxxxxxxxxxx", ff[0].BlockID)
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

			// the object is named by the row's key, extension included.
			assert.Equal(t, "organizations/org1/documents/"+_documentID.String()+"/files/", ff[0].Folder)
			assert.Equal(t, "f1xxxxxxxxxxxxxxxxxxx", ff[0].ID)
		}
	}

	cc := map[string]struct {
		DB         *DBMock
		Storer     *StorerMock
		NoSession  bool
		OmitDocID  bool
		OmitFileID bool
		// Ref is the route's file segment; empty sends a well-formed one.
		Ref       string
		NoneMatch string
		Checks    []check
	}{
		"File segment without a name": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			Ref:    "f1xxxxxxxxxxxxxxxxxxx",
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusNotFound, rec.Code)
					assert.JSONEq(t, `{"code":"general","message":"not found"}`, rec.Body.String())
				},
				wasFetchCalled(0),
				wasRetrieveCalled(0),
			),
		},
		"File segment with a short id": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			Ref:    "f1-shot.png",
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusNotFound, rec.Code)
				},
				wasFetchCalled(0),
				wasRetrieveCalled(0),
			),
		},
		"File segment with an id of the wrong charset": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			Ref:    "f1.................xx-shot.png",
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusNotFound, rec.Code)
				},
				wasFetchCalled(0),
				wasRetrieveCalled(0),
			),
		},
		// the name is what the address says, not what the row holds.
		"File segment with another name": {
			DB:     stubFileDB(_documentID, "shot.png", "image/png"),
			Storer: stubObjectStorer("image/png"),
			Ref:    "f1xxxxxxxxxxxxxxxxxxx-renamed-with-dashes.png",
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusOK, rec.Code)
					assert.Equal(t, "inline; filename=shot.png", rec.Header().Get("Content-Disposition"))
				},
				wasFetchCalled(1),
				wasRetrieveCalled(1),
			),
		},
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
			DB:     stubFileDB(xid.New(), "shot.png", "image/png"),
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
			DB: stubFileDB(_documentID, "shot.png", "image/png"),
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
			DB: stubFileDB(_documentID, "shot.png", "image/png"),
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
			DB:        stubFileDB(_documentID, "shot.png", "image/png"),
			Storer:    stubObjectStorer("image/png"),
			NoneMatch: "etag1",
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusNotModified, rec.Code)
					assert.Zero(t, rec.Body.Len(), rec.Body.String())
				},
				wasRetrieveCalled(1),
			),
		},
		// the row's type wins over the store's: the file system backend
		// re-sniffs on the way out and cannot tell a document from a zip.
		"Successful retrieval of a viewable file": {
			DB:     stubFileDB(_documentID, "shot.png", "image/png"),
			Storer: stubObjectStorer("application/octet-stream"),
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusOK, rec.Code)
					assert.Equal(t, "file-data", rec.Body.String())
					assert.Equal(t, "etag1", rec.Header().Get("ETag"))
					assert.Equal(t, "image/png", rec.Header().Get("Content-Type"))
					assert.Equal(t, "inline; filename=shot.png", rec.Header().Get("Content-Disposition"))
					assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
				},
				wasFetchCalled(1),
				wasRetrieveCalled(1),
			),
		},
		"Successful retrieval of a PDF": {
			DB:     stubFileDB(_documentID, "report.pdf", "application/pdf"),
			Storer: stubObjectStorer("application/pdf"),
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusOK, rec.Code)
					assert.Equal(t, "application/pdf", rec.Header().Get("Content-Type"))
					assert.Equal(t, "inline; filename=report.pdf", rec.Header().Get("Content-Disposition"))
					assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
				},
			),
		},
		"Successful retrieval of an archive": {
			DB:     stubFileDB(_documentID, "notes.zip", "application/zip"),
			Storer: stubObjectStorer("application/zip"),
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusOK, rec.Code)
					assert.Equal(t, "application/zip", rec.Header().Get("Content-Type"))
					assert.Equal(t, "attachment; filename=notes.zip", rec.Header().Get("Content-Disposition"))
					assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
				},
			),
		},
		"Successful retrieval of HTML is an attachment": {
			DB:     stubFileDB(_documentID, "evil.html", "text/html; charset=utf-8"),
			Storer: stubObjectStorer("text/html; charset=utf-8"),
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusOK, rec.Code)
					assert.Equal(t, "attachment; filename=evil.html", rec.Header().Get("Content-Disposition"))
					assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
				},
			),
		},
		"Successful retrieval under a quoted name": {
			DB:     stubFileDB(_documentID, `a "quoted"; name.txt`, "text/plain; charset=utf-8"),
			Storer: stubObjectStorer("text/plain; charset=utf-8"),
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusOK, rec.Code)
					assert.Equal(t, `inline; filename="a \"quoted\"; name.txt"`, rec.Header().Get("Content-Disposition"))
				},
			),
		},
		"Successful retrieval under a non-ASCII name": {
			DB:     stubFileDB(_documentID, "résumé.pdf", "application/pdf"),
			Storer: stubObjectStorer("application/pdf"),
			Checks: checks(
				func(t *testing.T, _ *DBMock, _ *StorerMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusOK, rec.Code)
					assert.Equal(t, `inline; filename*=utf-8''r%C3%A9sum%C3%A9.pdf`, rec.Header().Get("Content-Disposition"))
				},
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
				ref := c.Ref
				if ref == "" {
					ref = "f1xxxxxxxxxxxxxxxxxxx-shot.png"
				}

				ctx = testutil.AddChiCtx(ctx, "ref", ref)
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

func Test_fileID(t *testing.T) {
	cc := map[string]struct {
		Ref    string
		Result string
		OK     bool
	}{
		"Id and name":               {Ref: "f1xxxxxxxxxxxxxxxxxxx-notes.zip", Result: "f1xxxxxxxxxxxxxxxxxxx", OK: true},
		"Name with dashes":          {Ref: "f1xxxxxxxxxxxxxxxxxxx-a-b-c.zip", Result: "f1xxxxxxxxxxxxxxxxxxx", OK: true},
		"Id with dashes":            {Ref: "f1-x_-xxxxxxxxxxxxxxx-notes.zip", Result: "f1-x_-xxxxxxxxxxxxxxx", OK: true},
		"Empty name":                {Ref: "f1xxxxxxxxxxxxxxxxxxx-", Result: "f1xxxxxxxxxxxxxxxxxxx", OK: true},
		"No dash after the id":      {Ref: "f1xxxxxxxxxxxxxxxxxxxxnotes.zip"},
		"Id alone":                  {Ref: "f1xxxxxxxxxxxxxxxxxxx"},
		"Short":                     {Ref: "f1-notes.zip"},
		"Id with the wrong charset": {Ref: "f1.................xx-notes.zip"},
		"Empty":                     {Ref: ""},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, ok := fileID(c.Ref)
			assert.Equal(t, c.OK, ok)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_uploadPolicy(t *testing.T) {
	cc := map[string]struct {
		Kind   string
		Result storage.Policy
		OK     bool
	}{
		"Image kind": {
			Kind:   "image",
			Result: storage.ImagePolicy,
			OK:     true,
		},
		"File kind": {
			Kind:   "file",
			Result: storage.FilePolicy,
			OK:     true,
		},
		"Unknown kind": {
			Kind: "video",
		},
		"Empty kind": {},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, ok := uploadPolicy(c.Kind)
			assert.Equal(t, c.OK, ok)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_fileName(t *testing.T) {
	cc := map[string]struct {
		Raw    string
		Result string
	}{
		"Plain name":             {Raw: "notes.zip", Result: "notes.zip"},
		"Unix path":              {Raw: "/home/me/notes.zip", Result: "notes.zip"},
		"Windows path":           {Raw: `C:\Users\me\notes.zip`, Result: "notes.zip"},
		"Surrounding whitespace": {Raw: "  notes.zip  ", Result: "notes.zip"},
		"Empty name":             {Raw: "", Result: "file"},
		"Whitespace only":        {Raw: "   ", Result: "file"},
		"Dot":                    {Raw: ".", Result: "file"},
		"Root":                   {Raw: "/", Result: "file"},
		"Non-ASCII name":         {Raw: "résumé.pdf", Result: "résumé.pdf"},
		"Name over the rune cap": {Raw: strings.Repeat("é", 300), Result: strings.Repeat("é", 255)},
		"Name at the rune cap":   {Raw: strings.Repeat("a", 255), Result: strings.Repeat("a", 255)},
		"Path with a long base":  {Raw: "dir/" + strings.Repeat("b", 256), Result: strings.Repeat("b", 255)},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, fileName(c.Raw))
		})
	}
}
