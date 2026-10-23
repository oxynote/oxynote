package httpserver

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_LocationHeader(t *testing.T) {
	cc := map[string]struct {
		Code     int
		Location string
	}{
		"Error response": {
			Code: http.StatusBadGateway,
		},
		"Successful response": {
			Code:     http.StatusOK,
			Location: "/test/123",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdr := http.Header{}

			LocationHeader("/test/123")(hdr, c.Code)
			assert.Equal(t, c.Location, hdr.Get("Location"))
		})
	}
}

func Test_errorHeaders(t *testing.T) {
	hdr := http.Header{}

	hdr.Set("Location", "test")
	errorHeaders(hdr, http.StatusBadGateway)
	assert.NotContains(t, hdr, "Location")
}

func Test_Respond(t *testing.T) {
	cc := map[string]struct {
		Code        int
		Data        any
		WriteErr    error
		ContentType string
		Body        bool
		Log         string
	}{
		"Invalid data": {
			Code: http.StatusInternalServerError,
			Data: func() {},
			Log:  `"msg":"cannot marshal response"`,
		},
		"Error returned by ResponseWriter.Write": {
			Code:        http.StatusTeapot,
			Data:        []string{"hello"},
			WriteErr:    assert.AnError,
			ContentType: "application/json",
			Log:         `"msg":"cannot send response"`,
		},
		"Successful response without body": {
			Code: http.StatusTeapot,
			Body: false,
		},
		"Successful response with body": {
			Code:        http.StatusTeapot,
			Data:        errutil.New(0, "", "error"),
			ContentType: "application/json",
			Body:        true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var b bytes.Buffer

			out := bufio.NewWriter(&b)
			log := slog.New(slog.NewJSONHandler(out, nil))
			rec := httptest.NewRecorder()

			var w http.ResponseWriter = rec

			if c.WriteErr != nil {
				w = &MockResponseWriter{
					headerFn: rec.Header,
					writeFn: func(_ []byte) (int, error) {
						return 0, c.WriteErr
					},
					writeHeaderFn: rec.WriteHeader,
				}
			}

			Respond(
				log,
				w,
				c.Data,
				http.StatusTeapot,
				func(hdr http.Header, _ int) {
					hdr.Set("Hello1", "World1")
				},
				func(hdr http.Header, _ int) {
					hdr.Set("Hello2", "World2")
				},
			)
			assert.Equal(t, c.Code, rec.Code)
			assert.Equal(t, "World1", rec.Header().Get("Hello1"))
			assert.Equal(t, "World2", rec.Header().Get("Hello2"))
			assert.Equal(t, c.ContentType, rec.Header().Get("Content-Type"))

			if c.Body {
				assert.NotEmpty(t, rec.Body.String())
			} else {
				assert.Empty(t, rec.Body.String())
			}

			require.NoError(t, out.Flush())

			if c.Log == "" {
				assert.Empty(t, b.String())
				return
			}

			strs := strings.Split(b.String(), "\n")
			require.Len(t, strs, 2)
			assert.Regexp(t, c.Log, strs[0])
		})
	}
}

func Test_RespondError(t *testing.T) {
	cc := map[string]struct {
		Inp      error
		RespCode int
		Log      string
	}{
		"Client error": {
			Inp:      ErrInvalidJSON,
			RespCode: http.StatusBadRequest,
		},
		"Internal server error": {
			Inp:      assert.AnError,
			RespCode: http.StatusInternalServerError,
			Log:      `"msg":"internal server error"`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var b bytes.Buffer

			out := bufio.NewWriter(&b)
			log := slog.New(slog.NewJSONHandler(out, nil))

			rec := httptest.NewRecorder()
			rec.Header().Set("Location", "test")

			RespondError(log, rec, c.Inp)
			assert.Equal(t, c.RespCode, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			assert.NotContains(t, rec.Header(), "Location")
			assert.NotEmpty(t, rec.Body.String())

			require.NoError(t, out.Flush())

			if c.Log == "" {
				assert.Empty(t, b.String())
				return
			}

			strs := strings.Split(b.String(), "\n")
			require.Len(t, strs, 2)
			assert.Regexp(t, c.Log, strs[0])
		})
	}
}

func Test_RespondSuppressedError(t *testing.T) {
	var b bytes.Buffer

	out := bufio.NewWriter(&b)
	log := slog.New(slog.NewTextHandler(out, nil))

	rec := httptest.NewRecorder()
	rec.Header().Set("Location", "test")

	RespondSuppressedError(log, rec, ErrInvalidJSON)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.NotContains(t, rec.Header(), "Location")
	assert.NotEmpty(t, rec.Body.String())

	require.NoError(t, out.Flush())
	assert.Empty(t, b.String())

	rec = httptest.NewRecorder()
	rec.Header().Set("Location", "test")

	RespondSuppressedError(log, rec, assert.AnError)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.NotContains(t, rec.Header(), "Location")
	assert.NotEmpty(t, rec.Body.String())

	require.NoError(t, out.Flush())
	require.Empty(t, b.String())
}

