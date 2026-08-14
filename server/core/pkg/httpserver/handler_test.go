package httpserver

import (
	"bufio"
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_LocationHeader(t *testing.T) {
	hdr := http.Header{}

	LocationHeader("/test/123")(hdr, http.StatusBadGateway)
	assert.Empty(t, hdr.Get("Location"))

	LocationHeader("/test/123")(hdr, http.StatusOK)
	assert.Equal(t, "/test/123", hdr.Get("Location"))
}

func Test_errorHeaders(t *testing.T) {
	hdr := http.Header{}

	hdr.Set("Location", "test")
	errorHeaders(hdr, http.StatusBadGateway)
	assert.NotContains(t, hdr, "Location")
}

func Test_Respond(t *testing.T) {
	cc := map[string]struct {
		Code int
		Data any
		Body bool
		Log  string
	}{
		"Invalid data": {
			Code: http.StatusInternalServerError,
			Data: func() {},
			Log:  `"msg":"cannot marshal response"`,
		},
		"Successful response without body": {
			Code: http.StatusTeapot,
			Body: false,
		},
		"Successful response with body": {
			Code: http.StatusTeapot,
			Data: errutil.New(0, "", "error"),
			Body: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var b bytes.Buffer

			out := bufio.NewWriter(&b)
			log := slog.New(slog.NewJSONHandler(out, nil))
			rec := httptest.NewRecorder()

			header1Func := func(hdr http.Header, _ int) {
				hdr.Set("Hello1", "World1")
			}
			header2Func := func(hdr http.Header, _ int) {
				hdr.Set("Hello2", "World2")
			}

			Respond(
				log,
				rec,
				c.Data,
				http.StatusTeapot,
				header1Func,
				header2Func,
			)
			assert.Equal(t, c.Code, rec.Code)
			assert.Equal(t, "World1", rec.Header().Get("Hello1"))
			assert.Equal(t, "World2", rec.Header().Get("Hello2"))

			if c.Body {
				assert.Equal(t, "application/json",
					rec.Header().Get("Content-Type"))
				assert.NotEmpty(t, rec.Body.String())
			} else {
				assert.Empty(t, rec.Header().Get("Content-Type"))
				assert.Empty(t, rec.Body.String())
			}

			require.NoError(t, out.Flush())

			if c.Code < 500 {
				assert.Empty(t, b.String())
				return
			}

			strs := strings.Split(b.String(), "\n")
			require.Len(t, strs, 2)
			assert.Regexp(t, c.Log, strs[0])
		})
	}

	// special case
	var b bytes.Buffer

	out := bufio.NewWriter(&b)
	log := slog.New(slog.NewJSONHandler(out, nil))

	Respond(
		log,
		&MockResponseWriter{
			headerFn: func() http.Header {
				return http.Header{}
			},
			writeFn: func(_ []byte) (int, error) {
				return 0, assert.AnError
			},
			writeHeaderFn: func(_ int) {},
		},
		[]string{"hello"},
		http.StatusTeapot,
	)

	require.NoError(t, out.Flush())
	assert.Contains(t, b.String(), `"msg":"cannot send response"`)
}

func Test_RespondError(t *testing.T) {
	var b bytes.Buffer

	out := bufio.NewWriter(&b)
	log := slog.New(slog.NewJSONHandler(out, nil))

	rec := httptest.NewRecorder()
	rec.Header().Set("Location", "test")

	RespondError(log, rec, ErrInvalidJSON)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.NotContains(t, rec.Header(), "Location")
	assert.NotEmpty(t, rec.Body.String())

	require.NoError(t, out.Flush())
	assert.Empty(t, b.String())

	rec = httptest.NewRecorder()
	rec.Header().Set("Location", "test")

	RespondError(log, rec, assert.AnError)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.NotContains(t, rec.Header(), "Location")
	assert.NotEmpty(t, rec.Body.String())

	require.NoError(t, out.Flush())

	strs := strings.Split(b.String(), "\n")
	require.Len(t, strs, 2)
	assert.Regexp(t, `"msg":"internal server error"`, strs[0])
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

func Test_DecodeJSON(t *testing.T) {
	v := struct {
		Msg string `json:"msg"`
	}{}
	req := httptest.NewRequest("GET", "http://test.com/", strings.NewReader("{"))
	err := DecodeJSON(req, &v)
	assert.Equal(t, ErrInvalidJSON, err)
	assert.Zero(t, v)

	req = httptest.NewRequest("GET", "http://test.com/", strings.NewReader(`{"msg":"test"}`))
	err = DecodeJSON(req, &v)
	assert.NoError(t, err)
	assert.Equal(t, "test", v.Msg)
}

func Test_DecodeForm(t *testing.T) {
	v := struct{ Msg string }{}

	req := httptest.NewRequest("POST", "http://test.com/", http.NoBody)
	req.Body = nil

	assert.Equal(t, ErrInvalidForm, DecodeForm(req, v))

	req = httptest.NewRequest("GET", "http://test.com/", http.NoBody)
	assert.Equal(t, ErrInvalidForm, DecodeForm(req, v))

	req = httptest.NewRequest("GET", "http://test.com/", http.NoBody)
	q := req.URL.Query()
	q.Add("msg", "test")
	q.Add("hello", "test")
	req.URL.RawQuery = q.Encode()
	assert.NoError(t, DecodeForm(req, &v))
	assert.Equal(t, "test", v.Msg)

	v2 := struct{ IDs []xid.ID }{}
	id := xid.New()

	req = httptest.NewRequest("GET", "http://test.com/", http.NoBody)
	q = req.URL.Query()
	q.Add("ids", id.String())
	req.URL.RawQuery = q.Encode()
	assert.NoError(t, DecodeForm(req, &v2))
	assert.Equal(t, []xid.ID{id}, v2.IDs)
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

func Test_ExtractParam(t *testing.T) {
	req := httptest.NewRequest("GET", "http://test.com/", http.NoBody)
	val, err := ExtractParam(req, "key")
	assert.Empty(t, val)
	assert.Error(t, err)

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("key", "123")
	req = req.WithContext(context.WithValue(context.Background(),
		chi.RouteCtxKey, ctx))

	val, err = ExtractParam(req, "key")
	assert.Equal(t, "123", val)
	assert.NoError(t, err)
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
