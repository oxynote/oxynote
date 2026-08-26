package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_NewClient(t *testing.T) {
	cc := map[string]struct {
		Fake *fakeS3
		URL  string
		Err  error
	}{
		"Error returned by setupClient": {
			URL: "://invalid",
			Err: assert.AnError,
		},
		"Successful creation": {
			Fake: &fakeS3{bucketExists: true},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			opts := Options{
				Bucket:    "test-bucket",
				URL:       c.URL,
				AccessKey: "access-key",
				SecretKey: "secret-key",
			}

			if c.Fake != nil {
				opts.URL = newFakeS3Server(t, c.Fake)
			}

			client, err := NewClient(context.Background(), opts)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			require.NotNil(t, client)
			assert.NotNil(t, client.client)
			assert.Equal(t, "test-bucket", client.bucket)
		})
	}
}

func Test_setupClient(t *testing.T) {
	cc := map[string]struct {
		Fake       *fakeS3
		URL        string
		Region     string
		SignRegion string
		Created    bool
		Err        error
	}{
		"Invalid URL": {
			URL: "://invalid",
			Err: assert.AnError,
		},
		"URL without a host": {
			URL: "http://",
			Err: assert.AnError,
		},
		"Error returned by HeadBucket": {
			Fake: &fakeS3{bucketExists: true, failBucket: true},
			Err:  assert.AnError,
		},
		"Error returned by CreateBucket": {
			Fake: &fakeS3{failMakeBucket: true},
			Err:  assert.AnError,
		},
		"Bucket created when missing": {
			Fake:       &fakeS3{},
			Created:    true,
			SignRegion: "us-east-1",
		},
		"Existing bucket": {
			Fake:       &fakeS3{bucketExists: true},
			SignRegion: "us-east-1",
		},
		"Configured region signs requests": {
			Fake:       &fakeS3{bucketExists: true},
			Region:     "eu-central-1",
			SignRegion: "eu-central-1",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			opts := Options{
				Bucket:    "test-bucket",
				URL:       c.URL,
				Region:    c.Region,
				AccessKey: "access-key",
				SecretKey: "secret-key",
			}

			if c.Fake != nil {
				opts.URL = newFakeS3Server(t, c.Fake)
			}

			sc, err := setupClient(context.Background(), opts)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			require.NotNil(t, sc)
			assert.Contains(t, c.Fake.authorization, "/"+c.SignRegion+"/s3/aws4_request")

			if c.Created {
				assert.True(t, c.Fake.bucketExists)
			}
		})
	}
}

// prepClient creates a storage client backed by the provided fake S3
// server, marking the fake's bucket as existing so that setup succeeds.
func prepClient(t *testing.T, f *fakeS3) *Client {
	t.Helper()

	f.bucketExists = true

	client, err := NewClient(context.Background(), Options{
		Bucket:    "test-bucket",
		URL:       newFakeS3Server(t, f),
		AccessKey: "access-key",
		SecretKey: "secret-key",
	})
	require.NoError(t, err)

	return client
}

// newFakeS3Server starts an HTTP test server backed by the provided
// fake S3 implementation and returns its URL.
func newFakeS3Server(t *testing.T, f *fakeS3) string {
	t.Helper()

	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)

	return srv.URL
}

// fakeS3 is a minimal in-memory S3 implementation covering only the
// requests the storage package performs: bucket existence probing,
// bucket creation, object upload, object retrieval, server-side copy
// and object removal.
type fakeS3 struct {
	mu sync.Mutex

	// bucketExists indicates whether the bucket existence probe
	// should report the bucket as present.
	bucketExists bool

	// objects maps completed object keys to their data.
	objects map[string][]byte

	// contentTypes maps completed object keys to their content type.
	contentTypes map[string]string

	// authorization records the Authorization header of the last request,
	// whose credential scope carries the region requests were signed for.
	authorization string

	// failBucket forces the bucket existence probe to fail.
	failBucket bool

	// failMakeBucket forces bucket creation to fail.
	failMakeBucket bool

	// failUpload forces object uploads to fail.
	failUpload bool

	// failGet forces object retrieval to fail.
	failGet bool

	// failDelete forces object removal to fail.
	failDelete bool

	// failCopy forces server-side object copies to fail.
	failCopy bool
}