func Test_FormFile(t *testing.T) {
	t.Parallel()

	errTooLarge := errors.New("too large")

	cc := map[string]struct {
		Field   string
		Lookup  string
		Content string
		Limit   int64
		Err     error
	}{
		"Missing multipart field": {
			Field:  "other",
			Lookup: "file",
			Limit:  1 << 20,
			Err:    ErrInvalidForm,
		},
		"Body exceeding the limit": {
			Field:   "file",
			Lookup:  "file",
			Content: strings.Repeat("a", 1024),
			Limit:   64,
			Err:     errTooLarge,
		},
		"Successful parse": {
			Field:   "file",
			Lookup:  "file",
			Content: "file-data",
			Limit:   1 << 20,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var body bytes.Buffer

			mw := multipart.NewWriter(&body)

			fw, err := mw.CreateFormFile(c.Field, "file.bin")
			require.NoError(t, err)

			_, err = io.WriteString(fw, c.Content)
			require.NoError(t, err)
			require.NoError(t, mw.Close())

			req := httptest.NewRequest(http.MethodPut, "http://test.com/", &body)
			req.Header.Set("Content-Type", mw.FormDataContentType())

			f, err := FormFile(httptest.NewRecorder(), req, c.Lookup, c.Limit, errTooLarge)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, f)

				return
			}

			defer f.Close() //nolint:errcheck // error provides no meaningful info

			data, rerr := io.ReadAll(f)
			require.NoError(t, rerr)
			assert.Equal(t, c.Content, string(data))
		})
	}
}

func Test_DecodeJSON(t *testing.T) {
	type dest struct {
		Msg string `json:"msg"`
	}

	cc := map[string]struct {
		Body   string
		Result dest
		Err    error
	}{
		"Error returned by Decoder.Decode": {
			Body: "{",
			Err:  ErrInvalidJSON,
		},
		"Successful decode": {
			Body:   `{"msg":"test"}`,
			Result: dest{Msg: "test"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var v dest

			req := httptest.NewRequest("GET", "http://test.com/", strings.NewReader(c.Body))
			err := DecodeJSON(req, &v)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Zero(t, v)
				return
			}

			assert.Equal(t, c.Result, v)
		})
	}
}

func Test_DecodeForm(t *testing.T) {
	type msgDest struct {
		Msg string
	}

	type idsDest struct {
		IDs []xid.ID
	}

	type idTimeDest struct {
		ID   xid.ID
		Time time.Time
	}

	id := xid.New()
	tm := time.Date(2021, 1, 2, 3, 4, 5, 0, time.UTC)

	cc := map[string]struct {
		NilPostBody bool
		Query       url.Values
		Dest        any
		Result      any
		Err         error
	}{
		"Error returned by Request.ParseForm": {
			NilPostBody: true,
			Dest:        &msgDest{},
			Err:         ErrInvalidForm,
		},
		"Error returned by Decoder.Decode for non pointer destination": {
			Dest: msgDest{},
			Err:  ErrInvalidForm,
		},
		"Error returned by Decoder.Decode for invalid ID": {
			Query: url.Values{"id": {"invalid"}},
			Dest:  &idTimeDest{},
			Err:   ErrInvalidForm,
		},
		"Error returned by Decoder.Decode for invalid time": {
			Query: url.Values{"time": {"invalid"}},
			Dest:  &idTimeDest{},
			Err:   ErrInvalidForm,
		},
		"Successful decode with unknown keys": {
			Query:  url.Values{"msg": {"test"}, "hello": {"test"}},
			Dest:   &msgDest{},
			Result: &msgDest{Msg: "test"},
		},
		"Successful decode of IDs": {
			Query:  url.Values{"ids": {id.String()}},
			Dest:   &idsDest{},
			Result: &idsDest{IDs: []xid.ID{id}},
		},
		"Successful decode of ID and time": {
			Query:  url.Values{"id": {id.String()}, "time": {tm.Format(time.RFC3339Nano)}},
			Dest:   &idTimeDest{},
			Result: &idTimeDest{ID: id, Time: tm},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest("GET", "http://test.com/", http.NoBody)
			req.URL.RawQuery = c.Query.Encode()

			if c.NilPostBody {
				req.Method = "POST"
				req.Body = nil
			}

			err := DecodeForm(req, c.Dest)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, c.Dest)
		})
	}
}

func Test_NotFound(t *testing.T) {
	req := httptest.NewRequest("GET", "http://test.com/", http.NoBody)
	rec := httptest.NewRecorder()
	NotFound(slog.New(slog.DiscardHandler))(rec, req)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.NotZero(t, rec.Body.Len())
}

func Test_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest("GET", "http://test.com/", http.NoBody)
	rec := httptest.NewRecorder()
	MethodNotAllowed(slog.New(slog.DiscardHandler))(rec, req)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.NotZero(t, rec.Body.Len())
}

