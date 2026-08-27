package storage

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

// _testPNG is a data prefix carrying the PNG magic bytes so content
// type sniffing detects image/png.
var _testPNG = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{1}, 1024)...)

// _testJPEG is a data prefix carrying the JPEG magic bytes so content
// type sniffing detects image/jpeg.
var _testJPEG = append([]byte("\xff\xd8\xff"), bytes.Repeat([]byte{2}, 1024)...)

// _testWebP is a data prefix carrying the WebP magic bytes so content
// type sniffing detects image/webp.
var _testWebP = append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), bytes.Repeat([]byte{3}, 1024)...)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// errReader is a reader that always fails.
type errReader struct{}

// Read returns a read failure.
func (er *errReader) Read(_ []byte) (int, error) {
	return 0, assert.AnError
}

// chunkReader yields at most one byte per Read, the way a multipart part can
// at a chunk boundary.
type chunkReader struct {
	r io.Reader
}

// Read reads at most a single byte from the underlying reader.
func (cr *chunkReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}

	return cr.r.Read(p)
}