// ServeHTTP dispatches the S3 REST requests issued by the client.
func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// the maps are initialized independently: a fixture that seeds only
	// objects still needs the other one.
	if f.objects == nil {
		f.objects = make(map[string][]byte)
	}

	if f.contentTypes == nil {
		f.contentTypes = make(map[string]string)
	}

	f.authorization = r.Header.Get("Authorization")

	segments := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)

	var key string

	if len(segments) > 1 {
		key = segments[1]
	}

	switch {
	case key == "" && r.Method == http.MethodHead:
		f.handleHeadBucket(w)
	case key == "" && r.Method == http.MethodPut:
		f.handleMakeBucket(w)
	case r.Method == http.MethodPut && r.Header.Get("X-Amz-Copy-Source") != "":
		f.handleCopy(w, r, key)
	case r.Method == http.MethodPut:
		f.handlePut(w, r, key)
	case r.Method == http.MethodGet:
		f.handleGet(w, key)
	case r.Method == http.MethodDelete:
		f.handleDelete(w, key)
	default:
		w.WriteHeader(http.StatusNotImplemented)
	}
}

// handleHeadBucket serves the bucket existence probe.
func (f *fakeS3) handleHeadBucket(w http.ResponseWriter) {
	if f.failBucket {
		w.WriteHeader(http.StatusForbidden)

		return
	}

	if !f.bucketExists {
		w.WriteHeader(http.StatusNotFound)

		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleMakeBucket serves bucket creation.
func (f *fakeS3) handleMakeBucket(w http.ResponseWriter) {
	if f.failMakeBucket {
		writeS3Error(w, http.StatusForbidden, "AccessDenied")

		return
	}

	f.bucketExists = true

	w.WriteHeader(http.StatusOK)
}

// handlePut stores an object uploaded through a single PUT request,
// recording the object's content type provided by the client.
func (f *fakeS3) handlePut(w http.ResponseWriter, r *http.Request, key string) {
	if f.failUpload {
		writeS3Error(w, http.StatusForbidden, "AccessDenied")

		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeS3Error(w, http.StatusBadRequest, "IncompleteBody")

		return
	}

	f.objects[key] = data
	f.contentTypes[key] = r.Header.Get("Content-Type")

	w.Header().Set("ETag", `"test-etag"`)
	w.WriteHeader(http.StatusOK)
}

// handleGet serves object data requests.
func (f *fakeS3) handleGet(w http.ResponseWriter, key string) {
	if f.failGet {
		writeS3Error(w, http.StatusForbidden, "AccessDenied")

		return
	}

	data, ok := f.objects[key]
	if !ok {
		writeS3Error(w, http.StatusNotFound, "NoSuchKey")

		return
	}

	writeS3ObjectHeaders(w, data, f.contentTypes[key])
	w.Write(data) //nolint:errcheck,gosec // test server response errors are irrelevant
}

// handleDelete serves object removal requests.
func (f *fakeS3) handleDelete(w http.ResponseWriter, key string) {
	if f.failDelete {
		writeS3Error(w, http.StatusForbidden, "AccessDenied")

		return
	}

	delete(f.objects, key)

	w.WriteHeader(http.StatusNoContent)
}

// handleCopy serves server-side object copies, which arrive as a PUT
// naming their source in the x-amz-copy-source header.
func (f *fakeS3) handleCopy(w http.ResponseWriter, r *http.Request, key string) {
	if f.failCopy {
		writeS3Error(w, http.StatusForbidden, "AccessDenied")

		return
	}

	src, err := url.PathUnescape(strings.TrimPrefix(r.Header.Get("X-Amz-Copy-Source"), "/"))
	if err != nil {
		writeS3Error(w, http.StatusBadRequest, "InvalidArgument")

		return
	}

	// the header names the source as "<bucket>/<key>".
	_, srcKey, _ := strings.Cut(src, "/")

	data, ok := f.objects[srcKey]
	if !ok {
		writeS3Error(w, http.StatusNotFound, "NoSuchKey")

		return
	}

	f.objects[key] = data
	f.contentTypes[key] = f.contentTypes[srcKey]

	fmt.Fprint( //nolint:errcheck // test server response errors are irrelevant
		w,
		`<?xml version="1.0"?><CopyObjectResult><ETag>"test-etag"</ETag><LastModified>2006-01-02T15:04:05.000Z</LastModified></CopyObjectResult>`,
	)
}

// writeS3ObjectHeaders writes the object metadata headers the client
// reads back off a retrieval.
func writeS3ObjectHeaders(w http.ResponseWriter, data []byte, contentType string) {
	w.Header().Set("ETag", `"test-etag"`)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
}

// writeS3Error writes an S3-style XML error response.
func writeS3Error(w http.ResponseWriter, status int, code string) {
	w.WriteHeader(status)
	fmt.Fprintf( //nolint:errcheck // test server response errors are irrelevant
		w,
		`<?xml version="1.0"?><Error><Code>%s</Code><Message>%s</Message></Error>`,
		code,
		code,
	)
}