func Test_Recoverer(t *testing.T) {
	req := httptest.NewRequest("GET", "http://test.com/", http.NoBody)
	rec := httptest.NewRecorder()

	assert.NotPanics(t, func() {
		Recoverer(slog.New(slog.DiscardHandler))(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			panic("error")
		})).ServeHTTP(rec, req)
	})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), strings.ToLower(http.StatusText(http.StatusInternalServerError)))
}

func Test_ServeObject(t *testing.T) {
	cc := map[string]struct {
		Match       string
		Body        io.Reader
		RespCode    int
		RespBody    string
		RespETag    string
		RespType    string
		LogContains string
	}{
		"Cached copy is still current": {
			Match:    "etag-1",
			Body:     strings.NewReader("data"),
			RespCode: http.StatusNotModified,
		},
		"Error returned by io.Copy": {
			Body:        iotest.ErrReader(assert.AnError),
			RespCode:    http.StatusOK,
			RespETag:    "etag-1",
			RespType:    "image/png",
			LogContains: "cannot stream object",
		},
		"Successful stream": {
			Match:    "etag-0",
			Body:     strings.NewReader("data"),
			RespCode: http.StatusOK,
			RespBody: "data",
			RespETag: "etag-1",
			RespType: "image/png",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			buf := &bytes.Buffer{}

			req := httptest.NewRequest("GET", "http://test.com/", http.NoBody)
			req.Header.Set("If-None-Match", c.Match)

			rec := httptest.NewRecorder()

			ServeObject(
				slog.New(slog.NewTextHandler(buf, nil)),
				rec,
				req,
				"etag-1",
				"image/png",
				c.Body,
			)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Equal(t, c.RespBody, rec.Body.String())
			assert.Equal(t, c.RespETag, rec.Header().Get("ETag"))
			assert.Equal(t, c.RespType, rec.Header().Get("Content-Type"))

			if c.LogContains == "" {
				assert.Empty(t, buf.String())
				return
			}

			assert.Contains(t, buf.String(), c.LogContains)
		})
	}
}

func Test_ExtractNamedID(t *testing.T) {
	id := xid.New()

	cc := map[string]struct {
		Param  string
		Result xid.ID
		Err    error
	}{
		"Error returned by ExtractParam": {
			Err: errutil.ErrNotFound,
		},
		"Error returned by xid.FromString": {
			Param: "invalid",
			Err:   errutil.ErrNotFound,
		},
		"Successful extraction": {
			Param:  id.String(),
			Result: id,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest("GET", "http://test.com/", http.NoBody)

			if c.Param != "" {
				req = req.WithContext(testutil.AddChiCtx(context.Background(), "id", c.Param))
			}

			res, err := ExtractNamedID(req, "id")
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Zero(t, res)
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_ExtractParam(t *testing.T) {
	cc := map[string]struct {
		Param  string
		Result string
		Err    error
	}{
		"Missing parameter": {
			Err: errutil.ErrNotFound,
		},
		"Successful extraction": {
			Param:  "123",
			Result: "123",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest("GET", "http://test.com/", http.NoBody)

			if c.Param != "" {
				req = req.WithContext(testutil.AddChiCtx(context.Background(), "key", c.Param))
			}

			res, err := ExtractParam(req, "key")
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Empty(t, res)
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_ExtractQueryID(t *testing.T) {
	id := xid.New()

	cc := map[string]struct {
		Query  string
		Result xid.ID
		Err    error
	}{
		"Error returned by xid.FromString for missing ID": {
			Err: ErrInvalidForm,
		},
		"Error returned by xid.FromString for invalid ID": {
			Query: "?branchId=invalid",
			Err:   ErrInvalidForm,
		},
		"Successful extraction": {
			Query:  "?branchId=" + id.String(),
			Result: id,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest("GET", "http://test.com/"+c.Query, http.NoBody)

			res, err := ExtractQueryID(req, "branchId")
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Zero(t, res)
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_CacheBust(t *testing.T) {
	t.Parallel()

	assert.Regexp(t, `^/api/users/u1/image\?v=\d{14}$`, CacheBust("/api/users/u1/image"))
}

func Test_Timeout(t *testing.T) {
	Timeout(time.Millisecond)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		assert.Equal(t, r.Context().Err(), context.DeadlineExceeded)
	})).ServeHTTP(nil, httptest.NewRequest(http.MethodGet, "http://test.test", http.NoBody))
}

type MockResponseWriter struct {
	headerFn      func() http.Header
	writeFn       func([]byte) (int, error)
	writeHeaderFn func(int)
}

func (r *MockResponseWriter) Header() http.Header {
	return r.headerFn()
}

func (r *MockResponseWriter) Write(d []byte) (int, error) {
	return r.writeFn(d)
}

func (r *MockResponseWriter) WriteHeader(st int) {
	r.writeHeaderFn(st)
}
