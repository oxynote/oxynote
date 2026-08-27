package storage

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ReadObject(t *testing.T) {
	cc := map[string]struct {
		Reader      io.Reader
		Data        []byte
		ContentType string
		Err         error
	}{
		"Error returned by reader": {
			Reader: &errReader{},
			Err:    assert.AnError,
		},
		"Invalid content type": {
			Reader: strings.NewReader("plain text data"),
			Err:    ErrInvalidContentType,
		},
		"Size limit exceeded": {
			Reader: io.MultiReader(
				bytes.NewReader(_testPNG),
				bytes.NewReader(bytes.Repeat([]byte{4}, _maxObjectSize)),
			),
			Err: assert.AnError,
		},
		"Short reads still sniff the content type": {
			Reader:      &chunkReader{r: bytes.NewReader(_testPNG)},
			Data:        _testPNG,
			ContentType: "image/png",
		},
		"Object shorter than the sniff length": {
			Reader:      bytes.NewReader(_testPNG[:64]),
			Data:        _testPNG[:64],
			ContentType: "image/png",
		},
		"Successful PNG read": {
			Reader:      bytes.NewReader(_testPNG),
			Data:        _testPNG,
			ContentType: "image/png",
		},
		"Successful JPEG read": {
			Reader:      bytes.NewReader(_testJPEG),
			Data:        _testJPEG,
			ContentType: "image/jpeg",
		},
		"Successful WebP read": {
			Reader:      bytes.NewReader(_testWebP),
			Data:        _testWebP,
			ContentType: "image/webp",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			data, ct, err := ReadObject(c.Reader)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Data, data)
			assert.Equal(t, c.ContentType, ct)
		})
	}
}

func Test_SniffContentType(t *testing.T) {
	cc := map[string]struct {
		Reader      io.Reader
		Prefix      []byte
		ContentType string
		Err         error
	}{
		"Error returned by reader": {
			Reader: &errReader{},
			Err:    assert.AnError,
		},
		"Object shorter than the sniff length": {
			Reader:      bytes.NewReader(_testPNG[:32]),
			Prefix:      _testPNG[:32],
			ContentType: "image/png",
		},
		"Only the sniff length is consumed": {
			Reader:      bytes.NewReader(_testPNG),
			Prefix:      _testPNG[:_sniffLen],
			ContentType: "image/png",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			prefix, ct, err := SniffContentType(c.Reader)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Prefix, prefix)
			assert.Equal(t, c.ContentType, ct)
		})
	}
}

func Test_newLimitedReader(t *testing.T) {
	t.Parallel()

	r := strings.NewReader("data")
	lr := newLimitedReader(r, 10)

	require.NotNil(t, lr)
	assert.Equal(t, r, lr.r)
	assert.EqualValues(t, 10, lr.limit)
	assert.Zero(t, lr.read)
}

func Test_limitedReader_Read(t *testing.T) {
	cc := map[string]struct {
		Reader io.Reader
		Limit  int64
		Result []byte
		Err    error
	}{
		"Error returned by reader": {
			Reader: &errReader{},
			Limit:  10,
			Err:    assert.AnError,
		},
		"Limit exceeded": {
			Reader: strings.NewReader("oversized data"),
			Limit:  4,
			Err:    ErrSizeLimitExceeded,
		},
		"Read within limit": {
			Reader: strings.NewReader("data"),
			Limit:  10,
			Result: []byte("data"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			lr := newLimitedReader(c.Reader, c.Limit)

			buf := make([]byte, 64)

			n, err := lr.Read(buf)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, buf[:n])
		})
	}
}
