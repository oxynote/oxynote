package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
		Fake    *fakeS3
		URL     string
		Created bool
		Err     error
	}{
		"Invalid URL": {
			URL: "://invalid",
			Err: assert.AnError,
		},
		"Error returned by minio.New": {
			URL: "http://",
			Err: assert.AnError,
		},
		"Error returned by BucketExists": {
			Fake: &fakeS3{bucketExists: true, failBucket: true},
			Err:  assert.AnError,
		},
		"Error returned by MakeBucket": {
			Fake: &fakeS3{failMakeBucket: true},
			Err:  assert.AnError,
		},
		"Bucket created when missing": {
			Fake:    &fakeS3{},
			Created: true,
		},
		"Existing bucket": {
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

			mc, err := setupClient(context.Background(), opts)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			require.NotNil(t, mc)

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
// requests the minio client performs for the storage package: bucket
// location lookup, bucket creation, multipart upload, object stat,
// object retrieval and object removal.
type fakeS3 struct {
	mu sync.Mutex

	// bucketExists indicates whether the bucket location lookup
	// should report the bucket as present.
	bucketExists bool

	// objects maps completed object keys to their data.
	objects map[string][]byte

	// contentTypes maps completed object keys to their content type.
	contentTypes map[string]string

	// parts accumulates uploaded multipart data per object key.
	parts map[string][]byte

	// failBucket forces the bucket existence probe to fail.
	failBucket bool

	// failMakeBucket forces bucket creation to fail.
	failMakeBucket bool

	// failUpload forces multipart upload initiation to fail.
	failUpload bool

	// failStat forces object stat requests to fail.
	failStat bool

	// failDelete forces object removal to fail.
	failDelete bool
}

// ServeHTTP dispatches the S3 REST requests issued by the minio client.
func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.objects == nil {
		f.objects = make(map[string][]byte)
		f.contentTypes = make(map[string]string)
		f.parts = make(map[string][]byte)
	}

	segments := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)

	var key string

	if len(segments) > 1 {
		key = segments[1]
	}

	query := r.URL.Query()

	switch {
	case key == "" && r.Method == http.MethodGet && query.Has("location"):
		f.handleLocation(w)
	case key == "" && r.Method == http.MethodHead:
		f.handleHeadBucket(w)
	case key == "" && r.Method == http.MethodPut:
		f.handleMakeBucket(w)
	case r.Method == http.MethodPost && query.Has("uploads"):
		f.handleInitiateUpload(w, r, key)
	case r.Method == http.MethodPut && query.Get("uploadId") != "":
		f.handleUploadPart(w, r, key)
	case r.Method == http.MethodPost && query.Get("uploadId") != "":
		f.handleCompleteUpload(w, key)
	case r.Method == http.MethodHead:
		f.handleStat(w, key)
	case r.Method == http.MethodGet:
		f.handleGet(w, key)
	case r.Method == http.MethodDelete:
		f.handleDelete(w, key)
	default:
		w.WriteHeader(http.StatusNotImplemented)
	}
}

// handleLocation serves the bucket location lookup that backs the
// minio client's BucketExists call.
func (f *fakeS3) handleLocation(w http.ResponseWriter) {
	if !f.bucketExists {
		writeS3Error(w, http.StatusNotFound, "NoSuchBucket")

		return
	}

	fmt.Fprint(w, `<?xml version="1.0"?><LocationConstraint></LocationConstraint>`) //nolint:errcheck // test server response errors are irrelevant
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

// handleInitiateUpload serves multipart upload initiation, recording
// the object's content type provided by the client.
func (f *fakeS3) handleInitiateUpload(w http.ResponseWriter, r *http.Request, key string) {
	if f.failUpload {
		writeS3Error(w, http.StatusForbidden, "AccessDenied")

		return
	}

	f.contentTypes[key] = r.Header.Get("Content-Type")

	fmt.Fprintf( //nolint:errcheck,gosec // test server response errors and input taint are irrelevant
		w,
		`<?xml version="1.0"?><InitiateMultipartUploadResult><Key>%s</Key><UploadId>test-upload</UploadId></InitiateMultipartUploadResult>`,
		key,
	)
}

// handleUploadPart accumulates a single multipart upload part.
func (f *fakeS3) handleUploadPart(w http.ResponseWriter, r *http.Request, key string) {
	f.parts[key] = append(f.parts[key], readS3Body(r)...)

	w.Header().Set("ETag", `"part-etag"`)
	w.WriteHeader(http.StatusOK)
}

// handleCompleteUpload finalizes a multipart upload by promoting the
// accumulated parts to a stored object.
func (f *fakeS3) handleCompleteUpload(w http.ResponseWriter, key string) {
	f.objects[key] = f.parts[key]

	delete(f.parts, key)

	fmt.Fprintf( //nolint:errcheck,gosec // test server response errors and input taint are irrelevant
		w,
		`<?xml version="1.0"?><CompleteMultipartUploadResult><Bucket>test-bucket</Bucket><Key>%s</Key><ETag>"test-etag"</ETag></CompleteMultipartUploadResult>`,
		key,
	)
}

// handleStat serves object metadata requests.
func (f *fakeS3) handleStat(w http.ResponseWriter, key string) {
	if f.failStat {
		w.WriteHeader(http.StatusForbidden)

		return
	}

	data, ok := f.objects[key]
	if !ok {
		w.WriteHeader(http.StatusNotFound)

		return
	}

	writeS3ObjectHeaders(w, data, f.contentTypes[key])
	w.WriteHeader(http.StatusOK)
}

// handleGet serves object data requests.
func (f *fakeS3) handleGet(w http.ResponseWriter, key string) {
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

// writeS3ObjectHeaders writes the object metadata headers the minio
// client requires to build stat results.
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

// readS3Body reads a request body, decoding the aws-chunked streaming
// signature framing the minio client uses for unknown-size uploads.
// Each frame is a "<hex size>;chunk-signature=<sig>" header line
// followed by the payload, both CRLF-terminated; a zero-size frame
// ends the stream.
func readS3Body(r *http.Request) []byte {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil
	}

	if !strings.HasPrefix(r.Header.Get("X-Amz-Content-Sha256"), "STREAMING-") {
		return raw
	}

	var out []byte

	for {
		idx := bytes.Index(raw, []byte("\r\n"))
		if idx < 0 {
			return out
		}

		header := string(raw[:idx])
		raw = raw[idx+2:]

		if semi := strings.Index(header, ";"); semi >= 0 {
			header = header[:semi]
		}

		length, err := parseHexLength(header)
		if err != nil || length == 0 {
			return out
		}

		out = append(out, raw[:length]...)
		raw = raw[length+2:]
	}
}

// parseHexLength parses a hexadecimal chunk length.
func parseHexLength(val string) (int, error) {
	var length int

	_, err := fmt.Sscanf(val, "%x", &length)

	return length, err
}
