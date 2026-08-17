// Package httpserver provides helper functions and types to simplify HTTP
// request handling.
package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/schema"
	"github.com/oxynote/oxynote/server/core/pkg/errcode"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/logutil"
	"github.com/rs/xid"
)

// _criticalSkipFrames specifies the number of stacktrace frames to skip so
// that critical reports point at the handler instead of this helper.
const _criticalSkipFrames = 2

var (
	// ErrNotAuthenticated is returned when authorization process fails.
	ErrNotAuthenticated = errutil.New(http.StatusUnauthorized, errcode.AccountNotAuthenticated, "not authenticated")

	// ErrNotPermitted is returned when authorization process fails due
	// to insufficient permissions.
	ErrNotPermitted = errutil.New(http.StatusForbidden, "account.not_permitted", "not permitted")

	// _errMethodNotAllowed is returned when request's method is not
	// supported for the requested endpoint.
	_errMethodNotAllowed = errutil.New(http.StatusMethodNotAllowed, "request.method_not_allowed", "method not allowed") //nolint:errname // unexported globals are _-prefixed by convention

	// ErrInvalidJSON is returned when request's body contains invalid
	// JSON data.
	ErrInvalidJSON = errutil.New(http.StatusBadRequest, "request.invalid_json", "invalid JSON body")

	// ErrInvalidForm is returned when request's form contains invalid
	// JSON data.
	ErrInvalidForm = errutil.New(http.StatusBadRequest, "request.invalid_form", "invalid form data")
)

var _formDec = schema.NewDecoder()

func init() { //nolint:gochecknoinits // form decoder needs to be set up here
	_formDec.IgnoreUnknownKeys(true)

	_formDec.RegisterConverter(xid.ID{}, func(raw string) reflect.Value {
		id, err := xid.FromString(raw)
		if err != nil {
			return reflect.Value{}
		}

		return reflect.ValueOf(id)
	})

	_formDec.RegisterConverter(time.Time{}, func(raw string) reflect.Value {
		tm, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return reflect.Value{}
		}

		return reflect.ValueOf(tm)
	})
}

// ResponseHeader is used to set response headers.
type ResponseHeader func(h http.Header, code int)

// LocationHeader adds a location value to the Location header.
func LocationHeader(loc string) ResponseHeader {
	return func(h http.Header, code int) {
		if code >= http.StatusBadRequest {
			// error responses must not point at a location
			loc = ""
		}

		h.Set("Location", loc)
	}
}

// errorHeaders ensures that only error-related headers are set.
func errorHeaders(h http.Header, _ int) {
	h.Del("Location")
}

// Respond sends JSON type response to the client.
func Respond(
	log *slog.Logger,
	w http.ResponseWriter,
	data any,
	statusCode int,
	headers ...ResponseHeader,
) {
	setHeaders := func() {
		for _, h := range headers {
			h(w.Header(), statusCode)
		}
	}

	if data == nil {
		setHeaders()
		w.WriteHeader(statusCode)

		return
	}

	body, err := json.Marshal(data)
	if err != nil {
		logutil.Critical(log, err).Error("cannot marshal response")
		setHeaders()
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	setHeaders()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if _, err = w.Write(body); err != nil {
		logutil.Critical(log, err).Error("cannot send response")
	}
}

// RespondError sends the provided error to the client.
// Additionally if the error status code is greater than 500, it will log it
// as critical error.
func RespondError(
	log *slog.Logger,
	w http.ResponseWriter,
	err error,
	headers ...ResponseHeader,
) {
	if respondError(log, w, err, headers...) >= http.StatusInternalServerError {
		logutil.CriticalSkipFrames(log, err, _criticalSkipFrames).Error("internal server error")
	}
}

// RespondSuppressedError sends the provided error to the client.
func RespondSuppressedError(
	log *slog.Logger,
	w http.ResponseWriter,
	err error,
	headers ...ResponseHeader,
) {
	respondError(log, w, err, headers...)
}

// respondError sends the provided error to the client and returns the status
// code it responded with.
func respondError(
	log *slog.Logger,
	w http.ResponseWriter,
	err error,
	headers ...ResponseHeader,
) int {
	headers = append(headers, errorHeaders)
	respErr := errutil.Detect(err, false)
	statusCode := errutil.StatusCode(respErr, false)

	Respond(log, w, respErr, statusCode, headers...)

	return statusCode
}

// DecodeJSON decodes request's JSON body into destination object.
func DecodeJSON(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return ErrInvalidJSON
	}

	return nil
}

// DecodeForm decodes request's form values into destination object.
func DecodeForm(r *http.Request, v any) error {
	if err := r.ParseForm(); err != nil {
		return ErrInvalidForm
	}

	if err := _formDec.Decode(v, r.Form); err != nil {
		return ErrInvalidForm
	}

	return nil
}

// NotFound handles cases when request is sent to a non-existing endpoint.
func NotFound(log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		RespondError(log, w, errutil.ErrNotFound)
	}
}

// MethodNotAllowed handles cases when request's method is not supported for
// the requested endpoint.
func MethodNotAllowed(log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		RespondError(log, w, _errMethodNotAllowed)
	}
}

// Recoverer recovers from handler's panics.
func Recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			defer logutil.Recover(log,
				logutil.NewRecoveryPlan("cannot handle request").
					LocalFields(map[string]any{
						"url": r.URL.String(),
					}).
					BeforeSendFunc(func() {
						err := errutil.NewPlain(http.StatusInternalServerError)
						RespondSuppressedError(log, w, err)
					}),
			)

			next.ServeHTTP(w, r)
		}

		return http.HandlerFunc(fn)
	}
}

// ServeObject streams a stored object to the client, replying with 304 when
// the client's cached copy is still current.
func ServeObject(
	log *slog.Logger,
	w http.ResponseWriter,
	r *http.Request,
	etag string,
	contentType string,
	body io.Reader,
) {
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", contentType)

	if _, err := io.Copy(w, body); err != nil {
		log.Error("cannot stream object", slog.String("error", err.Error()))
	}
}

// ExtractNamedID extracts ID by a specific name from the URL.
func ExtractNamedID(r *http.Request, name string) (xid.ID, error) {
	strID, err := ExtractParam(r, name)
	if err != nil {
		return xid.ID{}, errutil.ErrNotFound
	}

	id, err := xid.FromString(strID)
	if err != nil {
		return xid.ID{}, errutil.ErrNotFound
	}

	return id, nil
}

// ExtractParam extracts a value by the provided parameter name from
// the URL.
func ExtractParam(r *http.Request, p string) (string, error) {
	id := chi.URLParam(r, p)
	if id == "" {
		return "", errutil.ErrNotFound
	}

	return id, nil
}

// ExtractQueryID extracts an ID by a specific name from the URL query.
func ExtractQueryID(r *http.Request, name string) (xid.ID, error) {
	id, err := xid.FromString(r.URL.Query().Get(name))
	if err != nil {
		return xid.ID{}, ErrInvalidForm
	}

	return id, nil
}

// Timeout is a middleware that cancels request's context after a given
// time period.
func Timeout(timeout time.Duration) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}
