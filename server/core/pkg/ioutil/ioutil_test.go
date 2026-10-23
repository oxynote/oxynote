package ioutil

import (
	"errors"
	"io"
	"testing"

	"github.com/hashicorp/go-multierror"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_MultiCloser(t *testing.T) {
	c1, c2 := &closerMock{}, &closerMock{}
	mc := MultiCloser(true, c1, c2)
	require.NotNil(t, mc)
	require.IsType(t, &multiCloser{}, mc)
	require.Len(t, mc.(*multiCloser).closers, 2)
	require.True(t, mc.(*multiCloser).force)
	assert.Same(t, c1, mc.(*multiCloser).closers[0])
	assert.Same(t, c2, mc.(*multiCloser).closers[1])
}

func Test_multiCloser_Close(t *testing.T) {
	cc := map[string]struct {
		Closers []*closerMock
		Force   bool
		Err     error
		Errs    int
		Calls   []int
	}{
		"Error returned by Closer.Close": {
			Closers: []*closerMock{{err: assert.AnError}, {}},
			Err:     assert.AnError,
			Calls:   []int{1, 0},
		},
		"Errors returned by Closer.Close with force": {
			Closers: []*closerMock{{err: assert.AnError}, {err: assert.AnError}},
			Force:   true,
			Err:     assert.AnError,
			Errs:    2,
			Calls:   []int{1, 1},
		},
		"Successful close": {
			Closers: []*closerMock{{}, {}},
			Calls:   []int{1, 1},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			closers := make([]io.Closer, len(c.Closers))
			for i, cl := range c.Closers {
				closers[i] = cl
			}

			mc := &multiCloser{closers: closers, force: c.Force}
			err := mc.Close()

			calls := make([]int, len(c.Closers))
			for i, cl := range c.Closers {
				calls[i] = cl.calls
			}

			assert.Equal(t, c.Calls, calls)

			testutil.AssertEqualError(t, c.Err, err)

			if err == nil {
				return
			}

			// Errs counts the errors a forced close collects. Zero means
			// the first error is returned as is.
			if c.Errs == 0 {
				_, ok := err.(*multierror.Error) //nolint:errorlint // it's a test
				assert.False(t, ok)

				return
			}

			require.IsType(t, &multierror.Error{}, err)           //nolint:testifylint // a direct type check is needed
			assert.Len(t, err.(*multierror.Error).Errors, c.Errs) //nolint:errorlint // it's a test
		})
	}
}

func Test_CloserFunc_Close(t *testing.T) {
	fn := CloserFunc(func() error {
		return assert.AnError
	})

	assert.Error(t, fn.Close())

	fn = CloserFunc(func() error {
		return nil
	})

	assert.NoError(t, fn.Close())
}

func Test_AppendCloseErr(t *testing.T) {
	cc := map[string]struct {
		Closer *closerMock
		Inp    error
		ErrMsg string
	}{
		"Error returned by Closer.Close": {
			Closer: &closerMock{err: errors.New("err2")},
			Inp:    errors.New("err1"),
			ErrMsg: "2 errors occurred:\n\t* err1\n\t* err2\n\n",
		},
		"Successful close": {
			Closer: &closerMock{},
			Inp:    errors.New("err1"),
			ErrMsg: "err1",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := AppendCloseErr(c.Closer, c.Inp)
			assert.Equal(t, 1, c.Closer.calls)
			assert.EqualError(t, err, c.ErrMsg)
		})
	}
}

type closerMock struct {
	calls int
	err   error
}

func (c *closerMock) Close() error {
	c.calls++
	return c.err
}
